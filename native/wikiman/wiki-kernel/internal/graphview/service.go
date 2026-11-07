// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package graphview

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"io/fs"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

type Block = blockview.Block

func DailyDirectories(ctx context.Context, books []conf.Notebook, read func(context.Context, string) ([]byte, error)) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, book := range books {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if book.Closed {
			continue
		}
		if !ast.IsNodeIDPattern(book.ID) {
			return nil, errors.New("graph_notebook_invalid")
		}
		cfg := conf.NewBoxConf()
		raw, e := read(ctx, book.ID+"/.siyuan/conf.json")
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return nil, e
		}
		if e == nil {
			if e = json.Unmarshal(raw, cfg); e != nil {
				return nil, e
			}
		}
		if strings.Count(cfg.DailyNoteSavePath, "/") > 1 {
			p := "/" + strings.Split(cfg.DailyNoteSavePath, "/")[1]
			if !seen[p] {
				out = append(out, p)
				seen[p] = true
			}
		}
	}
	return out, ctx.Err()
}

type Services struct {
	DB         *sql.DB
	Graph      conf.Graph
	Search     conf.Search
	Parser     *lute.Lute
	DailyPaths []string
}
type Result struct {
	Box   string
	Nodes []*GraphNode
	Links []*GraphLink
}
type run struct {
	Services
	ctx context.Context
	err error
}

func Build(ctx context.Context, id, query string, s Services) (Result, error) {
	if ctx == nil || s.DB == nil || s.Parser == nil || s.Graph.Local == nil || s.Graph.Global == nil || s.Graph.Local.TypeFilter == nil || s.Graph.Global.TypeFilter == nil || s.Graph.Local.D3 == nil || s.Graph.Global.D3 == nil {
		return Result{}, errors.New("graph_dependencies_unbound")
	}
	if s.Graph.MaxBlocks < 1 || s.Graph.MaxBlocks > 100000 || len(query) > 4096 {
		return Result{}, errors.New("graph_budget")
	}
	r := &run{Services: s, ctx: ctx}
	if r.stop() {
		return Result{}, r.err
	}
	var out Result
	if id == "" {
		out.Box, out.Nodes, out.Links = r.BuildGraph(query)
	} else {
		out.Box, out.Nodes, out.Links = r.BuildTreeGraph(id, query)
	}
	if r.stop() {
		return Result{}, r.err
	}
	return out, nil
}
func (r *run) stop() bool {
	if r.err == nil {
		r.err = r.ctx.Err()
	}
	return r.err != nil
}
func (r *run) warn(string, ...interface{}) {}
func getBlockIn(blocks []*Block, id string) *Block {
	for _, b := range blocks {
		if b.ID == id {
			return b
		}
	}
	return nil
}
func (r *run) rows(stmt string) []*Block {
	if r.stop() {
		return nil
	}
	rows, e := indexdb.SelectReadBlockRows(r.ctx, r.DB, stmt, 1, r.Graph.MaxBlocks)
	if e != nil {
		r.err = e
		return nil
	}
	return r.project(rows)
}
func (r *run) project(rows []indexdb.BlockRecord) (out []*Block) {
	for i := range rows {
		if r.stop() {
			return nil
		}
		out = append(out, blockview.FromRow(&rows[i], "", 0, r.Search.CaseSensitive))
	}
	return
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func list(ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, quote(id))
	}
	return strings.Join(parts, ",")
}
func (r *run) block(id string) *Block {
	rows := r.rows("SELECT * FROM blocks WHERE id=" + quote(id))
	if len(rows) == 0 {
		if r.err == nil {
			r.err = errors.New("graph_block_missing")
		}
		return nil
	}
	return rows[0]
}
func (r *run) roots() []*Block { return r.rows("SELECT * FROM blocks WHERE type='d'") }
func (r *run) allChildren(ids []string, condition string, limit int) []*Block {
	if len(ids) == 0 {
		return []*Block{}
	}
	stmt := "SELECT * FROM blocks AS ref WHERE ref.root_id IN (" + list(ids) + ")"
	if condition != "" {
		stmt += " AND " + condition
	}
	return r.rows(stmt + fmt.Sprintf(" LIMIT %d", limit))
}
func (r *run) children(id, condition string, limit int) []*Block {
	if r.stop() {
		return nil
	}
	ids, e := (indexdb.References{DB: r.DB}).BlockChildren(r.ctx, id)
	if e != nil {
		r.err = e
		return nil
	}
	if len(ids) == 0 {
		return []*Block{}
	}
	stmt := "SELECT * FROM blocks AS ref WHERE ref.id IN (" + list(ids) + ")"
	if condition != "" {
		stmt += " AND " + condition
	}
	return r.rows(stmt + fmt.Sprintf(" LIMIT %d", limit))
}
func (r *run) defRoots(id string) []*Block {
	if r.stop() {
		return nil
	}
	rows, e := (indexdb.References{DB: r.DB}).DefinitionRoots(r.ctx, id)
	if e != nil {
		r.err = e
		return nil
	}
	return r.project(rows)
}
func (r *run) refRoots(ids []string) map[string][]*Block {
	out := map[string][]*Block{}
	if r.stop() {
		return out
	}
	rows, e := (indexdb.References{DB: r.DB}).ReferringRoots(r.ctx, ids)
	if e != nil {
		r.err = e
		return out
	}
	for id, values := range rows {
		out[id] = r.project(values)
	}
	return out
}
func (r *run) tags(p string) []indexdb.SpanRecord {
	if r.stop() {
		return nil
	}
	rows, e := indexdb.TagSpans(r.ctx, r.DB, "path", p, r.Graph.MaxBlocks)
	if e != nil {
		r.err = e
	}
	return rows
}
func (r *run) fullLinks(condition string) (forward, back []*Block) {
	if r.stop() {
		return nil, nil
	}
	pairs, e := (indexdb.References{DB: r.DB}).DefinitionPairs(r.ctx, condition, r.Graph.MaxBlocks)
	if e != nil {
		r.err = e
		return nil, nil
	}
	defs, refs := map[string]*Block{}, map[string]*Block{}
	for _, pair := range pairs {
		if r.stop() {
			return nil, nil
		}
		ref, def := pair.Reference, pair.Definition
		if refs[ref.ID] == nil {
			refs[ref.ID] = blockview.FromRow(&ref, "", 0, r.Search.CaseSensitive)
		}
		if defs[ref.ID] == nil {
			defs[ref.ID] = refs[ref.ID]
		}
		if defs[def.ID] == nil {
			defs[def.ID] = blockview.FromRow(&def, "", 0, r.Search.CaseSensitive)
		}
	}
	for _, pair := range pairs {
		if r.stop() {
			return nil, nil
		}
		ref, def := refs[pair.Reference.ID], defs[pair.Definition.ID]
		if ref.ID == def.ID {
			continue
		}
		ref.Defs = append(ref.Defs, def)
		def.Refs = append(def.Refs, ref)
	}
	for _, def := range defs {
		back = append(back, def)
		forward = append(forward, def.Refs...)
	}
	return
}
