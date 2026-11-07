// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/model"
	"wiki-kernel/util"
)

func TestResolveSingleModeIDsDoesNotBootstrapSource(t *testing.T) {
	oldConf, oldDir, oldMode, oldBox, oldDoc := model.Conf, util.DataDir, util.SingleNotebookMode, util.SingleNotebookBoxID, util.SingleNotebookRootDocID
	t.Cleanup(func() {
		model.Conf, util.DataDir, util.SingleNotebookMode, util.SingleNotebookBoxID, util.SingleNotebookRootDocID = oldConf, oldDir, oldMode, oldBox, oldDoc
	})
	model.Conf = model.NewAppConf()
	util.SingleNotebookMode = true
	util.SingleNotebookBoxID = ""
	util.SingleNotebookRootDocID = ""
	util.DataDir = filepath.Join(t.TempDir(), "must-not-create")
	box, doc := resolveSingleModeIDs()
	if box != "" || doc != "" {
		t.Fatal(box, doc)
	}
	if _, err := os.Stat(util.DataDir); !os.IsNotExist(err) {
		t.Fatal("projection touched source before DB readiness", err)
	}
	model.Conf.SingleNotebookBoxID = "mirror-box"
	model.Conf.SingleNotebookRootDocID = "mirror-doc"
	box, doc = resolveSingleModeIDs()
	if box != "mirror-box" || doc != "mirror-doc" {
		t.Fatal(box, doc)
	}
}

// makeSyntheticConfData builds a synthetic data map mirroring what getConf
// produces, parameterized by the contents of the inner "system" object so
// tests can exercise the helper without booting the full kernel.
func makeSyntheticConfData(systemContents map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"conf": map[string]interface{}{
			"system":   systemContents,
			"logLevel": "debug",
		},
		"start":     true,
		"isPublish": false,
	}
}

func TestInjectSingleModeFields_PopulatedIDs_EmptySlices(t *testing.T) {
	prev := model.Conf
	t.Cleanup(func() { model.Conf = prev })
	model.Conf = model.NewAppConf()
	model.Conf.SingleNotebookMode = true
	model.Conf.SingleNotebookBoxID = "20240101000000-aaaa"
	model.Conf.SingleNotebookRootDocID = "20240101000001-bbbb"
	// UnmountedBoxes / IgnoredDocs left nil

	data := makeSyntheticConfData(map[string]interface{}{
		"kernelVersion": "3.4.2",
	})

	injectSingleModeFields(data)

	// Round-trip to JSON to verify final shape FE will see.
	out, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	system := got["conf"].(map[string]interface{})["system"].(map[string]interface{})

	if system["single"] != true {
		t.Errorf("expected system.single=true, got %v", system["single"])
	}
	if system["singleNotebookBoxID"] != "20240101000000-aaaa" {
		t.Errorf("box ID mismatch: %v", system["singleNotebookBoxID"])
	}
	if system["singleNotebookRootDocID"] != "20240101000001-bbbb" {
		t.Errorf("root doc ID mismatch: %v", system["singleNotebookRootDocID"])
	}
	// Empty slice expected, not null
	unmounted, ok := system["unmountedBoxes"].([]interface{})
	if !ok || len(unmounted) != 0 {
		t.Errorf("expected unmountedBoxes=[]; got %v (type %T)", system["unmountedBoxes"], system["unmountedBoxes"])
	}
	ignored, ok := system["ignoredDocs"].([]interface{})
	if !ok || len(ignored) != 0 {
		t.Errorf("expected ignoredDocs=[]; got %v (type %T)", system["ignoredDocs"], system["ignoredDocs"])
	}
	// Existing fields preserved
	if system["kernelVersion"] != "3.4.2" {
		t.Errorf("kernelVersion clobbered: %v", system["kernelVersion"])
	}
}

