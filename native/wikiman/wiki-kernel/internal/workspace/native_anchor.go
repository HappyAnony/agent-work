package workspace

import (
	"context"
	"crypto/sha256"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/indexdb"
)

// Detached original AST candidates and source hashes, not serialized source or
// save receipts. The R04 serializer/unique source transaction must consume them.
type DynamicAnchorPlan struct {
	Trees        map[string]*parse.Tree
	SourceHashes map[string][32]byte
	Changes      []anchor.Change
	Hints        map[string]string
}

func (r *resources) nativeTreeByBlock(ctx context.Context, w *WorkspaceContext, id string) (*parse.Tree, error) {
	db, err := r.native.Database(indexdb.BlockTree)
	if err != nil {
		return nil, err
	}
	rows, err := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrDocument
	}
	return r.locatedTree(ctx, w, rows[0].BoxID, rows[0].RootID)
}
func (r *resources) nativeReferenceCache(ctx context.Context, id string) (ret []indexdb.RefRecord, err error) {
	ret = r.nativeRefs.Get(id)
	if len(ret) > 0 {
		return ret, nil
	}
	db, err := r.native.Database(indexdb.Main)
	if err != nil {
		return nil, err
	}
	ret, err = (indexdb.References{DB: db}).ByDefinition(ctx, id, false)
	if err != nil {
		return nil, err
	}
	for _, ref := range ret {
		if err := r.nativeRefs.Put(ref); err != nil {
			r.nativeRefs.Delete(id)
			break
		}
	}
	return ret, nil
}
func (l *Lease) PlanNativeDynamicAnchors(ctx context.Context, ids []string) (plan DynamicAnchorPlan, err error) {
	if len(ids) > 10000 {
		return DynamicAnchorPlan{}, ErrBudget
	}
	for _, id := range ids {
		if !documentID.MatchString(id) {
			return DynamicAnchorPlan{}, ErrDocument
		}
	}
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if err := r.rebuildBlockTree(ctx, w); err != nil {
				return err
			}
		}
		plan = DynamicAnchorPlan{Trees: map[string]*parse.Tree{}, SourceHashes: map[string][32]byte{}, Hints: map[string]string{}}
		loaded := map[string]*parse.Tree{}
		load := func(ctx context.Context, id string) (*parse.Tree, error) {
			db, e := r.native.Database(indexdb.BlockTree)
			if e != nil {
				return nil, e
			}
			rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
			if e != nil {
				return nil, e
			}
			if len(rows) == 0 {
				return nil, ErrDocument
			}
			row := rows[0]
			if t := loaded[row.RootID]; t != nil {
				return t, nil
			}
			raw, e := r.load(ctx, w, row.BoxID, row.RootID)
			if e != nil {
				return nil, e
			}
			t, e := r.parseOriginalTree(ctx, w, raw, row.BoxID, row.RootID)
			if e != nil {
				return nil, e
			}
			t, e = locateOriginalTree(ctx, w, t, row.BoxID)
			if e != nil {
				return nil, e
			}
			plan.SourceHashes[row.RootID] = sha256.Sum256(raw.raw)
			loaded[row.RootID] = t
			return t, nil
		}
		defs := map[string]*ast.Node{}
		for _, id := range ids {
			tree, e := load(ctx, id)
			if e != nil {
				return e
			}
			ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if enter && n.ID == id {
					defs[id] = n
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
			if defs[id] == nil {
				return ErrDocument
			}
		}
		cache := anchor.Cache{Limit: 10240}
		defer cache.Close()
		services := anchor.Services{StrictLoad: true, Load: load, Embedded: indexdb.EmbeddedReference,
			References: func(ctx context.Context, n *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error) {
				return anchor.References(ctx, n, r.nativeReferenceCache)
			},
			Text: func(ctx context.Context, n *ast.Node) (string, error) {
				if r.configuration.Editor == nil {
					return "", anchor.ErrDependency
				}
				return anchor.Text(ctx, n, r.configuration.Editor.BlockRefDynamicAnchorTextMaxLen, r.nativeContent)
			},
			Set: func(n *ast.Node, text string) error { return anchor.SetText(n, text, cache.Put) },
			Notify: func(ctx context.Context, c anchor.Change) error {
				if len(plan.Changes) >= 100000 {
					return ErrBudget
				}
				plan.Changes = append(plan.Changes, c)
				return nil
			},
			IndexRefs: func(ctx context.Context, t *parse.Tree) error { return ctx.Err() },
			Stage:     func(ctx context.Context, t *parse.Tree) error { plan.Trees[t.ID] = t; return ctx.Err() },
			UpdateAVs: func(ctx context.Context, nodes map[string]*ast.Node) error {
				for _, n := range nodes {
					for p := n; p != nil; p = p.Parent {
						if p.IALAttr("custom-avs") != "" {
							return anchor.ErrDependency
						}
					}
				}
				return ctx.Err()
			},
		}
		if e := anchor.Refresh(ctx, defs, loaded, services); e != nil {
			return e
		}
		cache.Range(func(k, v interface{}) bool { plan.Hints[k.(string)] = v.(string); return true })
		return nil
	})
	if err != nil {
		return DynamicAnchorPlan{}, err
	}
	return
}
