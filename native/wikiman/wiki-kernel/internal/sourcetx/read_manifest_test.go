package sourcetx

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func replaceReadManifestTarget(t *testing.T, rootPath, name string) {
	t.Helper()
	target := filepath.Join(rootPath, filepath.FromSlash(name))
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		raw[i] = ' '
	}
	tmp := target + ".replacement"
	if err := os.WriteFile(tmp, raw, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}
}

func TestCheckedReadersRejectIncompleteOrReplacedSourcesWithoutPartialResults(t *testing.T) {
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"

	t.Run("files incomplete enumeration", func(t *testing.T) {
		rootPath := fixture(t)
		s, _ := open(t, rootPath)
		base := "pages/" + p2 + "/data"
		calls := 0
		incomplete := errors.New("incomplete enumeration")
		op := readManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			if name == base {
				calls++
				if calls == 2 {
					return nil, incomplete
				}
			}
			return fs.ReadDir(root.FS(), name)
		}}
		files, err := s.filesWithIO(context.Background(), p2, op)
		if !errors.Is(err, incomplete) || len(files) != 0 {
			t.Fatal(files, err)
		}
	})

	t.Run("files same metadata replacement", func(t *testing.T) {
		rootPath := fixture(t)
		s, _ := open(t, rootPath)
		calls := 0
		op := readManifestIO{lstat: func(root *os.Root, name string) (fs.FileInfo, error) {
			if name == docPath {
				calls++
				if calls == 5 {
					replaceReadManifestTarget(t, rootPath, name)
				}
			}
			return root.Lstat(name)
		}}
		files, err := s.filesWithIO(context.Background(), p2, op)
		if !errors.Is(err, errReadSourceChanged) || len(files) != 0 {
			t.Fatal(files, err)
		}
	})

	t.Run("document incomplete enumeration", func(t *testing.T) {
		rootPath := fixture(t)
		s, _ := open(t, rootPath)
		base := "pages/" + p2 + "/data/" + box
		calls := 0
		incomplete := errors.New("incomplete enumeration")
		op := readManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			if name == base {
				calls++
				if calls == 2 {
					return nil, incomplete
				}
			}
			return fs.ReadDir(root.FS(), name)
		}}
		name, data, err := s.documentWithIO(context.Background(), p2, box, doc, op)
		if !errors.Is(err, incomplete) || name != "" || data != nil {
			t.Fatal(name, data, err)
		}
	})

	t.Run("document same metadata replacement", func(t *testing.T) {
		rootPath := fixture(t)
		s, _ := open(t, rootPath)
		calls := 0
		op := readManifestIO{lstat: func(root *os.Root, name string) (fs.FileInfo, error) {
			if name == docPath {
				calls++
				if calls == 5 {
					replaceReadManifestTarget(t, rootPath, name)
				}
			}
			return root.Lstat(name)
		}}
		name, data, err := s.documentWithIO(context.Background(), p2, box, doc, op)
		if !errors.Is(err, errReadSourceChanged) || name != "" || data != nil {
			t.Fatal(name, data, err)
		}
	})
}

func TestCheckedReadersPreserveFilteringMissingAndLinkRules(t *testing.T) {
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"

	t.Run("filtering and missing roots", func(t *testing.T) {
		rootPath := fixture(t)
		hidden := filepath.Join(rootPath, "pages", p2, "data", box, ".hidden", doc+".docj")
		if err := os.MkdirAll(filepath.Dir(hidden), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hidden, []byte("hidden file remains listable"), 0600); err != nil {
			t.Fatal(err)
		}
		s, _ := open(t, rootPath)
		files, err := s.Files(context.Background(), p2)
		if err != nil || files == nil || !sort.SliceIsSorted(files, func(i, j int) bool { return files[i].Path < files[j].Path }) {
			t.Fatal(files, err)
		}
		hiddenName := filepath.ToSlash(strings.TrimPrefix(hidden, rootPath+string(filepath.Separator)))
		listed := false
		for _, file := range files {
			listed = listed || file.Path == hiddenName
		}
		if !listed {
			t.Fatal("Files stopped listing hidden directory contents", files)
		}
		name, data, err := s.Document(context.Background(), p2, box, doc)
		if err != nil || name != docPath || len(data) == 0 {
			t.Fatal(name, len(data), err)
		}
		missingPage := "1760000000000000099"
		files, err = s.Files(context.Background(), missingPage)
		if err != nil || files == nil || len(files) != 0 {
			t.Fatal(files, err)
		}
		if name, data, err = s.Document(context.Background(), p2, "20261019090000-absent1", doc); !errors.Is(err, os.ErrNotExist) || name != "" || data != nil {
			t.Fatal(name, data, err)
		}
	})

	t.Run("hidden symlink rejected before filtering", func(t *testing.T) {
		rootPath := fixture(t)
		base := filepath.Join(rootPath, "pages", p2, "data", box)
		if err := os.Symlink(filepath.Join(rootPath, docPath), filepath.Join(base, ".linked")); err != nil {
			t.Fatal(err)
		}
		s, _ := open(t, rootPath)
		if files, err := s.Files(context.Background(), p2); !errors.Is(err, ErrInvalid) || len(files) != 0 {
			t.Fatal(files, err)
		}
		if name, data, err := s.Document(context.Background(), p2, box, doc); !errors.Is(err, ErrInvalid) || name != "" || data != nil {
			t.Fatal(name, data, err)
		}
	})
}
