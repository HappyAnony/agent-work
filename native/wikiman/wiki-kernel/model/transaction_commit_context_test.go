//go:build fts5

// Copyright (c) 2020-present, b3log.org
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
	"testing"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/cache"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func commitFixture(t *testing.T) (*AppConf, *transactionEnvironment, *Transaction, string, []byte) {
	t.Helper()
	c, r, file, before, _ := conflictFixture(t)
	if e := cache.OpenLegacyIAL(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(cache.CloseLegacyIAL)
	env := legacyTransactionEnvironment()
	if e := bindLegacyCommit(context.Background(), env, r); e != nil {
		t.Fatal(e)
	}
	box := filepath.Base(filepath.Dir(file))
	tree, e := env.load(box, "/20261028140001-refdoc1.docj", env.parser())
	if e != nil {
		t.Fatal(e)
	}
	n := tree.Root.FirstChild
	n.FirstChild.Tokens = []byte("changed definition")
	tree.Root.SetIALAttr("title", "changed title")
	tx := &Transaction{environment: env, trees: map[string]*parse.Tree{tree.ID: tree}, nodes: map[string]*ast.Node{n.ID: n}}
	return c, env, tx, file, before
}
func TestCommitContextActualSaveDynamicReferenceCacheAndNotifications(t *testing.T) {
	c, env, tx, file, before := commitFixture(t)
	events := make(chan commitNotice, 32)
	env.commitContext.publish = func(n commitNotice) { events <- n }
	c.syncRuntime.same.Store(7)
	if e := tx.commitLegacy(); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(file)
	if e != nil || bytes.Equal(before, after) || !bytes.Contains(after, []byte("changed definition")) {
		t.Fatal("dynamic source not saved", e)
	}
	for _, tree := range tx.trees {
		doc, e := env.commitContext.r.main.DB.Query("SELECT id FROM blocks WHERE id=?", tree.ID)
		if e != nil {
			t.Fatal(e)
		}
		ok := doc.Next()
		doc.Close()
		if !ok {
			t.Fatal("commit returned before SQL")
		}
		if got := cache.GetDocIAL(tree.Path); got["title"] != "changed title" {
			t.Fatal("cache publication missing", got)
		}
	}
	if c.syncRuntime.same.Load() != 0 {
		t.Fatal("captured sync not planned")
	}
	// Mutating the original AST/transaction after completion must not alter UI data.
	for _, tree := range tx.trees {
		tree.Root.SetIALAttr("title", "late mutation")
	}
	tx.DoOperations = append(tx.DoOperations, &Operation{Action: "late mutation"})
	info, anchors, saved := 0, 0, 0
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for info < 2 || anchors < 1 || saved < 1 {
		select {
		case n := <-events:
			if bytes.Contains(n.payload, []byte("late mutation")) {
				t.Fatal("mutable payload escaped")
			}
			switch n.kind {
			case "info":
				info++
			case "saved":
				saved++
			case "anchor":
				anchors++
			}
		case <-deadline.C:
			t.Fatal("missing original notifications", info, anchors, saved)
		}
	}
	recordCloudBusinessEvidence(t, "commit-success.json", map[string]interface{}{"actualSourceSave": true, "dynamicReferenceSaved": true, "IALPublished": true, "SQLCommittedBeforeReturn": true, "ownedSync": true, "immutableNotices": true, "docInfoNotifications": info, "anchorNotifications": anchors})
}
func TestCommitContextFailureDoesNotPlanSyncOrPublishSuccess(t *testing.T) {
	for _, mode := range []string{"changed-source", "source-directory", "AV-error", "SQL-error", "canceled", "read-only", "stale-owner"} {
		t.Run(mode, func(t *testing.T) {
			c, env, tx, file, _ := commitFixture(t)
			notices := make(chan commitNotice, 32)
			env.commitContext.publish = func(n commitNotice) { notices <- n }
			c.syncRuntime.same.Store(9)
			switch mode {
			case "changed-source":
				for _, tree := range tx.trees {
					p := filepath.Join(util.DataDir, tree.Box, tree.Path)
					if e := os.WriteFile(p, []byte("external change"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			case "source-directory":
				for _, tree := range tx.trees {
					p := filepath.Join(util.DataDir, tree.Box, tree.Path)
					os.Remove(p)
					os.Mkdir(p, 0700)
				}
			case "AV-error":
				env.commitContext.relatedAV = func(context.Context, []string, func(string) error) error { return errors.New("fixture AV failure") }
			case "SQL-error":
				db := env.commitContext.r.main.DB
				if _, e := db.Exec("CREATE TRIGGER fail_commit BEFORE INSERT ON blocks BEGIN SELECT RAISE(ABORT,'fixture'); END"); e != nil {
					t.Fatal(e)
				}
				defer db.Exec("DROP TRIGGER fail_commit")
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				env.ctx = ctx
			case "read-only":
				c.m.Lock()
				c.persistenceReadOnly = true
				c.m.Unlock()
				defer func() { c.m.Lock(); c.persistenceReadOnly = false; c.m.Unlock() }()
			case "stale-owner":
				old := Conf
				Conf = &AppConf{}
				defer func() { Conf = old }()
			}
			if e := tx.commitLegacy(); e == nil {
				t.Fatal("failure swallowed")
			}
			if c.syncRuntime.same.Load() != 9 {
				t.Fatal("failure scheduled sync")
			}
			select {
			case <-notices:
				t.Fatal("failure published success")
			default:
			}
			if c.repoTasks != nil {
				t.Fatal("failure admitted delayed work")
			}
			_ = file
		})
	}
}

func TestCommitContextReferenceQueueWaitsForAcceptedStage(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	db := env.commitContext.r.main.DB
	if _, e := db.Exec("DELETE FROM refs"); e != nil {
		t.Fatal(e)
	}
	failure := errors.New("anchor AV stage rejected")
	env.commitContext.refreshAV = func(context.Context, map[string]*ast.Node, func(string) error) error {
		return failure
	}
	if e := tx.commitLegacy(); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	var count int
	if e := db.QueryRow("SELECT COUNT(*) FROM refs").Scan(&count); e != nil || count != 0 {
		t.Fatal("reference update escaped before source stage", count, e)
	}
}
func TestCommitContextOwnedDelayedCloseAndPayloadIsolation(t *testing.T) {
	c, env, _, _, _ := commitFixture(t)
	notices := make(chan commitNotice, 2)
	env.commitContext.publish = func(n commitNotice) { notices <- n }
	input := []commitNotice{{kind: "info", payload: json.RawMessage(`{"title":"original"}`), delay: 200 * time.Millisecond}}
	if e := env.commitContext.complete(context.Background(), input); e != nil {
		t.Fatal(e)
	}
	input[0].payload[10] = 'X'
	select {
	case n := <-notices:
		if !strings.Contains(string(n.payload), "original") {
			t.Fatal("payload not copied")
		}
	case <-time.After(time.Second):
		t.Fatal("timer lost")
	}
	if e := env.commitContext.complete(context.Background(), []commitNotice{{kind: "av", delay: 500 * time.Millisecond}}); e != nil {
		t.Fatal(e)
	}
	if e := c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-notices:
		t.Fatal("closed owner published delayed notice")
	default:
	}
	if e := env.commitContext.complete(context.Background(), nil); e == nil {
		t.Fatal("closed owner accepted notices")
	}
}
func TestCommitContextCapturesLanguageAndSyncInterval(t *testing.T) {
	c, env, tx, _, _ := commitFixture(t)
	initial := env.commitContext.relative(time.Now().Add(-time.Hour))
	for key := range util.TimeLangs[c.Lang] {
		util.TimeLangs[c.Lang][key] = "changed"
	}
	c.Sync.Interval = 9999
	if got := env.commitContext.relative(time.Now().Add(-time.Hour)); got != initial {
		t.Fatal("admitted language changed", got, initial)
	}
	events := make(chan commitNotice, 32)
	env.commitContext.publish = func(n commitNotice) { events <- n }
	start := time.Now()
	if e := tx.commitLegacy(); e != nil {
		t.Fatal(e)
	}
	c.syncRuntime.planMu.Lock()
	planned := c.syncRuntime.plan
	c.syncRuntime.planMu.Unlock()
	if planned.Before(start.Add(env.commitContext.interval)) || planned.After(time.Now().Add(env.commitContext.interval+time.Second)) {
		t.Fatal("used current global interval")
	}
}
func TestCommitContextRelatedAVReadErrorsAreNotIgnored(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	tx.relatedAvIDs = []string{"20261029120000-noav001"}
	if e := tx.commitLegacy(); e == nil {
		t.Fatal("missing related AV ignored")
	}
	_ = env
}
func TestCommitContextParentDocInfoReadDoesNotMigrateSource(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	parent := "20261029120000-parent1"
	parentPath := filepath.Join(util.DataDir, tree.Box, parent+util.DocExt)
	raw := []byte(`{"ID":"` + parent + `","Type":"NodeDocument","Properties":{"id":"` + parent + `","title":"parent"}}`)
	if e := os.WriteFile(parentPath, raw, 0600); e != nil {
		t.Fatal(e)
	}
	tree.Path = "/" + parent + "/" + tree.ID + util.DocExt
	os.MkdirAll(filepath.Join(util.DataDir, tree.Box, parent), 0700)
	tx.DoOperations = []*Operation{{Action: "create", Data: tree}}
	events := make(chan commitNotice, 32)
	env.commitContext.publish = func(n commitNotice) { events <- n }
	if e := tx.commitLegacy(); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(parentPath)
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("metadata read migrated old parent", e)
	}
	// Pure original projection retains hidden-count and all original keys.
	data := projectReloadDocInfo(tree, 123, 2, func(time.Time) string { return "relative" })
	for _, key := range []string{"rootID", "name", "alias", "name1", "memo", "bookmark", "size", "hSize", "mtime", "ctime", "hMtime", "hCtime", "subFileCount"} {
		if _, ok := data[key]; !ok {
			t.Fatal("missing original field", key)
		}
	}
}
func TestCommitContextRejectsMissingBinding(t *testing.T) {
	if e := (&Transaction{environment: &transactionEnvironment{}}).commitLegacy(); e == nil {
		t.Fatal("implicit fallback")
	}
}

func TestCommitContextOrdinaryFIFOAdmissionUsesBoundFinish(t *testing.T) {
	_, _, _, file, _ := commitFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	isolatedConflictQueue(t, func(tx *Transaction) { close(entered); <-release; flushTx(tx) })
	id := "20261029130000-commit1"
	box := filepath.Base(filepath.Dir(file))
	tree := treenode.NewTree(box, "/"+id+util.DocExt, "/FIFO create", "FIFO create")
	tx := &Transaction{DoOperations: []*Operation{{Action: "create", Data: tree}}}
	if e := PerformTransactions(&[]*Transaction{tx}); e != nil {
		t.Fatal(e)
	}
	<-entered
	if tx.environment.commitContext == nil {
		t.Fatal("admission did not capture finish")
	}
	notices := make(chan commitNotice, 32)
	tx.environment.commitContext.publish = func(n commitNotice) { notices <- n }
	close(release)
	FlushTxQueue()
	if _, e := os.Stat(filepath.Join(util.DataDir, box, id+util.DocExt)); e != nil {
		t.Fatal("FIFO did not save", e)
	}
	var count int
	if e := tx.environment.commitContext.r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&count); e != nil || count != 1 {
		t.Fatal("FIFO did not commit", count, e)
	}
	recordCloudBusinessEvidence(t, "ordinary-commit.json", map[string]interface{}{"originalFIFO": true, "capturedBeforeWorker": true, "actualSourceAndSQL": true})
}
func TestCommitContextAcceptedSQLCancellationWaitsForActualCompletion(t *testing.T) {
	c, env, tx, _, _ := commitFixture(t)
	tx.nodes = map[string]*ast.Node{}
	ctx, cancel := context.WithCancel(context.Background())
	env.ctx = ctx
	db := env.commitContext.r.main.DB
	db.SetMaxOpenConns(1)
	held, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	before := db.Stats().WaitCount
	done := make(chan struct{})
	var result error
	go func() { result = tx.commitLegacy(); close(done) }()
	t.Cleanup(func() { cancel(); held.Close(); <-done; db.SetMaxOpenConns(3) })
	deadline := time.Now().Add(2 * time.Second)
	for db.Stats().WaitCount == before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if db.Stats().WaitCount == before {
		t.Fatal("did not reach actual SQL transaction")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("accepted SQL not joined")
	case <-time.After(10 * time.Millisecond):
	}
	held.Close()
	<-done
	if !errors.Is(result, context.Canceled) {
		t.Fatal(result)
	}
	if c.repoTasks != nil {
		t.Fatal("canceled commit admitted notifications")
	}
	for id := range tx.trees {
		var count int
		if e := db.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&count); e != nil || count != 1 {
			t.Fatal("accepted SQL not committed", count, e)
		}
	}
	recordCloudBusinessEvidence(t, "commit-join.json", map[string]interface{}{"actualSQLBarrier": true, "acceptedFlushJoined": true, "canceledNoticesSuppressed": true})
}

func TestCommitContextCreateCannotOverwriteRememberedExistingSource(t *testing.T) {
	_, _, tx, _, _ := commitFixture(t)
	for _, tree := range tx.trees {
		file := filepath.Join(util.DataDir, tree.Box, tree.Path)
		before, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		tx.DoOperations = []*Operation{{Action: "create", Data: tree}}
		if e := tx.commitLegacy(); e == nil {
			t.Fatal("create overwrote existing remembered source")
		}
		after, e := os.ReadFile(file)
		if e != nil || !bytes.Equal(before, after) {
			t.Fatal("existing source changed", e)
		}
	}
}
