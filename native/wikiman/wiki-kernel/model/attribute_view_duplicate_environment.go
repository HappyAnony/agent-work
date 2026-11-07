// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

// Independent duplicate API: adapters stage owned candidates; R04 commits them.
type attributeViewDuplicateEnvironment struct {
	ownerGuard func() error
	complete   func() error
	ctx        context.Context
	legacy     bool
	source     func(string) (*av.AttributeView, []byte, error)
	usedID     func(string) (bool, error)
	newID      func() string
	now        func() time.Time
	registry   func(string, string) error
	stage      func(string, []byte) error
	bindings   *attributeViewDuplicateBindingsEnvironment
}

type attributeViewDuplicateBindingsEnvironment struct {
	strictDependencies bool
	names              func(string) (string, error)
	exists             func(string) (bool, error)
	node               func(string) (*ast.Node, *parse.Tree, error)
	name               func(string) (string, error)
	untitled           func() string
	attrs              func(*ast.Node, map[string]string) (map[string]string, error)
	cache              func(string, map[string]string) error
	notify             func(map[string]string, *ast.Node) error
	tree               func(*parse.Tree) error // Includes the original mirror-registry update.
}

func (e *attributeViewDuplicateEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.source == nil || e.usedID == nil || e.newID == nil || e.now == nil || e.registry == nil || e.stage == nil || e.bindings == nil {
		return errors.New("av_duplicate_dependencies_unbound")
	}
	b := e.bindings
	if b.exists == nil || b.node == nil || b.name == nil || b.untitled == nil || b.attrs == nil || b.cache == nil || b.notify == nil || b.tree == nil {
		return errors.New("av_duplicate_binding_dependencies_unbound")
	}
	return e.ctx.Err()
}

func legacyAttributeViewDuplicateEnvironment(contexts ...context.Context) *attributeViewDuplicateEnvironment {
	ctx := legacyAVContext(contexts)
	return duplicateWithBinding(nil, newLegacyAVBinding(ctx), ctx)
}
func duplicateWithBinding(tx *Transaction, b *legacyAVBinding, ctx context.Context) *attributeViewDuplicateEnvironment {
	return &attributeViewDuplicateEnvironment{ctx: ctx, legacy: true, ownerGuard: b.liveSourceGuard, complete: b.complete,
		source: b.duplicateSource, usedID: func(id string) (bool, error) { return b.duplicateIDUsed(tx, id) }, newID: ast.NewNodeID,
		now: func() time.Time {
			if b.err != nil {
				return time.Time{}
			}
			return b.scope.now
		}, registry: func(id, block string) error { return b.reserveDuplicateIDs(tx, id, block) }, stage: b.createDuplicateSource, bindings: duplicateBindings(tx, b)}
}

func (tx *Transaction) attributeViewDuplicateEnvironment() *attributeViewDuplicateEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avDuplicate == nil {
			return nil
		}
		e := *tx.environment.avDuplicate
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	return duplicateWithBinding(tx, legacyAVBindingForTransaction(tx), transactionAVContext(tx))
}

func (tx *Transaction) duplicateDatabaseBlock(id string) (string, string, error) {
	return duplicateDatabaseBlockWithEnvironment(id, tx.attributeViewDuplicateEnvironment())
}

func duplicateDatabaseBlockWithEnvironment(id string, e *attributeViewDuplicateEnvironment) (newID, blockID string, err error) {
	// IDs enter a legacy file path, so reject path fragments even in compatibility.
	if id == "" || strings.ContainsAny(id, "/\\\x00") || id == "." || id == ".." {
		return "", "", errors.New("invalid_av_duplicate_id")
	}
	if err = e.validate(); err != nil {
		return
	}
	guard := func(fn func() error) error {
		if e.ownerGuard != nil {
			if err := e.ownerGuard(); err != nil {
				return err
			}
		}
		if err := e.ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		if e.ownerGuard != nil {
			return e.ownerGuard()
		}
		return e.ctx.Err()
	}
	var old *av.AttributeView
	var raw []byte
	if err = guard(func() (readErr error) { old, raw, readErr = e.source(id); return }); err != nil {
		return
	}
	if err = validateCellSource(old); err != nil {
		return
	}
	if old.ID != id {
		err = errors.New("invalid_av_duplicate_source")
		return
	}
	var ids []string
	for i := 0; i < 2; i++ {
		var next string
		if err = guard(func() error {
			next = e.newID()
			if err := e.ctx.Err(); err != nil {
				return err
			}
			if next == "" || next == id || strings.ContainsAny(next, "/\\\x00") || next == "." || next == ".." || (len(ids) > 0 && next == ids[0]) {
				return errors.New("invalid_av_duplicate_generated_id")
			}
			used, err := e.usedID(next)
			if err != nil {
				return err
			}
			if used {
				return errors.New("av_duplicate_id_collision")
			}
			return nil
		}); err != nil {
			return
		}
		ids = append(ids, next)
	}
	newID, blockID = ids[0], ids[1]
	// Keep the original whole-JSON replacement, including self references and text.
	copy := &av.AttributeView{}
	if err = gulu.JSON.UnmarshalJSON(bytes.ReplaceAll(raw, []byte(id), []byte(newID)), copy); err != nil {
		return
	}
	if err = validateCellSource(copy); err != nil {
		return
	}
	if copy.ID != newID || copy.GetBlockKeyValues() == nil {
		err = errors.New("invalid_av_duplicate_candidate")
		return
	}
	if copy.Name != "" {
		if err = guard(func() error {
			copy.Name = old.Name + " (Duplicated " + e.now().Format("2006-01-02 15:04:05") + ")"
			return nil
		}); err != nil {
			return
		}
	}
	for _, kv := range copy.KeyValues {
		if kv.Key.Relation != nil && kv.Key.Relation.IsTwoWay {
			kv.Key.Relation.IsTwoWay, kv.Key.Relation.BackKeyID = false, ""
		}
	}
	raw, err = gulu.JSON.MarshalJSON(copy)
	if err != nil {
		return
	}
	// Validate/serialize first; malformed data must not reserve a mirror ID.
	if err = guard(func() error { return e.registry(newID, blockID) }); err != nil {
		return
	}
	if err = guard(func() error { return e.stage(newID, raw) }); err != nil {
		return
	}
	err = refreshDuplicateDatabaseBindings(copy, e, guard)
	if err == nil && e.complete != nil {
		err = e.complete()
	}
	return
}

