// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

type BacklinkRequest struct {
	DefaultChildren                              bool
	Mode, ID, RefTreeID, Keyword, MentionKeyword string
	BeforeLen, SortMode, MentionSortMode         int
	Children, Highlight                          bool
}
type BacklinkResult struct {
	Box                          string
	Links, Mentions              []*Path
	Documents                    []*Backlink
	Keywords                     []string
	ReferenceCount, MentionCount int
}
type ReadServices struct {
	DB           *sql.DB
	Search       *conf.Search
	Parser       *lute.Lute
	Content      indexdb.ContentServices
	Load         func(context.Context, string) (*parse.Tree, error)
	Trees        func(context.Context, []string) (map[string]*parse.Tree, error)
	Location     func(context.Context, string) (*indexdb.BlockTreeRecord, error)
	Notebooks    func(context.Context) ([]conf.Notebook, error)
	Breadcrumb   func(context.Context, *ast.Node, bool, int) ([]*BlockPath, error)
	Render       func(context.Context, []*ast.Node, *lute.Lute) (string, error)
	WarningLimit func(context.Context, int) error
}
type BacklinkServices = ReadServices

type blockReadScope struct {
	ctx   context.Context
	s     BacklinkServices
	err   error
	trees map[string]*parse.Tree
}

// All original grouping helpers run synchronously within one owner admission.
// A helper failure is sticky: the entrypoint discards every partial result.
func ReadBacklinks(ctx context.Context, q BacklinkRequest, s BacklinkServices) (out BacklinkResult, err error) {
	scope := &blockReadScope{ctx: ctx, s: s, trees: map[string]*parse.Tree{}}
	defer func() {
		if v := recover(); v != nil {
			if scope.err == nil {
				scope.err = fmt.Errorf("%w: backlink projection (%T)", indexdb.ErrProjection, v)
			}
		}
		if scope.err != nil {
			out = BacklinkResult{}
			err = scope.err
		}
	}()
	if ctx == nil || s.DB == nil || s.Search == nil || s.Parser == nil || s.Parser.ParseOptions == nil || s.Parser.RenderOptions == nil || s.Load == nil || s.Trees == nil || s.Location == nil || s.Notebooks == nil || s.Breadcrumb == nil || s.Render == nil {
		return out, errors.New("backlink_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return BacklinkResult{}, err
	}
	switch q.Mode {
	case "links":
		out.Box, out.Links, out.Mentions, out.ReferenceCount, out.MentionCount = scope.GetBacklink(q.ID, q.Keyword, q.MentionKeyword, q.BeforeLen, q.Children)
	case "links2":
		out.Box, out.Links, out.Mentions, out.ReferenceCount, out.MentionCount = scope.GetBacklink2(q.ID, q.Keyword, q.MentionKeyword, q.SortMode, q.MentionSortMode, q.Children)
	case "document":
		out.Documents, out.Keywords = scope.GetBacklinkDoc(q.ID, q.RefTreeID, q.Keyword, q.Children, q.Highlight)
	case "mention-document":
		out.Documents, out.Keywords = scope.GetBackmentionDoc(q.ID, q.RefTreeID, q.Keyword, q.Children, q.Highlight)
	default:
		return out, errors.New("backlink_mode_invalid")
	}
	scope.fail(ctx.Err())
	return
}
func (o *blockReadScope) fail(e error) {
	if o.err == nil && e != nil {
		o.err = e
	}
}
func (o *blockReadScope) guard() bool {
	if o.err != nil {
		return false
	}
	o.fail(o.ctx.Err())
	return o.err == nil
}
func (o *blockReadScope) block(id string) *indexdb.BlockRecord {
	if !o.guard() {
		return nil
	}
	b, ok, e := (indexdb.CachedBlocks{DB: o.s.DB}).Get(o.ctx, id)
	o.fail(e)
	if !ok || e != nil {
		return nil
	}
	return &b
}
func (o *blockReadScope) blocks(ids []string) []*indexdb.BlockRecord {
	if !o.guard() {
		return nil
	}
	out, e := (indexdb.CachedBlocks{DB: o.s.DB}).GetMany(o.ctx, ids)
	o.fail(e)
	return out
}
func (o *blockReadScope) refs(id string, children bool) []*indexdb.RefRecord {
	if !o.guard() {
		return nil
	}
	v, e := (indexdb.References{DB: o.s.DB}).ByDefinition(o.ctx, id, children)
	o.fail(e)
	out := []*indexdb.RefRecord{}
	for i := range v {
		out = append(out, &v[i])
	}
	return out
}
func (o *blockReadScope) pair(def, ref string) []indexdb.RefRecord {
	if !o.guard() {
		return nil
	}
	v, e := (indexdb.References{DB: o.s.DB}).ByPair(o.ctx, def, ref)
	o.fail(e)
	return v
}
func (o *blockReadScope) selectRows(stmt string, page, limit int) []*indexdb.BlockRecord {
	if !o.guard() {
		return nil
	}
	rows, e := indexdb.SelectReadBlockRows(o.ctx, o.s.DB, stmt, page, limit)
	o.fail(e)
	out := []*indexdb.BlockRecord{}
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out
}
func (o *blockReadScope) children(id, condition string, limit int) []*indexdb.BlockRecord {
	if !o.guard() {
		return nil
	}
	if condition != "" {
		o.fail(errors.New("backlink_child_condition_invalid"))
		return nil
	}
	ids, e := (indexdb.References{DB: o.s.DB}).BlockChildren(o.ctx, id)
	o.fail(e)
	out := o.blocks(ids)
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
func (o *blockReadScope) fromRow(v *indexdb.BlockRecord, terms string, before int) *Block {
	return FromRow(v, terms, before, o.s.Search.CaseSensitive)
}
func (o *blockReadScope) fromRows(v *[]*indexdb.BlockRecord, terms string, before int) []*Block {
	return FromRows(v, terms, before, o.s.Search.CaseSensitive)
}
func (o *blockReadScope) newParser() *lute.Lute {
	p := *o.s.Parser
	options := *o.s.Parser.RenderOptions
	p.RenderOptions = &options
	return &p
}
func (o *blockReadScope) load(id string) (*parse.Tree, error) {
	if !o.guard() {
		return nil, o.err
	}
	if t, ok := o.trees[id]; ok {
		return t, nil
	}
	t, e := o.s.Load(o.ctx, id)
	if e == nil && t == nil {
		e = errors.New("backlink_source_missing")
	}
	o.fail(e)
	o.trees[id] = t
	return t, e
}
func (o *blockReadScope) loadTrees(ids []string) map[string]*parse.Tree {
	if !o.guard() {
		return nil
	}
	trees, e := o.s.Trees(o.ctx, ids)
	o.fail(e)
	if e == nil {
		for _, id := range ids {
			if trees[id] == nil {
				o.fail(errors.New("backlink_source_missing"))
				return nil
			}
		}
	}
	for id, t := range trees {
		o.trees[id] = t
	}
	return trees
}
func (o *blockReadScope) location(id string) *indexdb.BlockTreeRecord {
	if !o.guard() {
		return nil
	}
	v, e := o.s.Location(o.ctx, id)
	o.fail(e)
	if v == nil {
		o.fail(errors.New("backlink_location_missing"))
	}
	return v
}
func (o *blockReadScope) projectBlock(id string, t *parse.Tree) (*Block, error) {
	if !o.guard() {
		return nil, o.err
	}
	if t == nil {
		var e error
		t, e = o.load(id)
		if e != nil {
			return nil, e
		}
	}
	if t == nil {
		o.fail(errors.New("backlink_source_missing"))
		return nil, o.err
	}
	n := docview.GetNodeInTree(t, id)
	if n == nil {
		o.fail(errors.New("backlink_block_missing"))
		return nil, o.err
	}
	v, e := indexdb.ProjectBlock(o.ctx, t, n, o.s.Parser, o.s.Content)
	o.fail(e)
	return o.fromRow(v, "", 0), e
}
func (o *blockReadScope) boxNames(ids []string) map[string]string {
	out := map[string]string{}
	if !o.guard() {
		return out
	}
	books, e := o.s.Notebooks(o.ctx)
	o.fail(e)
	for _, b := range books {
		if slices.Contains(ids, b.ID) {
			out[b.ID] = b.Name
		}
	}
	return out
}
func (o *blockReadScope) strings(stmt, root string) []string {
	if !o.guard() {
		return nil
	}
	rows, e := indexdb.Query(o.ctx, o.s.DB, stmt, root)
	o.fail(e)
	if e != nil {
		return nil
	}
	defer func() { o.fail(rows.Close()) }()
	out := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			o.fail(e)
			return nil
		}
		out = append(out, v)
	}
	o.fail(rows.Err())
	return out
}
func (o *blockReadScope) names(root string) []string {
	return o.strings("SELECT DISTINCT name FROM blocks WHERE root_id=? AND name!=''", root)
}
func (o *blockReadScope) aliases(root string) []string {
	out := []string{}
	for _, raw := range o.strings("SELECT alias FROM blocks WHERE root_id=? AND alias!=''", root) {
		for _, v := range strings.Split(raw, ",") {
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}
func (o *blockReadScope) headingLevel(n *ast.Node) int {
	for ; n != nil; n = n.Previous {
		if n.Type == ast.NodeHeading {
			return n.HeadingLevel
		}
	}
	return 0
}
func (o *blockReadScope) markSpan(n *ast.Node, unlinks *[]*ast.Node, keys []string, kind string, p *lute.Lute) bool {
	return docview.MarkReplaceSpan(n, unlinks, keys, kind, p, o.s.Search.CaseSensitive)
}
func (o *blockReadScope) markSplit(text string, keys []string, start, end string) string {
	return docview.MarkSplit(text, keys, start, end, o.s.Search.CaseSensitive)
}
func (o *blockReadScope) columnFilter() string { return indexdb.SearchColumns(*o.s.Search) }
func (o *blockReadScope) warningLimit(count int) {
	if o.s.WarningLimit != nil && o.guard() {
		o.fail(o.s.WarningLimit(o.ctx, count))
	}
}
func (o *blockReadScope) fillCounts(nodes []*ast.Node) {
	if !o.guard() {
		return
	}
	ids := []string{}
	for _, n := range nodes {
		ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !o.guard() {
				return ast.WalkStop
			}
			if enter && n.IsBlock() {
				ids = append(ids, n.ID)
			}
			return ast.WalkContinue
		})
	}
	counts, e := (indexdb.References{DB: o.s.DB}).DefinitionCounts(o.ctx, ids)
	o.fail(e)
	if !o.guard() {
		return
	}
	for _, n := range nodes {
		ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !o.guard() {
				return ast.WalkStop
			}
			if enter && n.IsBlock() && counts[n.ID] > 0 {
				n.SetIALAttr("refcount", strconv.Itoa(counts[n.ID]))
			}
			return ast.WalkContinue
		})
	}
}
func (o *blockReadScope) render(nodes []*ast.Node, p *lute.Lute) string {
	if !o.guard() {
		return ""
	}
	v, e := o.s.Render(o.ctx, nodes, p)
	o.fail(e)
	return v
}
func (o *blockReadScope) breadcrumb(n *ast.Node, exclude []string, embed bool, headingMode int) []*BlockPath {
	if !o.guard() {
		return nil
	}
	v, e := o.s.Breadcrumb(o.ctx, n, embed, headingMode)
	o.fail(e)
	return v
}
