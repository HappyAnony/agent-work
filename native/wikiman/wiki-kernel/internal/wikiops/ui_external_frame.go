// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"
	"wiki-kernel/internal/sourcetx"
)

func (p *externalPreviews) prepareExternalFrame(ctx context.Context, target *url.URL, html string) (string, string, error) {
	approved := p.frameScripts[target.String()]
	if len(approved) == 0 {
		return p.scripts.prepare(html, nil)
	}
	root, err := nethtml.Parse(strings.NewReader(html))
	if err != nil {
		return "", "", err
	}
	err = visitSandboxScripts(root, func(script *nethtml.Node) error {
		for i, attr := range script.Attr {
			if attr.Key != "src" {
				continue
			}
			src, err := target.Parse(attr.Val)
			script.Attr[i].Val = "about:blank"
			if err == nil && src.Scheme == target.Scheme && src.Host == target.Host && src.User == nil &&
				src.RawQuery == "" && !src.ForceQuery && src.Fragment == "" {
				name := strings.TrimPrefix(src.Path, "/")
				if approved[name] != "" {
					script.Attr[i].Val = name
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	files := make(map[string][]byte, len(approved))
	budget := int64(512 << 10)
	for name, digest := range approved {
		source := &url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/" + name}
		body, err := p.fetchExternalFrameScript(ctx, source, digest, min(budget, 128<<10))
		if err != nil {
			return "", "", err
		}
		files[name] = body
		budget -= int64(len(body))
	}
	var out strings.Builder
	if err = nethtml.Render(&out, root); err != nil {
		return "", "", err
	}
	return p.scripts.prepareAt(out.String(), files, path.Dir(strings.TrimPrefix(target.Path, "/")))
}

func (p *externalPreviews) fetchExternalFrameScript(ctx context.Context, source *url.URL, digest string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	kind, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || kind != "text/javascript" && kind != "application/javascript" {
		return nil, ErrUnauthorized
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, ErrExportLimit
	}
	if !utf8.Valid(body) || sourcetx.Hash(body) != digest {
		return nil, ErrUnauthorized
	}
	return body, nil
}
