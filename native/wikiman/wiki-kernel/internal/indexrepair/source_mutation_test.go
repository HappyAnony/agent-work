// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

func mutationFixture(t *testing.T) Source {
	t.Helper()
	s := sourceFixture(t)
	s.Data.MkdirAll("a/empty", 0700)
	s.Data.MkdirAll("b", 0700)
	if err := s.Data.WriteFile("a/doc", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSourceMutationOriginalFileAndDirectoryOperations(t *testing.T) {
	s := mutationFixture(t)
	ctx := context.Background()
	if err := s.Rename(ctx, "a/doc", "a/renamed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(ctx, "a/renamed", "b/doc"); err != nil {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, "b/doc", "original")
	info, _ := s.Data.Stat("b/doc")
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	s.Data.WriteFile("a/empty/child", []byte("retained"), 0600)
	if err := s.Rename(ctx, "a/empty", "b/tree"); err != nil {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, "b/tree/child", "retained")
	if err := s.Remove(ctx, "b/tree"); err == nil {
		t.Fatal("nonempty directory removed")
	}
	if err := s.Remove(ctx, "b/tree/child"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, "b/tree"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, "b/doc"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, "b/doc"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
func TestSourceMutationLocksAreOrderedAndHeldThroughClose(t *testing.T) {
	s := mutationFixture(t)
	var events []string
	s.Lock = func(name string) func() {
		events = append(events, "lock "+name)
		return func() { events = append(events, "unlock "+name) }
	}
	err := s.rename(context.Background(), "a/doc", "b/doc", sourceMutationIO{lifecycle: sourceLifecycleIO{closeRoot: func(root *os.Root) error { events = append(events, "close"); return root.Close() }}})
	if err != nil || strings.Join(events, ",") != "lock a/doc,lock b/doc,close,close,unlock b/doc,unlock a/doc" {
		t.Fatal(events, err)
	}
	events = nil
	if err = s.Rename(context.Background(), "b/doc", "a/doc"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "lock a/doc,lock b/doc,unlock b/doc,unlock a/doc" {
		t.Fatal(events)
	}
	events = nil
	if err = s.remove(context.Background(), "a/doc", sourceMutationIO{lifecycle: sourceLifecycleIO{closeRoot: func(root *os.Root) error { events = append(events, "close"); return root.Close() }}}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "lock a/doc,close,unlock a/doc" {
		t.Fatal(events)
	}
}
func TestSourceMutationCancellationAndAuthorityRevocationRejectBeforeAction(t *testing.T) {
	for _, action := range []string{"remove", "rename"} {
		for _, kind := range []string{"before", "lock", "final", "readonly"} {
			t.Run(action+"/"+kind, func(t *testing.T) {
				s := mutationFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls, mutations := 0, 0
				denied := errors.New("revoked")
				s.Authorize = func(c context.Context) error {
					calls++
					if calls == 3 {
						if kind == "final" {
							cancel()
						}
						if kind == "readonly" {
							return denied
						}
					}
					return c.Err()
				}
				s.Lock = func(string) func() {
					if kind == "lock" {
						cancel()
					}
					return func() {}
				}
				if kind == "before" {
					cancel()
				}
				op := sourceMutationIO{lifecycle: sourceLifecycleIO{remove: func(root *os.Root, name string) error { mutations++; return root.Remove(name) }}, rename: func(root *os.Root, from, to string) error { mutations++; return root.Rename(from, to) }}
				var err error
				if action == "remove" {
					err = s.remove(ctx, "a/doc", op)
				} else {
					err = s.rename(ctx, "a/doc", "b/doc", op)
				}
				expected := error(context.Canceled)
				if kind == "readonly" {
					expected = denied
				}
				if !errors.Is(err, expected) || mutations != 0 {
					t.Fatal(err, mutations)
				}
				rewriteExpectBytes(t, s.Data, "a/doc", "original")
			})
		}
	}
}
func TestSourceMutationWaitingLockRejectsReplacedSourceOrParent(t *testing.T) {
	for _, action := range []string{"remove", "rename"} {
		for _, kind := range []string{"file", "parent", "target-parent"} {
			t.Run(action+"/"+kind, func(t *testing.T) {
				s := mutationFixture(t)
				changed := false
				s.Lock = func(string) func() {
					if !changed {
						changed = true
						switch kind {
						case "file":
							s.Data.Rename("a/doc", "a/original")
							s.Data.WriteFile("a/doc", []byte("replacement"), 0600)
						case "parent":
							s.Data.Rename("a", "old-a")
							s.Data.Mkdir("a", 0700)
							s.Data.Rename("old-a/doc", "a/doc")
						case "target-parent":
							s.Data.Rename("b", "old-b")
							s.Data.Mkdir("b", 0700)
						}
					}
					return func() {}
				}
				if action == "remove" && kind == "target-parent" {
					return
				}
				var err error
				if action == "remove" {
					err = s.Remove(context.Background(), "a/doc")
				} else {
					err = s.Rename(context.Background(), "a/doc", "b/doc")
				}
				if !errors.Is(err, ErrSourceChanged) {
					t.Fatal(err)
				}
				want := "original"
				if kind == "file" {
					want = "replacement"
				}
				rewriteExpectBytes(t, s.Data, "a/doc", want)
			})
		}
	}
}
func TestSourceMutationFinalGuardRejectsReplacementUpdateAndSymlink(t *testing.T) {
	for _, action := range []string{"remove", "rename"} {
		for _, kind := range []string{"file", "bytes", "link", "parent"} {
			t.Run(action+"/"+kind, func(t *testing.T) {
				s := mutationFixture(t)
				calls := 0
				s.Authorize = func(ctx context.Context) error {
					calls++
					if calls == 3 {
						switch kind {
						case "file":
							s.Data.Rename("a/doc", "a/original")
							s.Data.WriteFile("a/doc", []byte("replacement"), 0600)
						case "bytes":
							s.Data.WriteFile("a/doc", []byte("new bytes"), 0600)
						case "link":
							s.Data.Rename("a/doc", "a/original")
							s.Data.Symlink("original", "a/doc")
						case "parent":
							s.Data.Rename("a", "old-a")
							s.Data.Mkdir("a", 0700)
							s.Data.WriteFile("a/doc", []byte("foreign"), 0600)
						}
					}
					return ctx.Err()
				}
				var err error
				if action == "remove" {
					err = s.Remove(context.Background(), "a/doc")
				} else {
					err = s.Rename(context.Background(), "a/doc", "b/doc")
				}
				if err == nil {
					t.Fatal("changed source mutated")
				}
				want := "replacement"
				if kind == "bytes" {
					want = "new bytes"
				}
				if kind == "link" {
					want = "original"
				}
				if kind == "parent" {
					want = "foreign"
				}
				rewriteExpectBytes(t, s.Data, "a/doc", want)
			})
		}
	}
}
func TestSourceMutationRenameTargetOccupancyAndLinksNeverOverwriteObservedEntry(t *testing.T) {
	for _, kind := range []string{"existing", "late", "target-link", "parent-link", "same", "missing-parent"} {
		t.Run(kind, func(t *testing.T) {
			s := mutationFixture(t)
			to := "b/doc"
			calls := 0
			switch kind {
			case "existing":
				s.Data.WriteFile(to, []byte("target"), 0600)
			case "target-link":
				s.Data.WriteFile("b/keep", []byte("target"), 0600)
				s.Data.Symlink("keep", to)
			case "parent-link":
				s.Data.Remove("b")
				s.Data.Symlink("a", "b")
			case "same":
				to = "a/doc"
			case "missing-parent":
				to = "missing/doc"
			}
			s.Authorize = func(ctx context.Context) error {
				calls++
				if kind == "late" && calls == 3 {
					s.Data.WriteFile(to, []byte("target"), 0600)
				}
				return ctx.Err()
			}
			if err := s.Rename(context.Background(), "a/doc", to); err == nil {
				t.Fatal("invalid target accepted")
			}
			rewriteExpectBytes(t, s.Data, "a/doc", "original")
			if kind == "existing" || kind == "late" || kind == "target-link" {
				rewriteExpectBytes(t, s.Data, "b/doc", "target")
			}
		})
	}
}
func TestSourceMutationOperationAndParentCloseErrorsJoin(t *testing.T) {
	for _, action := range []string{"remove", "rename"} {
		t.Run(action, func(t *testing.T) {
			s := mutationFixture(t)
			primary, closing := errors.New("operation"), errors.New("root close")
			closes := 0
			op := sourceMutationIO{lifecycle: sourceLifecycleIO{remove: func(*os.Root, string) error { return primary }, closeRoot: func(root *os.Root) error { closes++; return errors.Join(root.Close(), closing) }}, rename: func(*os.Root, string, string) error { return primary }}
			var err error
			if action == "remove" {
				err = s.remove(context.Background(), "a/doc", op)
			} else {
				err = s.rename(context.Background(), "a/doc", "b/doc", op)
			}
			expected := 1
			if action == "rename" {
				expected = 2
			}
			if !errors.Is(err, primary) || !errors.Is(err, closing) || closes != expected {
				t.Fatal(err, closes)
			}
			rewriteExpectBytes(t, s.Data, "a/doc", "original")
		})
	}
}
func TestSourceMutationCloseFailureAfterCommittedActionDoesNotRollback(t *testing.T) {
	for _, action := range []string{"remove", "rename"} {
		t.Run(action, func(t *testing.T) {
			s := mutationFixture(t)
			closing := errors.New("close after action")
			op := sourceMutationIO{lifecycle: sourceLifecycleIO{closeRoot: func(root *os.Root) error { return errors.Join(root.Close(), closing) }}}
			var err error
			if action == "remove" {
				err = s.remove(context.Background(), "a/doc", op)
			} else {
				err = s.rename(context.Background(), "a/doc", "b/doc", op)
			}
			if !errors.Is(err, closing) {
				t.Fatal(err)
			}
			if _, err := s.Data.Lstat("a/doc"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			if action == "rename" {
				rewriteExpectBytes(t, s.Data, "b/doc", "original")
			}
		})
	}
}
func TestSourceMutationCommittedRenamePreservesRecreatedSourceName(t *testing.T) {
	s := mutationFixture(t)
	err := s.rename(context.Background(), "a/doc", "a/moved", sourceMutationIO{rename: func(root *os.Root, from, to string) error {
		if err := root.Rename(from, to); err != nil {
			return err
		}
		return root.WriteFile(from, []byte("new writer"), 0600)
	}})
	if err != nil {
		t.Fatal(err)
	}
	rewriteExpectBytes(t, s.Data, "a/doc", "new writer")
	rewriteExpectBytes(t, s.Data, "a/moved", "original")
}
func TestSourceMutationRejectsEscapesReadonlyAndMissingSources(t *testing.T) {
	for _, action := range []string{"remove", "rename"} {
		for _, name := range []string{"../doc", "a\\doc", "/doc", "missing"} {
			t.Run(action+name, func(t *testing.T) {
				s := mutationFixture(t)
				var err error
				if action == "remove" {
					err = s.Remove(context.Background(), name)
				} else {
					err = s.Rename(context.Background(), name, "b/doc")
				}
				if err == nil {
					t.Fatal(name)
				}
				rewriteExpectBytes(t, s.Data, "a/doc", "original")
			})
		}
	}
	s := mutationFixture(t)
	s.Authorize = nil
	if err := s.Remove(context.Background(), "a/doc"); !errors.Is(err, ErrSourceWrite) {
		t.Fatal(err)
	}
	if err := s.Rename(context.Background(), "a/doc", "b/doc"); !errors.Is(err, ErrSourceWrite) {
		t.Fatal(err)
	}
}
