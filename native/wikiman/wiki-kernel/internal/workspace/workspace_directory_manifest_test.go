//go:build fts5

package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestWorkspaceDirectoryManifestRejectsPartialReadCloseAndMutation(t *testing.T) {
	ctx := context.Background()
	manifestErr := errors.New("injected manifest failure")

	t.Run("partial read", func(t *testing.T) {
		root := fixture(t)
		m := open(t, root, Options{Configuration: originalProjectionConfiguration})
		l, err := m.Acquire(ctx, storeID, pageA)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Release()
		closed := false
		op := workspaceDirectoryIO{
			readDir: func(_ string, dir *os.File, count int) ([]fs.DirEntry, error) {
				part, err := dir.ReadDir(count)
				if err != nil {
					return part, err
				}
				return part, manifestErr
			},
			closeDir: func(_ string, dir *os.File) error {
				closed = true
				return dir.Close()
			},
		}
		got, err := workspaceDirectorySourceManifest(ctx, l.workspace, boxID, op)
		if !errors.Is(err, manifestErr) || got.name != "" || !closed {
			t.Fatal(got, closed, err)
		}
	})

	t.Run("close failure", func(t *testing.T) {
		root := fixture(t)
		m := open(t, root, Options{Configuration: originalProjectionConfiguration})
		l, err := m.Acquire(ctx, storeID, pageA)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Release()
		op := workspaceDirectoryIO{closeDir: func(_ string, dir *os.File) error {
			return errors.Join(dir.Close(), manifestErr)
		}}
		got, err := workspaceDirectorySourceManifest(ctx, l.workspace, boxID, op)
		if !errors.Is(err, manifestErr) || got.name != "" {
			t.Fatal(got, err)
		}
	})

	t.Run("changed between confirmations", func(t *testing.T) {
		root := fixture(t)
		m := open(t, root, Options{Configuration: originalProjectionConfiguration})
		l, err := m.Acquire(ctx, storeID, pageA)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Release()
		file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
		opens, changed := 0, false
		op := workspaceDirectoryIO{
			openDir: func(root *os.Root, name string) (*os.File, error) {
				if name == boxID {
					opens++
				}
				return root.Open(name)
			},
			readDir: func(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
				part, err := dir.ReadDir(count)
				if name == boxID && opens == 2 && len(part) > 0 && !changed {
					f, openErr := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
					if openErr != nil {
						return part, openErr
					}
					_, writeErr := f.WriteString(" ")
					closeErr := f.Close()
					if writeErr != nil || closeErr != nil {
						return part, errors.Join(writeErr, closeErr)
					}
					changed = true
				}
				return part, err
			},
		}
		got, err := confirmedWorkspaceDirectoryManifest(ctx, l.workspace, boxID, op)
		if !changed || !errors.Is(err, errWorkspaceDirectorySourceChanged) || got.name != "" {
			t.Fatal(got, changed, err)
		}
	})

	t.Run("symlink directory", func(t *testing.T) {
		root := fixture(t)
		m := open(t, root, Options{Configuration: originalProjectionConfiguration})
		l, err := m.Acquire(ctx, storeID, pageA)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Release()
		base := filepath.Join(root, "pages", pageA, "data", boxID)
		if err = os.Symlink(t.TempDir(), filepath.Join(base, "linked")); err != nil {
			t.Fatal(err)
		}
		if _, err = confirmedWorkspaceDirectoryManifest(ctx, l.workspace, boxID+"/linked", workspaceDirectoryIO{}); !errors.Is(err, ErrDocument) {
			t.Fatal(err)
		}
	})
}

func TestNativeBlockTreeFinalManifestChangePreservesPublishedIndex(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if err = l.RebuildNativeBlockTree(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := l.NativeBlockTreeCount(ctx, indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil || before == 0 {
		t.Fatal(before, err)
	}
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	opens, changed := 0, false
	op := workspaceDirectoryIO{
		openDir: func(root *os.Root, name string) (*os.File, error) {
			if name == boxID {
				opens++
			}
			return root.Open(name)
		},
		readDir: func(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
			part, readErr := dir.ReadDir(count)
			if name == boxID && opens == 3 && len(part) > 0 && !changed {
				f, openErr := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
				if openErr != nil {
					return part, openErr
				}
				_, writeErr := f.WriteString(" ")
				closeErr := f.Close()
				if writeErr != nil || closeErr != nil {
					return part, errors.Join(writeErr, closeErr)
				}
				changed = true
			}
			return part, readErr
		},
	}
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		return l.workspace.resources.rebuildBlockTreeWithIO(ctx, l.workspace, op)
	})
	if !changed || !errors.Is(err, errWorkspaceDirectorySourceChanged) {
		t.Fatal(changed, err)
	}
	after, err := l.NativeBlockTreeCount(ctx, indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil || after != before {
		t.Fatal(before, after, err)
	}
}
