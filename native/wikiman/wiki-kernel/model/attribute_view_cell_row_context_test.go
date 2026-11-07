//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func loadCellRowAV(t *testing.T, file string) *av.AttributeView {
	t.Helper()
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	a, e := av.DecodeAttributeView(raw)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func selectCellData(a *av.AttributeView, value string) map[string]interface{} {
	row := a.GetBlockKeyValues().Values[0].BlockID
	return map[string]interface{}{"keyID": "status", "blockID": row, "type": "select", "mSelect": []interface{}{map[string]interface{}{"content": value}}}
}
func TestAVCellRowBoundSourcePortsCaptureRevisionAndOwner(t *testing.T) {
	for _, kind := range []string{"cell", "row", "bound"} {
		t.Run(kind, func(t *testing.T) {
			_, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
			a.Spec = 3
			before := writeLifecycleAV(t, file, a)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx.environment.ctx = ctx
			var read func(string) (*av.AttributeView, error)
			var save func(*av.AttributeView) error
			switch kind {
			case "cell":
				e := tx.attributeViewCellEnvironment()
				read, save = e.read, e.stage
			case "row":
				e := tx.attributeViewRowEnvironment()
				read, save = e.cell.read, e.cell.stage
			case "bound":
				e := tx.attributeViewBoundEnvironment()
				read, save = e.read, e.stage
			}
			candidate, e := read(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := os.ReadFile(file)
			if candidate.Spec != 3 || !bytes.Equal(raw, before) {
				t.Fatal("eager migration")
			}
			candidate.Name = "captured save"
			if e = save(candidate); e != nil {
				t.Fatal(e)
			}
			saved := loadCellRowAV(t, file)
			if saved.Name != candidate.Name || saved.Spec != 4 {
				t.Fatal("source not saved/upgraded")
			}
			external, _ := os.ReadFile(file)
			external = append(external, '\n')
			os.WriteFile(file, external, 0600)
			candidate.Name = "conflict"
			if e = save(candidate); e == nil {
				t.Fatal("stale source overwritten")
			}
			raw, _ = os.ReadFile(file)
			if !bytes.Equal(raw, external) {
				t.Fatal("external change lost")
			}
			old := util.DataDir
			wrong := t.TempDir()
			util.DataDir = wrong
			_, e = read(a.ID)
			util.DataDir = old
			if e == nil {
				t.Fatal("old owner allowed")
			}
			if _, e = os.Stat(filepath.Join(wrong, "storage")); !os.IsNotExist(e) {
				t.Fatal("wrote changed root")
			}
			cancel()
			if _, e = read(a.ID); !errors.Is(e, context.Canceled) {
				t.Fatal("cancel lost", e)
			}
		})
	}
}

func TestAVCellActualBatchRegroupsEveryEditAcrossLayouts(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, layout)
			row := a.GetBlockKeyValues().Values[0].BlockID
			if _, e := UpdateAttributeViewCell(nil, a.ID, "status", row, selectCellData(a, "Y")); e != nil {
				t.Fatal(e)
			}
			values := []interface{}{map[string]interface{}{"keyID": "status", "itemID": row, "value": selectCellData(a, "X")}, map[string]interface{}{"keyID": "status", "rowID": row, "value": selectCellData(a, "Y")}}
			before, _ := json.Marshal(values)
			if e := BatchUpdateAttributeViewCells(nil, a.ID, values); e != nil {
				t.Fatal(e)
			}
			after, _ := json.Marshal(values)
			if !bytes.Equal(before, after) {
				t.Fatal("mutated batch input")
			}
			saved := loadCellRowAV(t, file)
			if got := saved.GetValue("status", row); got.MSelect[0].Content != "Y" {
				t.Fatal("cell not saved")
			}
			x, y := saved.Views[0].GetGroupByGroupValue("X"), saved.Views[0].GetGroupByGroupValue("Y")
			if y == nil || !reflect.DeepEqual(y.GroupItemIDs, []string{row}) || x == nil || len(x.GroupItemIDs) != 0 {
				t.Fatal("projection reused from earlier edit", x, y)
			}
		})
	}
}

