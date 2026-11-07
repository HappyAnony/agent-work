//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/88250/lute"
	"wiki-kernel/internal/historyindex"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func assertHistoryRows(t *testing.T, h *historyIndexContext, want int) {
	t.Helper()
	var got int
	if err := h.queue.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&got); err != nil || got != want {
		t.Fatal(got, want, err)
	}
}

func TestHistoryIndexClosesBeforeQueueAndNotifiesAfterCommit(t *testing.T) {
	_, name, _, _ := historyIndexFixture(t)
	ctx := context.Background()
	h, err := captureHistoryIndexContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var closed *os.Root
	closes, notices := 0, 0
	h.notify = func(context.Context, indexdb.HistoryProgress) error {
		notices++
		if closed == nil {
			t.Fatal("notified before root close")
		}
		if _, err := closed.Stat("."); err == nil {
			t.Fatal("read root open during notification")
		}
		assertHistoryRows(t, h, 2)
		return nil
	}
	err = h.indexWithIO(ctx, name, util.NewLute(), historyIndexIO{closeRoot: func(r *os.Root) error {
		closes++
		closed = r
		assertHistoryRows(t, h, 0)
		return r.Close()
	}})
	if err != nil || closes != 1 || notices == 0 {
		t.Fatal(err, closes, notices)
	}
}

