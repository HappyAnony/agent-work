//go:build fts5

package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestCreateSavePathUsesOwnedConfigurationWithoutRepair(t *testing.T) {
	c, _, file, _, _ := conflictFixture(t)
	c.Export = conf.NewExport()
	box := filepath.Base(filepath.Dir(file))
	configuration := conf.NewBoxConf()
	configuration.DocCreateSavePath = "  /Owned {{.missing}}  "
	configuration.RefCreateSavePath = "/References"
	// Use a literal template to check original document-only trimming.
	configuration.DocCreateSavePath = "  /Owned  "
	raw, _ := json.Marshal(configuration)
	name := box + "/.siyuan/conf.json"
	archiveWrite(t, util.DataDir, name, raw)
	c.FileTree.DocCreateSavePath = "/Global"
	c.FileTree.RefCreateSavePath = "/GlobalRefs"
	for _, ref := range []bool{false, true} {
		b, p, e := CreateSavePathContext(context.Background(), box, ref)
		want := "/Owned"
		if ref {
			want = "/References"
		}
		if e != nil || b != box || p != want {
			t.Fatal(b, p, e)
		}
	}
	after, _ := os.ReadFile(filepath.Join(util.DataDir, name))
	if string(after) != string(raw) {
		t.Fatal("path preview rewrote notebook")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e := CreateSavePathContext(ctx, box, false); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