func TestInjectSingleModeFields_LegacyTakeover_PopulatedLists(t *testing.T) {
	prev := model.Conf
	t.Cleanup(func() { model.Conf = prev })
	model.Conf = model.NewAppConf()
	model.Conf.SingleNotebookMode = true
	model.Conf.SingleNotebookBoxID = "primary"
	model.Conf.SingleNotebookRootDocID = "rootDoc"
	model.Conf.UnmountedBoxes = []model.UnmountedBoxInfo{
		{ID: "extra1", Name: "OldNotebook", Reason: "extra-box"},
		{ID: "extra2", Name: "Archive", Reason: "extra-box"},
	}
	model.Conf.IgnoredDocs = []model.IgnoredDocInfo{
		{BoxID: "primary", ID: "doc2", Title: "Second"},
	}

	data := makeSyntheticConfData(map[string]interface{}{})
	injectSingleModeFields(data)

	out, _ := json.Marshal(data)
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	system := got["conf"].(map[string]interface{})["system"].(map[string]interface{})

	unmounted := system["unmountedBoxes"].([]interface{})
	if len(unmounted) != 2 {
		t.Fatalf("expected 2 unmounted boxes, got %d", len(unmounted))
	}
	first := unmounted[0].(map[string]interface{})
	if first["id"] != "extra1" || first["name"] != "OldNotebook" || first["reason"] != "extra-box" {
		t.Errorf("unmounted[0] mismatch: %+v", first)
	}
	// `error` field is `omitempty` and Reason="extra-box" leaves Error="" → key absent
	if _, hasError := first["error"]; hasError {
		t.Errorf("expected `error` key to be absent for clean unmount, got %v", first["error"])
	}

	ignored := system["ignoredDocs"].([]interface{})
	if len(ignored) != 1 {
		t.Fatalf("expected 1 ignored doc, got %d", len(ignored))
	}
	doc := ignored[0].(map[string]interface{})
	if doc["boxID"] != "primary" || doc["id"] != "doc2" || doc["title"] != "Second" {
		t.Errorf("ignored[0] mismatch: %+v", doc)
	}
}

func TestInjectSingleModeFields_PartialFailure_EmptyIDs(t *testing.T) {
	prev := model.Conf
	t.Cleanup(func() { model.Conf = prev })
	model.Conf = model.NewAppConf()
	model.Conf.SingleNotebookMode = true
	model.Conf.SingleNotebookBoxID = ""
	model.Conf.SingleNotebookRootDocID = ""

	data := makeSyntheticConfData(map[string]interface{}{})
	injectSingleModeFields(data)

	out, _ := json.Marshal(data)
	var got map[string]interface{}
	json.Unmarshal(out, &got)
	system := got["conf"].(map[string]interface{})["system"].(map[string]interface{})

	if system["single"] != true {
		t.Errorf("expected single=true even on partial failure (FE then enters fallback C)")
	}
	if system["singleNotebookBoxID"] != "" {
		t.Errorf("expected empty BoxID, got %v", system["singleNotebookBoxID"])
	}
	if system["singleNotebookRootDocID"] != "" {
		t.Errorf("expected empty RootDocID, got %v", system["singleNotebookRootDocID"])
	}
	// Nil slices still emit as [] per §EA-M6-3
	unmounted, ok := system["unmountedBoxes"].([]interface{})
	if !ok || len(unmounted) != 0 {
		t.Errorf("expected unmountedBoxes=[]; got %v", system["unmountedBoxes"])
	}
	ignored, ok := system["ignoredDocs"].([]interface{})
	if !ok || len(ignored) != 0 {
		t.Errorf("expected ignoredDocs=[]; got %v", system["ignoredDocs"])
	}
}

func TestInjectSingleModeFields_MissingSystemSubtree(t *testing.T) {
	prev := model.Conf
	t.Cleanup(func() { model.Conf = prev })
	model.Conf = model.NewAppConf()
	model.Conf.SingleNotebookMode = true
	model.Conf.SingleNotebookBoxID = "primary"
	model.Conf.SingleNotebookRootDocID = "rootDoc"

	// Synthetic data with no system subtree at all.
	data := map[string]interface{}{
		"conf":      map[string]interface{}{}, // no "system" key
		"start":     true,
		"isPublish": false,
	}
	injectSingleModeFields(data)

	out, _ := json.Marshal(data)
	var got map[string]interface{}
	json.Unmarshal(out, &got)
	confMap, ok := got["conf"].(map[string]interface{})
	if !ok {
		t.Fatal("conf is not a map")
	}
	system, ok := confMap["system"].(map[string]interface{})
	if !ok {
		t.Fatal("system was not synthesized when missing; expected an empty-system overlay")
	}
	if system["single"] != true {
		t.Errorf("expected single=true on synthesized overlay, got %v", system["single"])
	}
	if system["singleNotebookBoxID"] != "primary" {
		t.Errorf("expected singleNotebookBoxID=primary, got %v", system["singleNotebookBoxID"])
	}
	if system["singleNotebookRootDocID"] != "rootDoc" {
		t.Errorf("expected singleNotebookRootDocID=rootDoc, got %v", system["singleNotebookRootDocID"])
	}
}
