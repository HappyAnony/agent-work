//go:build fts5

package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/util"
)

func TestStructureOwnedDocumentToHeadingActualSources(t *testing.T) {
	c, _, file, _, _ := conflictFixture(t)
	c.FileTree = conf.NewFileTree()
	box := filepath.Base(filepath.Dir(file))
	source := "20261028140001-refdoc1"
	target := "20261028140004-refpar1"
	sourceFile := filepath.Join(util.DataDir, box, source+".docj")
	if _, e := os.Stat(sourceFile); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, _, e := Doc2HeadingContext(context.Background(), source, target, true); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	gotBox, gotPath, e := Doc2HeadingContext(context.Background(), source, target, true)
	if e != nil || gotBox != box || gotPath != "/"+source+".docj" {
		t.Fatal(gotBox, gotPath, e)
	}
	if _, e = os.Stat(sourceFile); !os.IsNotExist(e) {
		t.Fatal("source document remained", e)
	}
	raw, e := os.ReadFile(file)
	if e != nil || !strings.Contains(string(raw), source) || !strings.Contains(string(raw), "NodeHeading") {
		t.Fatal(string(raw), e)
	}
}

func TestStructureOwnedHeadingToDocumentActualSources(t *testing.T) {
	c, _, file, _, _ := conflictFixture(t)
	c.FileTree = conf.NewFileTree()
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	n := d["Children"].([]interface{})[0].(map[string]interface{})
	id := n["ID"].(string)
	n["Type"] = "NodeHeading"
	n["HeadingLevel"] = 2
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	box := filepath.Base(filepath.Dir(file))
	root, path, e := Heading2DocContext(context.Background(), id, box, "/", "")
	if e != nil || root != d["ID"] || path != "/"+id+".docj" {
		t.Fatal(root, path, e)
	}
	raw, e = os.ReadFile(filepath.Join(util.DataDir, box, id+".docj"))
	if e != nil || !strings.Contains(string(raw), `"NodeDocument"`) {
		t.Fatal(string(raw), e)
	}
	if _, e = runStructureConversion(context.Background(), doclife.StructureRequest{Action: "heading-to-doc", ID: id, TargetBox: box, TargetPath: "/"}); e == nil {
		t.Fatal("non-heading source accepted")
	}
}
