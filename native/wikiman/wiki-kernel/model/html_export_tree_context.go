// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type htmlExportTreeIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

func (m *markdownExportContext) mergeServices(box string) (services *exportMergeServices, closeServices func() error, err error) {
	return m.mergeServicesWithIO(box, htmlExportTreeIO{})
}
func (m *markdownExportContext) mergeServicesWithIO(box string, op htmlExportTreeIO) (services *exportMergeServices, closeServices func() error, err error) {
	if _, err = m.read.box(box); err != nil {
		return nil, nil, err
	}
	root, err := openArchivePublicationSource(m.read.binding.scope.commit.r.data)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, root.Close())
		}
	}()
	source := indexrepair.Source{Data: root}
	ctx := m.read.binding.scope.ctx
	readDir := op.readDir
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	notebook := conf.NewBoxConf()
	raw, err := source.Read(ctx, box+"/.siyuan/conf.json")
	if err == nil {
		if err = json.Unmarshal(raw, &notebook); err != nil {
			return nil, nil, err
		}
		if notebook == nil {
			return nil, nil, errors.New("export_merge_notebook_invalid")
		}
	} else if !indexrepair.IsSourceNotExist(err) {
		return nil, nil, err
	}
	err = nil
	sorts := map[string]int{}
	var sortsRead bool
	rooted := func(p string) (string, error) {
		if !strings.HasPrefix(p, "/") {
			return "", indexrepair.ErrSourcePath
		}
		rel := strings.Trim(p, "/")
		if rel == "" {
			return box, nil
		}
		if !fs.ValidPath(rel) || strings.Contains(rel, "\\") {
			return "", indexrepair.ErrSourcePath
		}
		return box + "/" + rel, nil
	}
	stat := func(_ context.Context, p string) (*doctree.Entry, error) {
		if e := m.guard(); e != nil {
			return nil, e
		}
		n, e := rooted(p)
		if e != nil {
			return nil, e
		}
		if e = source.Check(n, false); os.IsNotExist(e) {
			return nil, nil
		} else if e != nil {
			return nil, e
		}
		info, e := root.Stat(n)
		if e != nil {
			return nil, e
		}
		return &doctree.Entry{Path: p, Name: info.Name(), Size: info.Size(), Directory: info.IsDir()}, nil
	}
	list := func(_ context.Context, p string) ([]doctree.Entry, error) {
		if e := m.guard(); e != nil {
			return nil, e
		}
		n, e := rooted(p)
		if e != nil {
			return nil, e
		}
		if e = source.Check(n, false); e != nil {
			return nil, e
		}
		before, e := source.Data.Lstat(n)
		if e != nil {
			return nil, e
		}
		if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
			return nil, indexrepair.ErrSourcePath
		}
		entries, e := readDir(ctx, source, n)
		if e != nil {
			return nil, e
		}
		if e = m.guard(); e != nil {
			return nil, e
		}
		after, e := source.Data.Lstat(n)
		if e != nil {
			return nil, e
		}
		if !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(after, before) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return nil, indexrepair.ErrSourceChanged
		}
		result := []doctree.Entry{}
		for _, d := range entries {
			if e = m.guard(); e != nil {
				return nil, e
			}
			if util.IsReservedFilename(d.Name()) || strings.HasSuffix(d.Name(), ".tmp") {
				continue
			}
			if e = source.Check(n+"/"+d.Name(), false); e != nil {
				return nil, e
			}
			info, e := d.Info()
			if e != nil {
				return nil, e
			}
			fpath := path.Join(p, d.Name())
			if d.IsDir() {
				fpath += "/"
			}
			result = append(result, doctree.Entry{Path: fpath, Name: d.Name(), Size: info.Size(), Directory: d.IsDir()})
		}
		return result, nil
	}
	attributes := func(_ context.Context, p string) (map[string]string, error) {
		if !strings.HasSuffix(p, ".docj") {
			return nil, nil
		}
		if _, e := m.read.selectedTrees(box, []string{p}); e != nil {
			return nil, e
		}
		saved := m.read.documents[util.GetTreeID(p)]
		var doc struct{ Properties map[string]string }
		if e := json.Unmarshal(saved.raw, &doc); e != nil {
			return nil, e
		}
		if len(doc.Properties) == 0 || doc.Properties["id"] != util.GetTreeID(p) {
			return nil, errors.New("export_merge_properties_missing")
		}
		return doc.Properties, m.guard()
	}
	var refs map[string]int
	env := doctree.Services{
		Settings: func(_ context.Context, requested string) (int, int, error) {
			if requested != box {
				return 0, 0, indexrepair.ErrSourcePath
			}
			return notebook.SortMode, m.mergeSort, m.guard()
		},
		List: func(c context.Context, p string) ([]doctree.Entry, error) {
			if util.HasDocExt(p) {
				p = util.StripDocExt(p)
				entry, e := stat(c, p)
				if e != nil {
					return nil, e
				}
				if entry == nil || !entry.Directory {
					return []doctree.Entry{}, nil
				}
			}
			return list(c, p)
		},
		Stat: stat, Children: list, IAL: attributes,
		IsDir: func(c context.Context, p string) (bool, error) {
			entry, e := stat(c, p)
			return entry != nil && entry.Directory, e
		},
		Project: func(c context.Context, f doctree.Entry, ial map[string]string) (*doctree.File, error) {
			if saved, ok := m.read.documents[util.GetTreeID(f.Path)]; ok {
				f.Size = int64(len(saved.raw))
			}
			return doctree.Metadata(c, f, ial, time.Local, func(time.Time) (string, error) { return "", nil })
		},
		References: func(c context.Context) (map[string]int, error) {
			if refs == nil {
				var e error
				refs, e = (indexdb.References{DB: m.read.binding.scope.commit.r.main.DB}).RootCounts(c)
				if e != nil {
					return nil, e
				}
			}
			return refs, m.guard()
		},
		Sorts: func(c context.Context, files []*doctree.File) error {
			if !sortsRead {
				raw, e := source.Read(c, box+"/.siyuan/sort.json")
				if e != nil && !indexrepair.IsSourceNotExist(e) {
					return e
				}
				if e == nil {
					if e = json.Unmarshal(raw, &sorts); e != nil {
						return e
					}
					if sorts == nil {
						return errors.New("export_merge_sort_invalid")
					}
				}
				sortsRead = true
			}
			for _, f := range files {
				f.Sort = sorts[f.ID]
			}
			return m.guard()
		},
	}
	services = &exportMergeServices{guard: m.guard, load: m.exportTreeAt, list: func(requested, p string) ([]*File, error) {
		if requested != box {
			return nil, indexrepair.ErrSourcePath
		}
		files, _, e := doctree.List(ctx, doctree.Request{Box: box, Path: p, Sort: util.SortModeUnassigned, Max: m.mergeMax}, env)
		return files, e
	}}
	return services, root.Close, m.guard()
}
