package sourcetx

import (
	"context"
	"os"
	"path"
	"slices"
	"strings"
	"wiki-kernel/internal/wikistore"
)

type FileInfo struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int    `json:"size"`
}

func (s *Store) Lookup(ctx context.Context, id, clientHash string) (Receipt, bool, error) {
	var r Receipt
	var found bool
	if !hashID.MatchString(clientHash) {
		return r, false, ErrInvalid
	}
	err := s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		r, found, err = receipt(root, id)
		if err != nil {
			return err
		}
		if found && r.ClientHash != clientHash {
			return ErrConflict
		}
		return nil
	})
	return r, found, err
}

func (s *Store) Files(ctx context.Context, page string) ([]FileInfo, error) {
	return s.filesWithIO(ctx, page, readManifestIO{})
}

func (s *Store) filesWithIO(ctx context.Context, page string, op readManifestIO) ([]FileInfo, error) {
	if !pageID.MatchString(page) {
		return nil, ErrInvalid
	}
	files, _, err := s.sourceManifest(ctx, "pages/"+page+"/data", op, false)
	return files, err
}

// PageReadGuards pins the active page contents including additions and removals.
func (s *Store) PageReadGuards(ctx context.Context, page string) ([]FileInfo, *ReadGuards, error) {
	if !pageID.MatchString(page) {
		return nil, nil, ErrInvalid
	}
	return s.sourceManifest(ctx, "pages/"+page+"/data", readManifestIO{}, true)
}

// ArchiveFiles lists only a single owned box of deletion records or a single
// document's versions. The public service supplies IDs, never a filesystem path.
func (s *Store) ArchiveFiles(ctx context.Context, namespace, page, box, doc string) ([]FileInfo, error) {
	files, _, err := s.archiveManifest(ctx, namespace, page, box, doc, false)
	return files, err
}

func (s *Store) ArchiveReadGuards(ctx context.Context, namespace, page string) ([]FileInfo, *ReadGuards, error) {
	return s.archiveManifest(ctx, namespace, page, "", "", true)
}

func (s *Store) archiveManifest(ctx context.Context, namespace, page, box, doc string, guarded bool) ([]FileInfo, *ReadGuards, error) {
	if !pageID.MatchString(page) || box != "" && !docID.MatchString(box) {
		return nil, nil, ErrInvalid
	}
	base := namespace + "/documents/" + page
	if box != "" {
		base += "/" + box
	}
	switch namespace {
	case "notebooks":
		if box != "" || doc != "" {
			return nil, nil, ErrInvalid
		}
		base = "tombstones/notebooks/" + page
	case "tombstones", "deleted":
		if doc != "" {
			return nil, nil, ErrInvalid
		}
	case "versions":
		if doc != "" {
			if box == "" || !docID.MatchString(doc) {
				return nil, nil, ErrInvalid
			}
			base += "/" + doc
		}
	default:
		return nil, nil, ErrInvalid
	}
	files, guards, err := s.sourceManifest(ctx, base, readManifestIO{}, guarded)
	for _, file := range files {
		if !sourcePath(file.Path) || !strings.HasPrefix(file.Path, base+"/") {
			return nil, nil, ErrInvalid
		}
	}
	return files, guards, err
}

