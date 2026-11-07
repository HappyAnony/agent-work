// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"github.com/88250/gulu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/entity"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (store *Store) prepareSnapshotPurge() error {
	refs, e := store.readRefs()
	if e != nil {
		return e
	}
	for id := range refs {
		if e = store.check(); e != nil {
			return e
		}
		idx, e := store.GetIndex(id)
		if e != nil {
			return e
		}
		for _, f := range idx.Files {
			file, e := store.GetFile(f)
			if e != nil || !validFile(file) {
				return ErrSnapshot
			}
			for _, c := range file.Chunks {
				if _, e = store.GetChunk(c); e != nil {
					return e
				}
			}
		}
	}
	dir := filepath.Join(store.Path, "check", "indexes")
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if e = store.check(); e != nil {
			return e
		}
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return e
		}
		b, e = store.compressDecoder.DecodeAll(b, nil)
		if e != nil {
			return e
		}
		v := &entity.CheckIndex{}
		if e = gulu.JSON.UnmarshalJSON(b, v); e != nil {
			return e
		}
		if v.ID != entry.Name() || !validScopedObjectID(v.ID) || !validScopedObjectID(v.IndexID) {
			return ErrSnapshot
		}
	}
	return nil
}
func (c *snapshotCloud) localRoot() (string, bool) {
	if _, ok := c.Cloud.(*cloud.Local); !ok {
		return "", false
	}
	v := c.GetConf()
	return filepath.Join(v.Local.Endpoint, v.Dir), true
}
func (c *snapshotCloud) ListObjects(prefix string) (map[string]*entity.ObjectInfo, error) {
	root, ok := c.localRoot()
	if !ok {
		return c.Cloud.ListObjects(prefix)
	}
	base := filepath.Join(root, filepath.FromSlash(prefix))
	out := map[string]*entity.ObjectInfo{}
	e := filepath.WalkDir(base, func(p string, d fs.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if e = c.s.ctx.Err(); e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrSnapshot
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(base, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		out[rel] = &entity.ObjectInfo{Path: rel, Size: info.Size()}
		return nil
	})
	return out, e
}
func (c *snapshotCloud) GetRefsFiles() ([]string, []*cloud.Ref, error) {
	if _, ok := c.localRoot(); !ok {
		return c.Cloud.GetRefsFiles()
	}
	items, e := c.ListObjects("refs/")
	if e != nil {
		return nil, nil, e
	}
	set := map[string]bool{}
	refs := []*cloud.Ref{}
	for name := range items {
		b, e := c.DownloadObject("refs/" + name)
		if e != nil {
			return nil, nil, e
		}
		id := strings.TrimSpace(string(b))
		if !validScopedObjectID(id) {
			return nil, nil, ErrSnapshot
		}
		idx, e := c.GetIndex(id)
		if e != nil {
			return nil, nil, e
		}
		if idx == nil || idx.ID != id {
			return nil, nil, ErrSnapshot
		}
		for _, f := range idx.Files {
			if !validScopedObjectID(f) {
				return nil, nil, ErrSnapshot
			}
			set[f] = true
		}
		refs = append(refs, &cloud.Ref{ID: id, Name: name})
	}
	files := []string{}
	for f := range set {
		files = append(files, f)
	}
	return files, refs, nil
}
