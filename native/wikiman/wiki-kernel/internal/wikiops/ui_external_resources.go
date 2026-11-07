// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	css "github.com/gorilla/css/scanner"
	nethtml "golang.org/x/net/html"
	"wiki-kernel/internal/sourcetx"
)

type FrameFormPolicy struct {
	Action       string            `json:"action,omitempty"`
	Fields       []string          `json:"fields,omitempty"`
	HiddenFields []string          `json:"hiddenFields,omitempty"`
	Files        map[string]string `json:"files,omitempty"`
}

var allowedFrameUploads = map[string]bool{"image/png": true, "image/jpeg": true, "application/pdf": true, "text/plain": true}

func WithFrameSessions(ctx context.Context, sessions map[string]map[string]string) (context.Context, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	if p == nil || len(sessions) > 16 {
		return ctx, ErrArgument
	}
	p.frameClients = make(map[string]*http.Client, len(sessions))
	for address, values := range sessions {
		frame, err := p.interactiveFrameURL(address)
		if err != nil || p.frameForms[address].Action == "" || len(values) == 0 || len(values) > 8 {
			return ctx, ErrArgument
		}
		jar, _ := cookiejar.New(nil)
		cookies := make([]*http.Cookie, 0, len(values))
		for name, value := range values {
			cookie := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: frame.Scheme == "https"}
			if len(name) > 128 || len(value) > 2048 || value == "" || cookie.Valid() != nil {
				return ctx, ErrArgument
			}
			cookies = append(cookies, cookie)
		}
		jar.SetCookies(frame, cookies)
		client := *p.client
		client.Jar = jar
		p.frameClients[address] = &client
	}
	return ctx, nil
}

type frameAsset struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data []byte `json:"data"`
}

var frameAssetTypes = map[string][]string{
	".css": {"text/css"}, ".png": {"image/png"}, ".jpg": {"image/jpeg"}, ".jpeg": {"image/jpeg"},
	".gif": {"image/gif"}, ".webp": {"image/webp"}, ".wav": {"audio/wav", "audio/x-wav"},
	".mp3": {"audio/mpeg"}, ".ogg": {"audio/ogg"}, ".mp4": {"video/mp4"}, ".webm": {"video/webm"},
	".woff": {"font/woff"}, ".woff2": {"font/woff2"},
}

func WithFrameResources(ctx context.Context, assets map[string]map[string]string, forms map[string]FrameFormPolicy) (context.Context, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	if p == nil || len(assets) > 16 || len(forms) > 16 {
		return ctx, ErrArgument
	}
	p.frameAssets = make(map[string]map[string]string, len(assets))
	p.frameForms = make(map[string]FrameFormPolicy, len(forms))
	for address, files := range assets {
		if _, err := p.interactiveFrameURL(address); err != nil || len(files) > 12 {
			return ctx, ErrArgument
		}
		approved := make(map[string]string, len(files))
		for file, digest := range files {
			if !validFrameAssetPath(file) || !hashPattern.MatchString(digest) {
				return ctx, ErrArgument
			}
			approved[file] = digest
		}
		p.frameAssets[address] = approved
	}
	for address, form := range forms {
		frame, err := p.interactiveFrameURL(address)
		action, actionErr := parseExternalPreviewURL(form.Action)
		if err != nil || actionErr != nil || frame.Scheme != action.Scheme || frame.Host != action.Host ||
			action.Fragment != "" || action.RawQuery != "" || action.ForceQuery || !validFramePath(strings.TrimPrefix(action.Path, "/")) ||
			len(form.Fields)+len(form.Files) == 0 || len(form.Fields) > 12 || len(form.Files) > 2 {
			return ctx, ErrArgument
		}
		seen := map[string]bool{}
		for _, field := range form.Fields {
			if !widgetName.MatchString(field) || seen[field] {
				return ctx, ErrArgument
			}
			seen[field] = true
		}
		for _, key := range form.HiddenFields {
			if !seen[key] {
				return ctx, ErrArgument
			}
		}
		files := make(map[string]string, len(form.Files))
		for field, kind := range form.Files {
			if !widgetName.MatchString(field) || seen[field] || !allowedFrameUploads[kind] {
				return ctx, ErrArgument
			}
			files[field] = kind
		}
		p.frameForms[address] = FrameFormPolicy{Action: form.Action, Fields: slices.Clone(form.Fields),
			HiddenFields: slices.Clone(form.HiddenFields), Files: files}
	}
	return ctx, nil
}

func (p *externalPreviews) interactiveFrameURL(address string) (*url.URL, error) {
	u, err := parseExternalPreviewURL(address)
	if err != nil || u.Fragment != "" || !p.interactive[u.Scheme+"://"+strings.ToLower(u.Host)] {
		return nil, ErrArgument
	}
	return u, nil
}