func TestAVMutationNodeReadsUsePendingTreeAndReadonlySource(t *testing.T) {
	_, tx, a, _ := deepAVFixture(t, av.LayoutTypeTable)
	id := a.GetBlockKeyValues().Values[0].Block.ID
	e := tx.attributeViewCellEnvironment()
	n, tree, err := e.node(id)
	if err != nil {
		t.Fatal(err)
	}
	if tree != tx.trees[tree.ID] || !strings.Contains(n.Text(), "changed definition") {
		t.Fatal("pending candidate discarded")
	}
	file := filepath.Join(util.DataDir, tree.Box, tree.Path)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	standalone := (*Transaction)(nil).attributeViewCellEnvironment()
	disk, loaded, err := standalone.node(id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == tree || disk == n {
		t.Fatal("standalone borrowed pending transaction")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("metadata read migrated source")
	}
	oldPath := tree.Path
	tree.Path = "/wrong.docj"
	if _, _, err = e.node(id); err == nil {
		t.Fatal("wrong pending tree identity accepted")
	}
	tree.Path = oldPath
	old := util.DataDir
	util.DataDir = t.TempDir()
	_, _, err = e.node(id)
	util.DataDir = old
	if err == nil {
		t.Fatal("node read switched workspace")
	}
}

func TestAVCellBoundNamesAndAttributeErrorsAreOwned(t *testing.T) {
	_, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
	before, _ := os.ReadFile(file)
	e := tx.attributeViewCellEnvironment()
	label, err := e.names(a.ID)
	if err != nil || !strings.Contains(label, a.Name) {
		t.Fatal("captured names", label, err)
	}
	sentinel := errors.New("attribute publication failure")
	e.attrs = func(_ *ast.Node, _ *parse.Tree, _ map[string]string) error { return sentinel }
	id := a.GetBlockKeyValues().Values[0].Block.ID
	if err = e.binding(a.ID, id, true); !errors.Is(err, sentinel) {
		t.Fatal("binding swallowed failure", err)
	}
	n, tree, err := e.node(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.staticText(n, tree, a.ID, "static"); !errors.Is(err, sentinel) {
		t.Fatal("static text swallowed failure", err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("attribute failure changed AV")
	}
	bound := tx.attributeViewBoundEnvironment()
	os.Remove(file)
	if names, err := bound.names(a.ID); err != nil || names != "" {
		t.Fatal("missing AV name no longer skipped", names, err)
	}
	os.WriteFile(file, []byte("corrupt AV"), 0600)
	if _, err = bound.names(a.ID); err == nil {
		t.Fatal("corrupt bound name failure ignored")
	}
}

func TestAVCellTwoWayActualSourceAndFailedDestination(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "persist", true: "destination-directory-error"}[fail], func(t *testing.T) {
			op, _, src, dest := mutationRelationFixture(t)
			a := loadCellRowAV(t, src)
			k, _ := a.GetKey("rel")
			k.Relation = &av.Relation{AvID: op.ID, BackKeyID: "back", IsTwoWay: true}
			writeLifecycleAV(t, src, a)
			target := loadCellRowAV(t, dest)
			target.KeyValues = append(target.KeyValues, &av.KeyValues{Key: &av.Key{ID: "back", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: a.ID, BackKeyID: "rel", IsTwoWay: true}}})
			writeLifecycleAV(t, dest, target)
			e := (*Transaction)(nil).attributeViewCellEnvironment()
			source, err := e.read(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			row := source.GetBlockKeyValues().Values[0].BlockID
			before, _ := os.ReadFile(src)
			if fail {
				stage := e.stage
				e.stage = func(x *av.AttributeView) error {
					if x.ID == op.ID {
						if err := os.Remove(dest); err != nil {
							return err
						}
						if err := os.Mkdir(dest, 0700); err != nil {
							return err
						}
					}
					return stage(x)
				}
			}
			data := map[string]interface{}{"keyID": "rel", "blockID": row, "type": "relation", "relation": map[string]interface{}{"blockIDs": []string{row, row}}}
			_, err = updateAttributeViewValueWithEnvironment(e, source, "rel", row, data)
			if fail {
				if err == nil {
					t.Fatal("target failure swallowed")
				}
				after, _ := os.ReadFile(src)
				if !bytes.Equal(before, after) {
					t.Fatal("source saved after destination failed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = e.complete(); err != nil {
				t.Fatal(err)
			}
			saved := loadCellRowAV(t, src)
			back := loadCellRowAV(t, dest)
			if !reflect.DeepEqual(saved.GetValue("rel", row).Relation.BlockIDs, []string{row}) || !reflect.DeepEqual(back.GetValue("back", row).Relation.BlockIDs, []string{row}) {
				t.Fatal("two-way/dedup not persisted")
			}
		})
	}
}

func TestAVRowActualInsertReplaceRemoveAcrossLayouts(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			_, tx, a, file := deepAVFixture(t, layout)
			const added = "20261030110001-newrow1"
			input := []map[string]interface{}{{"isDetached": true, "itemID": added, "content": "Detached"}}
			before, _ := json.Marshal(input)
			e := tx.attributeViewRowEnvironment()
			candidate, err := e.cell.read(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.cell.groups(candidate); err != nil {
				t.Fatal(err)
			}
			group := candidate.Views[0].GetGroupByGroupValue("X")
			if group == nil {
				t.Fatal("missing insertion group")
			}
			if err := runAttributeViewRow(&Operation{Action: "insertAttrViewBlock", AvID: a.ID, Srcs: input, GroupID: group.ID, IgnoreDefaultFill: false}, e); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(input)
			if !bytes.Equal(before, after) {
				t.Fatal("input changed")
			}
			saved := loadCellRowAV(t, file)
			if len(saved.GetBlockKeyValues().Values) != 2 || saved.Views[0].ItemIDs[0] != added {
				t.Fatal("insert did not persist")
			}
			if err := runAttributeViewRow(&Operation{Action: "replaceAttrViewBlock", AvID: a.ID, PreviousID: added, IsDetached: true}, tx.attributeViewRowEnvironment()); err != nil {
				t.Fatal(err)
			}
			saved = loadCellRowAV(t, file)
			if v := saved.GetValue("block", added); v == nil || !v.IsDetached || v.Block.ID != "" {
				t.Fatal("replace did not persist")
			}
			// Exercise the original AV removal; history publication has a separate migration.
			e = tx.attributeViewRowEnvironment()
			history := 0
			e.history = func(*av.AttributeView) error { history++; return nil }
			if err := runAttributeViewRow(&Operation{Action: "removeAttrViewBlock", AvID: a.ID, SrcIDs: []string{added}}, e); err != nil {
				t.Fatal(err)
			}
			saved = loadCellRowAV(t, file)
			if saved.GetValue("block", added) != nil || len(saved.GetBlockKeyValues().Values) != 1 || history != 1 {
				t.Fatal("remove not saved/history not called")
			}
		})
	}
}

