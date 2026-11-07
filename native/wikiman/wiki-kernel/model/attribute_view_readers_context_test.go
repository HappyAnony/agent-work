//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func readerContextFixture(t *testing.T, layout av.LayoutType) (*Transaction, *av.AttributeView, string) {
	t.Helper()
	c, tx, a, file := deepAVFixture(t, layout)
	c.avLabels = map[string]string{}
	for k, v := range util.AttrViewLangs[util.Lang] {
		if s, ok := v.(string); ok {
			c.avLabels[k] = s
		}
	}
	row := a.GetBlockKeyValues().Values[0].BlockID
	a.GetBlockKeyValues().Values[0].IsDetached = true
	for _, id := range []string{"visible", "secret"} {
		a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: id, Type: av.KeyTypeMAsset}, Values: []*av.Value{{ID: id + "-value", KeyID: id, BlockID: row, Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Type: av.AssetTypeImage, Content: "assets/" + id + ".png"}, {Type: av.AssetTypeFile, Content: "assets/skip.pdf"}}}}})
		appendReaderField(a.Views[0], id, id == "secret")
	}
	b := legacyAVBindingForTransaction(tx)
	if err := b.groups(a); err != nil {
		t.Fatal(err)
	}
	a.Spec = 3
	appendReaderField(a.Views[0], "missing-field", false)
	v := a.Views[0]
	switch v.LayoutType {
	case av.LayoutTypeTable:
		f := v.Table.Columns
		v.Table.Columns = append(append(append([]*av.ViewTableColumn{}, f[:2]...), f[len(f)-1]), f[2:len(f)-1]...)
	case av.LayoutTypeGallery:
		f := v.Gallery.CardFields
		v.Gallery.CardFields = append(append(append([]*av.ViewGalleryCardField{}, f[:2]...), f[len(f)-1]), f[2:len(f)-1]...)
	case av.LayoutTypeKanban:
		f := v.Kanban.Fields
		v.Kanban.Fields = append(append(append([]*av.ViewKanbanField{}, f[:2]...), f[len(f)-1]), f[2:len(f)-1]...)
	}
	writeLifecycleAV(t, file, a)
	return tx, a, file
}
func appendReaderField(v *av.View, id string, hidden bool) {
	f := &av.BaseField{ID: id, Hidden: hidden}
	switch v.LayoutType {
	case av.LayoutTypeTable:
		v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: f})
	case av.LayoutTypeGallery:
		v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: f})
	case av.LayoutTypeKanban:
		v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: f})
	}
}
func TestAVReaderQueriesAcrossLayoutsDoNotWriteRepairs(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			tx, a, file := readerContextFixture(t, layout)
			before, _ := os.ReadFile(file)
			group := a.Views[0].GetGroupByGroupValue("X")
			if group == nil {
				t.Fatal("missing group")
			}
			defaults, err := tx.attrViewAddingBlockDefaultValues(a.ID, a.ViewID, group.ID, "", "20261030210000-newrow1")
			if err != nil {
				t.Fatal(err)
			}
			if defaults["status"] == nil || defaults["status"].MSelect[0].Content != "X" {
				t.Fatal("original group defaults changed", defaults)
			}
			images, err := tx.currentAttributeViewImages(a.ID, a.ViewID, "")
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(images)
			want := []string{"assets/visible.png"}
			if layout != av.LayoutTypeTable {
				want = []string{"assets/secret.png", "assets/visible.png"}
			}
			if !reflect.DeepEqual(images, want) {
				t.Fatal("original image projection/hidden rules", images, want)
			}
			row := a.GetBlockKeyValues().Values[0]
			ids, err := tx.attributeViewItemIDs(a.ID, []string{row.Block.ID})
			if err != nil || ids[row.Block.ID] != row.BlockID {
				t.Fatal("identity map changed", ids, err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("read query published missing-field repair/upgrade")
			}
			Conf.repoMu.Lock()
			scheduled := Conf.repoTasks != nil
			Conf.repoMu.Unlock()
			if scheduled {
				t.Fatal("read query scheduled write notification")
			}
		})
	}
}
func TestAVReaderBindingsRejectChangedOwnersAndCancel(t *testing.T) {
	for _, kind := range []string{"preview", "images", "identity"} {
		t.Run(kind, func(t *testing.T) {
			tx, a, file := readerContextFixture(t, av.LayoutTypeTable)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx.environment.ctx = ctx
			var read func(string) (*av.AttributeView, error)
			switch kind {
			case "preview":
				read = tx.attributeViewPreviewEnvironment().read
			case "images":
				read = tx.attributeViewImagesEnvironment().read
			case "identity":
				read = tx.attributeViewIdentityEnvironment().read
			}
			before, _ := os.ReadFile(file)
			old := util.DataDir
			wrong := t.TempDir()
			util.DataDir = wrong
			_, err := read(a.ID)
			util.DataDir = old
			if err == nil {
				t.Fatal("old owner accepted")
			}
			if _, err = os.Stat(filepath.Join(wrong, "storage")); !os.IsNotExist(err) {
				t.Fatal("wrote changed root")
			}
			cancel()
			if _, err = read(a.ID); !errors.Is(err, context.Canceled) {
				t.Fatal("cancel lost", err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("failed read changed source")
			}
		})
	}
}
func TestAVReadonlyBindingCannotRegainWriteCapabilities(t *testing.T) {
	tx, a, file := readerContextFixture(t, av.LayoutTypeTable)
	b := readonlyAVBinding(tx, context.Background())
	before, _ := os.ReadFile(file)
	for _, bound := range []*legacyAVBinding{b, b.withContext(context.Background())} {
		candidate, err := bound.readCandidate(a.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = bound.save(candidate); err == nil {
			t.Fatal("readonly save accepted")
		}
		if err = bound.create(candidate); err == nil {
			t.Fatal("readonly create accepted")
		}
		if err = bound.upsertRelation(a.ID, a.ID); err == nil {
			t.Fatal("readonly relation write accepted")
		}
		if err = bound.upsertMirror(a.ID, "20261030210100-mirror1"); err == nil {
			t.Fatal("readonly mirror write accepted")
		}
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("readonly scope changed source")
	}
	for _, name := range []string{"blocks.msgpack", "relations.msgpack"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(file), name)); !os.IsNotExist(err) {
			t.Fatal("readonly created registry", name, err)
		}
	}
}
func TestAVReaderActualSQLFailuresRemainReadonly(t *testing.T) {
	tx, a, file := readerContextFixture(t, av.LayoutTypeTable)
	a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "computed", Type: av.KeyTypeTemplate, Template: `.action{ querySQL "SELECT * FROM missing_reader_table" }`}})
	appendReaderField(a.Views[0], "computed", false)
	before := writeLifecycleAV(t, file, a)
	group := a.Views[0].GetGroupByGroupValue("X")
	if _, err := tx.attrViewAddingBlockDefaultValues(a.ID, a.ViewID, group.ID, "", "20261030210000-newrow1"); err == nil {
		t.Fatal("preview SQL failure swallowed")
	}
	if _, err := tx.currentAttributeViewImages(a.ID, a.ViewID, ""); err == nil {
		t.Fatal("images SQL failure swallowed")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("failed render wrote repair")
	}
}
func TestAVBlockPanelActualSourceKeysAndMirrorRepair(t *testing.T) {
	tx, a, file, n := managementContextFixture(t, av.LayoutTypeTable)
	row := a.GetBlockKeyValues().Values[0]
	node, _, err := tx.attributeViewCellEnvironment().node(row.Block.ID)
	if err != nil {
		t.Fatal(err)
	}
	node.SetIALAttr(av.NodeAttrNameAvs, a.ID)
	registry := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	os.Remove(registry)
	e := tx.attributeViewBlockKeysEnvironment()
	out, err := blockAttributeViewKeysWithEnvironment(row.Block.ID, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].AvID != a.ID || !reflect.DeepEqual(out[0].BlockIDs, []string{n.ID}) {
		t.Fatal("panel keys/mirror recovery", out)
	}
	var rel map[string][]string
	raw, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err = msgpack.Unmarshal(raw, &rel); err != nil || !reflect.DeepEqual(rel[a.ID], []string{n.ID}) {
		t.Fatal("fixed-root mirror repair missing", rel, err)
	}
	saved := loadCellRowAV(t, file)
	if len(saved.KeyIDs) == 0 {
		t.Fatal("original key-order repair not saved")
	}
}
func TestAVBlockPanelSourceErrorsCannotMasqueradeAsMissing(t *testing.T) {
	for _, mode := range []string{"corrupt-av", "save-failure", "bad-registry", "cancel", "changed-owner"} {
		t.Run(mode, func(t *testing.T) {
			tx, a, file, _ := managementContextFixture(t, av.LayoutTypeTable)
			row := a.GetBlockKeyValues().Values[0]
			node, _, err := tx.attributeViewCellEnvironment().node(row.Block.ID)
			if err != nil {
				t.Fatal(err)
			}
			node.SetIALAttr(av.NodeAttrNameAvs, a.ID)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx.environment.ctx = ctx
			e := tx.attributeViewBlockKeysEnvironment()
			switch mode {
			case "corrupt-av":
				os.WriteFile(file, []byte("corrupt"), 0600)
			case "save-failure":
				stage := e.stage
				e.stage = func(a *av.AttributeView) error { os.Remove(file); os.Mkdir(file, 0700); return stage(a) }
			case "bad-registry":
				os.WriteFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"), []byte("bad registry"), 0600)
			case "cancel":
				cancel()
			case "changed-owner":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			}
			if _, err = blockAttributeViewKeysWithEnvironment(row.Block.ID, e); err == nil {
				t.Fatal("panel failure skipped")
			}
		})
	}
}
func TestAVBlockPanelMirrorNullDoesNotPanicOrOverwrite(t *testing.T) {
	tx, a, file, n := managementContextFixture(t, av.LayoutTypeTable)
	registry := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	null := []byte{0xc0}
	os.WriteFile(registry, null, 0600)
	if err := legacyAVBindingForTransaction(tx).upsertMirror(a.ID, n.ID); err == nil {
		t.Fatal("null registry accepted")
	}
	after, _ := os.ReadFile(registry)
	if !bytes.Equal(null, after) {
		t.Fatal("null registry overwritten")
	}
}
func TestAVReaderIdentityUsesOriginalBoundAndRowIDs(t *testing.T) {
	tx, a, file := readerContextFixture(t, av.LayoutTypeTable)
	v := a.GetBlockKeyValues().Values[0]
	v.BlockID = "20261030210200-rowtest"
	v.IsDetached = false
	writeLifecycleAV(t, file, a)
	out, err := tx.attributeViewItemIDs(a.ID, []string{v.Block.ID, "missing"})
	if err != nil || out[v.Block.ID] != v.BlockID || out["missing"] != "" {
		t.Fatal("to rows", out, err)
	}
	out, err = tx.attributeViewBoundBlockIDs(a.ID, []string{v.BlockID, "missing"})
	if err != nil || out[v.BlockID] != v.Block.ID || out["missing"] != "" {
		t.Fatal("to blocks", out, err)
	}
}

