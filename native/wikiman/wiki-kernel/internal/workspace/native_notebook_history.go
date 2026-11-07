// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/notebooks"
)

func (l *Lease) NativeNotebookHistory(ctx context.Context) (out []notebooks.DeletedHistory, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) (result error) {
		name := filepath.Join(l.workspace.paths.RuntimeDir, "history")
		info, e := os.Lstat(name)
		if os.IsNotExist(e) {
			out = []notebooks.DeletedHistory{}
			return nil
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return ErrDocument
		}
		root, e := os.OpenRoot(name)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, root.Close()) }()
		actual, e := root.Stat(".")
		if e != nil {
			return e
		}
		if !os.SameFile(info, actual) {
			return ErrDocument
		}
		out, e = notebooks.Deleted(ctx, root)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
