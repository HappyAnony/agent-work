//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/util"
)

func TestAssetInsertActualOwnedFilesDedupAndFailures(t *testing.T) {
	c, _, id, _, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	host := t.TempDir()
	name := filepath.Join(host, "hello[world].TXT")
	body := []byte("original local bytes")
	if e := os.WriteFile(name, body, 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if out, e := InsertLocalAssetsContext(ctx, id, []string{name}, true); e == nil || out != nil {
		t.Fatal("readonly imported", out, e)
	}
	c.persistenceReadOnly = false
	out, e := InsertLocalAssetsContext(ctx, id, []string{name}, true)
	if e != nil {
		t.Fatal(e)
	}
	dest, ok := out[filepath.Base(name)].(string)
	if !ok || !strings.HasPrefix(dest, "assets/helloworld-") || !strings.HasSuffix(dest, ".txt") {
		t.Fatal(out)
	}
	raw, e := os.ReadFile(filepath.Join(util.DataDir, dest))
	if e != nil || !bytes.Equal(raw, body) {
		t.Fatal("source import missing", e)
	}
	again, e := InsertLocalAssetsContext(ctx, id, []string{name}, true)
	if e != nil || again[filepath.Base(name)] != dest {
		t.Fatal("same bytes not deduplicated", again, e)
	}
	directory := filepath.Join(util.DataDir, "assets")
	out, e = InsertLocalAssetsContext(ctx, id, []string{directory}, true)
	if e != nil || out[filepath.Base(directory)] != "file://"+directory {
		t.Fatal("directory link changed", out, e)
	}
	absent := filepath.Join(host, "absent.txt")
	out, e = InsertLocalAssetsContext(ctx, id, []string{absent}, false)
	if e != nil || out["absent.txt"] != "file://"+absent {
		t.Fatal(out, e)
	}
	out, e = InsertLocalAssetsContext(ctx, id, []string{absent}, true)
	if e == nil || out != nil {
		t.Fatal("missing source hidden", out, e)
	}
	for _, name := range []string{"file.", "plain", "file.tar.gz", "20261104100000-asset01.png", "named-20261104100000-asset01.png"} {
		if got, want := assetflow.Name(name, "20261104100001-asset02"), util.AssetName(name, "20261104100001-asset02"); got != want {
			t.Fatal(name, got, want)
		}
	}
}
