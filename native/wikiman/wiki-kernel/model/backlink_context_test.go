//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
)

func TestBacklinkActualSourceSQLOriginalModesReadOnlyAndFailure(t *testing.T) {
	c, r, id, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	for _, mode := range []string{"links", "links2", "document", "mention-document"} {
		out, e := BacklinksContext(ctx, blockview.BacklinkRequest{Mode: mode, ID: id, RefTreeID: "20261028140003-refdoc2", BeforeLen: 12, Highlight: true})
		if e != nil {
			t.Fatal(mode, e)
		}
		switch mode {
		case "links", "links2":
			if len(out.Links) != 1 || out.ReferenceCount != 1 || out.Box == "" {
				t.Fatal(mode, out)
			}
		case "document":
			if len(out.Documents) != 1 || !strings.Contains(out.Documents[0].DOM, "20261028140004-refpar1") {
				t.Fatal(out)
			}
		}
	}
	if raw, e := os.ReadFile(file); e != nil || !bytes.Equal(raw, before) {
		t.Fatal("backlink read changed source", e)
	}
	if e := os.WriteFile(file, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := BacklinksContext(ctx, blockview.BacklinkRequest{Mode: "document", ID: id, RefTreeID: "20261028140003-refdoc2"})
	if e == nil || out.Documents != nil {
		t.Fatal("source error returned partial success", out, e)
	}
	if _, e = r.main.DB.Exec("DROP TABLE refs"); e != nil {
		t.Fatal(e)
	}
	out, e = BacklinksContext(ctx, blockview.BacklinkRequest{Mode: "links2", ID: id})
	if e == nil || out.Links != nil {
		t.Fatal("SQL error swallowed", out, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = BacklinksContext(canceled, blockview.BacklinkRequest{Mode: "links", ID: id}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
