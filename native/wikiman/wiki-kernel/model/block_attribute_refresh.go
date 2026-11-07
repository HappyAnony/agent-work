// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/treenode"
)

func (e *sourceEventEnvironment) validateAttributeRefresh() error {
	if e == nil || e.ctx == nil || e.attributeGate == nil || e.attributeGate.token == nil || e.flush == nil || e.tree == nil || e.launch == nil || e.anchor == nil {
		return errors.New("attribute_refresh_dependencies_unbound")
	}
	a := e.anchor
	if a.References == nil || a.Load == nil || a.Text == nil || a.Set == nil || a.Embedded == nil || a.Notify == nil || a.IndexRefs == nil || a.UpdateAVs == nil || a.Stage == nil {
		return anchor.ErrDependency
	}
	return e.ctx.Err()
}

// Queue identities, never a mutable AST snapshot from the API handler. The
// owner joins callbacks on close; native stage/notify remain R04 candidates.
func queueAttributeReferences(id, root string, e *sourceEventEnvironment) error {
	if err := e.validateAttributeRefresh(); err != nil {
		return err
	}
	if id == "" || root == "" {
		return errors.New("invalid_attribute_refresh_identity")
	}
	return e.guard(func() error {
		return e.launch(func(job context.Context) error {
			if job == nil {
				return errors.New("attribute_refresh_job_context_unbound")
			}
			if err := job.Err(); err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(e.ctx)
			defer cancel()
			stop := context.AfterFunc(job, cancel)
			defer stop()
			owned := *e
			owned.ctx, owned.jobContext = ctx, job
			return owned.attributeGate.with(owned.ctx, func() error {
				if err := owned.guard(owned.flush); err != nil {
					return err
				}
				var tree *parse.Tree
				if err := owned.guard(func() (err error) { tree, err = owned.tree(root); return }); err != nil {
					return err
				}
				if tree == nil || tree.Root == nil || tree.ID != root || tree.Root.ID != root {
					return errors.New("invalid_attribute_refresh_tree")
				}
				n := treenode.GetNodeInTree(tree, id)
				if n == nil {
					return nil
				} // Removed before the owned task ran.
				return owned.guard(func() error {
					return anchor.Refresh(owned.ctx, map[string]*ast.Node{id: n}, map[string]*parse.Tree{root: tree}, owned.anchorServices())
				})
			})
		})
	})
}