func TestAVImagesFurtherSkipsMissingButReturnsCorruptSources(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "corrupt"}[broken], func(t *testing.T) {
			tx, a, file := readerContextFixture(t, av.LayoutTypeTable)
			const dest = "20261030210300-related"
			a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "link", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: dest}}}, &av.KeyValues{Key: &av.Key{ID: "sum", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: "link", KeyID: "target", Calc: &av.RollupCalc{Operator: av.CalcOperatorNone}}}})
			before := writeLifecycleAV(t, file, a)
			if broken {
				os.WriteFile(filepath.Join(filepath.Dir(file), dest+".json"), []byte("corrupt related source"), 0600)
			}
			images, err := tx.currentAttributeViewImages(a.ID, a.ViewID, "")
			if broken {
				if err == nil {
					t.Fatal("corrupt related source ignored")
				}
			} else if err != nil || !reflect.DeepEqual(images, []string{"assets/visible.png"}) {
				t.Fatal("original missing-source fallback", images, err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("related query changed source")
			}
		})
	}
}

func TestAVReaderMissingMiddleFieldPreservesFollowingLayoutFields(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			tx, a, file := readerContextFixture(t, layout)
			before, _ := os.ReadFile(file)
			b := readonlyAVBinding(tx, context.Background())
			source, err := b.readCandidate(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := b.clone(source)
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := b.scope.render(candidate, candidate.Views[0], "")
			if err != nil {
				t.Fatal(err)
			}
			var fields []string
			switch v := rendered.(type) {
			case *av.Table:
				for _, f := range v.Columns {
					fields = append(fields, f.ID)
				}
			case *av.Gallery:
				for _, f := range v.Fields {
					fields = append(fields, f.ID)
				}
			case *av.Kanban:
				for _, f := range v.Fields {
					fields = append(fields, f.ID)
				}
			}
			if !reflect.DeepEqual(fields, []string{"block", "status", "visible", "secret"}) {
				t.Fatal("missing-field repair skipped/duplicated a following field", fields)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("projection fix persisted source")
			}
		})
	}
}

func TestAVReadonlyDocumentReadsPreserveAdmittedRevision(t *testing.T) {
	c, tx, a, _ := deepAVFixture(t, av.LayoutTypeTable)
	id := a.GetBlockKeyValues().Values[0].Block.ID
	_, tree, err := tx.attributeViewCellEnvironment().node(id)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(util.DataDir, tree.Box, tree.Path)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	external := append(raw, '\n')
	if err = os.WriteFile(file, external, 0600); err != nil {
		t.Fatal(err)
	}
	b := readonlyAVBinding(tx, context.Background())
	if _, err = b.scope.trees([]string{id}); err != nil {
		t.Fatal(err)
	}
	io, closeIO, err := c.r.openRepairIOChecked(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	if err = c.r.writeRepairTree(context.Background(), io, tree, c.parser()); !errors.Is(err, indexrepair.ErrSourceChanged) {
		t.Fatal("metadata read replaced the admitted source revision", err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(after, external) {
		t.Fatal("query-enabled stale commit overwrote external source")
	}
}
