// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type externalPreviewKey struct{}
type externalPreviews struct {
	origins             map[string]bool
	widgets             map[string]string
	widgetScripts       map[string]map[string]string
	interactive         map[string]bool
	frameScripts        map[string]map[string]string
	frameAssets         map[string]map[string]string
	frameForms          map[string]FrameFormPolicy
	frameClients        map[string]*http.Client
	plugins             map[string]string
	pluginStorage       map[string]bool
	pluginHotkeys       map[string]map[string]string
	pluginSystemHotkeys map[string]map[string]string
	scripts             sandboxScripts
	client              *http.Client
}

var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

func externalPreviewDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	dial := net.Dialer{}
	if literal, err := netip.ParseAddr(host); err == nil && literal.IsLoopback() {
		return dial.DialContext(ctx, network, address)
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, candidate := range addresses {
		ip := candidate.Unmap()
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || sharedAddressSpace.Contains(ip) {
			continue
		}
		conn, err := dial.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, ErrUnauthorized
}

func parseExternalPreviewURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback()) {
		return nil, ErrArgument
	}
	return u, nil
}

var widgetName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

func WithExternalPreviews(ctx context.Context, origins []string, widgets map[string]string) (context.Context, error) {
	return WithExternalPreviewPolicy(ctx, origins, widgets, nil, nil, nil, nil)
}

func WithExternalPreviewPolicy(ctx context.Context, origins []string, widgets map[string]string, scripts map[string]map[string]string, frames []string, plugins map[string]string, frameScripts map[string]map[string]string) (context.Context, error) {
	if len(origins)+len(widgets)+len(plugins) == 0 || len(origins) > 32 || len(widgets) > 32 || len(plugins) > 8 {
		return ctx, ErrArgument
	}
	p := &externalPreviews{origins: make(map[string]bool), widgets: make(map[string]string),
		widgetScripts: make(map[string]map[string]string), interactive: make(map[string]bool),
		frameScripts: make(map[string]map[string]string),
		plugins:      make(map[string]string), client: &http.Client{
			Timeout: 8 * time.Second, Transport: &http.Transport{DisableKeepAlives: true, DialContext: externalPreviewDial},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}}
	for name, hash := range plugins {
		if !widgetName.MatchString(name) || !hashPattern.MatchString(hash) {
			return ctx, ErrArgument
		}
		p.plugins[name] = hash
	}
	for name, hash := range widgets {
		if !widgetName.MatchString(name) || !hashPattern.MatchString(hash) {
			return ctx, ErrArgument
		}
		p.widgets[name] = hash
	}
	for _, origin := range origins {
		u, err := parseExternalPreviewURL(origin)
		if err != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
			return ctx, ErrArgument
		}
		p.origins[u.Scheme+"://"+strings.ToLower(u.Host)] = true
	}
	if len(frames) > 32 {
		return ctx, ErrArgument
	}
	for _, origin := range frames {
		if !p.origins[origin] {
			return ctx, ErrArgument
		}
		p.interactive[origin] = true
	}
	for address, files := range frameScripts {
		u, err := parseExternalPreviewURL(address)
		if err != nil || u.Fragment != "" || !p.interactive[u.Scheme+"://"+strings.ToLower(u.Host)] ||
			len(frameScripts) > 16 || len(files) > 8 {
			return ctx, ErrArgument
		}
		checked := make(map[string]string, len(files))
		for file, digest := range files {
			if !validSandboxScript(file) || !hashPattern.MatchString(digest) {
				return ctx, ErrArgument
			}
			checked[file] = digest
		}
		p.frameScripts[address] = checked
	}
	for name, files := range scripts {
		if p.widgets[name] == "" || len(files) > 32 {
			return ctx, ErrArgument
		}
		checked := make(map[string]string, len(files))
		for file, digest := range files {
			if !validSandboxScript(file) || !hashPattern.MatchString(digest) {
				return ctx, ErrArgument
			}
			checked[file] = digest
		}
		p.widgetScripts[name] = checked
	}
	return context.WithValue(ctx, externalPreviewKey{}, p), nil
}

var externalTypes = map[string]string{
	"image/png": "image", "image/jpeg": "image", "image/gif": "image", "image/webp": "image",
	"audio/mpeg": "audio", "audio/ogg": "audio", "audio/wav": "audio", "audio/x-wav": "audio",
	"video/mp4": "video", "video/webm": "video",
	"text/html": "iframe", "text/plain": "iframe",
}

func runUIExternalPreview(ctx context.Context, r UIRequest) (any, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	if p == nil {
		return nil, ErrUIEndpoint
	}
	var q struct{ URL, Kind string }
	if err := Decode(bytes.NewReader(r.Data), &q); err != nil {
		return nil, err
	}
	switch q.Kind {
	case "image", "video", "audio", "iframe":
	default:
		return nil, ErrArgument
	}
	u, err := parseExternalPreviewURL(q.URL)
	if err != nil {
		return nil, err
	}
	if !p.origins[u.Scheme+"://"+strings.ToLower(u.Host)] {
		return nil, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrArgument
	}
	client := p.client
	if q.Kind == "iframe" && p.frameClients[u.String()] != nil {
		client = p.frameClients[u.String()]
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fs.ErrNotExist
	}
	kind, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || externalTypes[kind] != q.Kind {
		return nil, ErrArgument
	}
	limit := int64(6 << 20)
	if q.Kind == "iframe" {
		limit = 512 << 10
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(body) == 0 || int64(len(body)) > limit {
		return nil, ErrExportLimit
	}
	if q.Kind == "iframe" {
		interactive := kind == "text/html" && p.interactive[u.Scheme+"://"+strings.ToLower(u.Host)]
		html, scriptBase := string(body), ""
		var assets []frameAsset
		if interactive {
			html, scriptBase, err = p.prepareExternalFrame(ctx, u, html)
			if err != nil {
				return nil, err
			}
			html, assets, err = p.prepareFrameAssets(ctx, u, html)
			if err != nil {
				return nil, err
			}
		}
		return map[string]any{"type": kind, "text": html, "interactive": interactive, "scriptBase": scriptBase,
			"assets": assets, "form": p.frameForms[u.String()]}, nil
	}
	return map[string]any{"type": kind, "data": body}, nil
}
