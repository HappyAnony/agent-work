// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/internal/notebooks"
)

func GetNotebookHistoryContext(ctx context.Context) (out []*History, err error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	err = o.run(ctx, func(op, life context.Context) (result error) {
		info, e := os.Lstat(o.history)
		if os.IsNotExist(e) {
			out = []*History{}
			return nil
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return notebooks.ErrPath
		}
		root, e := os.OpenRoot(o.history)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, root.Close()) }()
		actual, e := root.Stat(".")
		if e != nil {
			return e
		}
		if !os.SameFile(info, actual) {
			return notebooks.ErrChanged
		}
		values, e := notebooks.Deleted(op, root)
		if e != nil {
			return e
		}
		out = []*History{}
		for _, v := range values {
			out = append(out, &History{HCreated: v.Created, Items: []*HistoryItem{{Title: v.Name, Path: filepath.Join(o.history, filepath.FromSlash(v.Path)), Op: HistoryOpDelete}}})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return
}
