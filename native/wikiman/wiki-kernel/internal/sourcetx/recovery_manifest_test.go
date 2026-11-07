package sourcetx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestJournalIDsRejectUnstableEnumerationWithoutPartialResults(t *testing.T) {
	t.Run("incomplete confirmation", func(t *testing.T) {
		rootPath := fixture(t)
		_, _ = open(t, rootPath)
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		calls := 0
		incomplete := errors.New("incomplete journal enumeration")
		op := readManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			entries, err := fs.ReadDir(root.FS(), name)
			if name == ".local/journal" {
				calls++
				if calls == 2 {
					return entries, incomplete
				}
			}
			return entries, err
		}}
		ids, err := journalIDsWithIO(root, op)
		if !errors.Is(err, incomplete) || ids != nil {
			t.Fatal(ids, err)
		}
	})

	t.Run("same metadata replacement", func(t *testing.T) {
		rootPath := fixture(t)
		_, _ = open(t, rootPath)
		name := ".local/journal/replaced.json"
		if err := os.WriteFile(filepath.Join(rootPath, filepath.FromSlash(name)), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		calls := 0
		op := readManifestIO{lstat: func(root *os.Root, candidate string) (fs.FileInfo, error) {
			if candidate == name {
				calls++
				if calls == 3 {
					replaceReadManifestTarget(t, rootPath, candidate)
				}
			}
			return root.Lstat(candidate)
		}}
		ids, err := journalIDsWithIO(root, op)
		if !errors.Is(err, ErrRecovery) || !errors.Is(err, errReadSourceChanged) || ids != nil {
			t.Fatal(ids, err)
		}
	})
}

func TestJournalIDsPreservePairingOrderingTemporaryAndTypeRules(t *testing.T) {
	openRoot := func(t *testing.T, rootPath string) *os.Root {
		t.Helper()
		_, _ = open(t, rootPath)
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = root.Close() })
		return root
	}

	t.Run("ordered ids paired commit and temporary ignored", func(t *testing.T) {
		rootPath := fixture(t)
		root := openRoot(t, rootPath)
		for name := range map[string]bool{"journal-b.json": true, "journal-a.json": true, "journal-a.commit": true, ".wiki-tmp-owned": true} {
			if err := os.WriteFile(filepath.Join(rootPath, ".local/journal", name), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		ids, err := journalIDs(root)
		if err != nil || !reflect.DeepEqual(ids, []string{"journal-a", "journal-b"}) {
			t.Fatal(ids, err)
		}
	})

	t.Run("orphan commit rejected", func(t *testing.T) {
		rootPath := fixture(t)
		root := openRoot(t, rootPath)
		if err := os.WriteFile(filepath.Join(rootPath, ".local/journal/orphan.commit"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		if ids, err := journalIDs(root); !errors.Is(err, ErrRecovery) || ids != nil {
			t.Fatal(ids, err)
		}
	})

	t.Run("directory rejected without descent", func(t *testing.T) {
		rootPath := fixture(t)
		root := openRoot(t, rootPath)
		dir := filepath.Join(rootPath, ".local/journal/bad.json")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "must-not-be-read"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if ids, err := journalIDs(root); !errors.Is(err, ErrRecovery) || ids != nil {
			t.Fatal(ids, err)
		}
	})

	t.Run("symlink rejected", func(t *testing.T) {
		rootPath := fixture(t)
		root := openRoot(t, rootPath)
		if err := os.Symlink(filepath.Join(rootPath, "store.json"), filepath.Join(rootPath, ".local/journal/linked.json")); err != nil {
			t.Fatal(err)
		}
		if ids, err := journalIDs(root); !errors.Is(err, ErrRecovery) || ids != nil {
			t.Fatal(ids, err)
		}
	})
}

func TestJournalIDsMissingDirectoryStillFailsClosed(t *testing.T) {
	rootPath := fixture(t)
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ids, err := journalIDsWithIO(root, readManifestIO{})
	if !errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrRecovery) || ids != nil {
		t.Fatal(ids, err)
	}
}
