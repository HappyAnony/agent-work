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
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/task"

	"github.com/88250/lute/ast"
	"github.com/vmihailenco/msgpack/v5"

	"wiki-kernel/internal/indexdb"
)

func TestNotebookIndexRepeatedActualProjectionNoDuplicatesAndHistory(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	c.Editor.VirtualBlockRef = false
	for i := 0; i < 2; i++ {
		if e := NotebookIndexContext(ctx, "index", box); e != nil {
			t.Fatal(e)
		}
	}
	var duplicates int
	if e := r.main.DB.QueryRow("SELECT COUNT(*) FROM (SELECT id FROM blocks GROUP BY id HAVING COUNT(*)>1)").Scan(&duplicates); e != nil || duplicates != 0 {
		t.Fatal(duplicates, e)
	}
	if _, ok := c.historyGenerated.Load(box); !ok {
		t.Fatal("history not initialized")
	}
	other := NewAppConf()
	if _, ok := other.historyGenerated.Load(box); ok {
		t.Fatal("shared history marker")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("canonical source changed")
	}
	recordCloudBusinessEvidence(t, "notebook-repeat.json", map[string]interface{}{"actualOwnedProjection": true, "repeatedWithoutDuplicates": true, "historyInitializedAfterFlush": true, "ownerHistoryIsolated": true, "sourceUnchanged": true})
}
func TestNotebookLoadFailureNoHistoryAndRetry(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	os.WriteFile(file, []byte("bad fixture"), 0600)
	if e := r.indexNotebook(ctx, box); e == nil {
		t.Fatal("source failure swallowed")
	}
	if _, ok := c.historyGenerated.Load(box); ok {
		t.Fatal("history marked after failure")
	}
	os.WriteFile(file, before, 0600)
	if e := r.indexNotebook(ctx, box); e != nil {
		t.Fatal(e)
	}
	if _, ok := c.historyGenerated.Load(box); !ok {
		t.Fatal("retry did not complete")
	}
	recordCloudBusinessEvidence(t, "notebook-load.json", map[string]interface{}{"sourceErrorReturned": true, "failureDoesNotMarkHistory": true, "retry": true})
}
func TestNotebookAVBatchOriginalFormatFailureAndRetry(t *testing.T) {
	c, r, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		t.Fatal(e)
	}
	defer closeIO()
	const name = "storage/av/blocks.msgpack"
	ioctx.Source.Data.MkdirAll("storage/av", 0700)
	bad := []byte{0xc1}
	ioctx.Source.Data.WriteFile(name, bad, 0600)
	if e = r.indexNotebook(ctx, box); e == nil {
		t.Fatal("bad AV accepted")
	}
	if _, ok := c.historyGenerated.Load(box); ok {
		t.Fatal("history marked on AV failure")
	}
	after, _ := ioctx.Source.Data.ReadFile(name)
	if !bytes.Equal(after, bad) {
		t.Fatal("bad AV overwritten")
	}
	ioctx.Source.Data.Remove(name)
	nodes := []*ast.Node{{Type: ast.NodeAttributeView, AttributeViewID: "av", ID: "first"}, {Type: ast.NodeAttributeView, AttributeViewID: "av", ID: "second"}, {Type: ast.NodeAttributeView, AttributeViewID: "av", ID: "first"}}
	if e = r.bindRepairAVBatch(ctx, ioctx.Source, nodes); e != nil {
		t.Fatal(e)
	}
	if e = r.indexNotebook(ctx, box); e != nil {
		t.Fatal(e)
	}
	raw, _ := ioctx.Source.Data.ReadFile(name)
	var rows map[string][]string
	if e = msgpack.Unmarshal(raw, &rows); e != nil || len(rows["av"]) != 2 || rows["av"][0] != "first" || rows["av"][1] != "second" {
		t.Fatal(rows, e)
	}
	recordCloudBusinessEvidence(t, "notebook-av.json", map[string]interface{}{"batchOriginalMsgpackAndOrder": true, "deduplicated": true, "badAVPreserved": true, "failedHistoryNotMarked": true, "retry": true})
}
func TestNotebookReferenceLoadAndSQLFailureAreReturned(t *testing.T) {
	_, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	os.WriteFile(file, []byte("TextMarkBlockRefID bad fixture"), 0600)
	if e := r.indexReferences(ctx); e == nil {
		t.Fatal("bad reference source swallowed")
	}
	os.WriteFile(file, before, 0600)
	if _, e := r.main.DB.Exec("CREATE TRIGGER fail_notebook_refs BEFORE INSERT ON refs BEGIN SELECT RAISE(ABORT,'fixture'); END"); e != nil {
		t.Fatal(e)
	}
	if e := r.indexReferences(ctx); e == nil {
		t.Fatal("ref SQL failure swallowed")
	}
	r.main.DB.Exec("DROP TRIGGER fail_notebook_refs")
	if e := r.indexReferences(ctx); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := r.main.DB.QueryRow("SELECT COUNT(*) FROM refs").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	recordCloudBusinessEvidence(t, "notebook-refs.json", map[string]interface{}{"actualReferenceCount": count, "sourceAndSQLFailuresReturned": true, "queueRetry": true})
}
func TestNotebookUnindexRecentFailureAndDerivedRetry(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	c.Editor.VirtualBlockRef = false
	root := filepath.Join(r.data, "storage")
	os.MkdirAll(root, 0700)
	recent := filepath.Join(root, "recent-doc.json")
	bad := []byte("bad fixture")
	os.WriteFile(recent, bad, 0600)
	if e := r.unindexNotebook(ctx, box); e == nil {
		t.Fatal("bad recent source swallowed")
	}
	after, _ := os.ReadFile(recent)
	if !bytes.Equal(after, bad) {
		t.Fatal("bad recent overwritten")
	}
	os.WriteFile(recent, []byte("[]"), 0600)
	if e := NotebookIndexContext(ctx, "unindex", box); e != nil {
		t.Fatal(e)
	}
	n, e := r.registry.Count(ctx, indexdb.BlockTreeFilter{Kind: "box", Value: box})
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	var blocks int
	if e = r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE box=?", box).Scan(&blocks); e != nil || blocks != 0 {
		t.Fatal(blocks, e)
	}
	source, _ := os.ReadFile(file)
	if !bytes.Equal(before, source) {
		t.Fatal("unindex removed source")
	}
	recordCloudBusinessEvidence(t, "notebook-unindex.json", map[string]interface{}{"badRecentPreserved": true, "errorReturned": true, "actualRegistryAndSQLRemoved": true, "retry": true, "sourceRetained": true})
}
func TestNotebookFullReadonlyAndCancellationDoNotResetDB(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	c.persistenceReadOnly = true
	e := NotebookIndexContext(ctx, "full", "")
	c.persistenceReadOnly = false
	if !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = NotebookIndexContext(canceled, "index", filepath.Base(filepath.Dir(file))); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var n int
	if e = r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&n); e != nil || n == 0 {
		t.Fatal("DB reset before guard", e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("guard changed source")
	}
}

