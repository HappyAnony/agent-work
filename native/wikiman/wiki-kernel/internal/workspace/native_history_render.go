// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func (l *Lease) NativeRenderHistory(ctx context.Context, name, keyword string, highlight bool) (out docview.HistoryResult, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) (result error) {
		r, w := l.workspace.resources, l.workspace
		if !fs.ValidPath(name) || name == "." || filepath.IsAbs(name) {
			return ErrDocument
		}
		history := filepath.Join(w.paths.RuntimeDir, "history")
		info, e := os.Lstat(history)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return ErrDocument
		}
		root, e := os.OpenRoot(history)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, root.Close()) }()
		pinned, e := root.Stat(".")
		if e != nil {
			return e
		}
		if !os.SameFile(info, pinned) {
			return ErrDocument
		}
		raw, e := (indexrepair.Source{Data: root}).Read(ctx, name)
		if indexrepair.IsSourceNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		if r.configuration.Search == nil {
			return ErrDocument
		}
		out, e = docview.RenderHistory(ctx, raw, keyword, highlight, r.configuration.Search.CaseSensitive, parser)
		return e
	})
	if err != nil {
		return docview.HistoryResult{}, err
	}
	return
}
