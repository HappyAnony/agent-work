// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"encoding/json"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
)

// File IDs are opaque 40-hex references in the original format. Scoped indexing
// extends the identity input so same-second versions cannot share/overwrite one
// immutable metadata object. Existing IDs and source document IDs are unchanged.
func (repo *Repo) putIndexedFile(f *entity.File) error {
	if repo.snapshot != nil && repo.snapshot.indexing {
		old := repo.snapshot.previous[f.Path]
		same := old != nil && old.Size == f.Size && old.Updated == f.Updated && len(old.Chunks) == len(f.Chunks)
		if same {
			for i := range f.Chunks {
				if old.Chunks[i] != f.Chunks[i] {
					same = false
					break
				}
			}
		}
		if same {
			f.ID = old.ID
			return repo.store.check()
		}
		b, e := json.Marshal(struct {
			Path          string
			Size, Updated int64
			Chunks        []string
		}{f.Path, f.Size, f.Updated, f.Chunks})
		if e != nil {
			return e
		}
		f.ID = util.Hash(append([]byte("wiki-file-v2\x00"), b...))
	}
	return repo.store.PutFile(f)
}

// Legacy incremental indexing must preserve opaque IDs of unchanged records,
// rather than inventing path+second IDs absent from the referenced snapshot.
func reuseUnchangedFileIDs(files, previous, upserts []*entity.File) {
	old := map[string]*entity.File{}
	written := map[string]bool{}
	for _, f := range previous {
		old[f.Path] = f
	}
	for _, f := range upserts {
		written[f.Path] = true
	}
	for _, f := range files {
		if !written[f.Path] && old[f.Path] != nil {
			f.ID = old[f.Path].ID
		}
	}
}
