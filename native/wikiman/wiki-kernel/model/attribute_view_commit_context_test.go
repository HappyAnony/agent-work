//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func deepAVFixture(t *testing.T, layout av.LayoutType) (*legacyCommitContext, *Transaction, *av.AttributeView, string) {
	t.Helper()
	_, env, tx, _, _ := commitFixture(t)
	oldLang, oldAV := util.Lang, util.AttrViewLangs
	rawLang, e := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if e != nil {
		t.Fatal(e)
	}
	var lang struct {
		AV map[string]interface{} `json:"_attrView"`
	}
	if e = json.Unmarshal(rawLang, &lang); e != nil {
		t.Fatal(e)
	}
	util.Lang = "en_US"
	util.AttrViewLangs = map[string]map[string]interface{}{"en_US": lang.AV}
	t.Cleanup(func() { util.Lang, util.AttrViewLangs = oldLang, oldAV })
	v, block, status := av.NewTableViewWithBlockKey("block")
	status.ID = "status"
	v.Table.Columns[1].ID = status.ID
	status.Options = []*av.SelectOption{{Name: "X"}, {Name: "Y"}, {Name: "Empty"}}
	switch layout {
	case av.LayoutTypeGallery:
		v.LayoutType = layout
		v.Gallery = av.NewLayoutGallery()
		v.Gallery.CoverFrom = av.CoverFromNone
		v.Gallery.CardFields = []*av.ViewGalleryCardField{{BaseField: &av.BaseField{ID: block.ID}}, {BaseField: &av.BaseField{ID: status.ID}}}
	case av.LayoutTypeKanban:
		v.LayoutType = layout
		v.Kanban = av.NewLayoutKanban()
		v.Kanban.CoverFrom = av.CoverFromNone
		v.Kanban.Fields = []*av.ViewKanbanField{{BaseField: &av.BaseField{ID: block.ID}}, {BaseField: &av.BaseField{ID: status.ID}}}
	}
	v.Group = &av.ViewGroup{Field: status.ID, Method: av.GroupMethodValue, Order: av.GroupOrderAsc}
	id := "20261029150000-deepav1"
	row := "20261028140002-refdef1"
	a := &av.AttributeView{ID: id, Name: "Scoped AV", Spec: 4, ViewID: v.ID, Views: []*av.View{v}, KeyValues: []*av.KeyValues{{Key: block, Values: []*av.Value{{ID: "block-value", KeyID: block.ID, BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row, Content: "before"}}}}, {Key: status, Values: []*av.Value{{ID: "status-value", KeyID: status.ID, BlockID: row, Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: "X"}}}}}}}
	file := filepath.Join(util.DataDir, "storage/av", id+".json")
	os.MkdirAll(filepath.Dir(file), 0700)
	raw, e := json.Marshal(a)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return env.commitContext, tx, a, file
}
func TestDeepAVContextOriginalGroupsAcrossLayouts(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			c, _, a, file := deepAVFixture(t, layout)
			scope := c.avContext(context.Background())
			scoped, e := scope.read(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			old, e := av.ParseAttributeView(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			if e = avrender.GenerateGroups(old.Views[0], old, avrender.GroupEnvironment{Now: time.Now, Read: av.ParseAttributeView, NewView: legacyGroupView, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
				return sql.RenderView(a, v, q), nil
			}}); e != nil {
				t.Fatal(e)
			}
			if e = scope.groups(scoped); e != nil {
				t.Fatal(e)
			}
			projection := func(v *av.View) map[string][]string {
				out := map[string][]string{}
				for _, g := range v.Groups {
					out[g.GetGroupValue()] = g.GroupItemIDs
				}
				return out
			}
			want, _ := json.Marshal(projection(old.Views[0]))
			got, _ := json.Marshal(projection(scoped.Views[0]))
			if !bytes.Equal(want, got) {
				t.Fatal("original group parity", string(want), string(got))
			}
			group := scoped.Views[0].GetGroupByGroupValue("X")
			if group == nil {
				t.Fatal("missing group")
			}
			group.GroupFolded = true
			group.GroupHidden = 2
			group.ID = "stable-group"
			scoped.RenderedViewables = map[string]av.Viewable{}
			if e = scope.groups(scoped); e != nil {
				t.Fatal(e)
			}
			group = scoped.Views[0].GetGroupByGroupValue("X")
			if group.ID != "stable-group" || group.GroupFolded != (layout != av.LayoutTypeKanban) || group.GroupHidden != 2 {
				t.Fatal("original states lost")
			}
			if e = scope.save(scoped); e != nil {
				t.Fatal(e)
			}
			raw, e := os.ReadFile(file)
			if e != nil || !bytes.Contains(raw, []byte("stable-group")) {
				t.Fatal("group not saved", e)
			}
		})
	}
	recordCloudBusinessEvidence(t, "deep-groups.json", map[string]interface{}{"layouts": 3, "originalGroupMembershipParity": true, "statePreserved": true, "rootedSave": true})
}
func TestDeepAVContextActualTextRefreshRelatedGroupsAndCommit(t *testing.T) {
	c, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
	for _, n := range tx.nodes {
		n.SetIALAttr(av.NodeAttrNameAvs, a.ID)
	}
	relations, _ := msgpack.Marshal(map[string][]string{a.ID: {a.ID}})
	os.WriteFile(filepath.Join(filepath.Dir(file), "relations.msgpack"), relations, 0600)
	tx.relatedAvIDs = []string{a.ID}
	events := make(chan commitNotice, 32)
	c.publish = func(n commitNotice) { events <- n }
	if e := tx.commitLegacy(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := av.DecodeAttributeView(raw)
	if e != nil {
		t.Fatal(e)
	}
	if saved.GetBlockKeyValues().Values[0].Block.Content != "changed definition" || len(saved.Views[0].Groups) == 0 {
		t.Fatal("deep commit did not persist text and groups")
	}
	var count int
	if e = c.r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", tx.nodes["20261028140002-refdef1"].ID).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	recordCloudBusinessEvidence(t, "deep-commit.json", map[string]interface{}{"actualTextAndRelatedGroupSave": true, "actualSQLCommit": true, "ownedNotifications": true})
}
func TestDeepAVContextBlockTextPreservesOriginalRules(t *testing.T) {
	c, _, a, _ := deepAVFixture(t, av.LayoutTypeTable)
	scope := c.avContext(context.Background())
	nodes := []*ast.Node{{Type: ast.NodeParagraph}, {Type: ast.NodeAttributeView, AttributeViewID: a.ID}, {Type: ast.NodeBlockQueryEmbed}, {Type: ast.NodeIFrame}, {Type: ast.NodeVideo}, {Type: ast.NodeAudio}}
	n := &ast.Node{Type: ast.NodeParagraph}
	n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(strings.Repeat("字", 1100) + "\nend")})
	nodes = append(nodes, n)
	for _, n := range nodes {
		icon, want, err := attributeViewBlockTextWithServices(n, a.ID, func(n *ast.Node) (string, error) { return getNodeRefText0(n, 1024, false), nil }, func() string { return Conf.language(105) })
		if err != nil {
			t.Fatal(err)
		}
		gotIcon, got, e := scope.text(n, a.ID)
		if e != nil || gotIcon != icon || got != want {
			t.Fatal(n.Type, want, got, e)
		}
	}
	n.SetIALAttr("name", " <alias> ")
	n.SetIALAttr("icon", "custom")
	n.SetIALAttr(av.NodeAttrViewStaticText+"-"+a.ID, "static override")
	icon, text, e := scope.text(n, a.ID)
	if e != nil || icon != "custom" || text != "static override" {
		t.Fatal(icon, text, e)
	}
}
func TestDeepAVContextReadFailureCancelAndOldOwnerDoNotPublish(t *testing.T) {
	for _, mode := range []string{"bad-relations", "bad-AV", "symlink", "canceled", "stale", "read-only"} {
		t.Run(mode, func(t *testing.T) {
			c, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
			tx.relatedAvIDs = []string{a.ID}
			if mode == "bad-relations" {
				for _, n := range tx.nodes {
					n.SetIALAttr(av.NodeAttrNameAvs, a.ID)
				}
				os.WriteFile(filepath.Join(filepath.Dir(file), "relations.msgpack"), []byte("bad msgpack"), 0600)
			}
			if mode == "bad-AV" {
				os.WriteFile(file, []byte("bad JSON"), 0600)
			}
			if mode == "symlink" {
				os.Remove(file)
				outside := filepath.Join(t.TempDir(), "source")
				os.WriteFile(outside, []byte("fixture"), 0600)
				os.Symlink(outside, file)
			}
			if mode == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				tx.environment.ctx = ctx
			}
			if mode == "stale" {
				old := Conf
				Conf = &AppConf{}
				defer func() { Conf = old }()
			}
			if mode == "read-only" {
				c.r.owner.m.Lock()
				c.r.owner.persistenceReadOnly = true
				c.r.owner.m.Unlock()
				defer func() { c.r.owner.m.Lock(); c.r.owner.persistenceReadOnly = false; c.r.owner.m.Unlock() }()
			}
			if e := tx.commitLegacy(); e == nil {
				t.Fatal("deep failure swallowed")
			}
			if c.r.owner.repoTasks != nil {
				t.Fatal("failure notified")
			}
		})
	}
}
func TestDeepAVContextMissingRelationsAndSourceVersionGuard(t *testing.T) {
	c, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	s := c.avContext(context.Background())
	ids, e := s.sources(a.ID)
	if e != nil || len(ids) != 0 {
		t.Fatal(ids, e)
	}
	copy, e := s.read(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(file)
	external := append(before, byte(' '))
	os.WriteFile(file, external, 0600)
	if e = s.save(copy); e == nil {
		t.Fatal("lost update accepted")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(external, after) {
		t.Fatal("changed source overwritten")
	}
}
func TestDeepAVContextRelatedAndTemplateFailuresPropagate(t *testing.T) {
	for _, mode := range []string{"relation", "template", "group-range", "SQL"} {
		t.Run(mode, func(t *testing.T) {
			c, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
			v := a.Views[0]
			switch mode {
			case "relation":
				key := a.KeyValues[1].Key
				key.Type = av.KeyTypeRelation
				key.Relation = &av.Relation{AvID: "20261029150001-missing"}
				a.KeyValues[1].Values[0].Type = av.KeyTypeRelation
				a.KeyValues[1].Values[0].Relation = &av.ValueRelation{}
			case "template":
				a.KeyValues[1].Key.Type = av.KeyTypeTemplate
				a.KeyValues[1].Key.Template = ".action{ invalidTemplateFunction }"
				a.KeyValues[1].Values[0].Type = av.KeyTypeTemplate
			case "group-range":
				v.Group.Method = av.GroupMethodRangeNum
				v.Group.Range = &av.GroupRange{NumStep: 0}
			case "SQL":
				a.KeyValues[1].Key.Type = av.KeyTypeTemplate
				a.KeyValues[1].Key.Template = `.action{ querySQL "SELECT * FROM missing_fixture_table" }`
				a.KeyValues[1].Values[0].Type = av.KeyTypeTemplate
			}
			raw, _ := json.Marshal(a)
			os.WriteFile(file, raw, 0600)
			tx.relatedAvIDs = []string{a.ID}
			if e := tx.commitLegacy(); e == nil {
				t.Fatal("deep render error ignored", mode)
			}
			if c.r.owner.repoTasks != nil {
				t.Fatal("failed deep render notified")
			}
		})
	}
}

func TestDeepAVContextRelatedRollupAndSQLTemplateMatchOriginal(t *testing.T) {
	c, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	row := "20261028140002-refdef1"
	// A second independent AV supplies the related numeric field.
	dest, e := cloneAttributeViewPreview(a)
	if e != nil {
		t.Fatal(e)
	}
	dest.ID = "20261029150001-deepav2"
	dest.Name = "Related AV"
	dest.KeyValues[1].Key.Type = av.KeyTypeNumber
	dest.KeyValues[1].Values[0].Type = av.KeyTypeNumber
	dest.KeyValues[1].Values[0].Number = &av.ValueNumber{Content: 7, IsNotEmpty: true}
	dest.Views[0].Group = nil
	raw, _ := json.Marshal(dest)
	destFile := filepath.Join(filepath.Dir(file), dest.ID+".json")
	os.WriteFile(destFile, raw, 0600)
	rel := &av.Key{ID: "relation", Name: "Relation", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: dest.ID}}
	rollup := &av.Key{ID: "rollup", Name: "Rollup", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: rel.ID, KeyID: "status", Calc: &av.RollupCalc{Operator: av.CalcOperatorSum}}}
	tpl := &av.Key{ID: "template", Name: "Template", Type: av.KeyTypeTemplate, Template: `.action{ (getBlock "20261028140002-refdef1").ID }`}
	for _, key := range []*av.Key{rel, rollup, tpl} {
		value := &av.Value{ID: key.ID + "-value", KeyID: key.ID, BlockID: row, Type: key.Type}
		avrender.FillNilValue(value, key.Type)
		if key == rel {
			value.Relation.BlockIDs = []string{row}
		}
		a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: key, Values: []*av.Value{value}})
		a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: key.ID}})
	}
	raw, _ = json.Marshal(a)
	os.WriteFile(file, raw, 0600)
	old, e := av.ParseAttributeView(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	want := sql.RenderView(old, old.Views[0], "").(av.Collection).GetItems()[0]
	scope := c.avContext(context.Background())
	scoped, e := scope.read(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	view, e := scope.render(scoped, scoped.Views[0], "")
	if e != nil {
		t.Fatal(e)
	}
	got := view.(av.Collection).GetItems()[0]
	for _, id := range []string{rel.ID, rollup.ID, tpl.ID} {
		if got.GetValue(id).String(false) != want.GetValue(id).String(false) {
			t.Fatal("related/template parity", id, got.GetValue(id).String(false), want.GetValue(id).String(false))
		}
	}
	if got.GetValue(tpl.ID).Template.Content != row {
		t.Fatal("template did not query captured SQL")
	}
	if after, _ := os.ReadFile(destFile); !bytes.Equal(after, func() []byte { b, _ := json.Marshal(dest); return b }()) {
		t.Fatal("related read rewrote source")
	}
	recordCloudBusinessEvidence(t, "deep-related.json", map[string]interface{}{"originalRelationAndRollupParity": true, "capturedSQLTemplateExecuted": true, "relatedSourceReadOnly": true})
}

func TestDeepAVContextLegacyReadIsDetachedAndSaveUsesCapturedFormat(t *testing.T) {
	c, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.Spec = 3
	raw, _ := json.Marshal(a)
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	scope := c.avContext(context.Background())
	copy, e := scope.read(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	if copy.Spec != av.CurrentSpec {
		t.Fatal("old format not adapted")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(raw, after) {
		t.Fatal("read migrated source")
	}
	old := util.UseSingleLineSave
	util.UseSingleLineSave = !c.r.singleLine
	defer func() { util.UseSingleLineSave = old }()
	if e = scope.save(copy); e != nil {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(file)
	if bytes.Contains(after, []byte("\n")) == c.r.singleLine {
		t.Fatal("save followed changed global format")
	}
	saved, e := av.DecodeAttributeView(after)
	if e != nil || saved.Spec != av.CurrentSpec {
		t.Fatal("explicit upgraded save failed", e)
	}
}
func TestDeepAVContextMissingFieldRepairIsScoped(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			c, _, a, file := deepAVFixture(t, layout)
			v := a.Views[0]
			switch layout {
			case av.LayoutTypeTable:
				v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: "missing-field"}})
			case av.LayoutTypeGallery:
				v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: "missing-field"}})
			case av.LayoutTypeKanban:
				v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: "missing-field"}})
			}
			raw, _ := json.Marshal(a)
			os.WriteFile(file, raw, 0600)
			scope := c.avContext(context.Background())
			copy, e := scope.read(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = scope.render(copy, copy.Views[0], ""); e != nil {
				t.Fatal(e)
			}
			after, _ := os.ReadFile(file)
			if bytes.Contains(after, []byte("missing-field")) {
				t.Fatal("original missing-field repair not persisted")
			}
		})
	}
}
