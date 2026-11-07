// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/88250/lute/ast"

	"wiki-kernel/cache"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type incrementalIndexWalkIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

type incrementalIndexManifestEntry struct {
	name string
	info fs.FileInfo
}

func (r *indexRepairContext) invalidateIncremental(ctx context.Context, root, p string) error {
	rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "root", Value: root})
	if e != nil {
		return e
	}
	ids := make([]string, len(rows))
	for i, b := range rows {
		ids[i] = b.ID
	}
	cache.InvalidateTreeIAL(p, ids)
	return nil
}
func (r *indexRepairContext) incrementalIndex(ctx context.Context, upserts, removes []string) (repairIOResult indexrepair.IncrementalResult, repairIOErr error) {
	if e := r.guard(ctx); e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	if _, e := indexrepair.DocumentPaths(upserts); e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	if _, e := indexrepair.DocumentPaths(removes); e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	if e := r.queue.Flush(ctx); e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	// Root handles remain borrowed until actual loads/repairs and drain finish.
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	engine := util.NewLute()
	part := func(n int) int32 {
		if n == 0 {
			return 0
		}
		return int32(10 / float64(n))
	}
	return indexrepair.Incremental(ctx, upserts, removes, indexrepair.IncrementalServices{
		Remove: func(ctx context.Context, p indexrepair.DocumentPath) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: p.ID})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				return nil
			}
			root := rows[0]
			msg := fmt.Sprintf(r.language(39), root.RootID)
			if e = r.incRepairBootProgress(ctx, part(len(removes)), msg); e != nil {
				return e
			}
			if e = r.pushRepairStatus(ctx, msg); e != nil {
				return e
			}
			// Finish accepted cache/registry/queue changes before a cancellation return.
			critical := context.WithoutCancel(ctx)
			if e = r.invalidateIncremental(critical, root.RootID, root.Path); e != nil {
				return e
			}
			if _, e = r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "root", Value: root.RootID}); e != nil {
				return e
			}
			return r.queue.Remove(critical, root.RootID)
		},
		Upsert: func(ctx context.Context, p indexrepair.DocumentPath) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			msg := fmt.Sprintf(r.language(40), p.ID)
			if e = r.incRepairBootProgress(ctx, part(len(upserts)), msg); e != nil {
				return e
			}
			if e = r.pushRepairStatus(ctx, msg); e != nil {
				return e
			}
			tree, e := ioctx.Load(ctx, p.Box, p.Path, engine, true)
			if e != nil {
				return e
			}
			critical := context.WithoutCancel(ctx)
			if e = r.registry.Upsert(critical, tree); e != nil {
				return e
			}
			if e = r.queue.Upsert(critical, tree); e != nil {
				return e
			}
			return r.invalidateIncremental(critical, tree.ID, tree.Path)
		},
		Flush: func(ctx context.Context) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			return r.queue.Flush(ctx)
		},
	})
}

// Folder expansion is relative to the captured root, never the selected global
// path. Reject traversal/symlinks and report a failed scan before indexing starts.
func (r *indexRepairContext) expandIndexPaths(ctx context.Context, paths []string) (repairIOResult []string, repairIOErr error) {
	return r.expandIndexPathsWithIO(ctx, paths, incrementalIndexWalkIO{})
}

