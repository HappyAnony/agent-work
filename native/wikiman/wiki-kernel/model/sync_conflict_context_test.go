//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	"wiki-kernel/conf"
	"wiki-kernel/internal/txqueue"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func conflictFixture(t *testing.T) (*AppConf, *indexRepairContext, string, []byte, *dejavu.MergeResult) {
	t.Helper()
	c, r, _, file, before := referenceModelFixture(t)
	c.Sync.GenerateConflictDoc = true
	c.Lang = "en_US"
	raw, e := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if e != nil {
		t.Fatal(e)
	}
	var labels struct {
		Time map[string]interface{} `json:"_time"`
	}
	if e = json.Unmarshal(raw, &labels); e != nil {
		t.Fatal(e)
	}
	oldTime := util.TimeLangs
	util.TimeLangs = map[string]map[string]interface{}{"en_US": labels.Time}
	t.Cleanup(func() { util.TimeLangs = oldTime })
	stamp := time.Date(2026, 10, 29, 20, 0, 0, 0, time.Local)
	rel := "/" + filepath.Base(filepath.Dir(file)) + "/" + filepath.Base(file)
	result := &dejavu.MergeResult{Time: stamp, Conflicts: []*entity.File{{Path: rel}}}
	backup := filepath.Join(util.TempDir, "repo/sync/conflicts", stamp.Format("2006-01-02-150405"), rel)
	if e = os.MkdirAll(filepath.Dir(backup), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(backup, before, 0600); e != nil {
		t.Fatal(e)
	}
	return c, r, file, before, result
}
func isolatedConflictQueue(t *testing.T, run func(*Transaction)) {
	t.Helper()
	legacyTxQueueLock.Lock()
	old := legacyTxQueue
	q := txqueue.Open(7, run)
	legacyTxQueue = q
	legacyTxQueueLock.Unlock()
	t.Cleanup(func() {
		q.Close(context.Background())
		legacyTxQueueLock.Lock()
		legacyTxQueue = old
		legacyTxQueueLock.Unlock()
	})
}
func TestSyncConflictActualOriginalCreateAndInternalReferenceRemap(t *testing.T) {
	_, _, file, before, result := conflictFixture(t)
	isolatedConflictQueue(t, flushTx)
	ctx := context.Background()
	backup := filepath.Join(util.TempDir, "repo/sync/conflicts", result.Time.Format("2006-01-02-150405"), result.Conflicts[0].Path)
	raw := bytes.ReplaceAll(before, []byte("20261028140002-refdef1"), []byte("20261028140004-refpar1"))
	var source map[string]interface{}
	json.Unmarshal(raw, &source)
	source["Properties"].(map[string]interface{})["custom-avs"] = "fixture"
	raw, _ = json.Marshal(source)
	os.WriteFile(backup, raw, 0600)
	reload, e := applySyncConflictCopies(ctx, result)
	if e != nil || !reload {
		t.Fatal(reload, e)
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "*.docj"))
	if len(files) != 3 {
		t.Fatal(files)
	}
	var copied string
	for _, name := range files {
		if filepath.Base(name) != "20261028140001-refdoc1.docj" && name != file {
			copied = name
		}
	}
	if copied == "" {
		t.Fatal("copy missing")
	}
	data, _ := os.ReadFile(copied)
	var doc struct {
		ID         string
		Properties map[string]string
		Children   []struct {
			ID       string
			Children []struct{ TextMarkBlockRefID string }
		}
	}
	if e = json.Unmarshal(data, &doc); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(doc.Properties["title"], "(Conflicted ") || doc.Properties["custom-avs"] != "" || doc.Children[0].ID == "20261028140004-refpar1" || doc.Children[0].Children[0].TextMarkBlockRefID != doc.Children[0].ID {
		t.Fatal("original reset rules lost", doc)
	}
	if sql.GetBlock(doc.ID) == nil {
		t.Fatal("returned before actual projection")
	}
	if _, e = os.Stat(filepath.Join(filepath.Dir(file), ".siyuan/sort.json")); e != nil {
		t.Fatal(e)
	}
	current, _ := os.ReadFile(file)
	saved, _ := os.ReadFile(backup)
	if !bytes.Equal(before, current) || !bytes.Equal(raw, saved) {
		t.Fatal("original/temporary source changed")
	}
	recordCloudBusinessEvidence(t, "conflict-copy.json", map[string]interface{}{"actualOriginalTransaction": true, "newRootAndBlockIDs": true, "internalReferencesRemapped": true, "originalTitleAndBindingRules": true, "SQLFlushed": true, "sortAdapterCalled": true, "originalAndTempUnchanged": true})
}
func TestSyncConflictBatchPreflightErrorsDoNotCreateEarlierCopies(t *testing.T) {
	for _, mode := range []string{"missing", "bad-json", "traversal", "nil-entry", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			_, _, file, _, result := conflictFixture(t)
			isolatedConflictQueue(t, flushTx)
			box := filepath.Base(filepath.Dir(file))
			second := "/" + box + "/20261029200001-missing.docj"
			result.Conflicts = append(result.Conflicts, &entity.File{Path: second})
			target := filepath.Join(util.TempDir, "repo/sync/conflicts", result.Time.Format("2006-01-02-150405"), second)
			switch mode {
			case "bad-json":
				os.WriteFile(target, []byte("bad fixture"), 0600)
			case "traversal":
				result.Conflicts[1].Path = "/" + box + "/../20261029200001-missing.docj"
			case "nil-entry":
				result.Conflicts[1] = nil
			case "symlink":
				outside := filepath.Join(t.TempDir(), "source")
				os.WriteFile(outside, []byte("fixture"), 0600)
				os.Symlink(outside, target)
			}
			reload, e := applySyncConflictCopies(context.Background(), result)
			if e == nil || reload {
				t.Fatal(mode, reload, e)
			}
			files, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "*.docj"))
			if len(files) != 2 {
				t.Fatal("created before complete source preflight", files)
			}
		})
	}
}
func TestSyncConflictDisabledReadonlyAndCancellation(t *testing.T) {
	c, _, file, before, result := conflictFixture(t)
	ctx := context.Background()
	c.Sync.GenerateConflictDoc = false
	if reload, e := applySyncConflictCopies(ctx, result); e != nil || reload {
		t.Fatal(reload, e)
	}
	c.Sync.GenerateConflictDoc = true
	c.persistenceReadOnly = true
	reload, e := applySyncConflictCopies(ctx, result)
	c.persistenceReadOnly = false
	if !errors.Is(e, ErrSettingReadOnly) || reload {
		t.Fatal(reload, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if reload, e = applySyncConflictCopies(canceled, result); !errors.Is(e, context.Canceled) || reload {
		t.Fatal(reload, e)
	}
	current, _ := os.ReadFile(file)
	if !bytes.Equal(current, before) {
		t.Fatal("guard changed source")
	}
}
func TestBoundCreateWriteFailureReturnedAndWorkerRemainsUsable(t *testing.T) {
	_, r, file, _, _ := conflictFixture(t)
	isolatedConflictQueue(t, flushTx)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	badID := "20261029201000-badcopy"
	tree := treenode.NewTree(box, "/"+badID+".docj", "/copy", "copy")
	os.Mkdir(filepath.Join(filepath.Dir(file), badID+".docj"), 0700)
	if e := r.createConflictTree(ctx, tree, r.guard); !errors.Is(e, errBoundTransaction) {
		t.Fatal("write error lost", e)
	}
	goodID := "20261029201001-goodcpy"
	good := treenode.NewTree(box, "/"+goodID+".docj", "/copy", "copy")
	if e := r.createConflictTree(ctx, good, r.guard); e != nil {
		t.Fatal(e)
	}
	if e := (sql.LegacyMainQueue{DB: r.main.DB}).Flush(ctx); e != nil {
		t.Fatal(e)
	}
	if sql.GetBlock(goodID) == nil {
		t.Fatal("worker did not continue after failed create")
	}
	recordCloudBusinessEvidence(t, "conflict-create-result.json", map[string]interface{}{"actualWriteFailureReturned": true, "failureNonfatal": true, "nextCreateSucceeded": true, "SQLFlushed": true})
}
func TestBoundCreateCancellationWaitsForAcceptedCallback(t *testing.T) {
	_, r, file, _, _ := conflictFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	isolatedConflictQueue(t, func(tx *Transaction) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		flushTx(tx)
	})
	ctx, cancel := context.WithCancel(context.Background())
	id := "20261029202000-cancelx"
	tree := treenode.NewTree(filepath.Base(filepath.Dir(file)), "/"+id+".docj", "/copy", "copy")
	done := make(chan error, 1)
	go func() { done <- r.createConflictTree(ctx, tree, r.guard) }()
	<-entered
	cancel()
	select {
	case <-done:
		t.Fatal("returned before accepted callback")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(file), id+".docj")); !os.IsNotExist(e) {
		t.Fatal("canceled pending create wrote source", e)
	}
	recordCloudBusinessEvidence(t, "conflict-create-join.json", map[string]interface{}{"acceptedWorkActuallyJoined": true, "canceledBeforeExecutionNoWrite": true})
}
func TestBoundTransactionPanicIsResultAndDoesNotPoisonWorker(t *testing.T) {
	q := txqueue.Open(2, flushTx)
	defer q.Close(context.Background())
	bad := &Transaction{completion: &transactionCompletion{ctx: context.Background(), guard: func(context.Context) error { panic("fixture") }, done: make(chan error, 1)}}
	raw, e := json.Marshal(bad)
	if e != nil || bytes.Contains(raw, []byte("completion")) {
		t.Fatal("private completion leaked", e)
	}
	q.Submit(context.Background(), bad)
	if e := <-bad.completion.done; !errors.Is(e, errBoundTransaction) {
		t.Fatal(e)
	}
	good := &Transaction{completion: &transactionCompletion{ctx: context.Background(), guard: func(context.Context) error { return nil }, done: make(chan error, 1)}}
	q.Submit(context.Background(), good)
	if e := <-good.completion.done; e != nil {
		t.Fatal(e)
	}
	if e := q.Drain(context.Background()); e != nil {
		t.Fatal("worker poisoned", e)
	}
}
func TestSyncMergeMissingConflictDoesNotScheduleSuccess(t *testing.T) {
	c, _, _, _, result := conflictFixture(t)
	os.Remove(filepath.Join(util.TempDir, "repo/sync/conflicts", result.Time.Format("2006-01-02-150405"), result.Conflicts[0].Path))
	e := processSyncMergeResult(context.Background(), false, true, result, &dejavu.TrafficStat{}, "fixture", time.Millisecond)
	if !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
	c.repoMu.Lock()
	tasks := c.repoTasks
	c.repoMu.Unlock()
	if tasks != nil {
		t.Fatal("success UI scheduled after failed conflict read")
	}
	recordCloudBusinessEvidence(t, "conflict-merge-error.json", map[string]interface{}{"originalMergeErrorReturned": true, "noSuccessFollowup": true})
}

