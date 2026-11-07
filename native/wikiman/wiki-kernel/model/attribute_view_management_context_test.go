//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/util"
)

func managementContextFixture(t *testing.T, layout av.LayoutType) (*Transaction, *av.AttributeView, string, *ast.Node) {
	t.Helper()
	c, tx, a, file := deepAVFixture(t, layout)
	if c.avLabels == nil {
		c.avLabels = map[string]string{}
	}
	for key, value := range map[string]string{"table": "Owner table", "gallery": "Owner gallery", "kanban": "Owner kanban"} {
		c.avLabels[key] = value
	}
	// Document effects remain transaction candidates; AV IO below is real rooted IO.
	tx.environment.stage = func(*parse.Tree) error { return nil }
	row := a.GetBlockKeyValues().Values[0].Block.ID
	cell := tx.attributeViewCellEnvironment()
	_, tree, err := cell.node(row)
	if err != nil {
		t.Fatal(err)
	}
	n := &ast.Node{Type: ast.NodeAttributeView, ID: "20261030193000-mirror1", AttributeViewID: a.ID, AttributeViewType: string(layout)}
	n.SetIALAttr("id", n.ID)
	n.SetIALAttr(av.NodeAttrView, a.ViewID)
	tree.Root.AppendChild(n)
	raw, err := msgpack.Marshal(map[string][]string{a.ID: {n.ID, n.ID, "20261030193001-missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return tx, a, file, n
}
func TestAVManagementNameSourcePortsCheckRevisionAndContext(t *testing.T) {
	for _, kind := range []string{"management", "name"} {
		t.Run(kind, func(t *testing.T) {
			tx, a, file, _ := managementContextFixture(t, av.LayoutTypeTable)
			a.Spec = 3
			before := writeLifecycleAV(t, file, a)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx.environment.ctx = ctx
			var read func(string) (*av.AttributeView, error)
			var save func(*av.AttributeView) error
			if kind == "management" {
				e := tx.attributeViewManagementEnvironment()
				read, save = e.read, e.stage
			} else {
				e := tx.attributeViewNameEnvironment()
				read, save = e.read, e.stage
			}
			candidate, err := read(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(file)
			if candidate.Spec != 3 || !bytes.Equal(before, raw) {
				t.Fatal("read upgraded source")
			}
			candidate.Name = "saved"
			if err = save(candidate); err != nil {
				t.Fatal(err)
			}
			if loadCellRowAV(t, file).Spec != 4 {
				t.Fatal("save did not upgrade")
			}
			raw, _ = os.ReadFile(file)
			external := append(raw, '\n')
			os.WriteFile(file, external, 0600)
			if err = save(candidate); err == nil {
				t.Fatal("revision conflict overwritten")
			}
			raw, _ = os.ReadFile(file)
			if !bytes.Equal(raw, external) {
				t.Fatal("external change lost")
			}
			old := util.DataDir
			util.DataDir = t.TempDir()
			_, err = read(a.ID)
			util.DataDir = old
			if err == nil {
				t.Fatal("owner change accepted")
			}
			cancel()
			if _, err = read(a.ID); !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation ignored", err)
			}
		})
	}
}
func TestAVManagementDefaultsCaptureLanguageAndOriginalShape(t *testing.T) {
	tx, _, _, _ := managementContextFixture(t, av.LayoutTypeTable)
	e := tx.attributeViewManagementEnvironment()
	oldLang, oldLabels := util.Lang, util.AttrViewLangs
	defer func() { util.Lang, util.AttrViewLangs = oldLang, oldLabels }()
	util.Lang = "other"
	util.AttrViewLangs = map[string]map[string]interface{}{"other": {"table": "Wrong", "gallery": "Wrong", "kanban": "Wrong"}}
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		v := e.makeView(layout)
		if v == nil || v.Name != "Owner "+string(layout) || v.LayoutType != layout || v.PageSize != av.ViewDefaultPageSize || len(v.Filters) != 0 || len(v.Sorts) != 0 {
			t.Fatal("factory followed global language", v)
		}
		switch layout {
		case av.LayoutTypeTable:
			if !v.Table.ShowIcon || v.Table.Spec != 0 {
				t.Fatal("table defaults")
			}
		case av.LayoutTypeGallery:
			if !v.Gallery.ShowIcon || v.Gallery.CoverFrom != av.CoverFromContentImage || v.Gallery.CardAspectRatio != av.CardAspectRatio16_9 || v.Gallery.CardSize != av.CardSizeMedium {
				t.Fatal("gallery defaults")
			}
		case av.LayoutTypeKanban:
			if !v.Kanban.ShowIcon || v.Kanban.CoverFrom != av.CoverFromContentImage || v.Kanban.CardAspectRatio != av.CardAspectRatio16_9 || v.Kanban.CardSize != av.CardSizeMedium {
				t.Fatal("kanban defaults")
			}
		}
	}
}
func TestAVManagementMirrorReadsUseRootAndPendingNodes(t *testing.T) {
	tx, a, file, n := managementContextFixture(t, av.LayoutTypeTable)
	e := tx.attributeViewManagementEnvironment()
	ids, err := e.mirrors(a.ID)
	if err != nil || !reflect.DeepEqual(ids, []string{n.ID}) {
		t.Fatal("mirror set/dedupe/pending", ids, err)
	}
	registry := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	os.Remove(registry)
	if ids, err = e.mirrors(a.ID); err != nil || len(ids) != 0 {
		t.Fatal("missing registry not empty", ids, err)
	}
	for _, bad := range [][]byte{[]byte("corrupt"), {0xc0}} {
		os.WriteFile(registry, bad, 0600)
		if _, err = e.mirrors(a.ID); err == nil {
			t.Fatal("corrupt registry accepted")
		}
		after, _ := os.ReadFile(registry)
		if !bytes.Equal(bad, after) {
			t.Fatal("registry overwritten")
		}
	}
	os.Remove(registry)
	external := filepath.Join(t.TempDir(), "external.msgpack")
	raw, _ := msgpack.Marshal(map[string][]string{a.ID: {n.ID}})
	os.WriteFile(external, raw, 0600)
	os.Symlink(external, registry)
	if _, err = e.mirrors(a.ID); err == nil {
		t.Fatal("linked registry accepted")
	}
}
func TestAVManagementActualViewLifecycleAcrossLayouts(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			tx, a, file, n := managementContextFixture(t, layout)
			run := func(op *Operation) {
				t.Helper()
				op.AvID = a.ID
				op.BlockID = n.ID
				if err := runAttributeViewManagement(op, tx.attributeViewManagementEnvironment()); err != nil {
					t.Fatal(op.Action, err)
				}
			}
			run(&Operation{Action: "setAttrViewShowIcon", Data: false})
			saved := loadCellRowAV(t, file)
			v := saved.Views[0]
			show := false
			switch v.LayoutType {
			case av.LayoutTypeTable:
				show = v.Table.ShowIcon
			case av.LayoutTypeGallery:
				show = v.Gallery.ShowIcon
			case av.LayoutTypeKanban:
				show = v.Kanban.ShowIcon
			}
			if show {
				t.Fatal("setting not saved")
			}
			const added = "20261030194000-newview"
			run(&Operation{Action: "addAttrViewView", ID: added, Layout: av.LayoutTypeGallery})
			saved = loadCellRowAV(t, file)
			if saved.GetView(added) == nil || saved.ViewID != added || n.IALAttr(av.NodeAttrView) != added {
				t.Fatal("new view not selected/saved")
			}
			run(&Operation{Action: "setAttrViewViewName", ID: added, Data: "Renamed view"})
			const cloned = "20261030194001-copyv01"
			run(&Operation{Action: "duplicateAttrViewView", ID: cloned, PreviousID: added})
			saved = loadCellRowAV(t, file)
			if saved.GetView(cloned) == nil || len(saved.Views) != 3 {
				t.Fatal("clone not saved")
			}
			run(&Operation{Action: "changeAttrViewLayout", Layout: av.LayoutTypeKanban})
			saved = loadCellRowAV(t, file)
			if saved.GetView(cloned).LayoutType != av.LayoutTypeKanban || n.AttributeViewType != "kanban" {
				t.Fatal("layout/mirror not updated")
			}
			run(&Operation{Action: "removeAttrViewView"})
			saved = loadCellRowAV(t, file)
			if len(saved.Views) != 2 || saved.GetView(cloned) != nil || n.IALAttr(av.NodeAttrView) == cloned {
				t.Fatal("removed view mirror not redirected")
			}
		})
	}
}
func TestAVManagementFailuresDoNotScheduleSuccess(t *testing.T) {
	for _, mode := range []string{"source-write", "bad-mirror-index", "known-mirror-source", "mirror-attribute"} {
		t.Run(mode, func(t *testing.T) {
			tx, a, file, n := managementContextFixture(t, av.LayoutTypeTable)
			e := tx.attributeViewManagementEnvironment()
			before, _ := os.ReadFile(file)
			op := &Operation{Action: "changeAttrViewLayout", AvID: a.ID, BlockID: n.ID, Layout: av.LayoutTypeGallery}
			switch mode {
			case "source-write":
				stage := e.stage
				e.stage = func(a *av.AttributeView) error { os.Remove(file); os.Mkdir(file, 0700); return stage(a) }
			case "bad-mirror-index":
				os.WriteFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"), []byte("bad index"), 0600)
			case "known-mirror-source":
				row := a.GetBlockKeyValues().Values[0].Block.ID
				raw, _ := msgpack.Marshal(map[string][]string{a.ID: {row}})
				os.WriteFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"), raw, 0600)
				_, tree, err := e.node(row)
				if err != nil {
					t.Fatal(err)
				}
				op.BlockID = ""
				tx.trees = nil
				os.Remove(filepath.Join(util.DataDir, tree.Box, tree.Path))
			case "mirror-attribute":
				e.attrs = func(*ast.Node, *parse.Tree, map[string]string) error { return errors.New("mirror write failure") }
			}
			if err := runAttributeViewManagement(op, e); err == nil {
				t.Fatal("operation failure lost")
			}
			Conf.repoMu.Lock()
			scheduled := Conf.repoTasks != nil
			Conf.repoMu.Unlock()
			if scheduled {
				t.Fatal("failed operation admitted success notice")
			}
			if mode == "bad-mirror-index" || mode == "known-mirror-source" {
				after, _ := os.ReadFile(file)
				if !bytes.Equal(before, after) {
					t.Fatal("source saved after dependency failed")
				}
			}
		})
	}
}
func TestAVNameActualRulesPendingNodesAndCachePublication(t *testing.T) {
	tx, a, file, _ := managementContextFixture(t, av.LayoutTypeTable)
	row := a.GetBlockKeyValues().Values[0].Block.ID
	e := tx.attributeViewNameEnvironment()
	n, tree, err := e.node(row)
	if err != nil {
		t.Fatal(err)
	}
	n.SetIALAttr(av.NodeAttrNameAvs, a.ID+",20261030195500-missing,"+a.ID)
	originalNotify := e.notify
	notified := false
	e.notify = func(old map[string]string, node *ast.Node) error {
		if cache.GetBlockIAL(node.ID)[av.NodeAttrViewNames] != node.IALAttr(av.NodeAttrViewNames) {
			t.Fatal("cache publication follows notification")
		}
		notified = true
		return originalNotify(old, node)
	}
	name := "  标题\n" + strings.Repeat("长", 520)
	if err = runAttributeViewName(&Operation{Action: "setAttrViewName", ID: a.ID, Data: name}, e); err != nil {
		t.Fatal(err)
	}
	saved := loadCellRowAV(t, file)
	if len([]rune(saved.Name)) != 512 || strings.Contains(saved.Name, "\n") || !notified || !strings.Contains(n.IALAttr(av.NodeAttrViewNames), saved.Name) {
		t.Fatal("name rules/cache not preserved")
	}
	if tree != tx.trees[tree.ID] {
		t.Fatal("pending tree replaced")
	}
	var doc map[string]interface{}
	raw, _ := os.ReadFile(filepath.Join(util.DataDir, tree.Box, tree.Path))
	json.Unmarshal(raw, &doc)
	if bytes.Contains(raw, []byte(saved.Name)) {
		t.Fatal("derived name persisted via general attribute writer")
	}
}
func TestAVNameKnownBindingFailureIsReportedAfterPartialSave(t *testing.T) {
	tx, a, file, _ := managementContextFixture(t, av.LayoutTypeTable)
	e := tx.attributeViewNameEnvironment()
	row := a.GetBlockKeyValues().Values[0].Block.ID
	_, tree, err := e.node(row)
	if err != nil {
		t.Fatal(err)
	}
	tx.trees = nil
	os.Remove(filepath.Join(util.DataDir, tree.Box, tree.Path))
	calls := 0
	e.notify = func(map[string]string, *ast.Node) error { calls++; return nil }
	if err = runAttributeViewName(&Operation{Action: "setAttrViewName", ID: a.ID, Data: "Partial saved name"}, e); err == nil {
		t.Fatal("known source error swallowed")
	}
	if loadCellRowAV(t, file).Name != "Partial saved name" || calls != 0 {
		t.Fatal("partial save/notification contract changed")
	}
}
func TestAVManagementMirrorSetParity(t *testing.T) {
	tx, a, file, n := managementContextFixture(t, av.LayoutTypeTable)
	raw, _ := msgpack.Marshal(map[string][]string{a.ID: {n.ID, a.GetBlockKeyValues().Values[0].Block.ID, n.ID, "20261030193001-missing"}})
	os.WriteFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"), raw, 0600)
	ids, err := tx.attributeViewManagementEnvironment().mirrors(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	want := []string{n.ID, a.GetBlockKeyValues().Values[0].Block.ID}
	sort.Strings(want)
	if !reflect.DeepEqual(ids, want) {
		t.Fatal("original mirror set changed", ids, want)
	}
}
