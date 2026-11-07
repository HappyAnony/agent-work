//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func referenceSource(t *testing.T) string {
	t.Helper()
	root := getDocSource(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	p := d["Children"].([]interface{})[0].(map[string]interface{})
	p["Children"] = append(p["Children"].([]interface{}), map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "block-ref", "TextMarkBlockRefID": getDocDefinition, "TextMarkBlockRefSubtype": "d", "TextMarkTextContent": "reference fixture"})
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return root
}
func countFor(rows []indexrepair.ReferenceCount, id string) *indexrepair.ReferenceCount {
	for i := range rows {
		if rows[i].BlockID == id {
			return &rows[i]
		}
	}
	return nil
}
func TestNativeReferenceRefreshActualIsolatedSourcesAndCommittedCounts(t *testing.T) {
	ctx := context.Background()
	root := referenceSource(t)
	before := digest(t, root)
	m := open(t, root, getDocOptions())
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	for _, l := range []*Lease{a, b} {
		rows, e := l.RefreshNativeReferenceIndex(ctx, getDocDefinition)
		if e != nil {
			t.Fatal(e)
		}
		count := countFor(rows, getDocDefinition)
		want := 0
		if l == a {
			want = 1
		}
		if count == nil || count.Count != want || count.RootID != getDocOther {
			t.Fatal("same ID selected wrong source/count", count, e)
		}
		if l.workspace.resources.contentQueue.Pending() != 0 {
			t.Fatal("returned before derived flush")
		}
	}
	if digest(t, root) != before {
		t.Fatal("reference refresh changed source")
	}
	recordCloudBusinessEvidence(t, "reference-native.json", map[string]interface{}{"owners": 2, "sourceUnchanged": true, "sameIDDifferentCounts": true, "derivedQueueDrained": true, "countsAfterCommit": true})
}
func TestNativeReferenceRefreshFailureRetainsQueueAndRetry(t *testing.T) {
	ctx := context.Background()
	root := referenceSource(t)
	m := open(t, root, getDocOptions())
	l := getDocLease(t, m, pageA)
	queueGetDoc(t, l)
	if e := l.FlushNativeMainQueue(ctx); e != nil {
		t.Fatal(e)
	}
	if e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		_, e := db.Exec("CREATE TRIGGER fail_refs BEFORE INSERT ON refs BEGIN SELECT RAISE(ABORT,'reference fixture failure'); END")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	rows, e := l.RefreshNativeReferenceIndex(ctx, getDocDefinition)
	if e == nil || rows != nil || l.workspace.resources.contentQueue.Pending() != 1 {
		t.Fatal("failed write published counts/discarded work", rows, e)
	}
	if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		_, e := db.Exec("DROP TRIGGER fail_refs")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	rows, e = l.RefreshNativeReferenceIndex(ctx, getDocDefinition)
	if e != nil || countFor(rows, getDocDefinition) == nil || countFor(rows, getDocDefinition).Count != 1 || l.workspace.resources.contentQueue.Pending() != 0 {
		t.Fatal(rows, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if rows, e = l.RefreshNativeReferenceIndex(canceled, getDocDefinition); !errors.Is(e, context.Canceled) || rows != nil {
		t.Fatal(rows, e)
	}
	l.Release()
	if _, e = l.RefreshNativeReferenceIndex(ctx, getDocDefinition); e == nil {
		t.Fatal("released owner admitted")
	}
	recordCloudBusinessEvidence(t, "reference-native-retry.json", map[string]interface{}{"failedQueueRetained": true, "failedCountsNotPublished": true, "retryCommitted": true, "cancelAndReleasedRejected": true})
}