func TestBoundCreateStaleAdmissionAndClosedQueue(t *testing.T) {
	_, r, file, before, _ := conflictFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	isolatedConflictQueue(t, func(tx *Transaction) { close(entered); <-release; flushTx(tx) })
	id := "20261029203000-stalecp"
	tree := treenode.NewTree(filepath.Base(filepath.Dir(file)), "/"+id+".docj", "/copy", "copy")
	done := make(chan error, 1)
	go func() { done <- r.createConflictTree(context.Background(), tree, r.guard) }()
	<-entered
	old := util.DataDir
	selected := t.TempDir()
	util.DataDir = selected
	close(release)
	e := <-done
	util.DataDir = old
	if !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal("stale queued owner accepted", e)
	}
	if names, _ := os.ReadDir(selected); len(names) != 0 {
		t.Fatal("stale callback wrote selected workspace")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("stale callback changed original source")
	}
	if e = CloseTransactionQueue(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = r.createConflictTree(context.Background(), tree, r.guard); !errors.Is(e, txqueue.ErrClosed) {
		t.Fatal("closed queue accepted create", e)
	}
	recordCloudBusinessEvidence(t, "conflict-create-identity.json", map[string]interface{}{"queuedStaleOwnerRejected": true, "selectedWorkspaceUntouched": true, "originalSourceUnchanged": true, "closedQueueRejected": true})
}
