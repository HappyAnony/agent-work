// SPDX-License-Identifier: AGPL-3.0-or-later
package historyindex

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"os"
	"testing"
	"time"
	"wiki-kernel/internal/indexrepair"
)

func historySource(t *testing.T) indexrepair.Source {
	r, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	return indexrepair.Source{Data: r}
}

const historyName = "2026-10-29-200000-sync"

func TestHistoryPlanOriginalDocumentAssetAndReadonlyFormat(t *testing.T) {
	s := historySource(t)
	s.Data.MkdirAll(historyName+"/box", 0700)
	s.Data.MkdirAll(historyName+"/assets/folder", 0700)
	raw := []byte(`{"ID":"20261029200000-docaaaa","Type":"NodeDocument","Properties":{"id":"20261029200000-docaaaa","title":""},"Children":[{"Type":"NodeParagraph","ID":"20261029200001-paraaaa","Children":[{"Type":"NodeText","Data":"synthetic history"}]}]}`)
	name := historyName + "/box/20261029200000-docaaaa.docj"
	s.Data.WriteFile(name, raw, 0600)
	s.Data.WriteFile(historyName+"/assets/picture-20261029200002-assetxx.png", []byte("asset"), 0600)
	s.Data.WriteFile(historyName+"/assets/folder/plain.bin", []byte("plain"), 0600)
	entries, e := Plan(context.Background(), s, historyName, "Untitled", time.UTC, lute.New())
	if e != nil || len(entries) != 3 {
		t.Fatal(entries, e)
	}
	if entries[0].Type != 1 || entries[0].Title != "Untitled" || entries[0].Content != "synthetic history" || entries[0].Path != name || entries[1].Type != 2 {
		t.Fatal(entries)
	}
	foundID := false
	for _, v := range entries {
		if v.ID == "20261029200002-assetxx" {
			foundID = true
		}
		if v.Created != "1793304000" {
			t.Fatal(v.Created)
		}
	}
	if !foundID {
		t.Fatal("original asset ID missing")
	}
	after, _ := s.Data.ReadFile(name)
	if string(after) != string(raw) {
		t.Fatal("old Spec source changed")
	}
}
func TestHistoryPlanRejectsIncompleteScanAndInvalidDirectory(t *testing.T) {
	s := historySource(t)
	s.Data.MkdirAll(historyName, 0700)
	s.Data.WriteFile(historyName+"/bad.docj", []byte("bad"), 0600)
	if out, e := Plan(context.Background(), s, historyName, "Untitled", time.Local, lute.New()); e == nil || out != nil {
		t.Fatal(out, e)
	}
	for _, name := range []string{"../", historyName + "/nested", "bad", "2026-10-29-200000-unknown"} {
		if _, _, e := Directory(name, time.Local); !errors.Is(e, ErrDirectory) {
			t.Fatal(name, e)
		}
	}
	notDirectory := "2026-10-29-210000-sync"
	s.Data.WriteFile(notDirectory, []byte("fixture"), 0600)
	if _, e := Plan(context.Background(), s, notDirectory, "Untitled", time.Local, lute.New()); !errors.Is(e, ErrDirectory) {
		t.Fatal("non-directory history root accepted", e)
	}
	s.Data.Remove(historyName + "/bad.docj")
	s.Data.Symlink(t.TempDir(), historyName+"/link")
	if _, e := Plan(context.Background(), s, historyName, "Untitled", time.Local, lute.New()); !errors.Is(e, indexrepair.ErrSourcePath) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Plan(ctx, s, historyName, "Untitled", time.Local, lute.New()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
