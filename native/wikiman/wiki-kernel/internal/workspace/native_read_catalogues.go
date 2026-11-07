// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeSearchTemplates(ctx context.Context, keyword string) (out []*blockview.Block, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		readDir := func(name string) ([]os.DirEntry, error) {
			manifest, e := confirmedWorkspaceDirectoryManifest(ctx, w, filepath.ToSlash(name), workspaceDirectoryIO{})
			if e != nil {
				return nil, e
			}
			out := make([]os.DirEntry, 0, len(manifest.entries))
			for _, entry := range manifest.entries {
				out = append(out, fs.FileInfoToDirEntry(entry.info))
			}
			return out, nil
		}
		walk := func(start string, visit fs.WalkDirFunc) error {
			var scan func(string) error
			scan = func(name string) error {
				info, e := treeFileStat(ctx, w, filepath.ToSlash(name))
				if e != nil {
					return visit(name, nil, e)
				}
				entry := fs.FileInfoToDirEntry(info)
				if e = visit(name, entry, nil); e != nil {
					if errors.Is(e, fs.SkipDir) && entry.IsDir() {
						return nil
					}
					return e
				}
				if !entry.IsDir() {
					return nil
				}
				children, e := readDir(name)
				if e != nil {
					return e
				}
				for _, child := range children {
					if e = scan(filepath.Join(name, child.Name())); e != nil {
						return e
					}
				}
				return ctx.Err()
			}
			return scan(start)
		}
		var e error
		out, e = catalogue.SearchTemplates(ctx, "templates", keyword, r.configuration.Search.CaseSensitive, readDir, walk)
		for _, item := range out {
			item.Path = filepath.ToSlash(item.Path)
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
func (l *Lease) NativeSearchWidgets(ctx context.Context, keyword string) ([]*blockview.Block, error) {
	value, e := l.NativeCatalogue(ctx, catalogue.Request{Action: "search", Kind: "widgets", Keyword: keyword})
	if e != nil {
		return nil, e
	}
	out := []*blockview.Block{}
	for _, item := range value.Items {
		out = append(out, &blockview.Block{Name: item.PreferredName, Content: item.Name})
	}
	return out, nil
}
