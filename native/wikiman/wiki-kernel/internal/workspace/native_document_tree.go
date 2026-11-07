package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
)

func treeFileStat(ctx context.Context, w *WorkspaceContext, rel string) (fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.data == nil || !fs.ValidPath(rel) {
		return nil, ErrDocument
	}
	parts := strings.Split(rel, "/")
	var info fs.FileInfo
	for i := range parts {
		var err error
		info, err = w.data.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrDocument
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, ErrDocument
		}
	}
	return info, ctx.Err()
}
func treeRelative(box, p string) (string, error) {
	p = strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
	if p != "" && !fs.ValidPath(p) {
		return "", ErrDocument
	}
	if p == "" {
		return box, nil
	}
	return box + "/" + p, nil
}
func treeReserved(name string) bool {
	return strings.HasPrefix(name, ".") || name == "assets" || name == "templates" || name == "widgets" || name == "emojis"
}

// Native listing is read-only: no corruption moves, notebook repair or tmp cleanup.
// Cards are explicitly owned services and must not reenter this lease's index gate.
func (l *Lease) NativeListDocumentTree(ctx context.Context, q doctree.Request, cards *docquery.Flashcards) (ret []*doctree.File, total int, err error) {
	if !documentID.MatchString(q.Box) {
		return nil, 0, ErrDocument
	}
	if cards != nil {
		copy := *cards
		cards = &copy
	}
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if q.Flashcard && cards == nil {
			cards = r.navigationFlashcards(w)
		}
		var formatter func(time.Time, time.Time) string
		now := time.Now()
		readDir := func(ctx context.Context, p string, filter bool) ([]doctree.Entry, error) {
			rel, e := treeRelative(q.Box, p)
			if e != nil {
				return nil, e
			}
			info, e := treeFileStat(ctx, w, rel)
			if e != nil {
				return nil, e
			}
			if !info.IsDir() {
				return nil, ErrDocument
			}
			manifest, e := confirmedWorkspaceDirectoryManifest(ctx, w, rel, workspaceDirectoryIO{})
			if e != nil {
				return nil, e
			}
			out := []doctree.Entry{}
			for _, entry := range manifest.entries {
				if e = ctx.Err(); e != nil {
					return nil, e
				}
				name := path.Base(entry.name)
				if filter && (treeReserved(name) || strings.HasSuffix(name, ".tmp")) {
					continue
				}
				if entry.info.Mode()&fs.ModeSymlink != 0 {
					return nil, ErrDocument
				}
				fpath := path.Join(p, name)
				if entry.info.IsDir() {
					fpath += "/"
				}
				out = append(out, doctree.Entry{Path: fpath, Name: name, Size: entry.info.Size(), Directory: entry.info.IsDir()})
			}
			return out, nil
		}
		services := doctree.Services{Cards: cards,
			Settings: func(ctx context.Context, box string) (int, int, error) {
				if r.configuration.FileTree == nil {
					return 0, 0, indexdb.ErrProjection
				}
				boxes, e := r.configurationBusiness.Notebooks(ctx)
				if e != nil {
					return 0, 0, e
				}
				for _, b := range boxes {
					if b.ID == box {
						return b.SortMode, r.configuration.FileTree.Sort, nil
					}
				}
				return 0, 0, ErrDocument
			},
			List: func(ctx context.Context, p string) ([]doctree.Entry, error) {
				if strings.HasSuffix(p, ".docj") {
					p = strings.TrimSuffix(p, ".docj")
					rel, e := treeRelative(q.Box, p)
					if e != nil {
						return nil, e
					}
					info, e := treeFileStat(ctx, w, rel)
					if errors.Is(e, fs.ErrNotExist) {
						return []doctree.Entry{}, nil
					}
					if e != nil {
						return nil, e
					}
					if !info.IsDir() {
						return []doctree.Entry{}, nil
					}
				}
				return readDir(ctx, p, true)
			},
			Stat: func(ctx context.Context, p string) (*doctree.Entry, error) {
				rel, e := treeRelative(q.Box, p)
				if e != nil {
					return nil, e
				}
				info, e := treeFileStat(ctx, w, rel)
				if errors.Is(e, fs.ErrNotExist) {
					return nil, nil
				}
				if e != nil {
					return nil, e
				}
				return &doctree.Entry{Path: p, Name: info.Name(), Size: info.Size(), Directory: info.IsDir()}, nil
			},
			IsDir: func(ctx context.Context, p string) (bool, error) {
				rel, e := treeRelative(q.Box, p)
				if e != nil {
					return false, e
				}
				info, e := treeFileStat(ctx, w, rel)
				if errors.Is(e, fs.ErrNotExist) {
					return false, nil
				}
				if e != nil {
					return false, e
				}
				return info.IsDir(), nil
			},
			Children: func(ctx context.Context, p string) ([]doctree.Entry, error) { return readDir(ctx, p, false) },
			IAL: func(ctx context.Context, p string) (map[string]string, error) {
				if !strings.HasSuffix(strings.ToLower(path.Base(p)), ".docj") {
					return nil, nil
				}
				rel, e := treeRelative(q.Box, p)
				if e != nil {
					return nil, e
				}
				raw, e := readNativeFile(ctx, w, rel, 16<<20)
				if e != nil {
					return nil, e
				}
				input, e := docast.OriginalTreeInput(raw)
				if e != nil {
					return nil, e
				}
				var doc struct {
					ID, Type   string
					Properties map[string]string
				}
				if e = json.Unmarshal(input, &doc); e != nil {
					return nil, e
				}
				if doc.Type != "NodeDocument" || doc.ID != strings.TrimSuffix(path.Base(p), ".docj") || doc.Properties["id"] != doc.ID || !documentID.MatchString(doc.ID) {
					return nil, ErrDocument
				}
				return doc.Properties, nil
			},
			Project: func(ctx context.Context, f doctree.Entry, ial map[string]string) (*doctree.File, error) {
				if formatter == nil {
					var e error
					formatter, e = r.configurationBusiness.HumanizeTime(r.configuration.Lang)
					if e != nil {
						return nil, e
					}
				}
				return doctree.Metadata(ctx, f, ial, time.Local, func(t time.Time) (string, error) { return formatter(t, now), nil })
			},
			References: func(ctx context.Context) (map[string]int, error) {
				db, e := bundle.Database(indexdb.Main)
				if e != nil {
					return nil, e
				}
				return (indexdb.References{DB: db}).RootCounts(ctx)
			},
			Sorts: func(ctx context.Context, files []*doctree.File) error {
				raw, e := readNativeFile(ctx, w, q.Box+"/.wiki/sort.json", 1<<20)
				if errors.Is(e, fs.ErrNotExist) {
					return nil
				}
				if e != nil {
					return e
				}
				var values map[string]int
				if e = json.Unmarshal(raw, &values); e != nil {
					return e
				}
				for _, f := range files {
					f.Sort = values[f.ID]
				}
				return ctx.Err()
			},
		}
		var e error
		ret, total, e = doctree.List(ctx, q, services)
		return e
	})
	if err != nil {
		return nil, 0, err
	}
	return
}
