//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/siyuan-note/dejavu"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestConflictSortRootedListMatchesOriginalModesAndLimit(t *testing.T) {
	c, r, file, _, _ := conflictFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	previous := "/" + filepath.Base(file)
	id := "20261029210000-newsort"
	raw, _ := os.ReadFile(file)
	var doc map[string]interface{}
	json.Unmarshal(raw, &doc)
	doc["ID"] = id
	doc["Properties"].(map[string]interface{})["id"] = id
	doc["Properties"].(map[string]interface{})["title"] = "AAA copy"
	created, _ := json.Marshal(doc)
	os.WriteFile(filepath.Join(filepath.Dir(file), id+".docj"), created, 0600)
	confDir := filepath.Join(filepath.Dir(file), ".siyuan")
	os.MkdirAll(confDir, 0700)
	sortFile := filepath.Join(confDir, "sort.json")
	settings := filepath.Join(confDir, "conf.json")
	for _, mode := range []int{util.SortModeNameASC, util.SortModeNameDESC, util.SortModeCustom, util.SortModeRefCountDESC} {
		for _, limit := range []int{1, 128} {
			existing := map[string]int{"unrelated": 77, id: 3, "20261028140003-refdoc2": 2, "20261028140001-refdoc1": 1}
			before, _ := json.Marshal(existing)
			os.WriteFile(sortFile, before, 0600)
			boxConf := []byte(`{"sortMode":` + fmtInt(mode) + `}`)
			os.WriteFile(settings, boxConf, 0600)
			c.FileTree.MaxListCount = limit
			docs, _, e := ListDocTree(box, "/", util.SortModeUnassigned, false, false, limit)
			if e != nil {
				t.Fatal(e)
			}
			want, _ := doctree.InsertSort(existing, docs, strings.TrimSuffix(filepath.Base(file), ".docj"), id)
			if e = r.addConflictSort(ctx, box, previous, id); e != nil {
				t.Fatal(mode, limit, e)
			}
			after, _ := os.ReadFile(sortFile)
			var got map[string]int
			json.Unmarshal(after, &got)
			if !reflect.DeepEqual(got, want) {
				t.Fatal(mode, limit, got, want)
			}
		}
	}
	recordCloudBusinessEvidence(t, "sort-modes.json", map[string]interface{}{"actualOriginalListParity": true, "modes": 4, "limits": 2, "unrelatedKeysRetained": true})
}
func TestConflictSortCheckedEnumerationBeforeRewrite(t *testing.T) {
	primary := errors.New("conflict sort enumeration incomplete")
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			_, r, file, _, _ := conflictFixture(t)
			ctx := context.Background()
			box := filepath.Base(filepath.Dir(file))
			previous := "/" + filepath.Base(file)
			id := "20261029210000-newsort"
			data := filepath.Dir(filepath.Dir(file))
			sortDir := filepath.Join(filepath.Dir(file), ".siyuan")
			if err := os.MkdirAll(sortDir, 0700); err != nil {
				t.Fatal(err)
			}
			before := []byte(`{"keep":42}`)
			sortFile := filepath.Join(sortDir, "sort.json")
			if err := os.WriteFile(sortFile, before, 0600); err != nil {
				t.Fatal(err)
			}
			notices := 0
			op := conflictSortIO{
				readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
					if name != box {
						return source.ReadDir(ctx, name)
					}
					if kind == "incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					entries, readErr := source.ReadDir(ctx, name)
					if readErr != nil {
						return nil, readErr
					}
					if err := source.Data.Rename(name, name+".old"); err != nil {
						return nil, err
					}
					if err := source.Data.Mkdir(name, 0700); err != nil {
						return nil, err
					}
					return entries, nil
				},
				notify: func(map[string]int) { notices++ },
			}
			err := r.addConflictSortWithIO(ctx, box, previous, id, op)
			if err == nil || notices != 0 {
				t.Fatal("failed enumeration admitted rewrite or notification", err, notices)
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete enumeration error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("replaced directory accepted", err)
			} else {
				sortFile = filepath.Join(data, box+".old", ".siyuan", "sort.json")
			}
			after, readErr := os.ReadFile(sortFile)
			if readErr != nil || !bytes.Equal(after, before) {
				t.Fatal("sort changed after failed enumeration", string(after), readErr)
			}
		})
	}
}
func fmtInt(n int) string { raw, _ := json.Marshal(n); return string(raw) }
func TestConflictSortCorruptionAndWriteFailureDoNotOverwrite(t *testing.T) {
	_, r, file, _, _ := conflictFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	previous := "/" + filepath.Base(file)
	id := "20261029210000-newsort"
	dir := filepath.Join(filepath.Dir(file), ".siyuan")
	os.MkdirAll(dir, 0700)
	name := filepath.Join(dir, "sort.json")
	bad := []byte("bad fixture")
	os.WriteFile(name, bad, 0600)
	if e := r.addConflictSort(ctx, box, previous, id); e == nil {
		t.Fatal("bad sort overwritten")
	}
	after, _ := os.ReadFile(name)
	if !bytes.Equal(bad, after) {
		t.Fatal("bad sort changed")
	}
	before := []byte(`{"keep":42}`)
	os.WriteFile(name, before, 0600)
	source, _ := os.ReadFile(file)
	os.WriteFile(file, bad, 0600)
	if e := r.addConflictSort(ctx, box, previous, id); e == nil {
		t.Fatal("bad document ignored")
	}
	os.WriteFile(file, source, 0600)
	after, _ = os.ReadFile(name)
	if !bytes.Equal(before, after) {
		t.Fatal("sort changed after document failure")
	}
	if os.Geteuid() != 0 {
		os.Chmod(dir, 0500)
		e := r.addConflictSort(ctx, box, previous, id)
		os.Chmod(dir, 0700)
		if e == nil {
			t.Fatal("actual write permission failure swallowed")
		}
		after, _ = os.ReadFile(name)
		if !bytes.Equal(before, after) {
			t.Fatal("failed write replaced sort")
		}
	}
	recordCloudBusinessEvidence(t, "sort-failure.json", map[string]interface{}{"badSortPreserved": true, "badDocNotMoved": true, "actualWriteFailureChecked": os.Geteuid() != 0})
}
func TestConflictSortOwnerReadonlyCancellationAndSQLFailure(t *testing.T) {
	c, r, file, _, _ := conflictFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	p := "/" + filepath.Base(file)
	id := "20261029210000-newsort"
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := r.addConflictSort(canceled, box, p, id); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	e := r.addConflictSort(ctx, box, p, id)
	c.persistenceReadOnly = false
	if !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	old := util.DataDir
	util.DataDir = t.TempDir()
	e = r.addConflictSort(ctx, box, p, id)
	util.DataDir = old
	if e == nil {
		t.Fatal("old data selector accepted")
	}
	if _, e = r.main.DB.Exec("ALTER TABLE refs RENAME TO refs_sort_saved"); e != nil {
		t.Fatal(e)
	}
	e = r.addConflictSort(ctx, box, p, id)
	r.main.DB.Exec("ALTER TABLE refs_sort_saved RENAME TO refs")
	if e == nil {
		t.Fatal("reference query failure swallowed")
	}
	if e = r.addConflictSort(ctx, box, p, id); e != nil {
		t.Fatal("sort retry", e)
	}
}
func historyIndexFixture(t *testing.T) (*indexRepairContext, string, string, []byte) {
	t.Helper()
	_, r, _, before, _ := conflictFixture(t)
	name := "2026-10-29-210000-sync"
	dir := filepath.Join(util.HistoryDir, name)
	os.MkdirAll(filepath.Join(dir, "box"), 0700)
	os.MkdirAll(filepath.Join(dir, "assets/folder"), 0700)
	file := filepath.Join(dir, "box/20261028140003-refdoc2.docj")
	os.WriteFile(file, before, 0600)
	os.WriteFile(filepath.Join(dir, "assets/picture-20261029210000-assetxx.png"), []byte("fixture"), 0600)
	return r, name, file, before
}
func TestHistoryIndexActualRowsFormatReadonlyAndDirectoryRetry(t *testing.T) {
	_, name, file, before := historyIndexFixture(t)
	ctx := context.Background()
	q := sql.CaptureLegacyHistoryQueue()
	if e := indexHistoryDirContext(ctx, name, util.NewLute()); e != nil {
		t.Fatal(e)
	}
	if e := indexHistoryDirContext(ctx, name, util.NewLute()); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := q.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); e != nil || count != 2 {
		t.Fatal(count, e)
	}
	var typ int
	var op, p, created, title string
	if e := q.DB.QueryRow("SELECT type,op,path,created,title FROM histories_fts_case_insensitive WHERE type=1").Scan(&typ, &op, &p, &created, &title); e != nil {
		t.Fatal(e)
	}
	stamp, _ := time.ParseInLocation("2006-01-02-150405", "2026-10-29-210000", time.Local)
	if typ != 1 || op != "sync" || p != name+"/box/20261028140003-refdoc2.docj" || created != fmtInt(int(stamp.Unix())) || title != "reference fixture" {
		t.Fatal(typ, op, p, created, title)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("history source changed")
	}
	recordCloudBusinessEvidence(t, "history-format.json", map[string]interface{}{"originalFields": true, "actualRows": count, "sourceUnchanged": true, "directoryRetryNoDuplicates": true})
}
func TestHistoryIndexIncompleteScanAndSQLFailureRetry(t *testing.T) {
	_, name, file, before := historyIndexFixture(t)
	ctx := context.Background()
	q := sql.CaptureLegacyHistoryQueue()
	bad := filepath.Join(filepath.Dir(file), "bad.docj")
	os.WriteFile(bad, []byte("bad fixture"), 0600)
	if e := indexHistoryDirContext(ctx, name, util.NewLute()); e == nil {
		t.Fatal("incomplete scan succeeded")
	}
	var n int
	q.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&n)
	if n != 0 {
		t.Fatal("partial plan indexed", n)
	}
	os.Remove(bad)
	if _, e := q.DB.Exec("ALTER TABLE histories_fts_case_insensitive RENAME TO histories_sort_saved"); e != nil {
		t.Fatal(e)
	}
	if e := indexHistoryDirContext(ctx, name, util.NewLute()); e == nil {
		t.Fatal("SQL error swallowed")
	}
	q.DB.Exec("ALTER TABLE histories_sort_saved RENAME TO histories_fts_case_insensitive")
	if e := indexHistoryDirContext(ctx, name, util.NewLute()); e != nil {
		t.Fatal(e)
	}
	q.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&n)
	if n != 2 {
		t.Fatal("failed tail retry duplicated directory", n)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("failed history index changed source")
	}
	recordCloudBusinessEvidence(t, "history-failure.json", map[string]interface{}{"incompleteScanNoEnqueue": true, "actualSQLFailure": true, "failedTailRetry": true, "rows": n, "sourceUnchanged": true})
}
func TestHistoryIndexMissingCancelInvalidAndSymlink(t *testing.T) {
	_, name, file, _ := historyIndexFixture(t)
	ctx := context.Background()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := indexHistoryDirContext(canceled, name, util.NewLute()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := indexHistoryDirContext(ctx, "bad-directory", util.NewLute()); e == nil {
		t.Fatal("invalid directory accepted")
	}
	if e := indexHistoryDirContext(ctx, "2026-10-29-220000-sync", util.NewLute()); e != nil {
		t.Fatal("missing sync payload not a no-op", e)
	}
	link := filepath.Join(filepath.Dir(file), "link")
	os.Symlink(t.TempDir(), link)
	if e := indexHistoryDirContext(ctx, name, util.NewLute()); e == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(link)

}

func TestHistoryIndexCapturedScopeAndCommittedNotifications(t *testing.T) {
	_, name, _, _ := historyIndexFixture(t)
	ctx := context.Background()
	h, e := captureHistoryIndexContext(ctx)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	h.notify = func(_ context.Context, _ indexdb.HistoryProgress) error {
		calls++
		var n int
		if e := h.queue.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&n); e != nil || n != 2 {
			t.Error("notification before actual commit", n, e)
		}
		return nil
	}
	old := util.HistoryDir
	util.HistoryDir = t.TempDir()
	e = h.index(ctx, name, util.NewLute())
	util.HistoryDir = old
	if e == nil || calls != 0 {
		t.Fatal("stale history root accepted", e, calls)
	}
	h.queue.DB.Exec("ALTER TABLE histories_fts_case_insensitive RENAME TO history_notify_saved")
	e = h.index(ctx, name, util.NewLute())
	h.queue.DB.Exec("ALTER TABLE history_notify_saved RENAME TO histories_fts_case_insensitive")
	if e == nil || calls != 0 {
		t.Fatal("failed commit notified", e, calls)
	}
	if e = h.index(ctx, name, util.NewLute()); e != nil || calls == 0 {
		t.Fatal(e, calls)
	}
	recordCloudBusinessEvidence(t, "history-notify.json", map[string]interface{}{"capturedRootRejected": true, "failedCommitNoNotification": true, "notificationAfterActualCommit": true})
}

