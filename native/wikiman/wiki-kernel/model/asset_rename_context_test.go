//go:build fts5

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
	"wiki-kernel/util"
)

func TestAssetRenameActualSourceReferencesAndAnnotation(t *testing.T) {
	c, r, _, doc, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	old := "assets/original.pdf"
	file := filepath.Join(util.DataDir, old)
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file, []byte("asset bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file+".sya", []byte("annotation"), 0600); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(doc)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	d["Properties"].(map[string]interface{})["custom-data-assets-test"] = old
	paragraph := d["Children"].([]interface{})[0].(map[string]interface{})
	paragraph["Children"] = append(paragraph["Children"].([]interface{}), map[string]interface{}{"Type": "NodeImage", "Children": []interface{}{map[string]string{"Type": "NodeLinkText", "Data": "asset"}, map[string]string{"Type": "NodeLinkDest", "Data": old}}})
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(doc, raw, 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, e = RenameAssetContext(ctx, old, "renamed"); e == nil {
		t.Fatal("readonly changed source")
	}
	c.persistenceReadOnly = false
	next, e := RenameAssetContext(ctx, old, "renamed")
	if e != nil || !strings.HasPrefix(next, "assets/renamed-") || !strings.HasSuffix(next, ".pdf") {
		t.Fatal(next, e)
	}
	copied, e := os.ReadFile(filepath.Join(util.DataDir, next))
	if e != nil || string(copied) != "asset bytes" {
		t.Fatal("copy missing", e)
	}
	annotation, e := os.ReadFile(filepath.Join(util.DataDir, next) + ".sya")
	if e != nil || string(annotation) != "annotation" {
		t.Fatal("annotation missing", e)
	}
	after, e := os.ReadFile(doc)
	if e != nil || bytes.Contains(after, []byte(old)) || !bytes.Contains(after, []byte(next)) {
		t.Fatal(string(after), e)
	}
	if _, e = os.Stat(file); e != nil {
		t.Fatal("original asset should remain", e)
	}
	// The actual owned SQL flush is joined before returning.
	var count int
	if e := r.main.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE path=?", next).Scan(&count); e != nil || count < 1 {
		t.Fatal("asset index not joined", count, e)
	}
}
