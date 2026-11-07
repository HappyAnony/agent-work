// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	jsoniter "github.com/json-iterator/go"
	"io"
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

type conflictSortIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
	notify  func(map[string]int)
}

func (r *indexRepairContext) addConflictSort(ctx context.Context, box, previous, id string) (repairIOErr error) {
	return r.addConflictSortWithIO(ctx, box, previous, id, conflictSortIO{})
}
func (r *indexRepairContext) addConflictSortWithIO(ctx context.Context, box, previous, id string, op conflictSortIO) (repairIOErr error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	if !ast.IsNodeIDPattern(box) || !ast.IsNodeIDPattern(id) {
		return indexrepair.ErrSourcePath
	}
	documents, e := indexrepair.DocumentPaths([]string{box + previous})
	if e != nil {
		return e
	}
	if len(documents) != 1 || !strings.HasPrefix(previous, "/") {
		return indexrepair.ErrSourcePath
	}
	var globalSort, max int
	if e := r.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.FileTree == nil {
			return conf.ErrBusinessUnbound
		}
		globalSort, max = v.FileTree.Sort, v.FileTree.MaxListCount
		return nil
	}); e != nil {
		return e
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	source := ioctx.Source
	readDir := op.readDir
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	name := box + "/.siyuan/sort.json"
	before, e := source.Read(ctx, name)
	if indexrepair.IsSourceNotExist(e) {
		before = nil
	} else if e != nil {
		return e
	}
	existing := map[string]int{}
	if before != nil {
		if e = json.Unmarshal(before, &existing); e != nil {
			return e
		}
		if existing == nil {
			return errors.New("invalid_sort_map")
		}
	}
	notebook := conf.NewBoxConf()
	raw, e := source.Read(ctx, box+"/.siyuan/conf.json")
	if e == nil {
		if e = json.Unmarshal(raw, notebook); e != nil {
			return e
		}
	} else if !indexrepair.IsSourceNotExist(e) {
		return e
	}
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
	stat := func(ctx context.Context, p string) (*doctree.Entry, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		n, e := rooted(p)
		if e != nil {
			return nil, e
		}
		if e = source.Check(n, false); errors.Is(e, fs.ErrNotExist) {
			return nil, nil
		} else if e != nil {
			return nil, e
		}
		info, e := source.Data.Stat(n)
		if e != nil {
			return nil, e
		}
		return &doctree.Entry{Path: p, Name: info.Name(), Size: info.Size(), Directory: info.IsDir()}, nil
	}
	list := func(ctx context.Context, p string) ([]doctree.Entry, error) {
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
		if e = r.guard(ctx); e != nil {
			return nil, e
		}
		after, e := source.Data.Lstat(n)
		if e != nil {
			return nil, e
		}
		if !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(after, before) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return nil, indexrepair.ErrSourceChanged
		}
		ret := []doctree.Entry{}
		for _, d := range entries {
			if e = r.guard(ctx); e != nil {
				return nil, e
			}
			if util.IsReservedFilename(d.Name()) || strings.HasSuffix(d.Name(), ".tmp") {
				continue
			}
			child := path.Join(n, d.Name())
			if e = source.Check(child, false); e != nil {
				return nil, e
			}
			info, e := d.Info()
			if e != nil {
				return nil, e
			}
			p := path.Join(p, d.Name())
			if d.IsDir() {
				p += "/"
			}
			ret = append(ret, doctree.Entry{Path: p, Name: d.Name(), Size: info.Size(), Directory: d.IsDir()})
		}
		return ret, nil
	}
	ial := func(ctx context.Context, p string) (properties map[string]string, result error) {
		if !strings.HasSuffix(p, ".docj") {
			return nil, nil
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
		if !before.Mode().IsRegular() {
			return nil, indexrepair.ErrSourcePath
		}
		f, e := source.Data.Open(n)
		if e != nil {
			return nil, e
		}
		defer func() {
			result = errors.Join(result, f.Close(), ctx.Err(), source.Check(n, false))
			after, err := source.Data.Lstat(n)
			result = errors.Join(result, err)
			if err == nil && (!os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime())) {
				result = errors.Join(result, indexrepair.ErrSourceChanged)
			}
			if result != nil {
				properties = nil
			}
		}()
		opened, e := f.Stat()
		if e != nil {
			return nil, e
		}
		if !os.SameFile(before, opened) {
			return nil, indexrepair.ErrSourceChanged
		}
		iter := jsoniter.Parse(jsoniter.ConfigCompatibleWithStandardLibrary, sortContextReader{ctx: ctx, reader: f}, 512)
		for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
			if field == "Properties" {
				iter.ReadVal(&properties)
				break
			}
			iter.Skip()
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if iter.Error != nil {
			return nil, iter.Error
		}
		if len(properties) == 0 {
			return nil, errors.New("missing_document_properties")
		}
		return properties, nil
	}
	docs, _, e := doctree.List(ctx, doctree.Request{Box: box, Path: path.Dir(previous), Sort: util.SortModeUnassigned, Max: max}, doctree.Services{
		Settings: func(context.Context, string) (int, int, error) { return notebook.SortMode, globalSort, nil }, List: list, Stat: stat, IAL: ial,
		IsDir: func(ctx context.Context, p string) (bool, error) {
			v, e := stat(ctx, p)
			return v != nil && v.Directory, e
		}, Children: list,
		// This private list publishes ordering IDs only. Humanized display strings
		// are unused; metadata's original numeric/name/count sort fields are shared.
		Project: func(ctx context.Context, f doctree.Entry, a map[string]string) (*doctree.File, error) {
			return doctree.Metadata(ctx, f, a, time.Local, func(time.Time) (string, error) { return "", nil })
		},
		References: (indexdb.References{DB: r.main.DB}).RootCounts,
		Sorts: func(_ context.Context, files []*doctree.File) error {
			for _, f := range files {
				f.Sort = existing[f.ID]
			}
			return nil
		},
	})
	if e != nil {
		return e
	}
	full, notice := doctree.InsertSort(existing, docs, strings.TrimSuffix(path.Base(previous), ".docj"), id)
	var ownedNotice *markdownSortChange
	if op.notify == nil {
		ownedNotice, e = prepareOwnedSortNotice(ctx, r, box, notice)
		if e != nil {
			return e
		}
	}
	after, e := json.Marshal(full)
	if e != nil {
		return e
	}
	if e = r.guard(ctx); e != nil {
		return e
	}
	if e = source.Rewrite(ctx, name, before, after); e != nil {
		return e
	}
	if e = r.guard(ctx); e != nil {
		return e
	}
	if e = closeIO(); e != nil {
		return e
	}
	if op.notify != nil {
		op.notify(notice)
		return nil
	}
	return publishOwnedSortNotice(ctx, r, ownedNotice)
}

// Preserve original streaming property lookup for large documents; sorting does
// not need to materialize the document body or touch its global IAL cache.
type sortContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r sortContextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.reader.Read(p)
	if e == nil {
		e = r.ctx.Err()
	}
	return n, e
}
