// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rewriteFaultFile struct {
	*os.File
	write    func([]byte) (int, error)
	sync     func() error
	stat     func() (fs.FileInfo, error)
	closeErr error
	closes   int
}

func (f *rewriteFaultFile) Write(b []byte) (int, error) {
	if f.write != nil {
		return f.write(b)
	}
	return f.File.Write(b)
}
func (f *rewriteFaultFile) Sync() error {
	if f.sync != nil {
		return f.sync()
	}
	return f.File.Sync()
}
func (f *rewriteFaultFile) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat()
	}
	return f.File.Stat()
}
func (f *rewriteFaultFile) Close() error { f.closes++; return errors.Join(f.File.Close(), f.closeErr) }
func rewriteOriginal(t *testing.T) (Source, string) {
	t.Helper()
	s := sourceFixture(t)
	if err := s.Data.MkdirAll("box", 0755); err != nil {
		t.Fatal(err)
	}
	if err := s.Data.WriteFile("box/doc", []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, "box/doc"
}
func rewriteExpectBytes(t *testing.T, root *os.Root, name, want string) {
	t.Helper()
	raw, err := root.ReadFile(name)
	if err != nil || string(raw) != want {
		t.Fatal(name, string(raw), want, err)
	}
}
func rewriteTemps(t *testing.T, root *os.Root, folder string) []string {
	t.Helper()
	entries, err := fs.ReadDir(root.FS(), folder)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wiki-repair-") {
			names = append(names, entry.Name())
		}
	}
	return names
}
func TestSourceRewriteFailureStagesCloseAndPreserveTarget(t *testing.T) {
	for _, kind := range []string{"open", "stat", "write", "short", "sync", "close", "rename"} {
		t.Run(kind, func(t *testing.T) {
			s, name := rewriteOriginal(t)
			primary := errors.New("primary " + kind)
			var file *rewriteFaultFile
			removed, rootClosed := 0, 0
			op := sourceRewriteIO{
				open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
					if kind == "open" {
						return nil, primary
					}
					raw, err := root.OpenFile(tmp, flags, mode)
					if err != nil {
						return nil, err
					}
					file = &rewriteFaultFile{File: raw}
					switch kind {
					case "stat":
						file.stat = func() (fs.FileInfo, error) { return nil, primary }
					case "write":
						file.write = func([]byte) (int, error) { return 0, primary }
					case "short":
						file.write = func(b []byte) (int, error) { return len(b) - 1, nil }
					case "sync":
						file.sync = func() error { return primary }
					case "close":
						file.closeErr = primary
					}
					return file, nil
				},
				remove: func(root *os.Root, tmp string) error { removed++; return root.Remove(tmp) },
				rename: func(root *os.Root, from, to string) error {
					if kind == "rename" {
						return primary
					}
					return root.Rename(from, to)
				},
				closeRoot: func(root *os.Root) error { rootClosed++; return root.Close() },
			}
			err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), op)
			expected := primary
			if kind == "short" {
				expected = io.ErrShortWrite
			}
			if !errors.Is(err, expected) || rootClosed != 1 {
				t.Fatal(err, rootClosed)
			}
			if file != nil && file.closes != 1 {
				t.Fatal("temporary descriptor not closed once", file.closes)
			}
			rewriteExpectBytes(t, s.Data, name, "old")
			if kind == "stat" {
				if !errors.Is(err, ErrSourceChanged) || removed != 0 || len(rewriteTemps(t, s.Data, "box")) != 1 {
					t.Fatal("unidentified file was deleted", err, removed)
				}
			} else if len(rewriteTemps(t, s.Data, "box")) != 0 {
				t.Fatal("owned temporary file remains")
			}
		})
	}
}
func TestSourceRewriteWriteCloseCleanupAndParentCloseErrorsJoin(t *testing.T) {
	s, name := rewriteOriginal(t)
	writeErr, closeErr, removeErr, rootErr := errors.New("write"), errors.New("close"), errors.New("remove"), errors.New("root close")
	var file *rewriteFaultFile
	err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
		open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
			raw, err := root.OpenFile(tmp, flags, mode)
			if err != nil {
				return nil, err
			}
			file = &rewriteFaultFile{File: raw, closeErr: closeErr, write: func([]byte) (int, error) { return 0, writeErr }}
			return file, nil
		},
		remove:    func(*os.Root, string) error { return removeErr },
		closeRoot: func(root *os.Root) error { return errors.Join(root.Close(), rootErr) },
	})
	for _, want := range []error{writeErr, closeErr, removeErr, rootErr} {
		if !errors.Is(err, want) {
			t.Fatal("lost failure", want, err)
		}
	}
	if file.closes != 1 || len(rewriteTemps(t, s.Data, "box")) != 1 {
		t.Fatal(file.closes)
	}
	rewriteExpectBytes(t, s.Data, name, "old")
}
func TestSourceRewriteRejectsTemporaryReplacementWithoutDeletingNewcomer(t *testing.T) {
	for _, kind := range []string{"file", "link", "directory", "missing"} {
		t.Run(kind, func(t *testing.T) {
			s, name := rewriteOriginal(t)
			outside := filepath.Join(t.TempDir(), "keep")
			os.WriteFile(outside, []byte("outside"), 0600)
			var temp string
			removed := 0
			err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
				open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
					temp = tmp
					raw, err := root.OpenFile(tmp, flags, mode)
					if err != nil {
						return nil, err
					}
					return &rewriteFaultFile{File: raw, sync: func() error {
						if err := raw.Sync(); err != nil {
							return err
						}
						if err := root.Rename(tmp, "owned-original"); err != nil {
							return err
						}
						switch kind {
						case "file":
							return root.WriteFile(tmp, []byte("newcomer"), 0600)
						case "link":
							return root.Symlink(outside, tmp)
						case "directory":
							return root.Mkdir(tmp, 0700)
						}
						return nil
					}}, nil
				},
				remove: func(root *os.Root, tmp string) error { removed++; return root.Remove(tmp) },
			})
			if err == nil || removed != 0 {
				t.Fatal(err, removed)
			}
			if kind != "missing" && !errors.Is(err, ErrSourceChanged) {
				t.Fatal(err)
			}
			rewriteExpectBytes(t, s.Data, name, "old")
			if kind == "file" {
				rewriteExpectBytes(t, s.Data, "box/"+temp, "newcomer")
			}
			if kind == "link" {
				info, err := s.Data.Lstat("box/" + temp)
				if err != nil || info.Mode()&fs.ModeSymlink == 0 {
					t.Fatal(info, err)
				}
			}
			if kind == "directory" {
				info, err := s.Data.Lstat("box/" + temp)
				if err != nil || !info.IsDir() {
					t.Fatal(info, err)
				}
			}
			if raw, err := os.ReadFile(outside); err != nil || string(raw) != "outside" {
				t.Fatal(err)
			}
		})
	}
}
func TestSourceRewriteParentReplacementCleansAnchoredOwnedTemporaryOnly(t *testing.T) {
	s, name := rewriteOriginal(t)
	var temp string
	err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
		open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
			temp = tmp
			raw, err := root.OpenFile(tmp, flags, mode)
			if err != nil {
				return nil, err
			}
			return &rewriteFaultFile{File: raw, sync: func() error {
				if err := raw.Sync(); err != nil {
					return err
				}
				if err := s.Data.Rename("box", "moved"); err != nil {
					return err
				}
				if err := s.Data.Mkdir("box", 0755); err != nil {
					return err
				}
				if err := s.Data.WriteFile("box/doc", []byte("foreign target"), 0600); err != nil {
					return err
				}
				return s.Data.WriteFile("box/"+tmp, []byte("foreign temporary"), 0600)
			}}, nil
		},
	})
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, name, "foreign target")
	rewriteExpectBytes(t, s.Data, "box/"+temp, "foreign temporary")
	rewriteExpectBytes(t, s.Data, "moved/doc", "old")
	if len(rewriteTemps(t, s.Data, "moved")) != 0 {
		t.Fatal("anchored owned temporary remains")
	}
}
func TestSourceRewriteDetectsLateTargetRevisionAndCreation(t *testing.T) {
	for _, create := range []bool{false, true} {
		t.Run(map[bool]string{false: "revision", true: "creation"}[create], func(t *testing.T) {
			s, name := rewriteOriginal(t)
			before := []byte("old")
			if create {
				before = nil
				if err := s.Data.Remove(name); err != nil {
					t.Fatal(err)
				}
			}
			err := s.rewrite(context.Background(), name, before, []byte("new"), sourceRewriteIO{
				open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
					raw, err := root.OpenFile(tmp, flags, mode)
					if err != nil {
						return nil, err
					}
					return &rewriteFaultFile{File: raw, sync: func() error {
						if err := raw.Sync(); err != nil {
							return err
						}
						return root.WriteFile("doc", []byte("external revision"), 0600)
					}}, nil
				},
			})
			if !errors.Is(err, ErrSourceChanged) {
				t.Fatal(err)
			}
			rewriteExpectBytes(t, s.Data, name, "external revision")
			if len(rewriteTemps(t, s.Data, "box")) != 0 {
				t.Fatal("owned temporary remains")
			}
		})
	}
}
func TestSourceRewriteCancellationAndRevokedAuthority(t *testing.T) {
	for _, kind := range []string{"before", "lock", "write", "revoked"} {
		t.Run(kind, func(t *testing.T) {
			s, name := rewriteOriginal(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			revoke := false
			revoked := errors.New("owner revoked")
			s.Authorize = func(ctx context.Context) error {
				if revoke {
					return revoked
				}
				return ctx.Err()
			}
			opens, unlocks := 0, 0
			s.Lock = func(string) func() {
				if kind == "lock" {
					cancel()
				}
				return func() { unlocks++ }
			}
			if kind == "before" {
				cancel()
			}
			err := s.rewrite(ctx, name, []byte("old"), []byte("new"), sourceRewriteIO{
				open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
					opens++
					raw, err := root.OpenFile(tmp, flags, mode)
					if err != nil {
						return nil, err
					}
					return &rewriteFaultFile{File: raw, sync: func() error {
						if kind == "write" {
							cancel()
						}
						if kind == "revoked" {
							revoke = true
						}
						return raw.Sync()
					}}, nil
				},
			})
			if kind == "write" {
				if err != nil || opens != 1 {
					t.Fatal(err, opens)
				}
				rewriteExpectBytes(t, s.Data, name, "new")
			} else {
				want := error(context.Canceled)
				if kind == "revoked" {
					want = revoked
				}
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
				rewriteExpectBytes(t, s.Data, name, "old")
				if kind != "revoked" && opens != 0 {
					t.Fatal(opens)
				}
			}
			if kind != "before" && unlocks != 1 {
				t.Fatal("lock not released", unlocks)
			}
			if len(rewriteTemps(t, s.Data, "box")) != 0 {
				t.Fatal("temporary remains")
			}
		})
	}
}
func TestSourceRewriteSuccessSkipsRemovalOfRecreatedTempAndJoinsRootClose(t *testing.T) {
	s, name := rewriteOriginal(t)
	closeErr := errors.New("parent close")
	var temp string
	removes := 0
	err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
		rename: func(root *os.Root, from, to string) error {
			temp = from
			if err := root.Rename(from, to); err != nil {
				return err
			}
			return root.WriteFile(from, []byte("new writer's file"), 0600)
		},
		remove:    func(root *os.Root, name string) error { removes++; return root.Remove(name) },
		closeRoot: func(root *os.Root) error { return errors.Join(root.Close(), closeErr) },
	})
	if !errors.Is(err, closeErr) || removes != 0 {
		t.Fatal(err, removes)
	}
	rewriteExpectBytes(t, s.Data, name, "new")
	rewriteExpectBytes(t, s.Data, "box/"+temp, "new writer's file")
}
func TestSourceRewriteExclusiveCreateDoesNotCleanForeignCollision(t *testing.T) {
	s, name := rewriteOriginal(t)
	var temp string
	removes := 0
	err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
		open: func(root *os.Root, tmp string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
			temp = tmp
			if err := root.WriteFile(tmp, []byte("foreign collision"), 0600); err != nil {
				return nil, err
			}
			return root.OpenFile(tmp, flags, mode)
		},
		remove: func(root *os.Root, name string) error { removes++; return root.Remove(name) },
	})
	if !errors.Is(err, fs.ErrExist) || removes != 0 {
		t.Fatal(err, removes)
	}
	rewriteExpectBytes(t, s.Data, name, "old")
	rewriteExpectBytes(t, s.Data, "box/"+temp, "foreign collision")
}
func TestSourceRewriteMissingCleanupDoesNotMaskPrimaryFailure(t *testing.T) {
	s, name := rewriteOriginal(t)
	primary := errors.New("rename failed")
	err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
		rename: func(root *os.Root, from, to string) error {
			if err := root.Remove(from); err != nil {
				return err
			}
			return primary
		},
	})
	if !errors.Is(err, primary) || errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, name, "old")
}

func TestSourceRewriteRenameCleanupErrorsAndConcurrentMissing(t *testing.T) {
	for _, kind := range []string{"denied", "missing"} {
		t.Run(kind, func(t *testing.T) {
			s, name := rewriteOriginal(t)
			renameErr, removeErr := errors.New("rename"), errors.New("cleanup")
			err := s.rewrite(context.Background(), name, []byte("old"), []byte("new"), sourceRewriteIO{
				rename: func(*os.Root, string, string) error { return renameErr },
				remove: func(root *os.Root, tmp string) error {
					if kind == "denied" {
						return removeErr
					}
					if err := root.Remove(tmp); err != nil {
						return err
					}
					return fs.ErrNotExist
				},
			})
			if !errors.Is(err, renameErr) {
				t.Fatal(err)
			}
			if kind == "denied" {
				if !errors.Is(err, removeErr) {
					t.Fatal(err)
				}
			} else if errors.Is(err, fs.ErrNotExist) {
				t.Fatal("normal missing cleanup reported as failure", err)
			}
			rewriteExpectBytes(t, s.Data, name, "old")
		})
	}
}