func (r *indexRepairContext) incrementalIndexManifest(ctx context.Context, source indexrepair.Source, root string, op incrementalIndexWalkIO) ([]incrementalIndexManifestEntry, error) {
	if err := r.guard(ctx); err != nil {
		return nil, err
	}
	if err := source.Check(root, false); err != nil {
		return nil, err
	}
	rootInfo, err := source.Data.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	readDir := op.readDir
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	manifest := []incrementalIndexManifestEntry{{name: root, info: rootInfo}}
	var scan func(string) error
	scan = func(folder string) error {
		if err := r.guard(ctx); err != nil {
			return err
		}
		before, err := source.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
			return indexrepair.ErrSourcePath
		}
		entries, err := readDir(ctx, source, folder)
		if err != nil {
			return err
		}
		after, err := source.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(after, before) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
		for _, entry := range entries {
			if err = r.guard(ctx); err != nil {
				return err
			}
			name := path.Join(folder, entry.Name())
			if err = source.Check(name, false); err != nil {
				return err
			}
			info, statErr := source.Data.Lstat(name)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
				return indexrepair.ErrSourceChanged
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return indexrepair.ErrSourcePath
			}
			manifest = append(manifest, incrementalIndexManifestEntry{name: name, info: info})
			if info.IsDir() {
				if err = scan(name); err != nil {
					return err
				}
			}
		}
		return r.guard(ctx)
	}
	if err = scan(root); err != nil {
		return nil, err
	}
	for _, entry := range manifest {
		if err = r.guard(ctx); err != nil {
			return nil, err
		}
		if err = source.Check(entry.name, false); err != nil {
			return nil, err
		}
		info, statErr := source.Data.Lstat(entry.name)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.info.IsDir() || info.Mode().Type() != entry.info.Mode().Type() || !os.SameFile(info, entry.info) || info.Size() != entry.info.Size() || !info.ModTime().Equal(entry.info.ModTime()) {
			return nil, indexrepair.ErrSourceChanged
		}
	}
	return manifest, r.guard(ctx)
}

func (r *indexRepairContext) expandIndexPathsWithIO(ctx context.Context, paths []string, op incrementalIndexWalkIO) (repairIOResult []string, repairIOErr error) {
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, false)
	if e != nil {
		return nil, e
	}
	defer func() {
		repairIOErr = joinRepairCloseError(repairIOErr, closeIO())
		if repairIOErr != nil {
			repairIOResult = nil
		}
	}()
	ret := []string{}
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			ret = append(ret, p)
		}
	}
	for _, p := range paths {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if !strings.HasSuffix(p, "/") {
			if util.HasDocExt(p) {
				add(p)
			}
			continue
		}
		dir := strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
		if !fs.ValidPath(dir) || dir == "." || strings.Contains(dir, "\\") {
			return nil, indexrepair.ErrSourcePath
		}
		for _, id := range strings.Split(dir, "/") {
			if !ast.IsNodeIDPattern(id) {
				return nil, indexrepair.ErrSourcePath
			}
		}
		var manifest []incrementalIndexManifestEntry
		manifest, e = r.incrementalIndexManifest(ctx, ioctx.Source, dir, op)
		if e != nil {
			return nil, e
		}
		for _, entry := range manifest {
			if !entry.info.IsDir() && util.HasDocExt(entry.name) {
				add(entry.name)
			}
		}
	}
	return ret, nil
}
func incrementalIndexesContext(ctx context.Context, paths []string, remove bool) error {
	s, e := Conf.CloudRepoService()
	if e != nil {
		return e
	}
	return s.IncrementalIndex(ctx, cloudrepo.IncrementalIndexRequest{Paths: paths, Remove: remove})
}
func UpsertIndexesContext(ctx context.Context, paths []string) error {
	return incrementalIndexesContext(ctx, paths, false)
}
func RemoveIndexesContext(ctx context.Context, paths []string) error {
	return incrementalIndexesContext(ctx, paths, true)
}

func incrementalReindexContext(ctx context.Context, upserts, removes []string) (indexrepair.IncrementalResult, error) {
	ups, e := indexrepair.DocumentPaths(upserts)
	if e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	rms, e := indexrepair.DocumentPaths(removes)
	if e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	if len(ups) == 0 && len(rms) == 0 {
		return indexrepair.IncrementalResult{UpsertRootIDs: []string{}, RemoveRootIDs: []string{}}, ctx.Err()
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return indexrepair.IncrementalResult{}, e
	}
	return r.incrementalIndex(ctx, upserts, removes)
}
