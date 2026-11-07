// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type tagMutationEnvironment struct {
	ctx      context.Context
	legacy   bool
	query    func(string) ([]indexdb.SpanRecord, error)
	load     func(string) (*parse.Tree, error)
	stage    func(*parse.Tree) error
	flush    func() error
	reload   func(string) error
	progress func(string, string) error
	clear    func(bool) error
	delay    func(int, int) error
	invalid  func(string) error
	empty    func() error
	av       *attributeViewTextEnvironment
}

func legacyTagMutationEnvironment() *tagMutationEnvironment {
	return &tagMutationEnvironment{ctx: context.Background(), legacy: true,
		query: func(label string) ([]indexdb.SpanRecord, error) {
			var ret []indexdb.SpanRecord
			for _, s := range sql.QueryTagSpansByLabel(label) {
				if s != nil {
					ret = append(ret, indexdb.SpanRecord(*s))
				}
			}
			return ret, nil
		},
		load: LoadTreeByBlockIDWithReindex, stage: writeTreeUpsertQueue, flush: func() error { sql.FlushQueue(); return nil }, reload: func(id string) error { ReloadProtyle(id); return nil },
		progress: func(kind, value string) error {
			switch kind {
			case "remove":
				util.PushEndlessProgress(Conf.Language(116))
			case "rename":
				util.PushEndlessProgress(Conf.Language(110))
			case "load":
				util.PushEndlessProgress("[" + value + "]")
			case "save":
				util.PushEndlessProgress(fmt.Sprintf(Conf.Language(111), util.EscapeHTML(value)))
			}
			return nil
		},
		clear: func(success bool) error {
			if success {
				util.PushClearProgress()
			} else {
				util.ClearPushProgress(100)
			}
			return nil
		}, delay: func(min, max int) error { util.RandomSleep(min, max); return nil },
		invalid: func(s string) error { return errors.New(fmt.Sprintf(Conf.Language(112), s)) }, empty: func() error { return errors.New(Conf.Language(114)) }, av: legacyAttributeViewTextEnvironment(),
	}
}
func (tx *Transaction) tagMutationEnvironment() *tagMutationEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.tagMutation == nil {
			return nil
		}
		e := *tx.environment.tagMutation
		e.ctx, e.legacy = tx.environment.ctx, false
		e.av = tx.attributeViewTextEnvironment()
		return &e
	}
	return legacyTagMutationEnvironment()
}
func (tx *Transaction) removeTag(label string) error {
	return mutateTagWithEnvironment(label, "", false, tx.tagMutationEnvironment())
}
func (tx *Transaction) renameTag(old, new string) error {
	return mutateTagWithEnvironment(old, new, true, tx.tagMutationEnvironment())
}
func (e *tagMutationEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.query == nil || e.load == nil || e.stage == nil || e.flush == nil || e.reload == nil || e.progress == nil || e.clear == nil || e.delay == nil || e.invalid == nil || e.empty == nil {
		return errors.New("tag_mutation_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	return e.av.validate()
}
func (e *tagMutationEnvironment) guard(fn func() error) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.ctx.Err()
}
func mutateTagWithEnvironment(old, new string, rename bool, e *tagMutationEnvironment) (err error) {
	if !rename && old == "" {
		return nil
	}
	if rename {
		if marker := treenode.ContainsMarker(new); marker != "" {
			if e == nil || e.invalid == nil {
				return errors.New("tag_mutation_dependencies_unbound")
			}
			return e.invalid(marker)
		}
		new = strings.TrimSpace(new)
		new = strings.TrimPrefix(new, "/")
		new = strings.TrimSuffix(new, "/")
		new = strings.TrimSpace(new)
		if new == "" {
			if e == nil || e.empty == nil {
				return errors.New("tag_mutation_dependencies_unbound")
			}
			return e.empty()
		}
		if old == new {
			return nil
		}
	}
	if err = e.validate(); err != nil {
		return err
	}
	started := true
	defer func() {
		if err != nil && started {
			_ = e.clear(false)
		}
	}() // Owner cleanup must also run after cancellation.
	kind, min, max := "remove", 1000, 2000
	if rename {
		kind, min, max = "rename", 500, 1000
	}
	if err = e.guard(func() error { return e.progress(kind, "") }); err != nil {
		return err
	}
	if err = e.guard(func() error { return e.delay(min, max) }); err != nil {
		return err
	}
	var spans []indexdb.SpanRecord
	if err = e.guard(func() (err error) { spans, err = e.query(old); return }); err != nil {
		return err
	}
	groups := map[string][]string{}
	for _, s := range spans {
		if s.RootID == "" || s.BlockID == "" {
			return errors.New("invalid_tag_span")
		}
		groups[s.RootID] = append(groups[s.RootID], s.BlockID)
	}
	var reloads []string
	nodes := map[string]*ast.Node{}
	for id, blocks := range groups {
		if err = e.guard(func() error { return e.progress("load", id) }); err != nil {
			return err
		}
		var tree *parse.Tree
		if err = e.guard(func() (err error) { tree, err = e.load(id); return }); err != nil {
			return err
		}
		if tree == nil || tree.Root == nil || tree.ID == "" || (!e.legacy && (tree.ID != id || tree.Root.ID != tree.ID || tree.Box == "" || tree.Path == "")) {
			return errors.New("invalid_tag_tree")
		}
		var unlinks []*ast.Node
		for _, block := range blocks {
			if err = e.ctx.Err(); err != nil {
				return err
			}
			n := treenode.GetNodeInTree(tree, block)
			if n == nil {
				continue
			}
			if n.Type == ast.NodeDocument {
				if tags := n.IALAttr("tags"); strings.Contains(tags, old) {
					var next []string
					for _, tag := range strings.Split(tags, ",") {
						if rename {
							if tag == old || strings.HasPrefix(tag, old+"/") {
								tag = strings.Replace(tag, old, new, 1)
							}
							next = append(next, tag)
						} else if tag != old {
							next = append(next, tag)
						}
					}
					n.SetIALAttr("tags", strings.Join(next, ","))
				}
				continue
			}
			for _, mark := range n.ChildrenByType(ast.NodeTextMark) {
				if !mark.IsTextMarkType("tag") {
					continue
				}
				if rename {
					if mark.TextMarkTextContent == old || strings.HasPrefix(mark.TextMarkTextContent, old+"/") {
						mark.TextMarkTextContent = strings.Replace(mark.TextMarkTextContent, old, new, 1)
					}
				} else if mark.TextMarkTextContent == old {
					unlinks = append(unlinks, mark)
				}
			}
			nodes[n.ID] = n
		}
		for _, n := range unlinks {
			n.Unlink()
		}
		if err = e.guard(func() error { return e.progress("save", tree.Root.IALAttr("title")) }); err != nil {
			return err
		}
		if err = e.guard(func() error { return e.stage(tree) }); err != nil {
			return err
		}
		if err = e.guard(func() error { return e.delay(50, 150) }); err != nil {
			return err
		}
		reloads = append(reloads, tree.ID)
	}
	if err = e.guard(e.flush); err != nil {
		return err
	}
	for _, id := range uniqueUpdatedAttributeViews(reloads) {
		if err = e.guard(func() error { return e.reload(id) }); err != nil {
			return err
		}
	}
	if err = updateAttributeViewBlockTextWithEnvironment(nodes, e.av); err != nil {
		return err
	}
	if err = e.guard(e.flush); err != nil {
		return err
	}
	if err = e.guard(func() error { return e.clear(true) }); err != nil {
		return err
	}
	started = false
	return nil
}
