// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"time"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Native enqueue/push/stage services belong to the owner. Until R04, they are
// candidates, not permission to publish a notification ahead of source commit.
type sourceEventEnvironment struct {
	attributeGate *attributeMutationGate
	avText        *attributeViewTextEnvironment
	bindingErr    error
	ctx           context.Context
	publication   context.Context
	jobContext    context.Context
	launch        func(func(context.Context) error) error
	legacy        bool
	flush         func() error
	tree          func(string) (*parse.Tree, error)
	lookup        func(string) (*treenode.BlockTree, error)
	children      func(string) ([]string, error)
	references    func(string, bool) ([]string, error)
	blockTrees    func([]string) (map[string]*treenode.BlockTree, error)
	anchor        *anchor.Services
	enqueue       func(string, string, time.Duration, func() error) error
	pushReload    func(string) error
	pushCount     func(string, string, []string, int, int) error
}

func legacySourceEventEnvironment() *sourceEventEnvironment {
	return legacySourceEventEnvironmentContext(context.Background())
}

func legacySourceEventEnvironmentContext(ctx context.Context) *sourceEventEnvironment {
	owner, ownerErr := captureLegacyAnchorContext(ctx)
	return sourceEventEnvironmentForOwner(ctx, owner, ownerErr)
}

func sourceEventEnvironmentForOwner(ctx context.Context, owner *legacyAnchorContext, ownerErr error) *sourceEventEnvironment {
	uiOwner, uiOwnerErr := captureLegacyDelayedUIOwner(ctx)
	if ownerErr == nil && uiOwnerErr != nil {
		ownerErr = uiOwnerErr
	}
	return sourceEventEnvironmentWithOwners(ctx, ctx, owner, uiOwner, ownerErr)
}

func sourceEventEnvironmentWithOwners(ctx, publication context.Context, owner *legacyAnchorContext, uiOwner *legacyDelayedUIOwner, ownerErr error) *sourceEventEnvironment {
	if ownerErr == nil && (owner == nil || owner.r == nil || uiOwner == nil || uiOwner.owner != owner.r.owner || uiOwner.data != owner.r.data || uiOwner.workspace != owner.r.workspace) {
		ownerErr = errors.New("source_event_owner_mismatch")
	}
	e := &sourceEventEnvironment{ctx: ctx, publication: publication, legacy: true, attributeGate: legacyAttributeMutationGate,
		pushReload: func(id string) error { util.PushReloadProtyle(id); return nil },
		pushCount: func(root, block string, defs []string, count, rootCount int) error {
			util.PushSetDefRefCount(root, block, defs, count, rootCount)
			return nil
		},
	}
	if uiOwner != nil {
		e.enqueue = func(kind, key string, delay time.Duration, fn func() error) error {
			return uiOwner.schedule(publication, kind, delay, func() {
				if err := fn(); err != nil {
					logging.LogErrorf("source event failed: %s", err)
				}
			}, key)
		}
	}
	if ownerErr != nil {
		e.bindingErr = ownerErr
		e.launch = func(func(context.Context) error) error { return ownerErr }
		e.flush = func() error { return ownerErr }
		e.tree = func(string) (*parse.Tree, error) { return nil, ownerErr }
		e.lookup = func(string) (*treenode.BlockTree, error) { return nil, ownerErr }
		e.children = func(string) ([]string, error) { return nil, ownerErr }
		e.references = func(string, bool) ([]string, error) { return nil, ownerErr }
		e.blockTrees = func([]string) (map[string]*treenode.BlockTree, error) { return nil, ownerErr }
		return e
	}
	e.launch = func(fn func(context.Context) error) error {
		if !owner.r.owner.startRepoBackground(func(ctx context.Context) {
			if err := fn(ctx); err != nil {
				logging.LogErrorf("attribute refresh failed: %s", err)
			}
		}) {
			return errors.New("attribute_refresh_owner_closed")
		}
		return nil
	}
	a := owner.services(ctx)
	e.anchor = &a
	refs := indexdb.References{DB: owner.r.main.DB}
	e.flush = func() error { return owner.r.queue.Flush(ctx) }
	e.tree = func(id string) (*parse.Tree, error) {
		bt, err := owner.r.referenceLookup(ctx, id)
		if err != nil || bt == nil {
			return nil, err
		}
		return owner.r.loadRepairTree(ctx, bt.BoxID, bt.Path, owner.commit.parser())
	}
	e.lookup = func(id string) (*treenode.BlockTree, error) {
		bt, err := owner.r.referenceLookup(ctx, id)
		if err != nil || bt == nil {
			return nil, err
		}
		return &treenode.BlockTree{ID: bt.ID, RootID: bt.RootID, ParentID: bt.ParentID, BoxID: bt.BoxID, Path: bt.Path, HPath: bt.HPath, Updated: bt.Updated, Type: bt.Type}, nil
	}
	e.children = func(id string) ([]string, error) { return refs.ChildDefinitionIDs(ctx, id) }
	e.references = func(id string, children bool) ([]string, error) { return refs.RefIDs(ctx, id, children) }
	e.blockTrees = func(ids []string) (map[string]*treenode.BlockTree, error) {
		rows, err := owner.r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
		if err != nil {
			return nil, err
		}
		ret := map[string]*treenode.BlockTree{}
		for _, bt := range rows {
			ret[bt.ID] = &treenode.BlockTree{ID: bt.ID, RootID: bt.RootID, ParentID: bt.ParentID, BoxID: bt.BoxID, Path: bt.Path, HPath: bt.HPath, Updated: bt.Updated, Type: bt.Type}
		}
		return ret, nil
	}
	e.pushReload = func(id string) error {
		if err := owner.r.guard(publication); err != nil {
			return err
		}
		// enqueue already runs this callback inside uiOwner.publish. Reentering
		// publishSyncUI here would acquire the same owner's repoMu twice.
		util.PushReloadProtyle(id)
		return owner.r.guard(publication)
	}
	e.pushCount = func(root, block string, defs []string, count, rootCount int) error {
		if err := owner.r.guard(ctx); err != nil {
			return err
		}
		owner.r.owner.publishSyncUI(ctx, func() { util.PushSetDefRefCount(root, block, defs, count, rootCount) })
		return owner.r.guard(ctx)
	}
	return e
}

