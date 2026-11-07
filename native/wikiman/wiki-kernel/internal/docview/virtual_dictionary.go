// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"strings"
	"wiki-kernel/internal/indexdb"
)

// VirtualDictionary belongs to one synchronous document-read admission. Its
// dictionary is loaded from the captured owner once, never a process TTL cache.
type VirtualDictionary struct {
	Options    VirtualOptions
	Content    indexdb.ContentServices
	Read       func(context.Context) ([]string, error)
	loaded     bool
	dictionary []string
}

func (d *VirtualDictionary) Keys(ctx context.Context, root *ast.Node) ([]string, error) {
	if ctx == nil || d == nil || root == nil {
		return nil, indexdb.ErrProjection
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !d.Options.Enabled {
		return nil, ctx.Err()
	}
	if d.Read == nil {
		return nil, indexdb.ErrProjection
	}
	if !d.loaded {
		var e error
		d.dictionary, e = d.Read(ctx)
		if e != nil {
			return nil, e
		}
		d.loaded = true
	}
	keys := FilterVirtualKeywords(root, d.dictionary, d.Options)
	var body strings.Builder
	var err error
	ast.Walk(root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = ctx.Err(); err != nil {
			return ast.WalkStop
		}
		if !enter || !n.IsBlock() {
			return ast.WalkContinue
		}
		var value string
		value, err = indexdb.StaticContent(ctx, n, nil, false, false, false, d.Content)
		if err != nil {
			return ast.WalkStop
		}
		body.WriteString(strings.ReplaceAll(value, editor.Zwsp, ""))
		return ast.WalkContinue
	})
	if err != nil {
		return nil, err
	}
	return MatchVirtualKeywords(body.String(), keys, d.Options.CaseSensitive), ctx.Err()
}
