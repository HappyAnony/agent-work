//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestOwnedDocumentDuplicateActualSourceIndexAndReadonly(t *testing.T) {
	c, r, file, before, _ := conflictFixture(t)
	isolatedConflictQueue(t, flushTx)
	id := util.GetTreeID(file)
	c.persistenceReadOnly = true
	if out, e := DuplicateDocContext(context.Background(), id); e == nil || out != nil {
		t.Fatal("readonly duplicated", out, e)
	}
	c.persistenceReadOnly = false
	out, e := DuplicateDocContext(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	if out == nil || out.ID == id || out.Root.ID != out.ID {
		t.Fatal("duplicate identity", out)
	}
	filename := filepath.Join(util.DataDir, out.Box, out.Path)
	raw, e := os.ReadFile(filename)
	if e != nil || !bytes.Contains(raw, []byte(out.ID)) {
		t.Fatal("duplicate source missing", filename, e)
	}
	original, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(original, before) {
		t.Fatal("original changed", e)
	}
	var count int
	if e = r.main.DB.QueryRow("SELECT count(*) FROM blocks WHERE root_id=?", out.ID).Scan(&count); e != nil || count < 2 {
		t.Fatal("actual index not joined", count, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, e = DuplicateDocContext(ctx, id); !errors.Is(e, context.Canceled) || out != nil {
		t.Fatal(out, e)
	}
	services, e := captureOwnedCreationServices(context.Background(), context.Background())
	if e != nil {
		t.Fatal(e)
	}
	old := Conf
	Conf = NewAppConf()
	_, e = services.Load(context.Background(), id)
	Conf = old
	if !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal("captured source followed another owner", e)
	}
}

func TestOwnedPeripheralMarkdownDailyAndBoundConfiguration(t *testing.T) {
	c, r, file, _, _ := conflictFixture(t)
	isolatedConflictQueue(t, flushTx)
	c.Export = conf.NewExport()
	box := filepath.Base(filepath.Dir(file))
	ctx := context.Background()
	id, e := CreateWithMarkdownContext(ctx, "tag", box, "/Owned Markdown", "owned markdown body", "", "", false, "")
	if e != nil {
		t.Fatal(e)
	}
	var content string
	if e = r.main.DB.QueryRow("SELECT content FROM blocks WHERE root_id=? AND type='p'", id).Scan(&content); e != nil || content != "owned markdown body" {
		t.Fatal(content, e)
	}
	cfg := conf.NewBoxConf()
	cfg.DailyNoteSavePath = "/Owned Daily"
	cfg.DailyNoteTemplatePath = "missing.md"
	raw, _ := json.Marshal(cfg)
	archiveWrite(t, util.DataDir, box+"/.siyuan/conf.json", raw)
	p, existed, e := CreateDailyNoteContext(ctx, box)
	if e != nil || existed || p == "" {
		t.Fatal(p, existed, e)
	}
	again, existed, e := CreateDailyNoteContext(ctx, box)
	if e != nil || !existed || again != p {
		t.Fatal(again, existed, e)
	}
	if _, e = os.Stat(filepath.Join(util.DataDir, box, p)); e != nil {
		t.Fatal("daily source missing", e)
	}
}
