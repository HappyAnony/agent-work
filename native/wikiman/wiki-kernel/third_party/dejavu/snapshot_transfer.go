// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
	"github.com/siyuan-note/eventbus"
	"path"
	"sync"
	"sync/atomic"
)

func (repo *Repo) snapshotDownload(ids []string, chunks bool, ui map[string]interface{}) (int64, []*entity.File, error) {
	for _, id := range ids {
		if !validScopedObjectID(id) {
			return 0, nil, ErrSnapshot
		}
	}
	var bytes atomic.Int64
	var files []*entity.File
	var mu sync.Mutex
	topic := eventbus.EvtCloudBeforeDownloadFiles
	if chunks {
		topic = eventbus.EvtCloudBeforeDownloadChunks
	}
	repo.emit(topic, ui, len(ids))
	e := repo.snapshotBatch(ids, func(id string, count int) error {
		if chunks {
			n, c, e := repo.downloadCloudChunk(id, count, len(ids), ui)
			if e != nil {
				return e
			}
			if util.Hash(c.Data) != id {
				return ErrSnapshot
			}
			if e = repo.store.PutChunk(c); e != nil {
				return e
			}
			bytes.Add(n)
			return nil
		}
		n, f, e := repo.downloadCloudFile(id, count, len(ids), ui)
		if e != nil {
			return e
		}
		if f == nil || f.ID != id || !validFile(f) {
			return ErrSnapshot
		}
		if e = repo.store.PutFile(f); e != nil {
			return e
		}
		bytes.Add(n)
		mu.Lock()
		files = append(files, f)
		mu.Unlock()
		return nil
	})
	return bytes.Load(), files, e
}
func (repo *Repo) snapshotUpload(ids []string, chunks bool, ui map[string]interface{}) (int64, error) {
	for _, id := range ids {
		if !validScopedObjectID(id) {
			return 0, ErrSnapshot
		}
	}
	var bytes atomic.Int64
	topic, item := eventbus.EvtCloudBeforeUploadFiles, eventbus.EvtCloudBeforeUploadFile
	if chunks {
		topic, item = eventbus.EvtCloudBeforeUploadChunks, eventbus.EvtCloudBeforeUploadChunk
	}
	repo.emit(topic, ui, len(ids))
	e := repo.snapshotBatch(ids, func(id string, count int) error {
		repo.emit(item, ui, count, len(ids))
		n, e := repo.cloud.UploadObject(path.Join("objects", id[:2], id[2:]), false)
		bytes.Add(n)
		return e
	})
	return bytes.Load(), e
}
func validFile(f *entity.File) bool {
	if f == nil || !validScopedObjectID(f.ID) || f.Size < 0 {
		return false
	}
	for _, id := range f.Chunks {
		if !validScopedObjectID(id) {
			return false
		}
	}
	return true
}

func (repo *Repo) validateSnapshotClosure(index *entity.Index) error {
	for _, id := range index.Files {
		f, e := repo.store.GetFile(id)
		if e != nil || !validFile(f) {
			return ErrSnapshot
		}
		var size int64
		for _, chunkID := range f.Chunks {
			c, e := repo.store.GetChunk(chunkID)
			if e != nil || util.Hash(c.Data) != chunkID {
				return ErrSnapshot
			}
			size += int64(len(c.Data))
		}
		if size != f.Size {
			return ErrSnapshot
		}
	}
	return repo.store.check()
}
