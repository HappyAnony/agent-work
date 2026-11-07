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
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func writeLifecycleAV(t *testing.T, file string, a *av.AttributeView) []byte {
	t.Helper()
	raw, e := json.Marshal(a)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestAVLifecycleCreateCapturesLabelsFormatAndRefusesOverwrite(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	b := newLegacyAVBinding(context.Background())
	want := b.scope.commit.avLabels["table"]
	oldLabel := util.AttrViewLangs[util.Lang]["table"]
	util.AttrViewLangs[util.Lang]["table"] = "changed after capture"
	defer func() { util.AttrViewLangs[util.Lang]["table"] = oldLabel }()
	oldFormat := util.UseSingleLineSave
	util.UseSingleLineSave = !b.scope.commit.r.singleLine
	defer func() { util.UseSingleLineSave = oldFormat }()
	id := "20261029190000-created"
	created, e := b.initialize(id)
	if e != nil {
		t.Fatal(e)
	}
	if created.Spec != 4 || created.Views[0].Name != want || len(created.KeyValues) != 2 || created.ViewID != created.Views[0].ID {
		t.Fatal("original defaults/captured labels lost")
	}
	target := filepath.Join(filepath.Dir(file), id+".json")
	before, e := os.ReadFile(target)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(before, []byte("\n")) == b.scope.commit.r.singleLine {
		t.Fatal("format followed new global")
	}
	if e = b.create(created); e == nil {
		t.Fatal("create overwrote existing AV")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("create changed existing source")
	}
	if _, e = b.initialize(a.ID); e != nil {
		t.Fatal("existing source failed", e)
	}
	recordCloudBusinessEvidence(t, "lifecycle-create.json", map[string]interface{}{"originalDefaults": true, "capturedLanguageAndFormat": true, "createOnlyCAS": true})
}
func TestAVLifecycleSelectionUsesReadonlyNodeAndPersistsView(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	other := av.NewTableView()
	a.Views = append(a.Views, other)
	writeLifecycleAV(t, file, a)
	rootFile := filepath.Join(util.DataDir, "20261028140000-refbox1", "20261028140001-refdoc1.docj")
	raw, _ := os.ReadFile(rootFile)
	var doc map[string]interface{}
	json.Unmarshal(raw, &doc)
	delete(doc, "Spec")
	p := doc["Children"].([]interface{})[0].(map[string]interface{})
	p["Properties"].(map[string]interface{})[av.NodeAttrView] = other.ID
	raw, _ = json.Marshal(doc)
	os.WriteFile(rootFile, raw, 0600)
	b := newLegacyAVBinding(context.Background())
	copy, e := b.initialize(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	v, e := b.selectView(copy, "", "20261028140002-refdef1")
	if e != nil || v.ID != other.ID {
		t.Fatal(v, e)
	}
	savedRaw, _ := os.ReadFile(file)
	saved, e := av.DecodeAttributeView(savedRaw)
	if e != nil || saved.ViewID != other.ID {
		t.Fatal("selection did not persist", e)
	}
	after, _ := os.ReadFile(rootFile)
	if !bytes.Equal(raw, after) {
		t.Fatal("node metadata read migrated document Spec")
	}
	v, e = b.selectView(copy, "missing-view", "")
	if e != nil || v.ID != other.ID {
		t.Fatal("current-view fallback changed", e)
	}
	recordCloudBusinessEvidence(t, "lifecycle-selection.json", map[string]interface{}{"nodeSelectedViewPersisted": true, "fallbackPreserved": true, "legacyDocumentReadOnly": true})
}
func TestAVLifecycleEmptyViewsCreateOriginalTable(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.Views = nil
	a.ViewID = ""
	writeLifecycleAV(t, file, a)
	b := newLegacyAVBinding(context.Background())
	copy, e := b.initialize(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	v, e := b.selectView(copy, "", "")
	if e != nil || v.LayoutType != av.LayoutTypeTable || len(v.Table.Columns) != 2 || v.PageSize != av.ViewDefaultPageSize {
		t.Fatal(v, e)
	}
	raw, _ := os.ReadFile(file)
	saved, e := av.DecodeAttributeView(raw)
	if e != nil || len(saved.Views) != 1 || saved.ViewID != v.ID {
		t.Fatal("empty-view table not saved", e)
	}
}
func TestAVLifecycleChecksRepairAndUpgradeActuallySave(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.Spec = 3
	a.Name = strings.Repeat("中", 513) + "\n"
	v := a.Views[0]
	v.Filters = []*av.ViewFilter{{Column: "missing"}}
	v.Sorts = []*av.ViewSort{{Column: "missing"}}
	a.KeyValues[1].Values[0].Type = av.KeyTypeNumber
	a.KeyValues[0].Values[0].Type = av.KeyTypeText
	a.KeyValues[0].Values[0].Text = &av.ValueText{Content: "converted primary"}
	a.KeyValues[0].Values[0].Block = nil
	writeLifecycleAV(t, file, a)
	b := newLegacyAVBinding(context.Background())
	copy, e := b.initialize(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	if copy.Spec != 3 {
		t.Fatal("read eagerly upgraded source")
	}
	if e = b.prepareView(copy, copy.Views[0]); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(file)
	saved, e := av.DecodeAttributeView(raw)
	if e != nil || saved.Spec != 4 || len([]rune(saved.Name)) != 512 || len(saved.Views[0].Filters) != 0 || len(saved.Views[0].Sorts) != 0 || saved.KeyValues[1].Values[0].Type != av.KeyTypeSelect || saved.GetBlockKeyValues().Values[0].Block.Content != "converted primary" {
		t.Fatal("repair/upgrade not saved", e)
	}
	recordCloudBusinessEvidence(t, "lifecycle-repair.json", map[string]interface{}{"rawReadDoesNotUpgrade": true, "originalRepairValuesSaved": true, "upgradeSaved": true})
}
func TestAVLifecycleSpecZeroIconBackfillDoesNotMigrateDoc(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.Spec = 0
	writeLifecycleAV(t, file, a)
	rootFile := filepath.Join(util.DataDir, "20261028140000-refbox1", "20261028140001-refdoc1.docj")
	raw, _ := os.ReadFile(rootFile)
	var doc map[string]interface{}
	json.Unmarshal(raw, &doc)
	delete(doc, "Spec")
	doc["Children"].([]interface{})[0].(map[string]interface{})["Properties"].(map[string]interface{})["icon"] = "captured-icon"
	raw, _ = json.Marshal(doc)
	os.WriteFile(rootFile, raw, 0600)
	b := newLegacyAVBinding(context.Background())
	copy, e := b.initialize(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = b.prepareView(copy, copy.Views[0]); e != nil {
		t.Fatal(e)
	}
	if copy.Spec != 4 || copy.GetBlockKeyValues().Values[0].Block.Icon != "captured-icon" {
		t.Fatal("original icon upgrade lost")
	}
	after, _ := os.ReadFile(rootFile)
	if !bytes.Equal(raw, after) {
		t.Fatal("icon lookup repaired source document")
	}
}
func TestAVLifecycleFailuresDoNotReplaceExistingSource(t *testing.T) {
	for _, mode := range []string{"bad-json", "directory", "symlink", "cancel", "readonly", "stale", "invalid-id", "future-spec"} {
		t.Run(mode, func(t *testing.T) {
			c, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
			before, _ := os.ReadFile(file)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newLegacyAVBinding(ctx)
			id := a.ID
			switch mode {
			case "bad-json":
				before = []byte("invalid source")
				os.WriteFile(file, before, 0600)
			case "directory":
				os.Remove(file)
				os.Mkdir(file, 0700)
			case "symlink":
				outside := filepath.Join(t.TempDir(), "source")
				os.WriteFile(outside, before, 0600)
				os.Remove(file)
				os.Symlink(outside, file)
			case "cancel":
				cancel()
			case "readonly":
				c.r.owner.m.Lock()
				c.r.owner.persistenceReadOnly = true
				c.r.owner.m.Unlock()
				defer func() { c.r.owner.m.Lock(); c.r.owner.persistenceReadOnly = false; c.r.owner.m.Unlock() }()
			case "stale":
				old := Conf
				Conf = &AppConf{}
				defer func() { Conf = old }()
			case "invalid-id":
				id = "../not-an-id"
			case "future-spec":
				a.Spec = 99
				before = writeLifecycleAV(t, file, a)
			}
			copy, e := b.initialize(id)
			if mode == "future-spec" && e == nil {
				e = b.prepareView(copy, copy.Views[0])
			}
			if e == nil {
				t.Fatal("failure ignored")
			}
			if mode != "directory" {
				after, _ := os.ReadFile(file)
				if !bytes.Equal(before, after) {
					t.Fatal("failed initialization replaced source")
				}
			}
		})
	}
}
func TestAVLifecycleSelectionAndRepairPropagateActualWriteFailure(t *testing.T) {
	for _, mode := range []string{"selection", "repair", "changed-source"} {
		t.Run(mode, func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
			other := av.NewTableView()
			a.Views = append(a.Views, other)
			writeLifecycleAV(t, file, a)
			b := newLegacyAVBinding(context.Background())
			copy, e := b.initialize(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "changed-source" {
				os.WriteFile(file, []byte("new external source"), 0600)
			} else {
				os.Remove(file)
				os.Mkdir(file, 0700)
			}
			if mode == "selection" {
				_, e = b.selectView(copy, other.ID, "")
			} else {
				copy.Name = strings.Repeat("x", 513)
				e = b.prepareView(copy, copy.Views[0])
			}
			if e == nil {
				t.Fatal("actual write/version failure swallowed")
			}
			if mode == "changed-source" {
				raw, _ := os.ReadFile(file)
				if string(raw) != "new external source" {
					t.Fatal("new source overwritten")
				}
			}
		})
	}
}
func TestAVLifecycleWaitUsesOwnerAndCancellation(t *testing.T) {
	c, _, _, file := deepAVFixture(t, av.LayoutTypeTable)
	before, _ := os.ReadFile(file)
	c.r.owner.syncRuntime.booting.Store(true)
	defer c.r.owner.syncRuntime.booting.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	b := newLegacyAVBinding(ctx)
	done := make(chan error, 1)
	go func() { done <- b.waitForStorages() }()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("storage wait ignored cancellation")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("wait changed source")
	}
	b = newLegacyAVBinding(context.Background())
	if e := c.r.owner.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := b.waitForStorages(); e == nil {
		t.Fatal("closed owner admitted source init")
	}
}
func TestAVLifecycleOriginalNoSaveMarkerRemains(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.Name = "line\nline"
	a.Views[0].Group = &av.ViewGroup{Field: "missing"}
	before := writeLifecycleAV(t, file, a)
	b := newLegacyAVBinding(context.Background())
	copy, e := b.initialize(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = b.prepareView(copy, copy.Views[0]); e != nil {
		t.Fatal(e)
	}
	if copy.Name != "line line" || copy.Views[0].Group != nil {
		t.Fatal("original in-memory repairs lost")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("changed original no-save marker")
	}
}

func TestAVLifecycleGalleryRepairReturnsErrorsWithoutPanic(t *testing.T) {
	for _, mode := range []string{"fallback-table", "remove-empty"} {
		t.Run(mode, func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
			v := a.Views[0]
			v.LayoutType = av.LayoutTypeGallery
			v.Gallery = nil
			if mode == "remove-empty" {
				v.Table = nil
			}
			writeLifecycleAV(t, file, a)
			out, _, e := RenderAttributeViewContext(context.Background(), "", a.ID, a.ViewID, "", 1, -1, nil)
			if mode == "fallback-table" {
				if e != nil || out.GetType() != av.LayoutTypeTable {
					t.Fatal("original table fallback failed", e)
				}
			} else {
				if e == nil {
					t.Fatal("removed current layout reported success")
				}
				raw, _ := os.ReadFile(file)
				saved, e := av.DecodeAttributeView(raw)
				if e != nil || len(saved.Views) != 0 {
					t.Fatal("invalid gallery cleanup not saved", e)
				}
			}
		})
	}
}
