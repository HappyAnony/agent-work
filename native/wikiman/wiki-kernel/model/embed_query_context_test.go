//go:build fts5

package model

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func TestEmbedQueryActualOrderSourceDerivedCommitAndReadOnly(t *testing.T) {
	c, r, def, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	const embed = "20261104193000-emb0001"
	const para = "20261028140004-refpar1"
	if e := indexdb.Transaction(ctx, r.main.DB, func(tx *sql.Tx) error {
		return indexdb.InsertBlocks(ctx, tx, []indexdb.BlockRecord{{ID: embed, RootID: "20261028140003-refdoc2", Box: "20261028140000-refbox1", Path: "/20261028140003-refdoc2.docj", Type: "query_embed"}}, false, nil)
	}); e != nil {
		t.Fatal(e)
	}
	blocks, e := GetEmbedBlockContext(ctx, embed, []string{para, def}, 0, true)
	if e != nil || len(blocks) != 2 || blocks[0].Block.ID != para || blocks[1].Block.ID != def || !strings.Contains(blocks[1].Block.Content, "definition") {
		t.Fatal(blocks, e)
	}
	var content string
	if e = r.main.DB.QueryRow("SELECT content FROM blocks WHERE id=?", embed).Scan(&content); e != nil || content != "definitiondefinition" {
		t.Fatal("derived update not joined", content, e)
	}
	blocks, e = SearchEmbedBlockContext(ctx, embed, "SELECT * FROM blocks WHERE id IN ('"+para+"','"+def+"')", []string{para}, 0, false)
	if e != nil || len(blocks) != 1 || blocks[0].Block.ID != def {
		t.Fatal(blocks, e)
	}
	c.persistenceReadOnly = true
	blocks, e = GetEmbedBlockContext(ctx, embed, []string{para, def}, 0, false)
	if e != nil || len(blocks) != 2 {
		t.Fatal(blocks, e)
	}
	if e = r.main.DB.QueryRow("SELECT content FROM blocks WHERE id=?", embed).Scan(&content); e != nil || content != "definition" {
		t.Fatal("read-only query changed derived data", content, e)
	}
	if raw, e := os.ReadFile(file); e != nil || !bytes.Equal(raw, before) {
		t.Fatal("embed read changed source", e)
	}
	if _, e = SearchEmbedBlockContext(ctx, embed, "DELETE FROM blocks", nil, 0, false); e == nil {
		t.Fatal("write query accepted")
	}
	if _, e = SearchEmbedBlockContext(ctx, embed, "SELECT * FROM missing_table", nil, 0, false); e == nil {
		t.Fatal("query failure swallowed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = GetEmbedBlockContext(canceled, embed, []string{def}, 0, false); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if blocks, e = GetEmbedBlockContext(ctx, embed, []string{para, def}, 0, false); e == nil || blocks != nil {
		t.Fatal("partial result after source failure", blocks, e)
	}
}
