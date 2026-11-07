// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"slices"
	"unicode/utf8"

	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/internal/workspacefile"
)

func runUIWidget(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	enabled := view.Mode == "wikiDesktop" && p != nil && len(p.widgets) > 0
	if r.Endpoint == "/api/widget/status" {
		return enabled, nil
	}
	if !enabled {
		return nil, ErrUIEndpoint
	}
	var q struct {
		Name, K      string
		ValidateOnly bool
	}
	if err := Decode(bytes.NewReader(r.Data), &q); err != nil {
		return nil, err
	}
	l, err := m.Acquire(ctx, r.StoreID, r.PageID)
	if err != nil {
		return nil, err
	}
	defer l.Release()
	if r.Endpoint == "/api/widget/list" {
		if len(q.K) > 256 {
			return nil, ErrArgument
		}
		items, err := l.NativeSearchWidgets(ctx, q.K)
		if err != nil {
			return nil, err
		}
		items = slices.DeleteFunc(items, func(b *blockview.Block) bool { return p.widgets[b.Content] == "" })
		return items, nil
	}
	hash := p.widgets[q.Name]
	if hash == "" {
		return nil, ErrUnauthorized
	}
	value, err := l.NativeFileRead(ctx, workspacefile.Request{Action: "read", Path: "widgets/" + q.Name + "/index.html", ReadLimit: 512 << 10})
	if err != nil {
		return nil, err
	}
	if len(value.Data) > 512<<10 || !utf8.Valid(value.Data) || sourcetx.Hash(value.Data) != hash {
		return nil, ErrUnauthorized
	}
	files := make(map[string][]byte)
	var bytesLeft int64 = 2 << 20
	for name, digest := range p.widgetScripts[q.Name] {
		file, err := l.NativeFileRead(ctx, workspacefile.Request{Action: "read", Path: "widgets/" + q.Name + "/" + name, ReadLimit: min(512<<10, bytesLeft)})
		if err != nil {
			return nil, err
		}
		bytesLeft -= int64(len(file.Data))
		if bytesLeft <= 0 || !utf8.Valid(file.Data) || sourcetx.Hash(file.Data) != digest {
			return nil, ErrUnauthorized
		}
		files[name] = file.Data
	}
	if q.ValidateOnly {
		return true, nil
	}
	html, scriptBase, err := p.scripts.prepare(string(value.Data), files)
	if err != nil {
		return nil, err
	}
	return map[string]string{"type": "widget", "text": html, "scriptBase": scriptBase}, nil
}
