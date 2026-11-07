// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/treenode"
)

// Attribute-panel queries also repair data. Native writes/publications here are
// owned candidates; the authoritative source boundary still belongs to R04.
type attributeViewBlockKeysEnvironment struct {
	ownerGuard         func() error
	strictDependencies bool
	complete           func() error
	ctx                context.Context
	legacy             bool
	wait               func() error
	attrs              func(string) (map[string]string, error)
	read               func(string) (*av.AttributeView, error)
	clone              func(*av.AttributeView) (*av.AttributeView, error)
	node               func(string) (*ast.Node, *parse.Tree, error)
	newView            func() (*av.View, error)
	stage              func(*av.AttributeView) error
	render             func(*av.AttributeView, *av.View) error
	mirrors            func(string) ([]string, error)
	allMirrors         func() ([]string, error)
	upsert             func(string, string) error
	unbind             func(string, string) error
	remove             func(*ast.Node, string, *parse.Tree) error
	untitled           func() string
}

func (e *attributeViewBlockKeysEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.wait == nil || e.attrs == nil || e.read == nil || e.clone == nil || e.node == nil || e.newView == nil || e.stage == nil || e.render == nil || e.mirrors == nil || e.allMirrors == nil || e.upsert == nil || e.unbind == nil || e.remove == nil || e.untitled == nil {
		return errors.New("av_block_keys_dependencies_unbound")
	}
	return e.ctx.Err()
}

func legacyAttributeViewBlockKeysEnvironment(contexts ...context.Context) *attributeViewBlockKeysEnvironment {
	ctx := legacyAVContext(contexts)
	return blockKeysWithBinding(nil, newLegacyAVBinding(ctx), ctx)
}
func blockKeysWithBinding(tx *Transaction, b *legacyAVBinding, ctx context.Context) *attributeViewBlockKeysEnvironment {
	cell := legacyAttributeViewCellWithBinding(tx, b)
	return &attributeViewBlockKeysEnvironment{ctx: ctx, legacy: true, strictDependencies: true, ownerGuard: b.guard, complete: b.complete, wait: b.waitForStorages,
		attrs: func(id string) (map[string]string, error) {
			n, _, e := b.node(tx, id)
			if errors.Is(e, ErrBlockNotFound) {
				return map[string]string{}, nil
			}
			if e != nil {
				return nil, e
			}
			if n == nil {
				return map[string]string{}, nil
			}
			return parse.IAL2MapUnEsc(n.KramdownIAL), b.guard()
		},
		read: b.readCandidate, clone: b.clone, node: cell.node,
		newView: func() (*av.View, error) {
			if e := b.guard(); e != nil {
				return nil, e
			}
			v, _, _, e := avmodel.DefaultTable(ast.NewNodeID(), b.scope.commit.avLabels, ast.NewNodeID)
			return v, e
		},
		stage: b.saveUpgraded, render: func(a *av.AttributeView, v *av.View) error {
			if e := b.adopt(a); e != nil {
				return e
			}
			_, e := b.scope.render(a, v, "")
			return e
		},
		mirrors: func(id string) ([]string, error) { return b.mirrors(tx, id) }, allMirrors: func() ([]string, error) { return b.allMirrors(tx) }, upsert: b.upsertMirror,
		unbind: func(id, block string) error { return cell.binding(id, block, false) },
		remove: func(n *ast.Node, id string, t *parse.Tree) error {
			e := &attributeViewRowEnvironment{cell: cell, exists: b.existsAV}
			return removeNodeAvIDWithEnvironment(n, id, t, e)
		},
		untitled: func() string {
			if b.err != nil {
				return ""
			}
			return b.scope.commit.avUntitled
		}}
}
func (tx *Transaction) attributeViewBlockKeysEnvironment() *attributeViewBlockKeysEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avBlockKeys == nil {
			return nil
		}
		e := *tx.environment.avBlockKeys
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	return blockKeysWithBinding(tx, legacyAVBindingForTransaction(tx), transactionAVContext(tx))
}

