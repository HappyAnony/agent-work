//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"wiki-kernel/conf"
)

func TestInvalidReferencesActualSourceReadOnlyPagingAndCorruption(t *testing.T) {
	c, _, def, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	if rows, n, roots, pages, e := ListInvalidBlockRefsContext(ctx, 1, 32); e != nil || len(rows) != 0 || n != 0 || roots != 0 || pages != 0 {
		t.Fatal(rows, n, roots, pages, e)
	}
	raw := []byte(strings.ReplaceAll(string(before), def, "20261104195500-missing"))
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	rows, n, roots, pages, e := ListInvalidBlockRefsContext(ctx, 1, 1)
	if e != nil || len(rows) != 1 || rows[0].ID != "20261028140004-refpar1" || n != 1 || roots != 1 || pages != 1 {
		t.Fatal(rows, n, roots, pages, e)
	}
	if rows, n, _, _, e = ListInvalidBlockRefsContext(ctx, 999999999, 1); e != nil || len(rows) != 0 || n != 1 {
		t.Fatal(rows, n, e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("read modified source", e)
	}
	if e = os.WriteFile(file, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if rows, n, roots, pages, e = ListInvalidBlockRefsContext(ctx, 1, 32); e == nil || rows != nil || n != 0 || roots != 0 || pages != 0 {
		t.Fatal("partial result on corruption", rows, n, roots, pages, e)
	}
}