func (s *Store) sourceManifest(ctx context.Context, base string, op readManifestIO, guarded bool) ([]FileInfo, *ReadGuards, error) {
	files := []FileInfo{}
	guards := &ReadGuards{Files: map[string]string{}, Directories: map[string][]string{}}
	err := s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := safePath(root, base); err != nil {
			return err
		}
		options := readManifestOptions{maxEntries: 100000}
		manifest, exists, err := confirmedSourceReadManifest(ctx, root, base, options, op)
		if err != nil {
			return err
		}
		if !exists {
			if guarded {
				guards.Missing = []string{base}
			}
			return nil
		}
		next := []FileInfo{}
		total := 0
		for _, entry := range manifest {
			if err := ctx.Err(); err != nil {
				return err
			}
			if guarded && entry.name != base {
				parent := path.Dir(entry.name)
				guards.Directories[parent] = append(guards.Directories[parent], path.Base(entry.name))
			}
			if guarded && entry.info.IsDir() {
				if _, ok := guards.Directories[entry.name]; !ok {
					guards.Directories[entry.name] = []string{}
				}
			}
			if entry.info.IsDir() {
				continue
			}
			raw, err := readManifestData(root, entry, maxFile, op)
			if err != nil {
				return err
			}
			total += len(raw)
			if total > 512<<20 {
				return ErrInvalid
			}
			h := Hash(raw)
			next = append(next, FileInfo{entry.name, h, len(raw)})
			if guarded {
				guards.Files[entry.name] = h
			}
		}
		if err := confirmFinalSourceReadManifest(ctx, root, base, options, manifest, op); err != nil {
			return err
		}
		for _, names := range guards.Directories {
			slices.Sort(names)
		}
		files = next
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return files, guards, nil
}

// Identity remains readable even when a pending source journal makes the page
// graph temporarily invalid. It is checked under the source lock for recovery.
func (s *Store) Identity(ctx context.Context) (wikistore.Store, error) {
	var result wikistore.Store
	l, err := s.lock(ctx, "source", true, true)
	if err != nil {
		return result, err
	}
	defer l.Close()
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if err := s.checkIdentity(root); err != nil {
		return result, err
	}
	raw, _, err := readFile(root, "store.json", 1<<20)
	if err != nil {
		return result, err
	}
	// Reuse the metadata schema validator, not a permissive recovery-only shape.
	result, err = wikistore.DecodeStore(raw)
	return result, err
}

// SourceFile never follows symlinks and only accepts whitelisted source paths,
// including classified preferences. Arbitrary configuration/runtime paths fail.
func (s *Store) SourceFile(ctx context.Context, name string) ([]byte, error) {
	if !sourcePath(name) {
		return nil, ErrInvalid
	}
	var result []byte
	err := s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		data, exists, err := readFile(root, name, maxFile)
		if err != nil {
			return err
		}
		if !exists {
			return os.ErrNotExist
		}
		result = data
		return ctx.Err()
	})
	return result, err
}

// Document resolves native nested notebook paths without exposing filesystem
// paths in the public operation schema. Ambiguous IDs and symlinks fail closed.
func (s *Store) Document(ctx context.Context, page, box, doc string) (string, []byte, error) {
	return s.documentWithIO(ctx, page, box, doc, readManifestIO{})
}

func (s *Store) documentWithIO(ctx context.Context, page, box, doc string, op readManifestIO) (string, []byte, error) {
	if !pageID.MatchString(page) || !docID.MatchString(box) || !docID.MatchString(doc) {
		return "", nil, ErrInvalid
	}
	var found string
	var data []byte
	err := s.Read(ctx, func() error {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		base := "pages/" + page + "/data/" + box
		if err := safePath(root, base); err != nil {
			return err
		}
		options := readManifestOptions{skipHiddenDirectories: true, maxEntries: 100000}
		manifest, exists, err := confirmedSourceReadManifest(ctx, root, base, options, op)
		if err != nil {
			return err
		}
		if !exists {
			return os.ErrNotExist
		}
		var candidate *readManifestEntry
		for i := range manifest {
			entry := &manifest[i]
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.info.IsDir() && path.Base(entry.name) == doc+".docj" {
				if candidate != nil {
					return ErrInvalid
				}
				candidate = entry
			}
		}
		if candidate == nil {
			return os.ErrNotExist
		}
		next, err := readManifestData(root, *candidate, 16<<20, op)
		if err != nil {
			return err
		}
		if err := confirmFinalSourceReadManifest(ctx, root, base, options, manifest, op); err != nil {
			return err
		}
		found = candidate.name
		data = next
		return nil
	})
	return found, data, err
}
