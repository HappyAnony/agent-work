// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/88250/lute/parse"
	"wiki-kernel/cache"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Stage/cache/reload are owner candidates until R04 supplies source commit.
type bookmarkMutationEnvironment struct {
	ctx              context.Context
	legacy, readOnly bool
	gate             *attributeMutationGate
	query            func() ([]indexdb.BlockRecord, error)
	load             func(string) (*parse.Tree, error)
	stage            func(*parse.Tree) error
	cache            func(string, map[string]string) error
	progress         func(string, string) error
	clear            func() error
	delay            func() error
	reload           func() error
	invalid          func(string) error
	empty            func() error
}

func legacyBookmarkMutationEnvironment() *bookmarkMutationEnvironment {
	return &bookmarkMutationEnvironment{ctx: context.Background(), legacy: true, gate: legacyAttributeMutationGate,
		query: func() (ret []indexdb.BlockRecord, err error) {
			for _, b := range sql.QueryBookmarkBlocksByKeyword("") {
				if b != nil {
					ret = append(ret, indexdb.BlockRecord(*b))
				}
			}
			return
		}, load: LoadTreeByBlockID, stage: writeTreeUpsertQueue,
		cache: func(id string, attrs map[string]string) error { cache.PutBlockIAL(id, attrs); return nil },
		progress: func(kind, value string) error {
			switch kind {
			case "rename":
				util.PushEndlessProgress(Conf.Language(110))
			case "remove":
				util.PushEndlessProgress(Conf.Language(116))
			case "load":
				util.PushEndlessProgress("[" + value + "]")
			case "save":
				util.PushEndlessProgress(fmt.Sprintf(Conf.Language(111), util.EscapeHTML(value)))
			}
			return nil
		}, clear: func() error { util.ClearPushProgress(100); return nil },
		delay:   func() error { util.RandomSleep(50, 150); return nil },
		reload:  func() error { util.ReloadUI(); return nil },
		invalid: func(s string) error { return fmt.Errorf(Conf.Language(112), s) },
		empty:   func() error { return errors.New(Conf.Language(126)) },
	}
}

func (tx *Transaction) bookmarkMutationEnvironment() *bookmarkMutationEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.bookmarkMutation == nil {
			return nil
		}
		e := *tx.environment.bookmarkMutation
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	return legacyBookmarkMutationEnvironment()
}
func (tx *Transaction) renameBookmark(old, next string) error {
	return mutateBookmarkWithEnvironment(old, next, true, tx.bookmarkMutationEnvironment())
}
func (tx *Transaction) removeBookmark(label string) error {
	return mutateBookmarkWithEnvironment(label, "", false, tx.bookmarkMutationEnvironment())
}
func (e *bookmarkMutationEnvironment) guard(fn func() error) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.ctx.Err()
}
func (e *bookmarkMutationEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.gate == nil || e.gate.token == nil || e.query == nil || e.load == nil || e.stage == nil || e.cache == nil || e.progress == nil || e.clear == nil || e.delay == nil || e.reload == nil || e.invalid == nil || e.empty == nil {
		return errors.New("bookmark_mutation_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.readOnly {
		return errors.New("bookmark_mutation_read_only")
	}
	return nil
}

func mutateBookmarkWithEnvironment(old, next string, rename bool, e *bookmarkMutationEnvironment) (err error) {
	if err = e.validate(); err != nil {
		return err
	}
	if rename {
		// Original checks marker characters before trimming and compares the old
		// name exactly. Bookmark paths are not tag paths; retain slashes.
		if marker := treenode.ContainsMarker(next); marker != "" {
			if err := e.guard(func() error { return e.invalid(marker) }); err != nil {
				return err
			}
			return errors.New("invalid_bookmark_name")
		}
		next = strings.TrimSpace(next)
		if next == "" {
			if err := e.guard(e.empty); err != nil {
				return err
			}
			return errors.New("empty_bookmark_name")
		}
		if old == next {
			return nil
		}
	}
	return e.gate.with(e.ctx, func() (err error) {
		// Cleanup must run even if a callback cancels then returns nil. Original
		// success still uses the original ReloadUI behavior.
		defer func() {
			if err != nil {
				_ = e.clear()
			}
		}()
		kind := "remove"
		if rename {
			kind = "rename"
		}
		if err = e.guard(func() error { return e.progress(kind, "") }); err != nil {
			return
		}
		var candidates []indexdb.BlockRecord
		if err = e.guard(func() (err error) { candidates, err = e.query(); return }); err != nil {
			return
		}
		groups := map[string][]string{}
		for _, b := range candidates {
			if b.ID == "" || b.RootID == "" {
				return errors.New("invalid_bookmark_candidate")
			}
			groups[b.RootID] = append(groups[b.RootID], b.ID)
		}
		for root, ids := range groups {
			if err = e.guard(func() error { return e.progress("load", root) }); err != nil {
				return
			}
			var tree *parse.Tree
			if err = e.guard(func() (err error) { tree, err = e.load(root); return }); err != nil {
				return
			}
			if tree == nil || tree.Root == nil || (!e.legacy && (tree.ID != root || tree.Root.ID != root || tree.Box == "" || tree.Path == "")) {
				return errors.New("invalid_bookmark_tree")
			}
			for _, id := range ids {
				if err = e.ctx.Err(); err != nil {
					return
				}
				n := treenode.GetNodeInTree(tree, id)
				if n == nil || n.IALAttr("bookmark") != old {
					continue
				}
				if rename {
					n.SetIALAttr("bookmark", next)
				} else {
					n.RemoveIALAttr("bookmark")
				}
				if err = e.guard(func() error { return e.cache(n.ID, parse.IAL2Map(n.KramdownIAL)) }); err != nil {
					return
				}
			}
			if err = e.guard(func() error { return e.progress("save", tree.Root.IALAttr("title")) }); err != nil {
				return
			}
			if err = e.guard(func() error { return e.stage(tree) }); err != nil {
				return
			}
			if err = e.guard(e.delay); err != nil {
				return
			}
		}
		return e.guard(e.reload)
	})
}
