//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func referenceModelFixture(t *testing.T) (*AppConf, *indexRepairContext, string, string, []byte) {
	t.Helper()
	c, _, p, _ := restoreFixture(t)
	c.Search = conf.NewSearch()
	c.Editor = conf.NewEditor()
	c.FileTree = conf.NewFileTree()
	oldDB, oldHistory, oldAsset, oldTree := util.DBPath, util.HistoryDBPath, util.AssetContentDBPath, util.BlockTreeDBPath
	base := t.TempDir()
	util.DBPath = filepath.Join(base, "main.db")
	util.HistoryDBPath = filepath.Join(base, "history.db")
	util.AssetContentDBPath = filepath.Join(base, "asset.db")
	util.BlockTreeDBPath = filepath.Join(base, "tree.db")
	t.Cleanup(func() {
		c.CloseCloudRepo(context.Background())
		sql.CloseDatabase()
		util.DBPath, util.HistoryDBPath, util.AssetContentDBPath, util.BlockTreeDBPath = oldDB, oldHistory, oldAsset, oldTree
	})
	if e := sql.InitDatabase(true); e != nil {
		t.Fatal(e)
	}
	sql.InitHistoryDatabase(false)
	sql.InitAssetContentDatabase(false)
	box := "20261028140000-refbox1"
	defdoc := "20261028140001-refdoc1"
	def := "20261028140002-refdef1"
	using := "20261028140003-refdoc2"
	para := "20261028140004-refpar1"
	timeRaw, e := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if e != nil {
		t.Fatal(e)
	}
	var timeLabels struct {
		Time map[string]string `json:"_time"`
	}
	if e = json.Unmarshal(timeRaw, &timeLabels); e != nil {
		t.Fatal(e)
	}
	c.business = conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return []conf.Notebook{{ID: box}}, nil }, nil, nil, map[string]map[string]string{c.Lang: timeLabels.Time})
	if e := os.MkdirAll(filepath.Join(p.Data, box), 0700); e != nil {
		t.Fatal(e)
	}
	makeDoc := func(id, block string, children []interface{}) map[string]interface{} {
		return map[string]interface{}{"ID": id, "Spec": "1", "Type": "NodeDocument", "Properties": map[string]string{"id": id, "title": "reference fixture", "updated": "20261028140000"}, "Children": []interface{}{map[string]interface{}{"ID": block, "Type": "NodeParagraph", "Properties": map[string]string{"id": block, "updated": "20261028140000"}, "Children": children}}}
	}
	sources := map[string]map[string]interface{}{defdoc: makeDoc(defdoc, def, []interface{}{map[string]string{"Type": "NodeText", "Data": "definition"}}), using: makeDoc(using, para, []interface{}{map[string]string{"Type": "NodeTextMark", "TextMarkType": "block-ref", "TextMarkBlockRefID": def, "TextMarkBlockRefSubtype": "d", "TextMarkTextContent": "definition"}})}
	var source []byte
	for id, d := range sources {
		raw, _ := json.Marshal(d)
		if e := os.WriteFile(filepath.Join(p.Data, box, id+".docj"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if id == using {
			source = raw
		}
	}
	for id := range sources {
		tree, e := filesys.LoadTree(box, "/"+id+".docj", util.NewLute())
		if e != nil {
			t.Fatal(e)
		}
		treenode.UpsertBlockTree(tree)
		sql.IndexTreeQueue(tree)
	}
	if e := sql.FlushQueueContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, e := captureIndexRepair(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	source, e = os.ReadFile(filepath.Join(p.Data, box, using+".docj"))
	if e != nil {
		t.Fatal(e)
	}
	return c, r, def, filepath.Join(p.Data, box, using+".docj"), source
}
func TestReferenceRefreshCompatibilityActualFailureRetryAndOwnedDelay(t *testing.T) {
	c, r, def, file, before := referenceModelFixture(t)
	db := r.main.DB
	if _, e := db.Exec("CREATE TRIGGER fail_refs BEFORE INSERT ON refs BEGIN SELECT RAISE(ABORT,'fixture failure'); END"); e != nil {
		t.Fatal(e)
	}
	if e := RefreshBacklinkContext(context.Background(), def); e == nil {
		t.Fatal("reference write failure swallowed")
	}
	if _, e := db.Exec("DROP TRIGGER fail_refs"); e != nil {
		t.Fatal(e)
	}
	if e := RefreshBacklinkContext(context.Background(), def); e != nil {
		t.Fatal(e)
	}
	count, e := r.referenceCount(context.Background(), def)
	if e != nil || count == nil || count.Count != 1 {
		t.Fatal(count, e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("derived refresh changed docj", e)
	}
	if e = c.scheduleReferenceCounts(context.Background(), r, []string{def}, time.Hour); e != nil {
		t.Fatal(e)
	}
	c.repoMu.Lock()
	prior := c.repoReferenceCounts[def]
	c.repoMu.Unlock()
	if e = c.scheduleReferenceCounts(context.Background(), r, []string{def}, time.Hour); e != nil {
		t.Fatal(e)
	}
	c.repoMu.Lock()
	latest := c.repoReferenceCounts[def]
	c.repoMu.Unlock()
	if prior == nil || prior == latest || prior.ctx.Err() == nil {
		t.Fatal("per-block debounce did not replace/cancel old timer")
	}
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e = c.CloseCloudRepo(timeout); e != nil {
		t.Fatal("delayed count task not canceled/joined", e)
	}
	recordCloudBusinessEvidence(t, "reference-compat.json", map[string]interface{}{"actualWriteFailureReturned": true, "sameOwnerRetry": true, "committedCount": 1, "sourceUnchanged": true, "longDelayCanceledAndJoined": true, "ownerBlockDebounce": true})
}
func TestIndexRepairDrainFailureNeverCompletes(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	_ = c
	marker := false
	e := runIndexRepairStages(context.Background(), []func(context.Context) error{func(context.Context) error {
		sql.UpdateBlockContentQueue(&sql.Block{ID: "missing", Content: "pending"})
		return nil
	}}, func() error { return indexdb.ErrClosed }, func() error { marker = true; return nil })
	if e == nil || marker {
		t.Fatal("failed drain consumed successful repair marker", e, marker)
	}
	sql.ClearQueue()
}