func TestConflictSortFailureStopsMergeHistoryAndSuccessFollowup(t *testing.T) {
	c, _, file, before, result := conflictFixture(t)
	isolatedConflictQueue(t, flushTx)
	ctx := context.Background()
	dir := filepath.Join(filepath.Dir(file), ".siyuan")
	os.MkdirAll(dir, 0700)
	sortFile := filepath.Join(dir, "sort.json")
	bad := []byte("bad sort fixture")
	os.WriteFile(sortFile, bad, 0600)
	historyDir := filepath.Join(util.HistoryDir, result.Time.Format("2006-01-02-150405")+"-sync", "box")
	os.MkdirAll(historyDir, 0700)
	os.WriteFile(filepath.Join(historyDir, filepath.Base(file)), before, 0600)
	if e := processSyncMergeResult(ctx, false, true, result, &dejavu.TrafficStat{}, "fixture", time.Millisecond); e == nil {
		t.Fatal("merge ignored sort failure")
	}
	q := sql.CaptureLegacyHistoryQueue()
	var rows int
	q.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&rows)
	if rows != 0 {
		t.Fatal("history indexed after failed sort", rows)
	}
	c.repoMu.Lock()
	tasks := c.repoTasks
	c.repoMu.Unlock()
	if tasks != nil {
		t.Fatal("success followup after failed sort")
	}
	after, _ := os.ReadFile(sortFile)
	if !bytes.Equal(after, bad) {
		t.Fatal("bad sort overwritten")
	}
	copies, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "*.docj"))
	if len(copies) != 3 {
		t.Fatal("expected already-created partial copy", len(copies))
	}
	os.WriteFile(sortFile, []byte("{}"), 0600)
	if e := processSyncMergeResult(ctx, false, true, result, &dejavu.TrafficStat{}, "fixture", time.Millisecond); e != nil {
		t.Fatal("merge retry", e)
	}
	q.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&rows)
	if rows != 1 {
		t.Fatal(rows)
	}
	if e := c.CloseCloudRepo(ctx); e != nil {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "sort-history-merge.json", map[string]interface{}{"sortErrorReturned": true, "noHistoryOrSuccessFollowupAfterError": true, "partialCopyExplicit": true, "retryHistoryCommitted": true})
}