func TestAVBoundActualCollectionAndPrimaryPruning(t *testing.T) {
	_, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
	e := tx.attributeViewBoundEnvironment()
	trees, nodes, err := collectAttributeViewBoundNodes(a, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || trees[nodes[0].ID] != tx.trees[trees[nodes[0].ID].ID] {
		t.Fatal("bound collection did not reuse candidate")
	}
	n, tree := nodes[0], trees[nodes[0].ID]
	n.SetIALAttr(av.NodeAttrNameAvs, a.ID)
	if err = deleteAttributeViewBindings(n, tree, e); err != nil {
		t.Fatal(err)
	}
	saved := loadCellRowAV(t, file)
	if len(saved.GetBlockKeyValues().Values) != 0 || len(saved.KeyValues[1].Values) != 1 {
		t.Fatal("prune changed non-primary payload")
	}
}

func TestAVBoundActualReadFailuresAndOwnerChangesCannotSkip(t *testing.T) {
	for _, kind := range []string{"known-tree-failure", "changed-owner", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			_, tx, a, _ := deepAVFixture(t, av.LayoutTypeTable)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx.environment.ctx = ctx
			e := tx.attributeViewBoundEnvironment()
			switch kind {
			case "known-tree-failure":
				tx.trees = nil
				id := a.GetBlockKeyValues().Values[0].Block.ID
				_, tree, err := (*Transaction)(nil).attributeViewCellEnvironment().node(id)
				if err != nil {
					t.Fatal(err)
				}
				os.Remove(filepath.Join(util.DataDir, tree.Box, tree.Path))
			case "changed-owner":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "cancel":
				cancel()
			}
			if _, _, err := collectAttributeViewBoundNodes(a, e); err == nil {
				t.Fatal("failed dependency skipped")
			}
		})
	}
}

