// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"wiki-kernel/cache"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

func runOwnedBlockAttributes(ctx context.Context, fn func(*blockAttributeEnvironment) error) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	return base.run(ctx, func(c, life context.Context) error {
		anchor, e := captureLegacyAnchorContext(c)
		if e != nil {
			return e
		}
		commit := anchor.commit
		r := anchor.r
		ui, e := captureLegacyDelayedUIOwner(c)
		if e != nil {
			return e
		}
		common := bookmarkMutationWithOwner(c, life, commit)
		evict := cache.CaptureBlockIALEvicter()
		backgroundCtx := context.WithoutCancel(c)
		background := sourceEventEnvironmentWithOwners(backgroundCtx, life, anchor, ui, nil)
		env := &blockAttributeEnvironment{ctx: c, legacy: true, gate: legacyAttributeMutationGate,
			wait: func() error {
				if e := FlushTxQueueContext(c); e != nil {
					return e
				}
				return r.queue.Flush(c)
			}, load: common.load,
			loadMany: func(ids []string) (map[string]*parse.Tree, error) {
				out := map[string]*parse.Tree{}
				for _, id := range ids {
					t, e := common.load(id)
					if e != nil {
						return nil, e
					}
					out[id] = t
				}
				return out, nil
			},
			stage: func(tree *parse.Tree) error {
				if e := common.stage(tree); e != nil {
					return e
				}
				return r.registry.Upsert(c, tree)
			}, cache: common.cache,
			sync: func() error {
				if e := r.guard(c); e != nil {
					return e
				}
				r.owner.syncRuntime.same.Store(0)
				r.owner.planSyncAfter(commit.interval)
				return nil
			},
		}
		env.evict = func(id string) error {
			if e := r.guard(c); e != nil {
				return e
			}
			evict(id)
			return nil
		}
		env.rename = func(tree *parse.Tree) error { return updateRefTextRenameDocContext(c, tree) }
		env.invalid = func(id string) error { return fmt.Errorf("%s", fmt.Sprintf(r.language(25), id)) }
		env.missing = func(id string) error { return fmt.Errorf("%s", fmt.Sprintf(r.language(15), id)) }
		env.tags = func() error {
			if e := r.guard(c); e != nil {
				return e
			}
			if !ui.publish(c, util.PushReloadTag) {
				return errors.New("attribute_owner_closed")
			}
			return nil
		}
		env.notify = func(old map[string]string, node *ast.Node) error {
			if e := r.guard(c); e != nil {
				return e
			}
			if !ui.publish(c, func() { pushBroadcastAttrTransactions(old, node) }) {
				return errors.New("attribute_owner_closed")
			}
			return nil
		}
		env.background = func(id, root string) error { return queueAttributeReferences(id, root, background) }
		return fn(env)
	})
}

func ReadBlockAttributesContext(ctx context.Context, ids []string) (map[string]map[string]string, error) {
	ids = append([]string(nil), ids...)
	return runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (map[string]map[string]string, error) {
		if o.settings.Editor == nil || o.settings.Editor.Markdown == nil {
			return nil, errors.New("attribute_parser_unbound")
		}
		o.parser = nativeparser.NewLute(*o.settings.Editor.Markdown)
		out := map[string]map[string]string{}
		for _, id := range ids {
			t, e := o.loadOptional(c, id)
			if e != nil {
				return nil, e
			}
			if t == nil {
				continue
			}
			n := getAttributeNode(t, id)
			if n == nil {
				continue
			}
			attrs := map[string]string{}
			for _, kv := range n.KramdownIAL {
				if len(kv) != 2 {
					return nil, errors.New("invalid_block_attributes")
				}
				attrs[kv[0]] = html.UnescapeAttrVal(kv[1])
			}
			out[id] = attrs
		}
		return out, nil
	})
}
func getAttributeNode(tree *parse.Tree, id string) *ast.Node {
	var found *ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.ID == id {
			found = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return found
}
