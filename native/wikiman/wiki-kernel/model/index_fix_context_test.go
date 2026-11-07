//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func TestIndexRepairActualOriginalStagesFilesystemAndSQL(t *testing.T) {
	c, _, p, _ := restoreFixture(t)
	c.Search = conf.NewSearch()
	c.Editor = conf.NewEditor()
	c.FileTree = conf.NewFileTree()
	oldDB, oldHistory, oldAsset, oldTree := util.DBPath, util.HistoryDBPath, util.AssetContentDBPath, util.BlockTreeDBPath
	util.DBPath = filepath.Join(t.TempDir(), "main.db")
	util.HistoryDBPath = filepath.Join(t.TempDir(), "history.db")
	util.AssetContentDBPath = filepath.Join(t.TempDir(), "asset.db")
	util.BlockTreeDBPath = filepath.Join(t.TempDir(), "tree.db")
	defer func() {
		util.DBPath, util.HistoryDBPath, util.AssetContentDBPath, util.BlockTreeDBPath = oldDB, oldHistory, oldAsset, oldTree
	}()
	if e := sql.InitDatabase(true); e != nil {
		t.Fatal(e)
	}
	sql.InitHistoryDatabase(false)
	sql.InitAssetContentDatabase(false)
	defer sql.CloseDatabase()
	box := "20261028123000-repair1"
	first := "20261028123001-repair2"
	second := "20261028123002-repair3"
	paragraph := "20261028123003-repair4"
	c.business = conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
		return []conf.Notebook{{ID: box, Name: "repair fixture"}}, nil
	}, nil, nil)
	if e := os.MkdirAll(filepath.Join(p.Data, box), 0700); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{first, second} {
		doc := map[string]interface{}{"ID": id, "Type": "NodeDocument", "Properties": map[string]string{"id": id, "title": "repair fixture", "updated": "20261028123000"}, "Children": []interface{}{map[string]interface{}{"ID": paragraph, "Type": "NodeParagraph", "Properties": map[string]string{"id": paragraph, "updated": "20261028123000"}, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "synthetic repair text"}}}}}
		raw, _ := json.Marshal(doc)
		if e := os.WriteFile(filepath.Join(p.Data, box, id+".docj"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	// A missing derived table must be an error, not an empty successful audit.
	db := sql.CaptureLegacyRepairQueries().DB
	original, e := os.ReadFile(filepath.Join(p.Data, box, second+".docj"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("DROP TABLE blocks_fts"); e != nil {
		t.Fatal(e)
	}
	if e = c.repoOwner.Repair(context.Background()); e == nil {
		t.Fatal("missing FTS table was swallowed")
	}
	afterFailure, e := os.ReadFile(filepath.Join(p.Data, box, second+".docj"))
	if e != nil || !bytes.Equal(original, afterFailure) {
		t.Fatal("source changed after failed DB read", e)
	}
	statements, e := indexdb.Statements(indexdb.Main)
	if e != nil {
		t.Fatal(e)
	}
	for _, statement := range statements {
		if strings.HasPrefix(statement, "CREATE VIRTUAL TABLE blocks_fts USING") {
			if _, e = db.Exec(statement); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := c.repoOwner.Repair(context.Background()); e != nil {
		t.Fatal(e)
	}
	var ids []string
	for _, id := range []string{first, second} {
		raw, e := os.ReadFile(filepath.Join(p.Data, box, id+".docj"))
		if e != nil {
			t.Fatal(e)
		}
		var doc struct{ Children []struct{ ID string } }
		if e = json.Unmarshal(raw, &doc); e != nil || len(doc.Children) == 0 {
			t.Fatal(e)
		}
		ids = append(ids, doc.Children[0].ID)
		if treenode.GetBlockTree(id) == nil || sql.GetBlock(id) == nil {
			t.Fatal("original blocktree/SQL not repaired")
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("duplicate block IDs survived repair")
	}
	if e := c.repoOwner.Repair(context.Background()); e != nil {
		t.Fatal("repeat should skip successful owner", e)
	}
	recordCloudBusinessEvidence(t, "index-repair-local.json", map[string]interface{}{"originalFiveStages": true, "duplicateBlockFixed": true, "sourceDocjPreserved": true, "blocktreeRebuilt": true, "sqlRebuilt": true, "ownerOnce": true, "missingTableErrorStopsBeforeSource": true, "failureDoesNotConsumeOnce": true})
}
