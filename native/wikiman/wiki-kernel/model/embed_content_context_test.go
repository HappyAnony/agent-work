//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"errors"
	"os"
	"testing"

	"wiki-kernel/internal/indexdb"
)

func TestEmbedContentUsesCapturedDatabaseAndCommitsDerivedProjection(t *testing.T) {
	_, r, _, sourceFile, sourceBefore := referenceModelFixture(t)
	ctx := context.Background()
	rows := []indexdb.BlockRecord{
		{ID: "20261103090000-embed1", RootID: "20261103090000-embed1", Box: "20261028140000-refbox1", Path: "/embed.docj", HPath: "/embed", Content: "", Markdown: "{{ SELECT * FROM blocks WHERE id = '20261103090001-source' }}", Type: "query_embed"},
		{ID: "20261103090001-source", RootID: "20261103090001-source", Box: "20261028140000-refbox1", Path: "/source.docj", HPath: "/source", Content: "automatic", Markdown: "automatic", Type: "p"},
		{ID: "20261103090002-empty", RootID: "20261103090002-empty", Box: "20261028140000-refbox1", Path: "/empty.docj", HPath: "/empty", Content: "", Markdown: "{{ SELECT * FROM blocks WHERE id = 'missing' }}", Type: "query_embed"},
		{ID: "20261103090003-js", RootID: "20261103090003-js", Box: "20261028140000-refbox1", Path: "/js.docj", HPath: "/js", Content: "", Markdown: "{{ //!js\nreturn []; }}", Type: "query_embed"},
	}
	if err := indexdb.Transaction(ctx, r.main.DB, func(tx *stdsql.Tx) error {
		return indexdb.InsertBlocks(ctx, tx, rows, false, nil)
	}); err != nil {
		t.Fatal(err)
	}

	if err := r.autoIndexEmbedBlocks(ctx); err != nil {
		t.Fatal(err)
	}
	assertContent := func(table, id, want string) {
		t.Helper()
		var got string
		if err := r.main.DB.QueryRowContext(ctx, "SELECT content FROM "+table+" WHERE id = ?", id).Scan(&got); err != nil || got != want {
			t.Fatalf("%s %s content = %q, %v; want %q", table, id, got, err, want)
		}
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		assertContent(table, "20261103090000-embed1", "automatic")
		assertContent(table, "20261103090002-empty", "no query result")
		assertContent(table, "20261103090003-js", "")
	}

	if err := UpdateEmbedBlockContext(ctx, "20261103090000-embed1", "manual"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		assertContent(table, "20261103090000-embed1", "manual")
	}
	if err := UpdateEmbedBlockContext(ctx, "20261103090001-source", "rejected"); err == nil {
		t.Fatal("non-embed block accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := r.updateEmbedBlockContent(canceled, "20261103090000-embed1", []string{"canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled projection admitted", err)
	}
	assertContent("blocks", "20261103090000-embed1", "manual")

	sourceAfter, err := os.ReadFile(sourceFile)
	if err != nil || !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("derived content indexing changed source", err)
	}
	recordCloudBusinessEvidence(t, "embed-content-owner.json", map[string]interface{}{"automaticSQL": true, "emptyFallback": true, "jsSkipped": true, "manualHTTPProjection": true, "allFTSTablesCommitted": true, "nonEmbedRejected": true, "canceledRejected": true, "sourceUnchanged": true})
}
