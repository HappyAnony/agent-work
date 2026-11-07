// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/historyindex"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/sourcetx"
)

type LegacyDocumentVersion struct {
	Path, Hash, Title string
	Size              int
}

// NativeDocumentVersions reads the original automatic history layout beneath
// the owned runtime root. Imported source paths cannot select another page.
func (l *Lease) NativeDocumentVersions(ctx context.Context, box, doc string) (out []LegacyDocumentVersion, err error) {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) {
		return nil, ErrDocument
	}
	err = l.withDocumentHistory(ctx, func(source indexrepair.Source, b *indexdb.Bundle) error {
		w, r := l.workspace, l.workspace.resources
		engine, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		dirs, e := source.ReadDir(ctx, ".")
		if e != nil {
			return e
		}
		if len(dirs) > 10000 {
			return ErrBudget
		}
		for _, dir := range dirs {
			stamp := dir.Name()
			if _, _, e = historyindex.Directory(stamp, time.Local); e != nil {
				continue
			}
			if !dir.IsDir() {
				return ErrDocument
			}
			entries, e := historyindex.Plan(ctx, source, stamp, "Untitled", time.Local, engine)
			if e != nil {
				return e
			}
			for _, entry := range entries {
				if entry.ID != doc || !validDocumentVersion(entry.Path, box, doc) {
					continue
				}
				raw, e := readDocumentVersion(ctx, source, entry.Path, doc)
				if e != nil {
					return e
				}
				out = append(out, LegacyDocumentVersion{Path: entry.Path, Hash: sourcetx.Hash(raw), Title: entry.Title, Size: len(raw)})
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (l *Lease) NativeDocumentVersion(ctx context.Context, box, doc, name string) (raw []byte, err error) {
	if !validDocumentVersion(name, box, doc) {
		return nil, ErrDocument
	}
	err = l.withDocumentHistory(ctx, func(source indexrepair.Source, _ *indexdb.Bundle) error {
		var e error
		raw, e = readDocumentVersion(ctx, source, name, doc)
		return e
	})
	if err == nil && raw == nil {
		err = fs.ErrNotExist
	}
	return
}

// NativeDocumentVersionAVs loads the original AV files captured in the same
// history generation as the selected document.
func (l *Lease) NativeDocumentVersionAVs(ctx context.Context, box, doc, name string, ids []string) (map[string][]byte, error) {
	if !validDocumentVersion(name, box, doc) {
		return nil, ErrDocument
	}
	stamp, _, _ := strings.Cut(name, "/")
	out := map[string][]byte{}
	err := l.withDocumentHistory(ctx, func(source indexrepair.Source, _ *indexdb.Bundle) error {
		for _, id := range ids {
			if !documentID.MatchString(id) {
				return ErrDocument
			}
			raw, e := source.ReadLimited(ctx, path.Join(stamp, "storage/av", id+".json"), 16<<20)
			if indexrepair.IsSourceNotExist(e) {
				continue
			}
			if e != nil {
				return e
			}
			var av struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &av) != nil || av.ID != id {
				return ErrDocument
			}
			out[id] = raw
		}
		return nil
	})
	return out, err
}

func (l *Lease) withDocumentHistory(ctx context.Context, fn func(indexrepair.Source, *indexdb.Bundle) error) error {
	return l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) (err error) {
		name := filepath.Join(l.workspace.paths.RuntimeDir, "history")
		info, e := os.Lstat(name)
		if errors.Is(e, fs.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return ErrDocument
		}
		root, e := os.OpenRoot(name)
		if e != nil {
			return e
		}
		defer func() { err = errors.Join(err, root.Close()) }()
		pinned, e := root.Stat(".")
		if e != nil {
			return e
		}
		if !os.SameFile(info, pinned) {
			return ErrDocument
		}
		return fn(indexrepair.Source{Data: root}, b)
	})
}

func validDocumentVersion(name, box, doc string) bool {
	if !fs.ValidPath(name) || strings.Contains(name, "\\") || len(name) > 4096 || !documentID.MatchString(box) || !documentID.MatchString(doc) {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) < 3 || len(parts) > 64 || parts[1] != box || path.Base(name) != doc+".docj" {
		return false
	}
	if _, _, e := historyindex.Directory(parts[0], time.Local); e != nil {
		return false
	}
	for _, part := range parts[2 : len(parts)-1] {
		if !documentID.MatchString(part) {
			return false
		}
	}
	return true
}

func readDocumentVersion(ctx context.Context, source indexrepair.Source, name, doc string) ([]byte, error) {
	raw, e := source.ReadLimited(ctx, name, 16<<20)
	if e != nil {
		return nil, e
	}
	if _, _, e = (&docast.Parser{}).Parse(ctx, raw, doc); e != nil {
		return nil, e
	}
	return raw, nil
}
