//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

func TestHistoryQueueCapturedIdentityFailureRetryAndIsolation(t *testing.T) {
	ctx := context.Background()
	a, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	old := historyDB
	defer func() { historyDB = old }()
	historyDB, _ = a.Database(indexdb.History)
	first := CaptureLegacyHistoryQueue()
	defer forgetLegacyHistoryQueue(first.DB)
	if e = first.Index(ctx, []indexdb.HistoryEntry{{ID: "same", Title: "A"}}); e != nil {
		t.Fatal(e)
	}
	historyDB, _ = b.Database(indexdb.History)
	second := CaptureLegacyHistoryQueue()
	defer forgetLegacyHistoryQueue(second.DB)
	if e = first.Flush(ctx, nil); e == nil {
		t.Fatal("stale queue flushed onto selected DB")
	}
	if e = second.Flush(ctx, nil); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = second.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	historyDB = first.DB
	if _, e = first.DB.Exec("ALTER TABLE histories_fts_case_insensitive RENAME TO history_fixture_saved"); e != nil {
		t.Fatal(e)
	}
	if e = first.Flush(ctx, nil); e == nil {
		t.Fatal("history SQL failure swallowed")
	}
	first.DB.Exec("ALTER TABLE history_fixture_saved RENAME TO histories_fts_case_insensitive")
	if e = first.Flush(ctx, nil); e != nil {
		t.Fatal(e)
	}
	if e = first.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive WHERE title='A'").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = first.Index(canceled, []indexdb.HistoryEntry{{ID: "cancel"}}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = closeLegacyHistoryQueue(ctx); e != nil {
		t.Fatal(e)
	}
	if e = CaptureLegacyHistoryQueue().Index(ctx, []indexdb.HistoryEntry{{ID: "closed"}}); !errors.Is(e, indexdb.ErrHistoryQueueClosed) {
		t.Fatal(e)
	}
}

func TestHistoryInitializationFailedDrainKeepsOldDBAndRetryDoesNotMigrateTail(t *testing.T) {
	ctx := context.Background()
	bundle, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	owned, _ := bundle.Database(indexdb.History)
	oldDB, oldPath := historyDB, util.HistoryDBPath
	historyDB = owned
	util.HistoryDBPath = filepath.Join(t.TempDir(), "new-history.db")
	q := CaptureLegacyHistoryQueue()
	t.Cleanup(func() {
		current := historyDB
		if current != nil && current != oldDB {
			_ = current.Close()
			forgetLegacyHistoryQueue(current)
		}
		forgetLegacyHistoryQueue(owned)
		historyDB = oldDB
		util.HistoryDBPath = oldPath
	})
	if e = q.Index(ctx, []indexdb.HistoryEntry{{ID: "old", Title: "old database only"}}); e != nil {
		t.Fatal(e)
	}
	owned.Exec("ALTER TABLE histories_fts_case_insensitive RENAME TO old_history_saved")
	if e = InitHistoryDatabaseContext(ctx, false); e == nil {
		t.Fatal("failed drain replaced DB")
	}
	if historyDB != owned || owned.Ping() != nil {
		t.Fatal("old DB closed after failed drain")
	}
	if _, e = os.Stat(util.HistoryDBPath); !os.IsNotExist(e) {
		t.Fatal("new DB created before successful drain", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = InitHistoryDatabaseContext(canceled, false); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	owned.Exec("ALTER TABLE old_history_saved RENAME TO histories_fts_case_insensitive")
	if e = InitHistoryDatabaseContext(ctx, false); e != nil {
		t.Fatal("retry initialization", e)
	}
	if historyDB == owned {
		t.Fatal("DB not rebound after retry")
	}
	if e = q.Flush(ctx, nil); e == nil {
		t.Fatal("old handle flushed after rebind")
	}
	var rows int
	if e = historyDB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&rows); e != nil || rows != 0 {
		t.Fatal("old batch migrated into new DB", rows, e)
	}
}