func validFramePath(file string) bool {
	if len(file) > 512 || path.Clean(file) != file {
		return false
	}
	for _, part := range strings.Split(file, "/") {
		if !scriptPart.MatchString(part) {
			return false
		}
	}
	return true
}

func validFrameAssetPath(file string) bool {
	return validFramePath(file) && len(frameAssetTypes[path.Ext(file)]) > 0
}

func frameAttr(node *nethtml.Node, name string) string {
	for _, a := range node.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func (p *externalPreviews) prepareFrameAssets(ctx context.Context, frame *url.URL, html string) (string, []frameAsset, error) {
	approved := p.frameAssets[frame.String()]
	if len(approved) == 0 {
		return html, nil, nil
	}
	root, err := nethtml.Parse(strings.NewReader(html))
	if err != nil {
		return "", nil, err
	}
	var assets []frameAsset
	used := make(map[string]int, len(approved))
	budget := int64(2 << 20)
	expandedBudget := 2 << 20
	pending := map[string]bool{}
	var load func(*url.URL, string, int) (frameAsset, error)
	load = func(base *url.URL, reference string, depth int) (frameAsset, error) {
		source, err := base.Parse(reference)
		if err != nil || source.Scheme != frame.Scheme || source.Host != frame.Host || source.User != nil ||
			source.Fragment != "" || source.RawQuery != "" || source.ForceQuery {
			return frameAsset{}, nil
		}
		file := strings.TrimPrefix(source.Path, "/")
		digest := approved[file]
		if digest == "" {
			return frameAsset{}, nil
		}
		if index, exists := used[file]; exists {
			return assets[index], nil
		}
		if depth > 6 || pending[file] {
			return frameAsset{}, ErrArgument
		}
		asset, err := p.fetchFrameAsset(ctx, source, digest, min(budget, 1<<20))
		if err != nil {
			return frameAsset{}, err
		}
		asset.Name = file
		budget -= int64(len(asset.Data))
		if asset.Type == "text/css" {
			pending[file] = true
			asset.Data, err = rewriteFrameCSS(asset.Data, &expandedBudget, func(reference string) (frameAsset, error) {
				return load(source, reference, depth+1)
			})
			delete(pending, file)
			if err != nil {
				return frameAsset{}, err
			}
			expandedBudget -= len(asset.Data)
		}
		used[file] = len(assets)
		assets = append(assets, asset)
		return asset, nil
	}
	var visit func(*nethtml.Node) error
	visit = func(node *nethtml.Node) error {
		if node.Type == nethtml.ElementNode && node.Namespace == "" {
			node.Attr = slices.DeleteFunc(node.Attr, func(a nethtml.Attribute) bool { return a.Key == "data-ironman-verified" })
			var attr, category string
			switch node.Data {
			case "link":
				if strings.EqualFold(frameAttr(node, "rel"), "stylesheet") {
					attr, category = "href", "text"
				}
			case "img":
				attr, category = "src", "image"
			case "audio", "video":
				attr, category = "src", node.Data
			case "source":
				if node.Parent != nil && (node.Parent.Data == "audio" || node.Parent.Data == "video") {
					attr, category = "src", node.Parent.Data
				}
			}
			if attr != "" {
				asset, err := load(frame, frameAttr(node, attr), 0)
				if err != nil {
					return err
				}
				if strings.HasPrefix(asset.Type, category+"/") {
					node.Attr = append(node.Attr, nethtml.Attribute{Key: "data-ironman-verified", Val: asset.Name})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return "", nil, err
	}
	var out strings.Builder
	if err := nethtml.Render(&out, root); err != nil {
		return "", nil, err
	}
	return out.String(), assets, nil
}

func rewriteFrameCSS(raw []byte, budget *int, load func(string) (frameAsset, error)) ([]byte, error) {
	tokens := css.New(string(raw))
	var out strings.Builder
	appendText := func(text string) error {
		if len(text) > *budget-out.Len() {
			return ErrExportLimit
		}
		out.WriteString(text)
		return nil
	}
	for token := tokens.Next(); token.Type != css.TokenEOF; token = tokens.Next() {
		if token.Type == css.TokenError {
			return nil, ErrArgument
		}
		text := token.Value
		switch {
		case token.Type == css.TokenAtKeyword && strings.EqualFold(text, "@import"):
			var directive []*css.Token
			for next := tokens.Next(); next.Type != css.TokenEOF; next = tokens.Next() {
				if next.Type == css.TokenError || len(directive) >= 64 || next.Value == "{" || next.Value == "}" {
					return nil, ErrArgument
				}
				if next.Value == ";" {
					break
				}
				if next.Type != css.TokenComment && next.Type != css.TokenS {
					directive = append(directive, next)
				}
			}
			text = ""
			if len(directive) == 1 && (directive[0].Type == css.TokenURI || directive[0].Type == css.TokenString) {
				asset, err := load(frameCSSReference(directive[0].Value))
				if err != nil {
					return nil, err
				}
				if asset.Type == "text/css" {
					text = "\n" + string(asset.Data) + "\n"
				}
			}
		case token.Type == css.TokenURI:
			reference := frameCSSReference(text)
			if strings.HasPrefix(reference, "data:") {
				break
			}
			asset, err := load(reference)
			if err != nil {
				return nil, err
			}
			text = `url("data:,")`
			if strings.HasPrefix(asset.Type, "image/") || strings.HasPrefix(asset.Type, "font/") {
				text = `url("data:` + asset.Type + ";base64," + base64.StdEncoding.EncodeToString(asset.Data) + `")`
			}
		}
		if err := appendText(text); err != nil {
			return nil, err
		}
	}
	return []byte(out.String()), nil
}

func frameCSSReference(value string) string {
	if len(value) >= 5 && strings.EqualFold(value[:4], "url(") && strings.HasSuffix(value, ")") {
		value = value[4 : len(value)-1]
	}
	return strings.Trim(strings.TrimSpace(value), "'\"")
}

func (p *externalPreviews) fetchFrameAsset(ctx context.Context, source *url.URL, digest string, limit int64) (frameAsset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.String(), nil)
	if err != nil {
		return frameAsset{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return frameAsset{}, err
	}
	defer resp.Body.Close()
	kind, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || !slices.Contains(frameAssetTypes[path.Ext(source.Path)], kind) {
		return frameAsset{}, ErrUnauthorized
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(raw) == 0 || int64(len(raw)) > limit {
		return frameAsset{}, ErrExportLimit
	}
	if sourcetx.Hash(raw) != digest || kind == "text/css" && !utf8.Valid(raw) {
		return frameAsset{}, ErrUnauthorized
	}
	return frameAsset{Type: kind, Data: raw}, nil
}

func runUIExternalFrameForm(ctx context.Context, r UIRequest) (any, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	var q struct {
		URL    string
		Fields map[string]string
		Files  map[string]frameUpload
	}
	if p == nil {
		return nil, ErrUnauthorized
	}
	if err := Decode(bytes.NewReader(r.Data), &q); err != nil {
		return nil, err
	}
	form, found := p.frameForms[q.URL]
	if !found || len(q.Fields) != len(form.Fields) || len(q.Files) != len(form.Files) {
		return nil, ErrUnauthorized
	}
	values := make(url.Values, len(form.Fields))
	for _, key := range form.Fields {
		value, ok := q.Fields[key]
		if !ok || len(value) > 2048 || strings.ContainsRune(value, 0) {
			return nil, ErrArgument
		}
		values.Set(key, value)
	}
	payload := values.Encode()
	if len(payload) > 12<<10 {
		return nil, ErrExportLimit
	}
	// A form submission is intentionally a single attempt, including a failed response.
	var body io.Reader = strings.NewReader(payload)
	kind := "application/x-www-form-urlencoded"
	if len(form.Files) != 0 {
		payload, contentType, err := encodeFrameUploads(form, values, q.Files)
		if err != nil {
			return nil, err
		}
		body, kind = payload, contentType
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, form.Action, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", kind)
	req.GetBody = nil
	client := p.client
	if p.frameClients[q.URL] != nil {
		client = p.frameClients[q.URL]
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrUnauthorized
	}
	kind, _, _ = mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if kind != "text/plain" {
		return map[string]string{"text": ""}, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (32<<10)+1))
	if err != nil || len(raw) > 32<<10 || !utf8.Valid(raw) {
		return nil, ErrExportLimit
	}
	return map[string]string{"text": string(raw)}, nil
}

type frameUpload struct {
	Name string
	Data []byte
}

func encodeFrameUploads(form FrameFormPolicy, values url.Values, files map[string]frameUpload) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, key := range form.Fields {
		if err := writer.WriteField(key, values.Get(key)); err != nil {
			return nil, "", err
		}
	}
	for key, kind := range form.Files {
		file, ok := files[key]
		if !ok || !scriptPart.MatchString(file.Name) || len(file.Data) == 0 || len(file.Data) > 256<<10 {
			return nil, "", ErrArgument
		}
		detected, _, _ := mime.ParseMediaType(http.DetectContentType(file.Data))
		if detected != kind {
			return nil, "", ErrUnauthorized
		}
		headers := textproto.MIMEHeader{}
		headers.Set("Content-Type", kind)
		headers.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q; filename=%q", key, file.Name))
		part, err := writer.CreatePart(headers)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(file.Data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body, writer.FormDataContentType(), nil
}
