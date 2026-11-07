//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"testing"
)

func TestHistoryDirectoryReplaceIsAtomicRetryableAndKeepsSiblings(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(History)
	var q HistoryQueue
	q.Index([]HistoryEntry{{ID: "old", Path: "stamp/doc"}, {ID: "sibling", Path: "stamp-extra/doc"}, {ID: "other", Path: "other/doc"}})
	if e = q.Flush(ctx, db, nil); e != nil {
		t.Fatal(e)
	}
	q.ReplaceDirectory("stamp", []HistoryEntry{{ID: "new", Path: "stamp/new"}})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected observer panic")
			}
		}()
		_ = q.Flush(ctx, db, func(HistoryProgress) { panic("fixture after insertion") })
	}()
	var old int
	db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive WHERE id='old'").Scan(&old)
	if old != 1 {
		t.Fatal("delete escaped rollback", old)
	}
	q.ReplaceDirectory("stamp", []HistoryEntry{{ID: "new", Path: "stamp/new"}})
	if e = q.Flush(ctx, db, nil); e != nil {
		t.Fatal(e)
	}
	var total, newCount int
	db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive WHERE id='new'").Scan(&newCount)
	if total != 3 || newCount != 1 {
		t.Fatal(total, newCount)
	}
	if e = q.ReplaceDirectory("stamp", nil); e != nil {
		t.Fatal(e)
	}
	if e = q.Flush(ctx, db, nil); e != nil {
		t.Fatal(e)
	}
	db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&total)
	if total != 2 {
		t.Fatal("empty directory replacement removed siblings", total)
	}
	if e = q.ReplaceDirectory("../", nil); e == nil {
		t.Fatal("invalid prefix accepted")
	}
}
