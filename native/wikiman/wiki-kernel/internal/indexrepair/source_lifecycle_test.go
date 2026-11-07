// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
)

type lifecycleFaultFile struct {
	*os.File
	readErr, dirErr, syncErr, closeErr error
	onDir, onSync                      func() error
	closes                             int
}

func (f *lifecycleFaultFile) Read(b []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.File.Read(b)
}
func (f *lifecycleFaultFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.onDir != nil {
		if err := f.onDir(); err != nil {
			return nil, err
		}
	}
	if f.dirErr != nil {
		return nil, f.dirErr
	}
	return f.File.ReadDir(n)
}
func (f *lifecycleFaultFile) Sync() error {
	err := f.File.Sync()
	if err == nil && f.onSync != nil {
		err = f.onSync()
	}
	return errors.Join(err, f.syncErr)
}
func (f *lifecycleFaultFile) Close() error {
	f.closes++
	return errors.Join(f.File.Close(), f.closeErr)
}
func historyFixture(t *testing.T) Source {
	t.Helper()
	s := sourceFixture(t)
	if err := s.Data.MkdirAll("box/.siyuan/history/sub", 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"box/.siyuan/history/first": "first", "box/.siyuan/history/sub/second": "second", "box/keep": "keep"} {
		if err := s.Data.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return s
}
func archiveFixture(t *testing.T) Source {
	t.Helper()
	s := sourceFixture(t)
	s.Data.MkdirAll("box", 0700)
	if err := s.Data.WriteFile("box/bad.docj", []byte("original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSourceHistorySuccessMissingAndInvalidTargets(t *testing.T) {
	s := historyFixture(t)
	if err := s.RemoveLegacyHistory(context.Background(), "box"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Data.Lstat("box/.siyuan/history"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, "box/keep", "keep")
	if err := s.RemoveLegacyHistory(context.Background(), "box"); err != nil {
		t.Fatal(err)
	}
	for _, box := range []string{"../box", ".", "box/x", "box\\x"} {
		if err := s.RemoveLegacyHistory(context.Background(), box); !errors.Is(err, ErrSourcePath) {
			t.Fatal(box, err)
		}
	}
	if err := s.Data.WriteFile("box/.siyuan/history", []byte("unknown file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveLegacyHistory(context.Background(), "box"); !errors.Is(err, ErrSourcePath) {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, "box/.siyuan/history", "unknown file")
}
func TestSourceHistoryPreflightLinksReadAndCloseFailuresPreserveTree(t *testing.T) {
	for _, kind := range []string{"link", "read", "close"} {
		t.Run(kind, func(t *testing.T) {
			s := historyFixture(t)
			primary := errors.New("history " + kind)
			var opened *lifecycleFaultFile
			if kind == "link" {
				s.Data.Symlink(t.TempDir(), "box/.siyuan/history/sub/linked")
			}
			err := s.removeLegacyHistory(context.Background(), "box", sourceLifecycleIO{open: func(root *os.Root, name string) (sourceLifecycleFile, error) {
				raw, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				opened = &lifecycleFaultFile{File: raw}
				if kind == "read" {
					opened.dirErr = primary
				}
				if kind == "close" {
					opened.closeErr = primary
				}
				return opened, nil
			}})
			if err == nil || kind != "link" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			if opened != nil && opened.closes != 1 {
				t.Fatal(opened.closes)
			}
			rewriteExpectBytes(t, s.Data, "box/.siyuan/history/first", "first")
			rewriteExpectBytes(t, s.Data, "box/.siyuan/history/sub/second", "second")
		})
	}
}
func TestSourceHistoryLateReplacementsAndNewEntriesAreRetained(t *testing.T) {
	for _, kind := range []string{"file", "new-entry", "history", "parent", "nested-link"} {
		t.Run(kind, func(t *testing.T) {
			s := historyFixture(t)
			calls, removes := 0, 0
			err := s.removeLegacyHistory(context.Background(), "box", sourceLifecycleIO{
				open: func(root *os.Root, name string) (sourceLifecycleFile, error) {
					raw, err := root.Open(name)
					if err != nil {
						return nil, err
					}
					wrapped := &lifecycleFaultFile{File: raw}
					if strings.HasSuffix(root.Name(), "/history") {
						calls++
						if calls == 2 {
							wrapped.onDir = func() error {
								switch kind {
								case "file":
									if err := s.Data.Rename("box/.siyuan/history/first", "box/saved-first"); err != nil {
										return err
									}
									return root.WriteFile("first", []byte("replacement"), 0600)
								case "new-entry":
									return root.WriteFile("new", []byte("newcomer"), 0600)
								case "history":
									if err := s.Data.Rename("box/.siyuan/history", "box/.siyuan/moved"); err != nil {
										return err
									}
									return s.Data.Mkdir("box/.siyuan/history", 0700)
								case "parent":
									if err := s.Data.Rename("box/.siyuan", "box/moved"); err != nil {
										return err
									}
									return s.Data.Mkdir("box/.siyuan", 0700)
								case "nested-link":
									if err := s.Data.Rename("box/.siyuan/history/sub", "box/saved-sub"); err != nil {
										return err
									}
									return root.Symlink(t.TempDir(), "sub")
								}
								return nil
							}
						}
					}
					return wrapped, nil
				}, remove: func(root *os.Root, name string) error { removes++; return root.Remove(name) },
			})
			if err == nil || removes != 0 {
				t.Fatal(err, removes)
			}
			if kind == "file" {
				rewriteExpectBytes(t, s.Data, "box/.siyuan/history/first", "replacement")
			}
			if kind == "new-entry" {
				rewriteExpectBytes(t, s.Data, "box/.siyuan/history/new", "newcomer")
			}
			rewriteExpectBytes(t, s.Data, "box/keep", "keep")
		})
	}
}
func TestSourceHistoryStopsOnCancellationRevocationAndRemovalError(t *testing.T) {
	for _, kind := range []string{"before", "lock", "during", "revoked", "remove"} {
		t.Run(kind, func(t *testing.T) {
			s := historyFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			primary := errors.New("history denied")
			revoked := false
			removes := 0
			s.Authorize = func(c context.Context) error {
				if revoked {
					return primary
				}
				return c.Err()
			}
			if kind == "before" {
				cancel()
			}
			s.Lock = func(string) func() {
				if kind == "lock" {
					cancel()
				}
				return func() {}
			}
			err := s.removeLegacyHistory(ctx, "box", sourceLifecycleIO{remove: func(root *os.Root, name string) error {
				removes++
				if kind == "remove" {
					return primary
				}
				if err := root.Remove(name); err != nil {
					return err
				}
				if kind == "during" {
					cancel()
				}
				if kind == "revoked" {
					revoked = true
				}
				return nil
			}})
			if err == nil {
				t.Fatal("expected rejection")
			}
			if kind == "revoked" || kind == "remove" {
				if !errors.Is(err, primary) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if (kind == "before" || kind == "lock") && removes != 0 {
				t.Fatal(removes)
			}
			if (kind == "during" || kind == "revoked") && removes != 1 {
				t.Fatal("deletion continued", removes)
			}
			rewriteExpectBytes(t, s.Data, "box/keep", "keep")
		})
	}
}
func TestSourceHistoryDoesNotRecursivelyRemoveUnscannedLateFile(t *testing.T) {
	s := sourceFixture(t)
	s.Data.MkdirAll("box/.siyuan/history", 0700)
	s.Data.WriteFile("box/.siyuan/history/first", []byte("old"), 0600)
	err := s.removeLegacyHistory(context.Background(), "box", sourceLifecycleIO{remove: func(root *os.Root, name string) error {
		err := root.Remove(name)
		if err == nil && name == "first" {
			err = root.WriteFile("new", []byte("newcomer"), 0600)
		}
		return err
	}})
	if err == nil {
		t.Fatal("unscanned child unexpectedly removed")
	}
	rewriteExpectBytes(t, s.Data, "box/.siyuan/history/new", "newcomer")
}
func TestSourceHistoryRemovalAndRootCloseErrorsJoin(t *testing.T) {
	s := historyFixture(t)
	removeErr, closeErr := errors.New("remove"), errors.New("root close")
	closes := 0
	err := s.removeLegacyHistory(context.Background(), "box", sourceLifecycleIO{
		remove: func(*os.Root, string) error { return removeErr },
		closeRoot: func(root *os.Root) error {
			closes++
			err := root.Close()
			if strings.HasSuffix(root.Name(), "/history") || strings.HasSuffix(root.Name(), "/.siyuan") {
				return errors.Join(err, closeErr)
			}
			return err
		},
	})
	if !errors.Is(err, removeErr) || !errors.Is(err, closeErr) || closes < 2 {
		t.Fatal(err, closes)
	}
}
func TestSourceArchiveSuccessUniqueAndCriticalCancellation(t *testing.T) {
	for _, kind := range []string{"normal", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s := archiveFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			op := sourceLifecycleIO{}
			if kind == "cancel" {
				op.rewrite.closeRoot = func(root *os.Root) error { err := root.Close(); cancel(); return err }
			}
			dest, err := s.archive(ctx, "box/bad.docj", "same-second", op)
			if err != nil || !strings.HasPrefix(dest, "corrupted/same-second-") {
				t.Fatal(dest, err)
			}
			rewriteExpectBytes(t, s.ArchiveRoot, dest, "original bytes")
			if _, err := s.Data.Lstat("box/bad.docj"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			s.Data.WriteFile("box/bad.docj", []byte("original bytes"), 0600)
			next, err := s.Archive(context.Background(), "box/bad.docj", "same-second")
			if err != nil || next == dest {
				t.Fatal(next, err)
			}
		})
	}
}
func TestSourceArchiveSourceReadAndCloseFailuresJoinBeforePublication(t *testing.T) {
	s := archiveFixture(t)
	readErr, closeErr := errors.New("source read"), errors.New("source close")
	var input *lifecycleFaultFile
	dest, err := s.archive(context.Background(), "box/bad.docj", "stamp", sourceLifecycleIO{open: func(root *os.Root, name string) (sourceLifecycleFile, error) {
		raw, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		input = &lifecycleFaultFile{File: raw, readErr: readErr, closeErr: closeErr}
		return input, nil
	}})
	if dest != "" || !errors.Is(err, readErr) || !errors.Is(err, closeErr) || input.closes != 1 {
		t.Fatal(dest, err)
	}
	rewriteExpectBytes(t, s.Data, "box/bad.docj", "original bytes")
}
func TestSourceArchivePublicationAndDirectorySyncErrorsKeepSource(t *testing.T) {
	for _, kind := range []string{"publication", "sync-close", "verify-close"} {
		t.Run(kind, func(t *testing.T) {
			s := archiveFixture(t)
			primary, closing := errors.New("archive failure"), errors.New("archive close")
			op := sourceLifecycleIO{}
			if kind == "publication" {
				op.rewrite.open = func(*os.Root, string, int, fs.FileMode) (sourceRewriteFile, error) { return nil, primary }
			} else {
				sourceReads := 0
				op.open = func(root *os.Root, name string) (sourceLifecycleFile, error) {
					raw, err := root.Open(name)
					if err != nil {
						return nil, err
					}
					f := &lifecycleFaultFile{File: raw}
					info, err := raw.Stat()
					if err != nil {
						raw.Close()
						return nil, err
					}
					if kind == "sync-close" && info.IsDir() {
						f.syncErr = primary
						f.closeErr = closing
					}
					if kind == "verify-close" && strings.HasPrefix(root.Name(), s.Data.Name()) && name == "bad.docj" {
						sourceReads++
						if sourceReads == 2 {
							f.closeErr = closing
						}
					}
					return f, nil
				}
			}
			dest, err := s.archive(context.Background(), "box/bad.docj", "stamp", op)
			if dest == "" || err == nil {
				t.Fatal(dest, err)
			}
			if kind == "publication" || kind == "sync-close" {
				if !errors.Is(err, primary) {
					t.Fatal(err)
				}
			}
			if kind != "publication" {
				if !errors.Is(err, closing) {
					t.Fatal(err)
				}
				rewriteExpectBytes(t, s.ArchiveRoot, dest, "original bytes")
			}
			rewriteExpectBytes(t, s.Data, "box/bad.docj", "original bytes")
		})
	}
}
func TestSourceArchiveObservedSourceAndBackupReplacementsPreventRemoval(t *testing.T) {
	for _, kind := range []string{"source-inode", "source-bytes", "source-parent", "backup-inode", "backup-bytes", "backup-parent"} {
		t.Run(kind, func(t *testing.T) {
			s := archiveFixture(t)
			changed := false
			op := sourceLifecycleIO{open: func(root *os.Root, name string) (sourceLifecycleFile, error) {
				raw, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				f := &lifecycleFaultFile{File: raw}
				info, _ := raw.Stat()
				if info != nil && info.IsDir() && !changed {
					f.onSync = func() error {
						if changed {
							return nil
						}
						changed = true
						switch kind {
						case "source-inode":
							if err := s.Data.Rename("box/bad.docj", "box/original.docj"); err != nil {
								return err
							}
							return s.Data.WriteFile("box/bad.docj", []byte("original bytes"), 0600)
						case "source-bytes":
							return s.Data.WriteFile("box/bad.docj", []byte("new source bytes"), 0600)
						case "source-parent":
							if err := s.Data.Rename("box", "original-box"); err != nil {
								return err
							}
							s.Data.Mkdir("box", 0700)
							return s.Data.WriteFile("box/bad.docj", []byte("foreign source"), 0600)
						case "backup-inode":
							if err := root.Rename(path.Join(name, "bad.docj"), path.Join(name, "retained.docj")); err != nil {
								return err
							}
							return root.WriteFile(path.Join(name, "bad.docj"), []byte("original bytes"), 0600)
						case "backup-bytes":
							return root.WriteFile(path.Join(name, "bad.docj"), []byte("bad backup"), 0600)
						case "backup-parent":
							if err := root.Rename(name, name+"-original"); err != nil {
								return err
							}
							return root.Mkdir(name, 0700)
						}
						return nil
					}
				}
				return f, nil
			}}
			dest, err := s.archive(context.Background(), "box/bad.docj", "stamp", op)
			if dest == "" || err == nil {
				t.Fatal(dest, err)
			}
			want := "original bytes"
			if kind == "source-bytes" {
				want = "new source bytes"
			}
			if kind == "source-parent" {
				want = "foreign source"
			}
			rewriteExpectBytes(t, s.Data, "box/bad.docj", want)
		})
	}
}
func TestSourceArchiveRevokedAuthorityAndRemoveErrorPreserveSource(t *testing.T) {
	for _, kind := range []string{"revoked", "remove"} {
		t.Run(kind, func(t *testing.T) {
			s := archiveFixture(t)
			primary := errors.New("archive denied")
			revoked := false
			s.Authorize = func(ctx context.Context) error {
				if revoked {
					return primary
				}
				return ctx.Err()
			}
			op := sourceLifecycleIO{}
			if kind == "revoked" {
				op.rewrite.closeRoot = func(root *os.Root) error { err := root.Close(); revoked = true; return err }
			} else {
				op.remove = func(*os.Root, string) error { return primary }
			}
			dest, err := s.archive(context.Background(), "box/bad.docj", "stamp", op)
			if dest == "" || !errors.Is(err, primary) {
				t.Fatal(dest, err)
			}
			rewriteExpectBytes(t, s.Data, "box/bad.docj", "original bytes")
			rewriteExpectBytes(t, s.ArchiveRoot, dest, "original bytes")
		})
	}
}
func TestSourceArchiveLateRootCloseReportsPublishedState(t *testing.T) {
	s := archiveFixture(t)
	closing := errors.New("borrowed root close")
	closes := 0
	dest, err := s.archive(context.Background(), "box/bad.docj", "stamp", sourceLifecycleIO{closeRoot: func(root *os.Root) error { closes++; return errors.Join(root.Close(), closing) }})
	if dest == "" || !errors.Is(err, closing) || closes != 2 {
		t.Fatal(dest, err, closes)
	}
	rewriteExpectBytes(t, s.ArchiveRoot, dest, "original bytes")
	if _, err := s.Data.Lstat("box/bad.docj"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
func TestSourceArchiveInvalidLinkedAndCanceledInputsDoNotPublish(t *testing.T) {
	for _, kind := range []string{"stamp", "path", "link", "cancel", "lock", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			s := archiveFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			name, stamp := "box/bad.docj", "stamp"
			switch kind {
			case "stamp":
				stamp = "unsafe\\path"
			case "path":
				name = "../bad.docj"
			case "link":
				s.Data.Rename(name, "box/keep.docj")
				s.Data.Symlink("keep.docj", name)
			case "cancel":
				cancel()
			case "lock":
				s.Lock = func(string) func() { cancel(); return func() {} }
			case "readonly":
				s.Authorize = nil
			}
			dest, err := s.Archive(ctx, name, stamp)
			if dest != "" || err == nil {
				t.Fatal(dest, err)
			}
			entries, _ := fs.ReadDir(s.ArchiveRoot.FS(), ".")
			if len(entries) != 0 {
				t.Fatal(entries)
			}
		})
	}
}
