// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/lex"
	"github.com/88250/lute/parse"
	"wiki-kernel/cache"
	"wiki-kernel/filesys"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// These are owner candidates until R04 supplies the authoritative commit.
// A batch deliberately does not schedule dynamic anchor refreshes, as before.
type blockAttributeEnvironment struct {
	gate             *attributeMutationGate
	ctx              context.Context
	legacy, readOnly bool
	wait             func() error
	load             func(string) (*parse.Tree, error)
	loadMany         func([]string) (map[string]*parse.Tree, error)
	stage            func(*parse.Tree) error
	cache            func(string, map[string]string) error
	sync             func() error
	notify           func(map[string]string, *ast.Node) error
	evict            func(string) error
	rename           func(*parse.Tree) error
	invalid, missing func(string) error
	tags             func() error
	background       func(string, string) error
}

func legacyBlockAttributeEnvironment() *blockAttributeEnvironment {
	return legacyBlockAttributeEnvironmentContext(context.Background())
}
func legacyBlockAttributeEnvironmentContext(ctx context.Context) *blockAttributeEnvironment {
	return &blockAttributeEnvironment{gate: legacyAttributeMutationGate, ctx: ctx, legacy: true, readOnly: util.ReadOnly,
		wait: func() error { return FlushTxQueueContext(ctx) }, load: LoadTreeByBlockID,
		loadMany: func(ids []string) (map[string]*parse.Tree, error) { return filesys.LoadTrees(ids), nil }, stage: indexWriteTreeUpsertQueue,
		cache: func(id string, attrs map[string]string) error { cache.PutBlockIAL(id, attrs); return nil },
		evict: func(id string) error { cache.RemoveBlockIAL(id); return nil }, rename: func(tree *parse.Tree) error { return updateRefTextRenameDocContext(ctx, tree) },
		sync: func() error { IncSync(); return nil }, notify: func(old map[string]string, n *ast.Node) error { pushBroadcastAttrTransactions(old, n); return nil },
		invalid: func(id string) error { return fmt.Errorf(Conf.Language(25), id) }, missing: func(id string) error { return fmt.Errorf(Conf.Language(15), id) },
		tags: func() error { ReloadTag(); return nil }, background: func(id, root string) error { return queueAttributeReferences(id, root, legacySourceEventEnvironment()) },
	}
}
func (e *blockAttributeEnvironment) guard(fn func() error) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.ctx.Err()
}
func (e *blockAttributeEnvironment) failure(format func(string) error, id, code string) error {
	if err := e.guard(func() error { return format(id) }); err != nil {
		return err
	}
	return errors.New(code)
}
func (e *blockAttributeEnvironment) validate(mode string) error {
	if e == nil || e.ctx == nil || e.gate == nil || e.gate.token == nil || e.stage == nil || e.sync == nil || e.invalid == nil || e.missing == nil {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if mode == "node" || mode == "single" || mode == "batch" {
		if e.cache == nil || e.notify == nil || e.tags == nil {
			return errors.New("block_attribute_dependencies_unbound")
		}
	}
	if mode == "single" && (e.wait == nil || e.load == nil || e.background == nil) {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if mode == "batch" && (e.wait == nil || e.loadMany == nil) {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if mode == "reset" && (e.wait == nil || e.load == nil || e.evict == nil || e.rename == nil) {
		return errors.New("block_attribute_dependencies_unbound")
	}
	return e.ctx.Err()
}

func resetBlockAttributesWithEnvironment(id string, attrs map[string]string, e *blockAttributeEnvironment) error {
	if e == nil || e.ctx == nil {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.readOnly {
		return nil
	}
	if err := e.validate("reset"); err != nil {
		return err
	}
	if err := e.guard(e.wait); err != nil {
		return err
	}
	return e.gate.with(e.ctx, func() error {
		var tree *parse.Tree
		if err := e.guard(func() (err error) { tree, err = e.load(id); return }); err != nil {
			return err
		}
		if tree == nil || tree.Root == nil {
			return errors.New("invalid_block_attribute_tree")
		}
		node := treenode.GetNodeInTree(tree, id)
		if node == nil {
			return e.failure(e.missing, id, "block_attribute_node_missing")
		}
		if err := e.checkNode(node, tree); err != nil {
			return err
		}
		for name := range attrs {
			for i := 0; i < len(name); i++ {
				if !lex.IsASCIILetterNumHyphen(name[i]) {
					return e.failure(e.invalid, id, "invalid_block_attribute_name")
				}
			}
		}
		node.ClearIALAttrs()
		for name, value := range attrs {
			if value != "" {
				node.SetIALAttr(name, value)
			}
		}
		if node.Type == ast.NodeDocument {
			if err := e.guard(func() error { return e.rename(tree) }); err != nil {
				return err
			}
		}
		if err := e.guard(func() error { return e.stage(tree) }); err != nil {
			return err
		}
		if err := e.guard(e.sync); err != nil {
			return err
		}
		return e.guard(func() error { return e.evict(id) })
	})
}

func (tx *Transaction) blockAttributeEnvironment(background bool) (*blockAttributeEnvironment, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.blockAttributes == nil {
			return nil, errors.New("block_attribute_dependencies_unbound")
		}
		e := *tx.environment.blockAttributes
		e.ctx, e.legacy = tx.environment.ctx, false
		if background && !e.readOnly {
			s, err := tx.sourceEventServices(true)
			if err != nil {
				return nil, err
			}
			s.attributeGate = e.gate
			if err = s.validateAttributeRefresh(); err != nil {
				return nil, err
			}
			e.background = func(id, root string) error { return queueAttributeReferences(id, root, s) }
		}
		return &e, nil
	}
	return legacyBlockAttributeEnvironment(), nil
}
func (tx *Transaction) setBlockAttributes(id string, attrs map[string]string) error {
	e, err := tx.blockAttributeEnvironment(true)
	if err != nil {
		return err
	}
	return setBlockAttributesWithEnvironment(id, attrs, e)
}
func (tx *Transaction) batchSetBlockAttributes(input []map[string]interface{}) error {
	e, err := tx.blockAttributeEnvironment(false)
	if err != nil {
		return err
	}
	return batchSetBlockAttributesWithEnvironment(input, e)
}
func copyBlockAttributes(attrs map[string]string) map[string]string {
	ret := make(map[string]string, len(attrs))
	for k, v := range attrs {
		ret[k] = v
	}
	return ret
}
func (e *blockAttributeEnvironment) checkNode(n *ast.Node, t *parse.Tree) error {
	if n == nil || t == nil || t.Root == nil {
		return errors.New("invalid_block_attribute_node")
	}
	if !e.legacy && (n.ID == "" || t.ID == "" || t.Root.ID != t.ID || t.Box == "" || t.Path == "" || treenode.GetNodeInTree(t, n.ID) != n) {
		return errors.New("invalid_block_attribute_owner")
	}
	return nil
}
func (e *blockAttributeEnvironment) mutate(n *ast.Node, t *parse.Tree, attrs map[string]string) (map[string]string, error) {
	if err := e.checkNode(n, t); err != nil {
		return nil, err
	}
	if !e.legacy {
		attrs = copyBlockAttributes(attrs)
	}
	var old map[string]string
	err := e.guard(func() (err error) {
		old, err = setNodeAttrs0WithServices(n, attrs, func(id string) error { return e.failure(e.invalid, id, "invalid_block_attribute_name") }, func() error { return e.guard(e.tags) })
		return
	})
	return old, err
}
func setNodeAttributesWithEnvironment(n *ast.Node, t *parse.Tree, attrs map[string]string, e *blockAttributeEnvironment, background bool) error {
	if err := e.validate("node"); err != nil {
		return err
	}
	if background && e.background == nil {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if n == nil || t == nil {
		return errors.New("invalid_block_attribute_node")
	}
	id, root := n.ID, t.ID
	err := e.gate.with(e.ctx, func() error {
		return setNodeAttributesUnlocked(n, t, attrs, e)
	})
	if err != nil {
		return err
	}
	if background {
		return e.guard(func() error { return e.background(id, root) })
	}
	return nil
}
func setNodeAttributesUnlocked(n *ast.Node, t *parse.Tree, attrs map[string]string, e *blockAttributeEnvironment) error {
	if err := e.validate("node"); err != nil {
		return err
	}
	old, err := e.mutate(n, t, attrs)
	if err != nil {
		return err
	}
	if err = e.guard(func() error { return e.stage(t) }); err != nil {
		return err
	}
	if err = e.guard(e.sync); err != nil {
		return err
	}
	if err = e.guard(func() error { return e.cache(n.ID, parse.IAL2Map(n.KramdownIAL)) }); err != nil {
		return err
	}
	if err = e.guard(func() error { return e.notify(old, n) }); err != nil {
		return err
	}
	return nil
}
func setBlockAttributesWithEnvironment(id string, attrs map[string]string, e *blockAttributeEnvironment) error {
	if e == nil || e.ctx == nil {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.readOnly {
		return nil
	}
	if err := e.validate("single"); err != nil {
		return err
	}
	if err := e.guard(e.wait); err != nil {
		return err
	}
	var root string
	err := e.gate.with(e.ctx, func() error {
		var t *parse.Tree
		if err := e.guard(func() (err error) { t, err = e.load(id); return }); err != nil {
			return err
		}
		if t == nil || t.Root == nil {
			return errors.New("invalid_block_attribute_tree")
		}
		n := treenode.GetNodeInTree(t, id)
		if n == nil {
			return e.failure(e.missing, id, "block_attribute_node_missing")
		}
		root = t.ID
		return setNodeAttributesUnlocked(n, t, attrs, e)
	})
	if err != nil {
		return err
	}
	return e.guard(func() error { return e.background(id, root) })
}

func batchSetBlockAttributesWithEnvironment(input []map[string]interface{}, e *blockAttributeEnvironment) error {
	if e == nil || e.ctx == nil {
		return errors.New("block_attribute_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.readOnly {
		return nil
	}
	if err := e.validate("batch"); err != nil {
		return err
	}
	ids := make([]string, 0, len(input))
	values := make([]map[string]string, 0, len(input))
	for _, item := range input {
		id, ok := item["id"].(string)
		if !ok || id == "" {
			return errors.New("invalid_block_attribute_input")
		}
		attrs, ok := item["attrs"].(map[string]string)
		if !ok {
			return errors.New("invalid_block_attribute_input")
		}
		ids = append(ids, id)
		values = append(values, attrs)
	}
	if err := e.guard(e.wait); err != nil {
		return err
	}
	return e.gate.with(e.ctx, func() error {
		var trees map[string]*parse.Tree
		if err := e.guard(func() (err error) { trees, err = e.loadMany(ids); return }); err != nil {
			return err
		}
		unique := map[string]*parse.Tree{}
		for i, id := range ids {
			t := trees[id]
			if t == nil || t.Root == nil {
				return e.failure(e.missing, id, "block_attribute_node_missing")
			}
			if previous := unique[t.ID]; previous != nil {
				if !e.legacy && previous != t {
					return errors.New("inconsistent_block_attribute_tree")
				}
				t = previous // One candidate per root; duplicate IDs apply in input order.
			} else {
				unique[t.ID] = t
			}
			n := treenode.GetNodeInTree(t, id)
			if n == nil {
				return e.failure(e.missing, id, "block_attribute_node_missing")
			}
			old, err := e.mutate(n, t, values[i])
			if err != nil {
				return err
			}
			if err = e.guard(func() error { return e.cache(id, parse.IAL2Map(n.KramdownIAL)) }); err != nil {
				return err
			}
			if err = e.guard(func() error { return e.notify(old, n) }); err != nil {
				return err
			}
		}
		for _, t := range unique {
			if err := e.guard(func() error { return e.stage(t) }); err != nil {
				return err
			}
		}
		return e.guard(e.sync)
	})
}
