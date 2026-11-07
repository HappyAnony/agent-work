// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestIndexRepairStagesCancelErrorPanicAndDrain(t *testing.T) {
	t.Run("sync-panic", func(t *testing.T) {
		fn := func() (e error) { defer recoverRepositoryWorkflow(&e, "full_sync"); panic("synthetic") }
		if e := fn(); e == nil || e.Error() != "full_sync_failed" {
			t.Fatal(e)
		}
	})
	for _, mode := range []string{"success", "cancel", "error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var order []int
			flushed, completed := 0, false
			var stages []func(context.Context) error
			for i := 1; i <= 5; i++ {
				i := i
				stages = append(stages, func(context.Context) error {
					order = append(order, i)
					if i == 2 {
						switch mode {
						case "cancel":
							cancel()
						case "error":
							return errors.New("io failed")
						case "panic":
							panic("failed")
						}
					}
					return nil
				})
			}
			e := runIndexRepairStages(ctx, stages, func() error { flushed++; return nil }, func() error { completed = true; return nil })
			if mode == "success" {
				if e != nil || !completed || !reflect.DeepEqual(order, []int{1, 2, 3, 4, 5}) {
					t.Fatal(order, e)
				}
			} else {
				if e == nil || completed || !reflect.DeepEqual(order, []int{1, 2}) {
					t.Fatal(order, e, completed)
				}
			}
			if flushed < len(order) {
				t.Fatal("accepted SQL queue was not drained")
			}
		})
	}
	t.Run("completion-error", func(t *testing.T) {
		completionErr := errors.New("completion owner changed")
		completed := false
		e := runIndexRepairStages(context.Background(), []func(context.Context) error{
			func(context.Context) error { return nil },
		}, func() error { return nil }, func() error {
			completed = true
			return completionErr
		})
		if !errors.Is(e, completionErr) || !completed {
			t.Fatal("completion failure was not propagated", e, completed)
		}
	})
}
func TestSyncRuntimeStateBelongsToOwner(t *testing.T) {
	a, b := &AppConf{}, &AppConf{}
	a.syncRuntime.same.Store(3)
	a.syncRuntime.errors.Store(2)
	a.syncRuntime.boot.Store(2)
	a.syncRuntime.files.Store("block", true)
	if b.syncRuntime.same.Load() != 0 || b.syncRuntime.errors.Load() != 0 || b.syncRuntime.boot.Load() != 0 {
		t.Fatal("shared counters")
	}
	if _, ok := b.syncRuntime.files.Load("block"); ok {
		t.Fatal("shared syncing IDs")
	}
	old := Conf
	Conf = b
	defer func() { Conf = old }()
	if BootSyncStatus() != -1 || ExitSyncStatus() != -1 {
		t.Fatal("wrong initial status")
	}
	Conf = a
	if BootSyncStatus() != 1 {
		t.Fatal("lost boot failure")
	}
}
func TestFullSyncCompatibilityGuardAndDisabledModes(t *testing.T) {
	c, _, p, _ := restoreFixture(t)
	c.Sync.Enabled = false
	c.Sync.Perception = false
	for _, mode := range []string{"normal", "upload", "download", "boot", "exit", "boot-followup"} {
		out, e := SyncDataContext(context.Background(), mode, true)
		if e != nil || !out.Started {
			t.Fatal(mode, out, e)
		}
	}
	c.persistenceReadOnly = true
	if _, e := SyncDataContext(context.Background(), "normal", true); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	detached := AppConf{AppData: c.AppData}
	if e := detached.executeLegacyFullSync(context.Background(), c.AppData, cloudrepo.FullSyncRequest{Mode: "normal"}, p); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal("detached write admitted", e)
	}
}
func TestFullSyncActualLocalProviderUploadDownloadAndMerge(t *testing.T) {
	c, _, p, _ := restoreFixture(t)
	c.Sync.Enabled = false
	c.Sync.Perception = false
	// Sync history indexing now consumes actual commit results, so this original
	// Local-provider fixture needs its real derived history database as well.
	previousHistory := util.HistoryDBPath
	util.HistoryDBPath = filepath.Join(t.TempDir(), "history.db")
	sql.InitHistoryDatabase(true)
	history := sql.CaptureLegacyHistoryQueue()
	t.Cleanup(func() { _ = history.DB.Close(); util.HistoryDBPath = previousHistory })
	c.System.ID = "r02-local-device"
	// Original membership checks are exercised by controller tests; invoke the
	// original repository algorithm directly with synthetic Local storage only.
	if e := os.Remove(filepath.Join(p.Data, "source.docj")); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(p.Data, "assets"), 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(p.Data, "assets", "sync-fixture.txt")
	if e := os.WriteFile(file, []byte("synthetic cloud version"), 0600); e != nil {
		t.Fatal(e)
	}
	c.Repo.SyncIndexTiming = 1 << 50
	if e := syncRepoUpload(context.Background()); e != nil {
		t.Fatal("original upload", e)
	}
	if e := os.WriteFile(file, []byte("changed local source"), 0600); e != nil {
		t.Fatal(e)
	}
	stamp := time.Now().Add(3 * time.Second)
	os.Chtimes(file, stamp, stamp)
	if e := syncRepoDownload(context.Background()); e != nil {
		t.Fatal("original download", e)
	}
	data, e := os.ReadFile(file)
	if e != nil || string(data) != "synthetic cloud version" {
		t.Fatal("download not applied", e)
	}
	if _, e := syncRepo(context.Background(), false, true); e != nil {
		t.Fatal("original merge", e)
	}
	if e := c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	var historyRows int
	if e := history.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&historyRows); e != nil || historyRows == 0 {
		t.Fatal("history not committed before sync returned", historyRows, e)
	}
	recordCloudBusinessEvidence(t, "full-sync-local", map[string]interface{}{"originalUpload": true, "originalDownload": true, "originalMerge": true, "sourceRestored": true, "actualHistoryRows": historyRows})
}
func TestFullSyncOwnedUIWaitCloseAndNoLatePublication(t *testing.T) {
	c, _, p, _ := restoreFixture(t)
	loaded := util.IsUILoaded.Load()
	util.IsUILoaded.Store(false)
	defer util.IsUILoaded.Store(loaded)
	c.scheduleIndexNotice(128, p)
	c.scheduleSyncUI(func(context.Context) { t.Error("published after close") })
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := c.CloseCloudRepo(timeout); e != nil {
		t.Fatal("UI wait did not cancel/join", e)
	}
}
