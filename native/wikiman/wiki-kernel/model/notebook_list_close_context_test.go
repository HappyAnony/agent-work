//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/util"
)

const notebookCloseID = "20261101093000-book001"

func notebookListFixture(t *testing.T) (*AppConf, string) {
	t.Helper()
	historyIndexFixture(t)
	c := Conf
	c.FileTree = conf.NewFileTree()
	name := filepath.Join(util.DataDir, notebookCloseID, ".siyuan/conf.json")
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(`{"name":"Notebook","closed":false,"sort":3}`), 0600); err != nil {
		t.Fatal(err)
	}
	return c, name
}

func listedNotebook(items []*Box) *Box {
	for _, v := range items {
		if v.ID == notebookCloseID {
			return v
		}
	}
	return nil
}

func TestNotebookListCheckedCloseAndOriginalPublicProjection(t *testing.T) {
	_, name := notebookListFixture(t)
	before, _ := os.ReadFile(name)
	var closed *os.Root
	calls := 0
	out, err := listNotebooksContextWithIO(context.Background(), notebookListIO{closeRoot: func(r *os.Root) error { calls++; closed = r; return r.Close() }})
	item := listedNotebook(out)
	if err != nil || item == nil || item.Name != "Notebook" || item.Closed || item.Sort != 3 || calls != 1 {
		t.Fatal(out, err, calls)
	}
	if _, err = closed.Stat("."); err == nil {
		t.Fatal("descriptor not closed")
	}
	if out, err = ListNotebooksContext(context.Background()); err != nil || listedNotebook(out) == nil {
		t.Fatal(out, err)
	}
	after, _ := os.ReadFile(name)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}

func TestNotebookListFailureAndCloseErrorsDiscardList(t *testing.T) {
	for _, kind := range []string{"close", "list-and-close", "panic-and-close"} {
		t.Run(kind, func(t *testing.T) {
			notebookListFixture(t)
			primary, closing := errors.New("list failed"), errors.New("root close")
			closes := 0
			op := notebookListIO{closeRoot: func(r *os.Root) error { closes++; return errors.Join(r.Close(), closing) }}
			if kind != "close" {
				op.list = func(c *AppConf, ctx context.Context, r *os.Root, g func(context.Context) error, a func(context.Context, string) error) ([]*Box, error) {
					if kind == "panic-and-close" {
						panic("synthetic notebook list")
					}
					return []*Box{{ID: notebookCloseID}}, primary
				}
			}
			out, err := listNotebooksContextWithIO(context.Background(), op)
			if out != nil || !errors.Is(err, closing) || closes != 1 {
				t.Fatal(out, err, closes)
			}
			if kind == "list-and-close" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
		})
	}
}