func refreshDuplicateDatabaseBindings(a *av.AttributeView, e *attributeViewDuplicateEnvironment, guard func(func() error) error) error {
	return refreshAttributeViewBoundAttributes([]*av.AttributeView{a}, e.ctx, e.legacy, e.bindings, guard)
}

func refreshAttributeViewBoundAttributes(views []*av.AttributeView, ctx context.Context, legacy bool, b *attributeViewDuplicateBindingsEnvironment, guard func(func() error) error) error {
	owned := map[string]*parse.Tree{}
	var changed []*parse.Tree
	for _, a := range views {
		for _, value := range a.GetBlockKeyValues().Values {
			if err := ctx.Err(); err != nil {
				return err
			}
			if value.IsDetached {
				continue
			}
			// Original helper deliberately uses row BlockID, not value.Block.ID.
			id := value.BlockID
			var exists bool
			if err := guard(func() (err error) { exists, err = b.exists(id); return }); err != nil {
				return err
			}
			if !exists {
				continue
			}
			var node *ast.Node
			var tree *parse.Tree
			if err := guard(func() (err error) { node, tree, err = b.node(id); return }); err != nil {
				if legacy && !b.strictDependencies && ctx.Err() == nil {
					continue
				}
				return err
			}
			if tree == nil || tree.Root == nil || tree.ID == "" || tree.Box == "" || tree.Path == "" {
				if legacy && !b.strictDependencies {
					continue
				}
				return errors.New("invalid_av_duplicate_tree")
			}
			if first := owned[tree.ID]; first != nil {
				if first.Box != tree.Box || first.Path != tree.Path {
					return errors.New("invalid_av_duplicate_tree_identity")
				}
				tree, node = first, treenode.GetNodeInTree(first, id)
			}
			if node == nil {
				continue
			}
			if node.ID != id {
				return errors.New("invalid_av_duplicate_node_identity")
			}
			attrs := parse.IAL2Map(node.KramdownIAL)
			members := []string{a.ID}
			if attrs[av.NodeAttrNameAvs] != "" {
				members = gulu.Str.RemoveDuplicatedElem(append(strings.Split(attrs[av.NodeAttrNameAvs], ","), a.ID))
			}
			attrs[av.NodeAttrNameAvs] = strings.Join(members, ",")
			var names string
			var err error
			if b.names != nil {
				err = guard(func() error { var readErr error; names, readErr = b.names(attrs[av.NodeAttrNameAvs]); return readErr })
			} else {
				names, err = attributeViewNamesWithServices(attrs[av.NodeAttrNameAvs], func(id string) (name string, err error) {
					err = guard(func() error {
						if id == a.ID {
							name = a.Name
							return nil
						}
						name, err = b.name(id)
						return err
					})
					return
				}, b.untitled, legacy)
			}
			if err != nil {
				return err
			}
			if names != "" {
				attrs[av.NodeAttrViewNames] = names
			}
			var old map[string]string
			if err = guard(func() (err error) { old, err = b.attrs(node, attrs); return }); err != nil {
				if legacy && !b.strictDependencies && ctx.Err() == nil {
					continue
				}
				return err
			}
			if owned[tree.ID] == nil {
				owned[tree.ID] = tree
				changed = append(changed, tree)
			}
			if err = guard(func() error { return b.cache(id, parse.IAL2Map(node.KramdownIAL)) }); err != nil {
				return err
			}
			if err = guard(func() error { return b.notify(old, node) }); err != nil {
				return err
			}
			if names != "" {
				node.RemoveIALAttr(av.NodeAttrViewNames)
			}
		}
	}
	for _, tree := range changed {
		if err := guard(func() error { return b.tree(tree) }); err != nil {
			return err
		}
	}
	return ctx.Err()
}
