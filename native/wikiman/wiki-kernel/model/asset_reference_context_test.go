//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestAssetReferencesActualCompleteScanAndReadOnly(t *testing.T) {
	c, _, _, file, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	props := d["Properties"].(map[string]interface{})
	props["custom-data-assets-used"] = "assets/used.txt"
	props["title-img"] = "background-image: url(assets/missing.png)"
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"used.txt", "unused.txt", "ocr-texts.json"} {
		p := filepath.Join(util.DataDir, "assets", name)
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(p, []byte(name), 0600); e != nil {
			t.Fatal(e)
		}
	}
	unused, e := UnusedAssetsContext(ctx)
	if e != nil || !reflect.DeepEqual(unused, []string{"assets/unused.txt"}) {
		t.Fatal(unused, e)
	}
	missing, e := MissingAssetsContext(ctx)
	if e != nil || !reflect.DeepEqual(missing, []string{"assets/missing.png"}) {
		t.Fatal(missing, e)
	}
	found, e := DocAssetsContext(ctx, d["ID"].(string))
	if e != nil || !reflect.DeepEqual(found, []string{"assets/used.txt"}) {
		t.Fatal(found, e)
	}
	names, e := SearchAssetsByNameContext(ctx, "unused", []string{".txt"})
	if e != nil || len(names) != 1 || names[0].Path != "assets/unused.txt" {
		t.Fatal(names, e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("asset scan rewrote source", e)
	}
	if e = os.WriteFile(file, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := UnusedAssetsContext(ctx); e == nil || out != nil {
		t.Fatal("incomplete scan accepted", out, e)
	}
	if out, e := MissingAssetsContext(ctx); e == nil || out != nil {
		t.Fatal("incomplete scan accepted", out, e)
	}
}
