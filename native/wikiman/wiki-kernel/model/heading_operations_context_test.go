//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
)

const appendedHeadingDOM = `<div data-node-id="20261104190000-par0001" data-type="NodeParagraph"><div contenteditable="true">appended heading child</div></div>`

func TestHeadingOperationsActualReadOnlyPreviewAndOwnedAppend(t *testing.T) {
	commit, _, tree, file := transactionHistoryFixture(t)
	c := commit.r.owner
	c.Export = conf.NewExport()
	ctx := context.Background()
	id := tree.Root.FirstChild.ID
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	n := d["Children"].([]interface{})[0].(map[string]interface{})
	n["Type"] = "NodeHeading"
	n["HeadingLevel"] = 1
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	for _, mode := range []string{"delete", "insert", "level"} {
		tx, e := HeadingOperationsContext(ctx, doclife.HeadingOperationsRequest{Mode: mode, ID: id, Level: 2})
		if e != nil || tx == nil || len(tx.DoOperations) == 0 || len(tx.UndoOperations) != len(tx.DoOperations) {
			t.Fatal(mode, tx, e)
		}
	}
	if after, e := os.ReadFile(file); e != nil || !bytes.Equal(after, raw) {
		t.Fatal("preview changed source", e)
	}
	if e = AppendHeadingChildrenContext(ctx, id, appendedHeadingDOM); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	if e = AppendHeadingChildrenContext(ctx, id, appendedHeadingDOM); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Contains(after, []byte("appended heading child")) {
		t.Fatal(string(after), e)
	}
	var count int
	if e = commit.r.main.DB.QueryRow("SELECT count(*) FROM blocks WHERE id=?", "20261104190000-par0001").Scan(&count); e != nil || count != 1 {
		t.Fatal("append returned before SQL", count, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = GetHeadingDeleteTransactionContext(canceled, id); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
