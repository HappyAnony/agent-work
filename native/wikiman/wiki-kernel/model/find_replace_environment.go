// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/treenode"
)

type findReplaceRequest struct {
	Keyword, Replacement     string
	ReplaceTypes, Types      map[string]bool
	IDs, Paths, Boxes        []string
	Method, OrderBy, GroupBy int
}
type findReplaceEnvironment struct {
	ctx                   context.Context
	legacy, caseSensitive bool
	gate                  *attributeMutationGate
	parser                func() (*lute.Lute, error)
	reject                func(int) error
	search                func(findReplaceRequest) ([]string, error)
	lookup                func(string) (*treenode.BlockTree, error)
	load                  func(string) (*parse.Tree, error)
	beginHistory          func() (string, error)
	history               func(string, *parse.Tree) error
	indexHistory          func(string) error
	stage                 func(*parse.Tree) error
	rename                func(*parse.Tree, string) error
	progress              func(string, int, int) error
	clear                 func(bool) error
	flush, tags           func() error
	reload, refCount      func(string) error
	av                    *attributeViewTextEnvironment
}

func (e *findReplaceEnvironment) guard(fn func() error) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.ctx.Err()
}
func (e *findReplaceEnvironment) newParser() (engine *lute.Lute, err error) {
	err = e.guard(func() (err error) { engine, err = e.parser(); return })
	return
}
func copyFindReplaceRequest(q findReplaceRequest) findReplaceRequest {
	q.ReplaceTypes = copyReplaceFlags(q.ReplaceTypes)
	q.Types = copyReplaceFlags(q.Types)
	q.IDs = append([]string(nil), q.IDs...)
	q.Paths = append([]string(nil), q.Paths...)
	q.Boxes = append([]string(nil), q.Boxes...)
	return q
}
func (e *findReplaceEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.gate == nil || e.gate.token == nil || e.parser == nil || e.reject == nil || e.search == nil || e.lookup == nil || e.load == nil || e.beginHistory == nil || e.history == nil || e.indexHistory == nil || e.stage == nil || e.rename == nil || e.progress == nil || e.clear == nil || e.flush == nil || e.tags == nil || e.reload == nil || e.refCount == nil {
		return errors.New("find_replace_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	return e.av.validate()
}
func (e *findReplaceEnvironment) rejection(code int) error {
	if e == nil || e.ctx == nil || e.reject == nil {
		return errors.New("find_replace_dependencies_unbound")
	}
	if err := e.guard(func() error { return e.reject(code) }); err != nil {
		return err
	}
	return errors.New("find_replace_rejected")
}
func (tx *Transaction) findReplace(q findReplaceRequest) error {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.findReplace == nil {
			return errors.New("find_replace_dependencies_unbound")
		}
		e := *tx.environment.findReplace
		e.ctx, e.legacy = tx.environment.ctx, false
		e.av = tx.attributeViewTextEnvironment()
		return findReplaceWithEnvironment(q, &e)
	}
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil && tx.environment.legacy {
		return findReplaceContext(tx.environment.ctx, q)
	}
	return errors.New("find_replace_dependencies_unbound")
}
func findReplaceWithEnvironment(q findReplaceRequest, e *findReplaceEnvironment) (err error) {
	if e == nil || e.ctx == nil {
		return errors.New("find_replace_dependencies_unbound")
	}
	if err = e.ctx.Err(); err != nil {
		return err
	}
	if q.Method == 2 {
		return e.rejection(132)
	}
	if q.Method == 1 {
		q.Method = 0
	}
	if q.GroupBy != 0 {
		return e.rejection(221)
	}
	if q.Keyword == q.Replacement {
		return nil
	}
	if err = e.validate(); err != nil {
		return err
	}
	q = copyFindReplaceRequest(q)
	execute := func() (err error) {
		defer func() {
			if err != nil {
				_ = e.clear(false)
			}
		}() // Owner cleanup also runs after cancellation.
		return executeFindReplace(q, e)
	}
	// Legacy RenameDoc drains the old transaction queue. Holding the attribute
	// gate across that drain would deadlock transaction attribute helpers.
	if e.legacy {
		return execute()
	}
	return e.gate.with(e.ctx, execute)
}
func copyReplaceFlags(input map[string]bool) map[string]bool {
	if input == nil {
		return nil
	}
	ret := make(map[string]bool, len(input))
	for k, v := range input {
		ret[k] = v
	}
	return ret
}
