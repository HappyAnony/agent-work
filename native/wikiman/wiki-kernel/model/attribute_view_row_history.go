// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"wiki-kernel/av"
)

type attributeViewHistoryFile struct {
	Path   string
	Data   []byte
	SHA256 string
}
type attributeViewRowHistory struct {
	AVID      string
	Operation string
	Files     []attributeViewHistoryFile
}
type attributeViewRowHistoryEnvironment struct {
	mirrors   func(string) ([]string, error)
	tree      func(string) (*parse.Tree, error)
	document  func(*parse.Tree) ([]byte, error)
	attribute func(*av.AttributeView) ([]byte, error)
	stage     func(attributeViewRowHistory) error
}

// Snapshot the same mirror document bytes and post-removal AV as the original
// history algorithm. No source/history files are published by this builder.
func stageAttributeViewRowHistory(ctx context.Context, a *av.AttributeView, e *attributeViewRowHistoryEnvironment) error {
	if ctx == nil || a == nil || !ast.IsNodeIDPattern(a.ID) || e == nil || e.mirrors == nil || e.tree == nil || e.document == nil || e.attribute == nil || e.stage == nil {
		return errors.New("av_row_history_dependencies_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ids, err := e.mirrors(a.ID)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	plan := attributeViewRowHistory{AVID: a.ID, Operation: HistoryOpUpdate}
	seen := map[string]bool{}
	add := func(p string, b []byte) error {
		if len(b) == 0 || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.HasPrefix(p, "../") {
			return errors.New("invalid_av_row_history_file")
		}
		if seen[p] {
			return nil
		}
		seen[p] = true
		data := bytes.Clone(b)
		sum := sha256.Sum256(data)
		plan.Files = append(plan.Files, attributeViewHistoryFile{p, data, hex.EncodeToString(sum[:])})
		return nil
	}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return err
		}
		t, e2 := e.tree(id)
		if e2 != nil {
			return e2
		}
		if t == nil {
			return ErrBlockNotFound
		}
		// Tree.Path uses a leading slash in the original parser representation.
		p := t.Box + "/" + strings.TrimPrefix(t.Path, "/")
		if !ast.IsNodeIDPattern(t.Box) || !ast.IsNodeIDPattern(t.ID) || path.Base(p) != t.ID+".docj" {
			return errors.New("invalid_av_row_history_tree")
		}
		if seen[p] {
			continue
		}
		data, e2 := e.document(t)
		if e2 != nil {
			return e2
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = add(p, data); err != nil {
			return err
		}
	}
	raw, err := e.attribute(a)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = add("storage/av/"+a.ID+".json", raw); err != nil {
		return err
	}
	if err = e.stage(plan); err != nil {
		return err
	}
	return ctx.Err()
}
