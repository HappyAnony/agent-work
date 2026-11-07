// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexrepair"
)

func resolveAssetContext(ctx context.Context, relative string) (out string, err error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return "", e
	}
	err = o.run(ctx, func(c, life context.Context) (result error) {
		info, e := os.Lstat(o.data)
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return indexrepair.ErrSourcePath
		}
		root, e := os.OpenRoot(o.data)
		if e != nil {
			return e
		}
		guard := func() error {
			if e := o.guard(c); e != nil {
				return e
			}
			now, e := os.Lstat(o.data)
			if e != nil {
				return e
			}
			if now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(info, now) {
				return indexrepair.ErrSourceChanged
			}
			return nil
		}
		defer func() { result = errors.Join(result, guard(), root.Close()) }()
		actual, e := root.Stat(".")
		if e != nil {
			return e
		}
		if !os.SameFile(info, actual) {
			return indexrepair.ErrSourceChanged
		}
		source := indexrepair.Source{Data: root}
		// Global assets need no notebook, parser or SQL capability. Only the original
		// fallback branch enumerates notebooks, still within this pinned source.
		rel, e := assetflow.Resolve(c, source, nil, relative)
		if errors.Is(e, assetflow.ErrNotFound) {
			boxes, readErr := o.owner.business.Notebooks(c)
			if readErr != nil {
				return readErr
			}
			ids := []string{}
			for _, box := range boxes {
				ids = append(ids, box.ID)
			}
			rel, e = assetflow.Resolve(c, source, ids, relative)
		}
		if e != nil {
			return e
		}
		out = filepath.Join(o.data, filepath.FromSlash(rel))
		return guard()
	})
	if err != nil {
		return "", err
	}
	return out, nil
}