func (e *attributeViewBlockKeysEnvironment) guard(fn func() error) error {
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

func (e *attributeViewBlockKeysEnvironment) resolve(id string) (n *ast.Node, t *parse.Tree, err error) {
	err = e.guard(func() (err error) { n, t, err = e.node(id); return })
	if err != nil {
		if e.legacy && e.ctx.Err() == nil && (!e.strictDependencies || errors.Is(err, ErrBlockNotFound)) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if n == nil || t == nil {
		return nil, nil, nil
	}
	if (!e.legacy || e.strictDependencies) && (t.Root == nil || t.ID == "" || t.Root.ID != t.ID || t.Box == "" || t.Path == "" || n.ID != id || treenode.GetNodeInTree(t, id) != n) {
		return nil, nil, errors.New("invalid_av_block_keys_tree")
	}
	return
}

// Shared with the original renderer: create an empty table if needed, prefer
// block-selected view, persist a changed selection, and retain first-view fallback.
func selectBlockAttributeView(a *av.AttributeView, viewID, nodeID string, e *attributeViewBlockKeysEnvironment) (*av.View, error) {
	if len(a.Views) == 0 {
		var v *av.View
		if err := e.guard(func() (err error) { v, err = e.newView(); return }); err != nil {
			return nil, err
		}
		if v == nil || v.ID == "" {
			return nil, errors.New("invalid_av_block_keys_view")
		}
		a.Views = append(a.Views, v)
		a.ViewID = v.ID
		if err := e.guard(func() error { return e.stage(a) }); err != nil {
			return nil, err
		}
	}
	if viewID == "" && nodeID != "" {
		n, _, err := e.resolve(nodeID)
		if err != nil {
			return nil, err
		}
		if n != nil {
			viewID = n.IALAttr(av.NodeAttrView)
		}
	}
	var view *av.View
	if viewID != "" {
		view, _ = a.GetCurrentView(viewID)
		if view != nil && view.ID != a.ViewID {
			a.ViewID = view.ID
			if err := e.guard(func() error { return e.stage(a) }); err != nil {
				return nil, err
			}
		}
	} else {
		view = a.GetView(a.ViewID)
	}
	if view == nil {
		view = a.Views[0]
	}
	if view == nil {
		return nil, errors.New("invalid_av_block_keys_view")
	}
	return view, e.ctx.Err()
}

func (tx *Transaction) blockAttributeViewKeys(nodeID string) ([]*BlockAttributeViewKeys, error) {
	return blockAttributeViewKeysWithEnvironment(nodeID, tx.attributeViewBlockKeysEnvironment())
}

func blockAttributeViewKeysWithEnvironment(nodeID string, e *attributeViewBlockKeysEnvironment) ([]*BlockAttributeViewKeys, error) {
	ret, err := blockAttributeViewKeysRun(nodeID, e)
	if err != nil {
		return nil, err
	}
	if e.complete != nil {
		if err = e.complete(); err != nil {
			return nil, err
		}
	}
	return ret, nil
}
func blockAttributeViewKeysRun(nodeID string, e *attributeViewBlockKeysEnvironment) ([]*BlockAttributeViewKeys, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}
	if nodeID == "" {
		return nil, errors.New("invalid_av_block_keys_id")
	}
	if err := e.guard(e.wait); err != nil {
		return nil, err
	}
	ret := []*BlockAttributeViewKeys{}
	var attrs map[string]string
	if err := e.guard(func() (err error) { attrs, err = e.attrs(nodeID); return }); err != nil {
		return nil, err
	}
	if attrs[av.NodeAttrNameAvs] == "" {
		return ret, nil
	}
	cache := map[string]*av.AttributeView{}
	for _, id := range strings.Split(attrs[av.NodeAttrNameAvs], ",") {
		if err := e.ctx.Err(); err != nil {
			return nil, err
		}
		a := cache[id]
		if a == nil {
			var source *av.AttributeView
			err := e.guard(func() (err error) { source, err = e.read(id); return })
			if err != nil || source == nil {
				if e.legacy && e.ctx.Err() == nil && (!e.strictDependencies || os.IsNotExist(err)) {
					continue
				}
				if err != nil {
					return nil, err
				}
				return nil, errors.New("invalid_av_block_keys_source")
			}
			if source.ID != id {
				return nil, errors.New("invalid_av_block_keys_source")
			}
			if err = validateUpdatedAttributeView(source); err != nil {
				return nil, err
			}
			if err = validateAttributeViewFieldSource(source); err != nil {
				return nil, err
			}
			if source.GetBlockKeyValues() == nil {
				return nil, errors.New("invalid_av_block_keys_primary")
			}
			if err = e.guard(func() (err error) { a, err = e.clone(source); return }); err != nil {
				return nil, err
			}
			if a == nil || a == source || a.ID != id {
				return nil, errors.New("invalid_av_block_keys_clone")
			}
			if err = validateUpdatedAttributeView(a); err != nil {
				return nil, err
			}
			if err = validateAttributeViewFieldSource(a); err != nil {
				return nil, err
			}
			if a.GetBlockKeyValues() == nil {
				return nil, errors.New("invalid_av_block_keys_primary")
			}
			cache[id] = a
		}
		// Preserve original Block.ID semantics, including legacy detached payloads;
		// record identity below is BlockID and may differ from the bound block.
		if !a.ExistBoundBlock(nodeID) {
			err := e.guard(func() error { return e.unbind(id, nodeID) })
			if err != nil && (!e.legacy || e.strictDependencies || e.ctx.Err() != nil) {
				return nil, err
			}
			return ret, nil
		}
		value := a.GetBlockValueByBoundID(nodeID)
		if value == nil {
			continue
		}
		view, err := selectBlockAttributeView(a, "", nodeID, e)
		if err != nil {
			if e.legacy && e.ctx.Err() == nil && !e.strictDependencies {
				continue
			}
			return nil, err
		}
		if err = validateAttributeViewPreviewLayout(view); err != nil {
			return nil, err
		}
		if err = e.guard(func() error { return e.render(a, view) }); err != nil {
			return nil, err
		}
		values := avrender.BlockValues(a, value.BlockID)
		refreshAttributeViewKeyIDs(a)
		if err = e.guard(func() error { return e.stage(a) }); err != nil {
			if !e.legacy || e.strictDependencies || e.ctx.Err() != nil {
				return nil, err
			}
		}
		order := map[string]int{}
		for i, k := range a.KeyIDs {
			order[k] = i
		}
		sort.Slice(values, func(i, j int) bool { return order[values[i].Key.ID] < order[values[j].Key.ID] })
		var blocks []string
		if err = e.guard(func() (err error) { blocks, err = e.mirrors(id); return }); err != nil {
			return nil, err
		}
		if len(blocks) == 0 {
			var candidates []string
			if err = e.guard(func() (err error) { candidates, err = e.allMirrors(); return }); err != nil {
				return nil, err
			}
			for _, block := range candidates {
				n, _, err := e.resolve(block)
				if err != nil {
					return nil, err
				}
				if n != nil && n.AttributeViewID == id {
					blocks = append(blocks, block)
				}
			}
			if len(blocks) == 0 {
				n, t, err := e.resolve(nodeID)
				if err != nil {
					return nil, err
				}
				if n != nil {
					if err = e.guard(func() error { return e.remove(n, id, t) }); err != nil {
						if !e.legacy || e.strictDependencies || e.ctx.Err() != nil {
							return nil, err
						}
					}
				}
				continue
			}
			blocks = uniqueUpdatedAttributeViews(blocks)
			for _, block := range blocks {
				if err = e.guard(func() error { return e.upsert(id, block) }); err != nil {
					return nil, err
				}
			}
		}
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = e.untitled()
		}
		if err = e.ctx.Err(); err != nil {
			return nil, err
		}
		ret = append(ret, &BlockAttributeViewKeys{AvID: id, AvName: name, BlockIDs: blocks, KeyValues: values})
	}
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
