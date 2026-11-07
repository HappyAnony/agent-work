//go:build fts5

package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
)

func TestNativeDocumentLifecycleFinalEnumerationChangeClearsPlan(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	l := getDocLease(t, m, pageA)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID, infoChild+".docj")
	childrenRoot := boxID + "/" + docID
	opens, changed := 0, false
	op := workspaceDirectoryIO{
		openDir: func(root *os.Root, name string) (*os.File, error) {
			if name == childrenRoot {
				opens++
			}
			return root.Open(name)
		},
		readDir: func(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
			part, readErr := dir.ReadDir(count)
			if name == childrenRoot && opens == 3 && len(part) > 0 && !changed {
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
	plan, err := l.planNativeDocumentLifecycleWithIO(ctx, doclife.Request{Action: "rename", Box: boxID, Path: "/" + docID + ".docj", Title: "Changed"}, op)
	if !changed || !errors.Is(err, errWorkspaceDirectorySourceChanged) || plan.Tree != nil || plan.Changes != nil || plan.SourceHashes != nil {
		t.Fatal(changed, plan, err)
	}
}

func TestNativeFlashcardEnumerationRejectsIncompleteAndChangedSources(t *testing.T) {
	ctx := context.Background()
	injected := errors.New("injected flashcard enumeration failure")

	for _, tc := range []struct {
		name string
		op   workspaceDirectoryIO
	}{
		{name: "partial deck read", op: workspaceDirectoryIO{readDir: func(_ string, dir *os.File, count int) ([]fs.DirEntry, error) {
			part, err := dir.ReadDir(count)
			if err != nil {
				return part, err
			}
			return part, injected
		}}},
		{name: "deck close failure", op: workspaceDirectoryIO{closeDir: func(_ string, dir *os.File) error {
			return errors.Join(dir.Close(), injected)
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := flashSource(t)
			m := open(t, root, flashOptions(root))
			l := getDocLease(t, m, pageA)
			err := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
				s, err := l.workspace.resources.flashcardStoreWithIO(ctx, l.workspace, tc.op)
				if err != nil {
					return err
				}
				_, err = s.Decks(ctx)
				return err
			})
			if !errors.Is(err, injected) {
				t.Fatal(err)
			}
		})
	}

	t.Run("notebook changed between confirmations", func(t *testing.T) {
		root := flashSource(t)
		m := open(t, root, flashOptions(root))
		l := getDocLease(t, m, pageA)
		if err := l.RebuildNativeBlockTree(ctx); err != nil {
			t.Fatal(err)
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
				return part, readErr
			},
		}
		var ids []string
		err := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
			var err error
			ids, err = l.workspace.resources.flashcardNotebookIDsWithIO(ctx, l.workspace, boxID, op)
			return err
		})
		if !changed || !errors.Is(err, errWorkspaceDirectorySourceChanged) || ids != nil {
			t.Fatal(changed, ids, err)
		}
	})
}