func TestNotebookListCancellationAndOwnerChanges(t *testing.T) {
	for _, kind := range []string{"pre-cancel", "cancel-list", "cancel-close", "conf", "data", "workspace", "closed", "root", "link"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := notebookListFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			oldConf, data, workspace := Conf, util.DataDir, util.WorkspaceDir
			defer func() {
				Conf = oldConf
				util.DataDir = data
				util.WorkspaceDir = workspace
				c.repoMu.Lock()
				c.repoClosed = false
				c.repoMu.Unlock()
			}()
			if kind == "pre-cancel" {
				cancel()
			}
			opens, closes := 0, 0
			op := notebookListIO{openRoot: func(n string) (*os.Root, error) { opens++; return os.OpenRoot(n) }, closeRoot: func(r *os.Root) error {
				closes++
				err := r.Close()
				switch kind {
				case "cancel-close":
					cancel()
				case "conf":
					Conf = &AppConf{}
				case "data":
					util.DataDir = t.TempDir()
				case "workspace":
					util.WorkspaceDir = t.TempDir()
				case "closed":
					c.repoMu.Lock()
					c.repoClosed = true
					c.repoMu.Unlock()
				case "root", "link":
					os.Rename(data, data+".old")
					t.Cleanup(func() { os.RemoveAll(data + ".old") })
					if kind == "root" {
						os.Mkdir(data, 0700)
					} else {
						os.Symlink(data+".old", data)
					}
				}
				return err
			}}
			if kind == "cancel-list" {
				op.list = func(c *AppConf, ctx context.Context, r *os.Root, g func(context.Context) error, a func(context.Context, string) error) ([]*Box, error) {
					v, err := c.listNotebooksAt(ctx, r, g, a)
					cancel()
					return v, err
				}
			}
			out, err := listNotebooksContextWithIO(ctx, op)
			if out != nil || err == nil || opens != closes {
				t.Fatal(out, err, opens, closes)
			}
			if kind == "pre-cancel" && opens != 0 {
				t.Fatal(opens)
			}
			if (kind == "pre-cancel" || kind == "cancel-list" || kind == "cancel-close") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestNotebookListReadOnlyAndRepairSideEffectsSurviveCloseFailure(t *testing.T) {
	for _, readonly := range []bool{true, false} {
		t.Run(map[bool]string{true: "readonly", false: "repair"}[readonly], func(t *testing.T) {
			c, name := notebookListFixture(t)
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			c.m.Lock()
			c.persistenceReadOnly = readonly
			c.m.Unlock()
			defer func() { c.m.Lock(); c.persistenceReadOnly = false; c.m.Unlock() }()
			// Exercise the real settings/repair gates and unindex callback. A bounded
			// context makes an accidental recursive settings acquisition observable.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			closing := errors.New("after repair close")
			out, err := listNotebooksContextWithIO(ctx, notebookListIO{closeRoot: func(r *os.Root) error { return errors.Join(r.Close(), closing) }})
			if out != nil || !errors.Is(err, closing) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(out, err)
			}
			_, statErr := os.Stat(name)
			if readonly {
				if !os.IsNotExist(statErr) {
					t.Fatal("readonly repaired", statErr)
				}
			} else if statErr != nil {
				t.Fatal("completed repair was rolled back", statErr)
			}
			out, err = ListNotebooksContext(ctx)
			if err != nil || listedNotebook(out) == nil || !listedNotebook(out).Closed {
				t.Fatal(out, err)
			}
		})
	}
}

func TestNotebookListGuardAfterScanAndWhileWaitingGates(t *testing.T) {
	c, name := notebookListFixture(t)
	root, err := os.OpenRoot(util.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = os.Remove(name); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("stale after scan")
	calls, after := 0, 0
	out, err := c.listNotebooksAt(context.Background(), root, func(context.Context) error {
		calls++
		if calls == 2 {
			return sentinel
		}
		return nil
	}, func(context.Context, string) error { after++; return nil })
	if out != nil || !errors.Is(err, sentinel) || after != 0 {
		t.Fatal(out, err, after)
	}
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("stale scan repaired")
	}
	for _, gate := range []string{"settings", "repair"} {
		t.Run(gate, func(t *testing.T) {
			if gate == "settings" {
				c.m.Lock()
				g := c.settingsGate
				c.m.Unlock()
				g <- struct{}{}
				defer func() { <-g }()
			} else {
				notebookRepairGate <- struct{}{}
				defer func() { <-notebookRepairGate }()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
			defer cancel()
			closes := 0
			out, err := listNotebooksContextWithIO(ctx, notebookListIO{closeRoot: func(r *os.Root) error { closes++; return r.Close() }})
			if out != nil || !errors.Is(err, context.DeadlineExceeded) || closes != 1 {
				t.Fatal(out, err, closes)
			}
		})
	}
}

func TestNotebookListInvalidOwnerRootAndOpenFailures(t *testing.T) {
	for _, kind := range []string{"nil-owner", "closed-owner", "symlink", "wrong-descriptor", "open"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := notebookListFixture(t)
			data := util.DataDir
			old := Conf
			defer func() { Conf = old; c.repoMu.Lock(); c.repoClosed = false; c.repoMu.Unlock() }()
			switch kind {
			case "nil-owner":
				Conf = nil
			case "closed-owner":
				c.repoMu.Lock()
				c.repoClosed = true
				c.repoMu.Unlock()
			case "symlink":
				if err := os.Rename(data, data+".old"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(data + ".old") })
				if err := os.Symlink(data+".old", data); err != nil {
					t.Fatal(err)
				}
			}
			opens, closes := 0, 0
			sentinel := errors.New("open failed")
			op := notebookListIO{openRoot: func(n string) (*os.Root, error) {
				opens++
				if kind == "open" {
					return nil, sentinel
				}
				if kind == "wrong-descriptor" {
					n = t.TempDir()
				}
				return os.OpenRoot(n)
			}, closeRoot: func(r *os.Root) error { closes++; return r.Close() }}
			out, err := listNotebooksContextWithIO(context.Background(), op)
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
			if kind == "open" {
				if !errors.Is(err, sentinel) || opens != 1 || closes != 0 {
					t.Fatal(err, opens, closes)
				}
			} else if kind == "wrong-descriptor" {
				if opens != 1 || closes != 1 {
					t.Fatal(opens, closes)
				}
			} else if opens != 0 || closes != 0 {
				t.Fatal(opens, closes)
			}
		})
	}
}
