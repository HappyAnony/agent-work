// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"time"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

type NativeReferenceChangeRequest struct {
	Mode            string
	FromID, ToID    string
	RefIDs          []string
	IncludeChildren bool
}
type NativeReferenceChangePlan struct {
	Trees        []*parse.Tree
	SourceHashes map[string][32]byte
}

// PlanNativeReferenceChange prepares detached candidates. R04 owns publication.
func (l *Lease) PlanNativeReferenceChange(ctx context.Context, q NativeReferenceChangeRequest) (out NativeReferenceChangePlan, err error) {
	q.RefIDs = append([]string(nil), q.RefIDs...)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		s, hashes, guard, e := r.documentLifecycleServices(ctx, w, b, "")
		if e != nil {
			return e
		}
		cache := map[string]*parse.Tree{}
		load := func(id string) (*parse.Tree, error) {
			loc, ok, e := s.Lookup(ctx, id)
			if e != nil {
				return nil, e
			}
			if !ok {
				return nil, ErrDocument
			}
			key := loc.Box + loc.Path
			if tree := cache[key]; tree != nil {
				return tree, nil
			}
			tree, e := s.Load(ctx, loc.Box, loc.Path)
			if e == nil {
				cache[key] = tree
			}
			return tree, e
		}
		from, e := load(q.FromID)
		if e != nil {
			return e
		}
		to, e := load(q.ToID)
		if e != nil {
			return e
		}
		switch q.Mode {
		case "swap":
			stamp := time.Now().Format("20060102150405")
			changed, e := doclife.SwapReferences(ctx, from, to, q.FromID, q.ToID, q.IncludeChildren, doclife.ReferenceSwapPorts{NewID: ast.NewNodeID, Updated: func(n *ast.Node) { doclife.RefreshUpdatedAt(n, stamp) }})
			if e != nil {
				return e
			}
			if changed {
				out.Trees = append(out.Trees, from)
				if to != from {
					out.Trees = append(out.Trees, to)
				}
			}
		case "transfer":
			node := docview.GetNodeInTree(to, q.ToID)
			if node == nil {
				return ErrDocument
			}
			content := r.readBlockContent(ctx, w)
			if r.configuration.Editor == nil {
				return ErrDocument
			}
			text, e := anchor.Text(ctx, node, r.configuration.Editor.BlockRefDynamicAnchorTextMaxLen, content)
			if e != nil {
				return e
			}
			db, e := b.Database(indexdb.Main)
			if e != nil {
				return e
			}
			ids := q.RefIDs
			if len(ids) == 0 {
				ids, e = (indexdb.References{DB: db}).RefIDs(ctx, q.FromID, false)
				if e != nil {
					return e
				}
			}
			trees := make([]*parse.Tree, len(ids))
			for i, id := range ids {
				trees[i], e = load(id)
				if e != nil {
					return e
				}
			}
			seen := map[*parse.Tree]bool{}
			for i, id := range ids {
				changed, e := doclife.TransferReferences(ctx, trees[i], id, q.FromID, q.ToID, text)
				if e != nil {
					return e
				}
				if changed && !seen[trees[i]] {
					seen[trees[i]] = true
					out.Trees = append(out.Trees, trees[i])
				}
			}
		default:
			return ErrDocument
		}
		if e = guard.Confirm(ctx); e != nil {
			return e
		}
		out.SourceHashes = hashes
		return nil
	})
	if err != nil {
		return NativeReferenceChangePlan{}, err
	}
	return
}