func TestAVDefaultsTemplateParsingAndReverseUseSharedOwner(t *testing.T) {
	t.Run("parse-only-and-cancel", func(t *testing.T) {
		_, tx, a, _ := deepAVFixture(t, av.LayoutTypeTable)
		a.KeyValues[1].Key.Name = "Status"
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		tx.environment.ctx = ctx
		e := tx.attributeViewRowEnvironment()
		k := &av.Key{ID: "template", Type: av.KeyTypeTemplate, Template: `.action{ querySQL "SELECT * FROM must_not_execute" } .action{.Status}`}
		keys, err := e.defaults.templateKeys(a, k)
		if err != nil || len(keys) != 1 || keys[0].ID != "status" {
			t.Fatal("captured static dependencies", keys, err)
		}
		cancel()
		if _, err = e.defaults.templateKeys(a, k); !errors.Is(err, context.Canceled) {
			t.Fatal("template dependency cancel lost", err)
		}
	})
	t.Run("reverse-failure-returned", func(t *testing.T) {
		op, _, src, dest := mutationRelationFixture(t)
		a := loadCellRowAV(t, src)
		k, _ := a.GetKey("rel")
		k.Relation = &av.Relation{AvID: op.ID, BackKeyID: "back", IsTwoWay: true}
		writeLifecycleAV(t, src, a)
		e := (*Transaction)(nil).attributeViewRowEnvironment()
		if err := os.Remove(dest); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dest, 0700); err != nil {
			t.Fatal(err)
		}
		val := &av.Value{BlockID: a.GetBlockKeyValues().Values[0].BlockID, Relation: &av.ValueRelation{BlockIDs: []string{a.GetBlockKeyValues().Values[0].BlockID}}}
		if err := e.defaults.reverse(a, k, val); err == nil {
			t.Fatal("default reverse swallowed destination IO error")
		}
	})
}

func TestAVMutationPendingNewNodeDoesNotRequireCommittedIndex(t *testing.T) {
	_, tx, a, _ := deepAVFixture(t, av.LayoutTypeTable)
	id := a.GetBlockKeyValues().Values[0].Block.ID
	e := tx.attributeViewCellEnvironment()
	_, tree, err := e.node(id)
	if err != nil {
		t.Fatal(err)
	}
	const newID = "20261030180001-pending"
	n := &ast.Node{Type: ast.NodeParagraph, ID: newID}
	n.SetIALAttr("id", newID)
	tree.Root.AppendChild(n)
	node, loaded, err := e.node(newID)
	if err != nil || node != n || loaded != tree {
		t.Fatal("new pending node discarded", node, err)
	}
	bound := tx.attributeViewBoundEnvironment()
	if yes, err := bound.exists(newID); err != nil || !yes {
		t.Fatal("new pending node invisible to collection", yes, err)
	}
	if loaded, err := bound.load(newID); err != nil || loaded != tree {
		t.Fatal("new node tree load", err)
	}
}

func TestAVRowBatchCompletesOnlyAfterFinalSourceSave(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "save-failure"}[fail], func(t *testing.T) {
			_, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
			e := tx.attributeViewRowEnvironment()
			row := a.GetBlockKeyValues().Values[0].BlockID
			completed := 0
			complete := e.cell.complete
			e.cell.complete = func() error { completed++; return complete() }
			if fail {
				stage := e.cell.stage
				e.cell.stage = func(candidate *av.AttributeView) error {
					if err := os.Remove(file); err != nil {
						return err
					}
					if err := os.Mkdir(file, 0700); err != nil {
						return err
					}
					return stage(candidate)
				}
			}
			err := batchReplaceAttributeViewBlocksWithEnvironment(a.ID, true, []map[string]string{{row: ""}, {row: ""}}, e)
			if fail {
				if err == nil || completed != 0 {
					t.Fatal("failed batch completed", err, completed)
				}
				return
			}
			if err != nil || completed != 1 {
				t.Fatal("batch completion", err, completed)
			}
			if v := loadCellRowAV(t, file).GetValue("block", row); v == nil || !v.IsDetached {
				t.Fatal("batch not saved")
			}
		})
	}
}
