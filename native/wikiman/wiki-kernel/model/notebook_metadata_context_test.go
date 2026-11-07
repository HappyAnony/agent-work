//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestNotebookMetadataActualOwnedWriteAndFailure(t *testing.T) {
	c, _, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	id := filepath.Base(filepath.Dir(file))
	name := filepath.Join(util.DataDir, id, ".siyuan/conf.json")
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		t.Fatal(e)
	}
	raw := []byte(`{"name":"before","closed":false,"sort":10,"sortMode":256,"dailyNoteSavePath":"/daily"}`)
	if e := os.WriteFile(name, raw, 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if e := SetBoxIconContext(ctx, id, "icon.svg"); e == nil {
		t.Fatal("readonly source written")
	}
	got, e := os.ReadFile(name)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("readonly source changed", e)
	}
	c.persistenceReadOnly = false
	value, e := SetNotebookConfigurationContext(ctx, id, []byte(`{"refCreateSavePath":" root / next ","dailyNoteSavePath":" daily / note ","dailyNoteTemplatePath":" template ","docCreateSavePath":" first / second "}`))
	if e != nil || value.Name != "before" || value.RefCreateSavePath != "root/next/" || value.DailyNoteSavePath != "/daily/note" || value.DailyNoteTemplatePath != "/template.md" || value.DocCreateSavePath != "first/second" {
		t.Fatal(value, e)
	}
	if e = SetBoxIconContext(ctx, id, "api/icon/name.svg"); e != nil {
		t.Fatal(e)
	}
	if e = ChangeBoxSortContext(ctx, []string{id, id}); e != nil {
		t.Fatal("duplicate order", e)
	}
	if e = RenameBoxContext(ctx, id, "after"); e != nil {
		t.Fatal(e)
	}
	value, e = NotebookConfigurationContext(ctx, id)
	if e != nil || value.Name != "after" || value.Sort != 2 || value.Icon != "api/icon/name.svg" {
		t.Fatal(value, e)
	}
	before, e := os.ReadFile(name)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = SetNotebookConfigurationContext(ctx, id, []byte(`{"dailyNoteSavePath":" / "}`)); e == nil {
		t.Fatal("invalid daily path accepted")
	}
	got, e = os.ReadFile(name)
	if e != nil || !bytes.Equal(got, before) {
		t.Fatal("invalid patch published", e)
	}
	if e = os.WriteFile(name, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = SetBoxIconContext(ctx, id, "next"); e == nil {
		t.Fatal("corruption overwritten")
	}
	got, _ = os.ReadFile(name)
	if string(got) != "broken" {
		t.Fatal("corrupt preimage lost")
	}
	var stored conf.BoxConf
	if e = json.Unmarshal(before, &stored); e != nil || stored.Name != "after" {
		t.Fatal(stored, e)
	}
}

func TestNotebookCreateActualSortedMetadataAndReadOnly(t *testing.T) {
	c, _, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	existing := filepath.Base(filepath.Dir(file))
	name := filepath.Join(util.DataDir, existing, ".siyuan/conf.json")
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(name, []byte(`{"name":"existing","closed":false,"sort":8}`), 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if id, e := CreateBoxContext(ctx, "new"); e == nil || id != "" {
		t.Fatal("readonly created notebook", id, e)
	}
	c.persistenceReadOnly = false
	id, e := CreateBoxContext(ctx, "new notebook")
	if e != nil || id == "" {
		t.Fatal(id, e)
	}
	value, e := NotebookConfigurationContext(ctx, id)
	if e != nil || value.Name != "new notebook" || !value.Closed {
		t.Fatal(value, e)
	}
	old, e := NotebookConfigurationContext(ctx, existing)
	if e != nil || old.Sort != 1 {
		t.Fatal(old, e)
	}
}