func TestNotebookInitReturnsAfterActualProjectionFlush(t *testing.T) {
	c, r, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	c.Editor.VirtualBlockRef = false
	if _, e := r.registry.Remove(ctx, indexdb.BlockTreeFilter{Kind: "box", Value: box}); e != nil {
		t.Fatal(e)
	}
	if _, e := r.main.DB.Exec("DELETE FROM blocks"); e != nil {
		t.Fatal(e)
	}
	if e := InitBoxesContext(ctx); e != nil {
		t.Fatal(e)
	}
	var roots int
	if e := r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE type='d'").Scan(&roots); e != nil || roots != 2 {
		t.Fatal("init returned before projection", roots, e)
	}
	if _, ok := c.historyGenerated.Load(box); !ok {
		t.Fatal("init history missing")
	}
	recordCloudBusinessEvidence(t, "notebook-init.json", map[string]interface{}{"initReturnsAfterSQL": true, "actualRootCount": roots, "historyInitialized": true})
}
func TestNotebookSQLFailureDoesNotPublishAVOrHistory(t *testing.T) {
	c, r, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	changed := bytes.ReplaceAll(before, []byte("reference fixture"), []byte("notebook changed fixture"))
	os.WriteFile(file, changed, 0600)
	if _, e := r.main.DB.Exec("CREATE TRIGGER fail_notebook_index BEFORE INSERT ON blocks BEGIN SELECT RAISE(ABORT,'fixture'); END"); e != nil {
		t.Fatal(e)
	}
	if e := r.indexNotebook(ctx, box); e == nil {
		t.Fatal("SQL failure swallowed")
	}
	if _, ok := c.historyGenerated.Load(box); ok {
		t.Fatal("history initialized after failed flush")
	}
	if _, e := os.Stat(filepath.Join(r.data, "storage/av/blocks.msgpack")); !os.IsNotExist(e) {
		t.Fatal("AV published after failed SQL flush", e)
	}
	r.main.DB.Exec("DROP TRIGGER fail_notebook_index")
	if e := r.indexNotebook(ctx, box); e != nil {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "notebook-sql.json", map[string]interface{}{"actualSQLFailureReturned": true, "noAVOrHistoryAfterFailedFlush": true, "retry": true})
}

func TestNotebookFullParentUsesLegacyTaskSlotAndActualJoin(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	ctx := context.Background()
	c.repoOwner.Close(ctx)
	entered, release := make(chan struct{}), make(chan struct{})
	o := cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) { return conf.AppData{}, nil }, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
		return cloudrepo.Result{}, nil
	}, NotebookIndex: func(ctx context.Context, _ conf.AppData, _ cloudrepo.NotebookIndexRequest) error {
		close(entered)
		<-ctx.Done()
		<-release
		return ctx.Err()
	}}
	s, e := cloudrepo.New(o)
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	task.OpenLegacyTasks()
	defer task.CloseLegacyTasks(ctx)
	if !c.scheduleFullNotebookReindex() {
		t.Fatal("parent not admitted")
	}
	deadline := time.Now().Add(time.Second)
	for !task.ContainIndexTask() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !task.ContainIndexTask() {
		t.Fatal("full parent bypassed legacy task slot")
	}
	select {
	case <-entered:
		t.Fatal("started before legacy dispatcher")
	default:
	}
	jobDone := make(chan struct{})
	go func() { task.ExecTaskJob(); close(jobDone) }()
	<-entered
	timeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if e = c.CloseCloudRepo(timeout); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("fake parent join", e)
	}
	select {
	case <-jobDone:
		t.Fatal("task returned with active callback")
	default:
	}
	close(release)
	if e = c.CloseCloudRepo(ctx); e != nil {
		t.Fatal(e)
	}
	<-jobDone
	recordCloudBusinessEvidence(t, "notebook-parent.json", map[string]interface{}{"legacyTaskSlotPreserved": true, "parentActualJoin": true, "closeCancelsWork": true})
}