func TestHistoryIndexScanAndCloseFailuresNeverEnqueue(t *testing.T) {
	for _, kind := range []string{"close", "plan-and-close", "missing-and-close", "mixed-missing-open", "mixed-missing-plan", "panic-and-close"} {
		t.Run(kind, func(t *testing.T) {
			_, name, _, _ := historyIndexFixture(t)
			h, err := captureHistoryIndexContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			primary, closing := errors.New("history scan"), errors.New("history root close")
			closes, notices := 0, 0
			h.notify = func(context.Context, indexdb.HistoryProgress) error { notices++; return nil }
			op := historyIndexIO{closeRoot: func(r *os.Root) error { closes++; return errors.Join(r.Close(), closing) }}
			switch kind {
			case "panic-and-close":
				op.plan = func(context.Context, indexrepair.Source, string, string, *time.Location, *lute.Lute) ([]indexdb.HistoryEntry, error) {
					panic("synthetic scan")
				}
			case "plan-and-close":
				op.plan = func(context.Context, indexrepair.Source, string, string, *time.Location, *lute.Lute) ([]indexdb.HistoryEntry, error) {
					return []indexdb.HistoryEntry{{Title: "incomplete"}}, primary
				}
			case "missing-and-close":
				name = "2026-10-29-220000-sync"
			case "mixed-missing-open":
				op.openRoot = func(string) (*os.Root, error) { return nil, errors.Join(os.ErrNotExist, primary) }
			case "mixed-missing-plan":
				op.plan = func(context.Context, indexrepair.Source, string, string, *time.Location, *lute.Lute) ([]indexdb.HistoryEntry, error) {
					return nil, errors.Join(os.ErrNotExist, primary)
				}
			}
			err = h.indexWithIO(context.Background(), name, util.NewLute(), op)
			if err == nil || notices != 0 {
				t.Fatal(err, notices)
			}
			if kind != "mixed-missing-open" && (!errors.Is(err, closing) || closes != 1) {
				t.Fatal(err, closes)
			}
			if (kind == "plan-and-close" || kind == "mixed-missing-open" || kind == "mixed-missing-plan") && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			if err = h.queue.Flush(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
			assertHistoryRows(t, h, 0)
		})
	}
}

func TestHistoryIndexPostCloseCancellationIdentityAndOwner(t *testing.T) {
	for _, kind := range []string{"cancel-plan", "cancel-close", "root", "directory", "symlink", "missing-appears", "history-owner", "workspace-owner", "conf-owner", "db-owner", "closed-owner"} {
		t.Run(kind, func(t *testing.T) {
			_, name, _, _ := historyIndexFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h, err := captureHistoryIndexContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			oldRoot, oldWorkspace, oldConf, oldQueue := util.HistoryDir, util.WorkspaceDir, Conf, h.queue
			defer func() {
				util.HistoryDir = oldRoot
				util.WorkspaceDir = oldWorkspace
				Conf = oldConf
				h.queue = oldQueue
				h.owner.repoMu.Lock()
				h.owner.repoClosed = false
				h.owner.repoMu.Unlock()
			}()
			if kind == "missing-appears" {
				name = "2026-10-29-220000-sync"
			}
			closes, notices := 0, 0
			h.notify = func(context.Context, indexdb.HistoryProgress) error { notices++; return nil }
			op := historyIndexIO{plan: func(ctx context.Context, s indexrepair.Source, n, u string, l *time.Location, e *lute.Lute) ([]indexdb.HistoryEntry, error) {
				out, err := historyindex.Plan(ctx, s, n, u, l, e)
				if kind == "cancel-plan" {
					cancel()
				}
				return out, err
			}, closeRoot: func(r *os.Root) error {
				closes++
				err := r.Close()
				switch kind {
				case "cancel-close":
					cancel()
				case "root":
					os.Rename(h.root, h.root+".old")
					t.Cleanup(func() { os.RemoveAll(h.root + ".old") })
					os.Mkdir(h.root, 0700)
				case "directory", "symlink":
					p := filepath.Join(h.root, name)
					os.Rename(p, p+".old")
					if kind == "directory" {
						os.Mkdir(p, 0700)
					} else {
						os.Symlink(p+".old", p)
					}
				case "missing-appears":
					os.Mkdir(filepath.Join(h.root, name), 0700)
				case "history-owner":
					util.HistoryDir = t.TempDir()
				case "workspace-owner":
					util.WorkspaceDir = t.TempDir()
				case "conf-owner":
					Conf = &AppConf{}
				case "db-owner":
					h.queue.DB = nil
				case "closed-owner":
					h.owner.repoMu.Lock()
					h.owner.repoClosed = true
					h.owner.repoMu.Unlock()
				}
				return err
			}}
			err = h.indexWithIO(ctx, name, util.NewLute(), op)
			if err == nil || closes != 1 || notices != 0 {
				t.Fatal(err, closes, notices)
			}
			if (kind == "cancel-close" || kind == "cancel-plan") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			h.queue = oldQueue
			if err = h.queue.Flush(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
			assertHistoryRows(t, h, 0)
		})
	}
}

func TestHistoryIndexInitialAbsenceAndInvalidRoot(t *testing.T) {
	for _, kind := range []string{"root-missing", "directory-missing", "root-symlink", "pre-cancel", "closed-owner"} {
		t.Run(kind, func(t *testing.T) {
			_, name, _, _ := historyIndexFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h, err := captureHistoryIndexContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "root-missing":
				os.RemoveAll(h.root)
			case "directory-missing":
				name = "2026-10-29-220000-sync"
			case "root-symlink":
				os.Rename(h.root, h.root+".old")
				t.Cleanup(func() { os.RemoveAll(h.root + ".old") })
				os.Symlink(h.root+".old", h.root)
			case "pre-cancel":
				cancel()
			case "closed-owner":
				h.owner.repoMu.Lock()
				h.owner.repoClosed = true
				h.owner.repoMu.Unlock()
				defer func() { h.owner.repoMu.Lock(); h.owner.repoClosed = false; h.owner.repoMu.Unlock() }()
			}
			opens, closes := 0, 0
			err = h.indexWithIO(ctx, name, util.NewLute(), historyIndexIO{openRoot: func(n string) (*os.Root, error) { opens++; return os.OpenRoot(n) }, closeRoot: func(r *os.Root) error { closes++; return r.Close() }})
			if kind == "root-missing" || kind == "directory-missing" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid source accepted")
			}
			if kind == "directory-missing" {
				if opens != 1 || closes != 1 {
					t.Fatal(opens, closes)
				}
			} else if opens != 0 || closes != 0 {
				t.Fatal(opens, closes)
			}
			assertHistoryRows(t, h, 0)
		})
	}
}

func TestHistoryIndexNotificationFailureRetryKeepsCommittedRows(t *testing.T) {
	_, name, _, _ := historyIndexFixture(t)
	h, err := captureHistoryIndexContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("history notification")
	h.notify = func(context.Context, indexdb.HistoryProgress) error { assertHistoryRows(t, h, 2); return sentinel }
	if err = h.index(context.Background(), name, util.NewLute()); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	assertHistoryRows(t, h, 2)
	h.notify = nil
	if err = h.index(context.Background(), name, util.NewLute()); err != nil {
		t.Fatal(err)
	}
	assertHistoryRows(t, h, 2)
}