func TestHistoryIndexAcceptedFlushDefersCancellationUntilActualSQLReturn(t *testing.T) {
	_, name, _, _ := historyIndexFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	h, e := captureHistoryIndexContext(ctx)
	if e != nil {
		t.Fatal(e)
	}
	h.queue.DB.SetMaxOpenConns(1)
	held, e := h.queue.DB.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	beforeWait := h.queue.DB.Stats().WaitCount
	notices := 0
	h.notify = func(context.Context, indexdb.HistoryProgress) error { notices++; return nil }
	done := make(chan struct{})
	var result error
	go func() { result = h.index(ctx, name, util.NewLute()); close(done) }()
	t.Cleanup(func() { cancel(); held.Close(); <-done; h.queue.DB.SetMaxOpenConns(3) })
	deadline := time.Now().Add(2 * time.Second)
	for h.queue.DB.Stats().WaitCount == beforeWait && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if h.queue.DB.Stats().WaitCount == beforeWait {
		t.Fatal("history transaction did not reach the held SQL connection")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before actual accepted SQL callback")
	case <-time.After(10 * time.Millisecond):
	}
	held.Close()
	<-done
	if !errors.Is(result, context.Canceled) || notices != 0 {
		t.Fatal(result, notices)
	}
	var rows int
	if e = h.queue.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&rows); e != nil || rows != 2 {
		t.Fatal("accepted history batch not actually finished", rows, e)
	}
	recordCloudBusinessEvidence(t, "history-join.json", map[string]interface{}{"actualSQLConnectionBarrier": true, "acceptedFlushActuallyJoined": true, "canceledNotificationSuppressed": true, "committedRows": rows})
}
