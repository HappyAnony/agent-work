// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/eventbus"
	"github.com/vmihailenco/msgpack/v5"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type LocalRepoRequest struct{ Kind, Memo string }
type LocalRepoResult struct {
	ID                                   string
	Changed, Applied, Uncertain, Skipped bool
	Indexes, Objects                     int
	Bytes                                int64
}
type RetentionSelector func(func(int) ([]*entity.Index, int, error)) ([]string, error)

func (repo *Repo) LocalRepoContext(ctx context.Context, r LocalRepoRequest, o SnapshotOptions, selectRetention RetentionSelector) (out LocalRepoResult, err error) {
	if r.Kind != "index" && r.Kind != "auto-purge" {
		return out, ErrSnapshot
	}
	if e := lock.LockContext(ctx); e != nil {
		return out, e
	}
	defer lock.Unlock()
	if e := filepath.WalkDir(repo.Path, func(_ string, d fs.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrSnapshot
		}
		return ctx.Err()
	}); e != nil {
		return out, e
	}
	if o.Open == nil {
		o.Open = cloud.OpenContext
	}
	source := repo.cloud
	s := &snapshotScope{ctx: ctx, event: o.Event, open: o.Open}
	repo.snapshot = s
	repo.store.operation = ctx
	repo.store.modified = func() { s.mutated.Store(true) }
	client, done, e := o.Open(source, ctx)
	if e != nil {
		repo.snapshot = nil
		repo.store.operation = nil
		repo.store.modified = nil
		return out, e
	}
	repo.cloud = &snapshotCloud{Cloud: client, s: s}
	defer func() {
		if recover() != nil {
			err = ErrSnapshot
		}
		if err != nil && s.mutated.Load() {
			out.Uncertain = true
		}
		done()
		repo.cloud = source
		repo.snapshot = nil
		repo.store.operation = nil
		repo.store.modified = nil
	}()
	switch r.Kind {
	case "index":
		before, e := repo.Latest()
		if e != nil && !errors.Is(e, ErrNotFoundIndex) {
			return out, e
		}
		idx, e := repo.index(r.Memo, true, nil)
		if idx != nil {
			out.ID = idx.ID
			out.Changed = before == nil || before.ID != idx.ID
		}
		// A cancellation after atomic latest publication must retain the actual result.
		b, _ := os.ReadFile(filepath.Join(repo.Path, "refs", "latest"))
		out.Applied = idx != nil && strings.TrimSpace(string(b)) == idx.ID
		return out, e
	case "auto-purge":
		if selectRetention == nil {
			return out, ErrSnapshot
		}
		ids, e := selectRetention(func(page int) ([]*entity.Index, int, error) {
			rows, _, pages, e := repo.getIndexes(page, 512)
			if os.IsNotExist(e) {
				return nil, 0, nil
			}
			return rows, pages, e
		})
		if e != nil {
			return out, e
		}
		if len(ids) < 3 {
			out.Skipped = true
			return out, nil
		}
		for _, id := range ids {
			if !validScopedObjectID(id) {
				return out, ErrSnapshot
			}
		}
		stat, e := repo.store.Purge(ids...)
		if stat != nil {
			out.Indexes, out.Objects, out.Bytes = stat.Indexes, stat.Objects, stat.Size
		}
		out.Applied = e == nil
		return out, e
	}
	return out, ErrSnapshot
}
func (repo *Repo) scopedLatestFiles(index *entity.Index, checkChunks bool, ui map[string]interface{}) ([]*entity.File, error) {
	var mu sync.Mutex
	files := []*entity.File{}
	repo.emit(eventbus.EvtIndexBeforeGetLatestFiles, ui, len(index.Files))
	e := repo.snapshotBatch(index.Files, func(id string, count int) error {
		f, e := repo.store.GetFile(id)
		if e != nil {
			return e
		}
		if !validFile(f) {
			return ErrSnapshot
		}
		if checkChunks {
			for _, chunk := range f.Chunks {
				if _, e = repo.store.Stat(chunk); e != nil {
					return e
				}
			}
		}
		mu.Lock()
		files = append(files, f)
		mu.Unlock()
		repo.emit(eventbus.EvtIndexGetLatestFile, ui, count, len(index.Files))
		return nil
	})
	return files, e
}
func (repo *Repo) scopedIndexUpserts(files []*entity.File, ui map[string]interface{}) error {
	ids := []string{}
	byID := map[string]*entity.File{}
	for _, f := range files {
		if f == nil || !validScopedObjectID(f.ID) {
			return ErrSnapshot
		}
		ids = append(ids, f.ID)
		byID[f.ID] = f
	}
	return repo.snapshotBatch(ids, func(id string, count int) error {
		f := byID[id]
		if e := repo.putFileChunks(f, ui, count, len(files)); e != nil {
			return e
		}
		if len(f.Chunks) == 0 {
			return ErrSnapshot
		}
		return nil
	})
}

// Same full-latest msgpack format, prepared before publishing the latest ref.
func (repo *Repo) updateLatestScoped(index *entity.Index) error {
	files, e := repo.getFiles(index.Files)
	if e != nil {
		return e
	}
	b, e := msgpack.Marshal(&FullIndex{ID: index.ID, Files: files, Spec: 0})
	if e != nil {
		return e
	}
	if e = repo.store.writing(); e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(repo.Path, "refs"), 0755); e != nil {
		return e
	}
	if e = gulu.File.WriteFileSafer(filepath.Join(repo.Path, "full-latest.json"), b, 0644); e != nil {
		return e
	}
	if e = repo.store.check(); e != nil {
		return e
	}
	return gulu.File.WriteFileSafer(filepath.Join(repo.Path, "refs", "latest"), []byte(index.ID), 0644)
}
