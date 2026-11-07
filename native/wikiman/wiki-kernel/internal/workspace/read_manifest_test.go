//go:build fts5

package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceReadSourceRejectsIncompleteFileAndFinalManifest(t *testing.T) {
	ctx := context.Background()
	injected := errors.New("injected workspace read failure")

	for _, tc := range []struct {
		name string
		op   func(string) workspaceReadIO
		want error
	}{
		{
			name: "partial directory read",
			op: func(string) workspaceReadIO {
				return workspaceReadIO{directory: workspaceDirectoryIO{readDir: func(_ string, dir *os.File, count int) ([]fs.DirEntry, error) {
					part, err := dir.ReadDir(count)
					if err != nil {
						return part, err
					}
					return part, injected
				}}}
			},
			want: injected,
		},
		{
			name: "partial file read",
			op: func(string) workspaceReadIO {
				return workspaceReadIO{readFile: func(_ string, file *os.File, buffer []byte) (int, error) {
					n, err := file.Read(buffer)
					if err != nil {
						return n, err
					}
					return n, injected
				}}
			},
			want: injected,
		},
		{
			name: "file close failure",
			op: func(string) workspaceReadIO {
				return workspaceReadIO{closeFile: func(_ string, file *os.File) error {
					return errors.Join(file.Close(), injected)
				}}
			},
			want: injected,
		},
		{
			name: "candidate replaced before open",
			op: func(root string) workspaceReadIO {
				changed := false
				return workspaceReadIO{openFile: func(w *WorkspaceContext, name string) (*os.File, error) {
					if !changed {
						file := filepath.Join(root, "pages", pageA, "data", filepath.FromSlash(name))
						data, err := os.ReadFile(file)
						if err != nil {
							return nil, err
						}
						info, err := os.Stat(file)
						if err != nil {
							return nil, err
						}
						replacement := file + ".replacement"
						if err = os.WriteFile(replacement, data, info.Mode().Perm()); err != nil {
							return nil, err
						}
						if err = os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
							return nil, err
						}
						if err = os.Rename(replacement, file); err != nil {
							return nil, err
						}
						changed = true
					}
					return openNativeFile(w, name)
				}}
			},
			want: errWorkspaceDirectorySourceChanged,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			m := open(t, root, Options{Configuration: originalProjectionConfiguration})
			l, err := m.Acquire(ctx, storeID, pageA)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Release()
			data, sourcePath, err := readSourceWithIO(ctx, l.workspace, boxID, docID, tc.op(root))
			if !errors.Is(err, tc.want) || data != nil || sourcePath != "" {
				t.Fatal(len(data), sourcePath, err)
			}
		})
	}

	t.Run("final manifest changed", func(t *testing.T) {
		root := fixture(t)
		m := open(t, root, Options{Configuration: originalProjectionConfiguration})
		l, err := m.Acquire(ctx, storeID, pageA)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Release()
		file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
		opens, changed := 0, false
		op := workspaceReadIO{directory: workspaceDirectoryIO{openDir: func(dataRoot *os.Root, name string) (*os.File, error) {
			if name == boxID {
				opens++
				if opens == 3 && !changed {
					candidate, openErr := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
					if openErr != nil {
						return nil, openErr
					}
					_, writeErr := candidate.WriteString(" ")
					closeErr := candidate.Close()
					if writeErr != nil || closeErr != nil {
						return nil, errors.Join(writeErr, closeErr)
					}
					changed = true
				}
			}
			return dataRoot.Open(name)
		}}}
		data, sourcePath, err := readSourceWithIO(ctx, l.workspace, boxID, docID, op)
		if !changed || !errors.Is(err, errWorkspaceDirectorySourceChanged) || data != nil || sourcePath != "" {
			t.Fatal(changed, len(data), sourcePath, err)
		}
	})
}

func TestWorkspaceReadSourcePreservesHiddenAndDuplicateRules(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	base := filepath.Join(root, "pages", pageA, "data", boxID)
	original, err := os.ReadFile(filepath.Join(base, docID+".docj"))
	if err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(base, ".hidden")
	if err = os.Mkdir(hidden, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(hidden, docID+".docj"), original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Run("hidden duplicate ignored", func(t *testing.T) {
		data, sourcePath, readErr := readSourceWithIO(ctx, l.workspace, boxID, docID, workspaceReadIO{})
		if readErr != nil || data == nil || sourcePath != "/"+docID+".docj" {
			t.Fatal(len(data), sourcePath, readErr)
		}
	})
	t.Run("visible duplicate rejected", func(t *testing.T) {
		visible := filepath.Join(base, "visible")
		if err = os.Mkdir(visible, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(visible, docID+".docj"), original, 0600); err != nil {
			t.Fatal(err)
		}
		data, sourcePath, readErr := readSourceWithIO(ctx, l.workspace, boxID, docID, workspaceReadIO{})
		if !errors.Is(readErr, ErrDocument) || data != nil || sourcePath != "" {
			t.Fatal(len(data), sourcePath, readErr)
		}
	})
}