func (e *sourceEventEnvironment) guard(fn func() error) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.jobContext != nil {
		if err := e.jobContext.Err(); err != nil {
			return err
		}
	}
	if err := fn(); err != nil {
		return err
	}
	if e.jobContext != nil {
		if err := e.jobContext.Err(); err != nil {
			return err
		}
	}
	return e.ctx.Err()
}

func (e *sourceEventEnvironment) publicationGuard(fn func() error) error {
	ctx := e.publication
	if ctx == nil {
		ctx = e.ctx
	}
	if ctx == nil {
		return errors.New("source_event_publication_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.bindingErr != nil {
		return e.bindingErr
	}
	if err := fn(); err != nil {
		return err
	}
	return ctx.Err()
}
func (e *sourceEventEnvironment) validate(reload bool) error {
	if e == nil || e.ctx == nil || e.children == nil || e.references == nil {
		return errors.New("source_event_dependencies_unbound")
	}
	if e.bindingErr != nil {
		return e.bindingErr
	}
	if reload {
		if e.tree == nil || e.blockTrees == nil || e.enqueue == nil || e.pushReload == nil || e.anchor == nil {
			return errors.New("source_event_dependencies_unbound")
		}
		a := e.anchor
		if a.References == nil || a.Load == nil || a.Text == nil || a.Set == nil || a.Embedded == nil || a.Notify == nil || a.IndexRefs == nil || a.UpdateAVs == nil || a.Stage == nil {
			return anchor.ErrDependency
		}
	} else if e.flush == nil || e.lookup == nil || e.pushCount == nil {
		return errors.New("source_event_dependencies_unbound")
	}
	return e.ctx.Err()
}

// Wrap every original anchor callback, including setters without a ctx parameter.
// An owner callback returning nil after cancellation must not trigger the next one.
func (e *sourceEventEnvironment) anchorServices() anchor.Services {
	a := *e.anchor
	return anchor.Services{StrictLoad: !e.legacy || a.StrictLoad,
		References: func(_ context.Context, n *ast.Node) (r []indexdb.RefRecord, nodes []*ast.Node, err error) {
			err = e.guard(func() (err error) { r, nodes, err = a.References(e.ctx, n); return })
			return
		},
		Load: func(_ context.Context, id string) (t *parse.Tree, err error) {
			err = e.guard(func() (err error) { t, err = a.Load(e.ctx, id); return })
			return
		},
		Text: func(_ context.Context, n *ast.Node) (s string, err error) {
			err = e.guard(func() (err error) { s, err = a.Text(e.ctx, n); return })
			return
		},
		Set:      func(n *ast.Node, text string) error { return e.guard(func() error { return a.Set(n, text) }) },
		Embedded: a.Embedded,
		Notify: func(_ context.Context, c anchor.Change) error {
			return e.guard(func() error { return a.Notify(e.ctx, c) })
		},
		IndexRefs: func(_ context.Context, t *parse.Tree) error {
			return e.guard(func() error { return a.IndexRefs(e.ctx, t) })
		},
		UpdateAVs: func(_ context.Context, nodes map[string]*ast.Node) error {
			return e.guard(func() error {
				if !e.legacy && e.avText != nil {
					av := *e.avText
					av.ctx, av.jobContext = e.ctx, e.jobContext
					return updateAttributeViewBlockTextWithEnvironment(nodes, &av)
				}
				return a.UpdateAVs(e.ctx, nodes)
			})
		},
		Stage: func(_ context.Context, t *parse.Tree) error {
			return e.guard(func() error { return a.Stage(e.ctx, t) })
		},
	}
}

func (tx *Transaction) sourceEventServices(reload bool) (*sourceEventEnvironment, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.sourceEvents == nil {
			return nil, errors.New("source_event_dependencies_unbound")
		}
		e := *tx.environment.sourceEvents
		e.ctx, e.legacy = tx.environment.ctx, false
		if reload && e.anchor != nil {
			av := tx.attributeViewTextEnvironment()
			if err := av.validate(); err != nil {
				return nil, err
			}
			e.avText = av
			a := *e.anchor
			a.UpdateAVs = func(_ context.Context, nodes map[string]*ast.Node) error {
				return updateAttributeViewBlockTextWithEnvironment(nodes, av)
			}
			e.anchor = &a
		}
		return &e, nil
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	e := legacySourceEventEnvironmentContext(ctx)
	return e, nil
}
func (tx *Transaction) sourceEventEnvironment(reload bool) (*sourceEventEnvironment, error) {
	e, err := tx.sourceEventServices(reload)
	if err != nil {
		return nil, err
	}
	return e, e.validate(reload)
}
func (tx *Transaction) reloadProtyle(root string) error {
	e, err := tx.sourceEventEnvironment(true)
	if err != nil {
		return err
	}
	return reloadProtyleWithEnvironment(root, e)
}
func (tx *Transaction) refreshRefCount(block string) error {
	e, err := tx.sourceEventEnvironment(false)
	if err != nil {
		return err
	}
	return refreshRefCountWithEnvironment(block, e)
}

func reloadProtyleWithEnvironment(root string, e *sourceEventEnvironment) error {
	if err := e.validate(true); err != nil {
		return err
	}
	var tree *parse.Tree
	if err := e.guard(func() (err error) { tree, err = e.tree(root); return }); err != nil && !e.legacy {
		return err
	}
	if tree != nil {
		if tree.Root == nil || (!e.legacy && (tree.ID != root || tree.Root.ID != root)) {
			return errors.New("invalid_reload_tree")
		}
		var ids []string
		if err := e.guard(func() (err error) { ids, err = e.children(root); return }); err != nil {
			return err
		}
		set := map[string]bool{}
		for _, id := range ids {
			set[id] = true
		}
		var defs []*ast.Node
		ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if entering && n.IsBlock() && set[n.ID] {
				defs = append(defs, n)
			}
			return ast.WalkContinue
		})
		for _, n := range defs {
			if err := e.guard(func() error {
				return anchor.Refresh(e.ctx, map[string]*ast.Node{n.ID: n}, map[string]*parse.Tree{tree.ID: tree}, e.anchorServices())
			}); err != nil {
				if !e.legacy {
					return err
				}
				logging.LogErrorf("refresh anchors failed: %s", err)
			}
		}
	}
	var refs []string
	if err := e.guard(func() (err error) { refs, err = e.references(root, true); return }); err != nil {
		return err
	}
	var trees map[string]*treenode.BlockTree
	if err := e.guard(func() (err error) { trees, err = e.blockTrees(refs); return }); err != nil {
		return err
	}
	var roots []string
	for _, bt := range trees {
		if bt == nil || (!e.legacy && bt.RootID == "") {
			return errors.New("invalid_reload_blocktree")
		}
		roots = append(roots, bt.RootID)
	}
	for _, id := range uniqueUpdatedAttributeViews(roots) {
		id := id
		if err := e.guard(func() error {
			return e.enqueue(task.ReloadProtyle, id, 200*time.Millisecond, func() error { return e.publicationGuard(func() error { return e.pushReload(id) }) })
		}); err != nil {
			return err
		}
	}
	// Keep the final requested-root event even when it also occurs in embeds.
	return e.guard(func() error {
		return e.enqueue(task.ReloadProtyle, root, 200*time.Millisecond, func() error { return e.publicationGuard(func() error { return e.pushReload(root) }) })
	})
}

func refreshRefCountWithEnvironment(block string, e *sourceEventEnvironment) error {
	if err := e.validate(false); err != nil {
		return err
	}
	if err := e.guard(e.flush); err != nil {
		return err
	}
	count, err := indexrepair.ReadReferenceCount(e.ctx, block, indexrepair.CountServices{
		Lookup: func(ctx context.Context, id string) (out *indexdb.BlockTreeRecord, err error) {
			err = e.guard(func() (err error) {
				bt, err := e.lookup(id)
				if bt != nil {
					v := indexdb.BlockTreeRecord(*bt)
					out = &v
				}
				return err
			})
			return
		},
		References: func(ctx context.Context, id string, children bool) (out []string, err error) {
			err = e.guard(func() (err error) { out, err = e.references(id, children); return })
			return
		},
		Children: func(ctx context.Context, id string) (out []string, err error) {
			err = e.guard(func() (err error) { out, err = e.children(id); return })
			return
		},
	})
	if err != nil {
		return err
	}
	if count == nil {
		return nil
	}
	return e.guard(func() error {
		return e.pushCount(count.RootID, count.BlockID, count.DefinitionIDs, count.Count, count.RootCount)
	})
}
