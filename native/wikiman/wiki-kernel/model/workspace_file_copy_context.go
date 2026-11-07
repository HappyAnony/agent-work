// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/workspacefile"
)

func CopyWorkspaceFilesContext(ctx context.Context, srcs []string, directory string) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	if base.owner.persistenceReadOnly {
		return ErrSettingReadOnly
	}
	directory, e = workspacefile.Name(directory, true)
	if e != nil {
		return e
	}
	srcs = append([]string(nil), srcs...)
	return base.run(ctx, func(c, _ context.Context) error {
		for _, src := range srcs {
			if e := base.guard(c); e != nil {
				return e
			}
			if _, e := os.Lstat(src); os.IsNotExist(e) {
				continue
			} else if e != nil {
				return e
			}
			raw, e := capturePDFWatermarkFile(src).read(512<<20, func() error { return base.guard(c) })
			if e != nil {
				return e
			}
			if _, e = WorkspaceFileContext(c, workspacefile.Request{Action: "write", Path: path.Join(directory, filepath.Base(src)), Data: raw}, true); e != nil {
				return e
			}
		}
		return nil
	})
}

func CopyAssetToHostContext(ctx context.Context, src, dest string) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	if base.owner.persistenceReadOnly {
		return ErrSettingReadOnly
	}
	return base.run(ctx, func(c, _ context.Context) (result error) {
		selected, e := GetAssetAbsPathContext(c, src)
		if e != nil {
			return e
		}
		raw, e := capturePDFWatermarkFile(selected).read(512<<20, func() error { return base.guard(c) })
		if e != nil {
			return e
		}
		absolute, e := filepath.Abs(dest)
		if e != nil {
			return e
		}
		root, e := os.OpenRoot(filepath.Dir(absolute))
		if e != nil {
			return e
		}
		closed := false
		closeRoot := func() error {
			if closed {
				return nil
			}
			closed = true
			return root.Close()
		}
		defer func() { result = errors.Join(result, closeRoot()) }()
		source := indexrepair.Source{Data: root, Authorize: base.guard}
		name := filepath.Base(absolute)
		before, e := source.ReadLimited(c, name, 512<<20)
		if indexrepair.IsSourceNotExist(e) {
			before = nil
		} else if e != nil {
			return e
		}
		if e = source.Rewrite(c, name, before, raw); e != nil {
			return e
		}
		if e = closeRoot(); e != nil {
			return e
		}
		return IncSyncContext(c)
	})
}
