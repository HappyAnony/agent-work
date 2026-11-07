//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/util"
)

func TestDocumentViewActualOwnerReadOnlyHighlightRecentAndFailure(t *testing.T) {
	c, _, _, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	const root = "20261028140001-refdoc1"
	c.persistenceReadOnly = true
	out, e := DocumentViewContext(ctx, docview.Request{ID: root, Size: 100, Query: "definition", QueryTypes: map[string]bool{"paragraph": true}, Highlight: true})
	if e != nil || out.RootID != root || !strings.Contains(out.DOM, "definition") || !strings.Contains(out.DOM, "search-mark") || !strings.Contains(out.DOM, `refcount="1"`) {
		t.Fatal(out, e)
	}
	recentFile := filepath.Join(util.DataDir, "storage", "recent-doc.json")
	if _, e = os.Stat(recentFile); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("read-only document wrote recent metadata", e)
	}
	c.persistenceReadOnly = false
	if _, e = DocumentViewContext(ctx, docview.Request{ID: root, Size: 100}); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(recentFile)
	var recent []*pageprefs.RecentDoc
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &recent); e != nil || len(recent) != 1 || recent[0].RootID != root {
		t.Fatal(string(raw), e)
	}
	if raw, e := os.ReadFile(file); e != nil || !bytes.Equal(raw, before) {
		t.Fatal("document read migrated source", e)
	}
	if e = os.Remove(recentFile); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(recentFile, 0700); e != nil {
		t.Fatal(e)
	}
	out, e = DocumentViewContext(ctx, docview.Request{ID: root, Size: 100})
	if e == nil || out.DOM != "" {
		t.Fatal("private storage failure hidden", out, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = DocumentViewContext(canceled, docview.Request{ID: root}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
