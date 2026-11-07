//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
)

const archiveDocID = "20261028030000-doc0001"
const archiveBlockID = "20261028030000-block01"

func archiveImportDoc(t *testing.T) NativeFlashcardImportDocument {
	t.Helper()
	raw, e := json.Marshal(map[string]interface{}{"ID": archiveDocID, "Type": "NodeDocument", "Properties": map[string]string{"id": archiveDocID, "title": "Imported"}, "Children": []interface{}{map[string]interface{}{"ID": archiveBlockID, "Type": "NodeParagraph", "Properties": map[string]string{"id": archiveBlockID, "custom-riff-decks": flashcard.BuiltinDeckID}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": "Imported archive"}}}}})
	if e != nil {
		t.Fatal(e)
	}
	return NativeFlashcardImportDocument{Box: boxID, Path: "/" + archiveDocID + ".docj", Data: raw}
}
func TestNativeFlashcardArchiveActualOwnersExportImportAndNoSourceWrites(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	dir := filepath.Join(root, "pages", pageA, "data", "storage", "riff")
	data := flashMemory{}
	for _, ext := range []string{".deck", ".cards"} {
		b, e := os.ReadFile(filepath.Join(dir, flashcard.BuiltinDeckID+ext))
		if e != nil {
			t.Fatal(e)
		}
		data[flashcard.BuiltinDeckID+ext] = b
	}
	c := conf.NewFlashcard()
	d, e := riff.LoadDeck(data, flashcard.BuiltinDeckID, c.RequestRetention, c.MaximumInterval, c.Weights, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	d.AddCard("20261028030000-card001", positionInside)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	for name, b := range data {
		if e = os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, flashOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		q := NativeFlashcardExportRequest{Scope: "tree", ID: docID}
		export, e := l.PlanNativeFlashcardExport(ctx, q)
		want := 2
		if l.PageID() == pageB {
			want = 1
		}
		if e != nil || export.Count != want || len(export.Files) != 2 || len(export.SourceHashes) < 1 {
			t.Fatal("foreign source export", export, e)
		}
		request := NativeFlashcardImportRequest{Files: export.Files, BlockIDs: map[string]string{blockID: archiveBlockID}, Documents: []NativeFlashcardImportDocument{archiveImportDoc(t)}}
		imported, e := l.PlanNativeFlashcardImport(ctx, request)
		if e != nil || imported.Count != 1 || len(imported.Changes) != 2 || len(imported.Trees) != 1 || imported.Trees[0].Box != boxID {
			t.Fatal(imported, e)
		}
		mem := flashMemory{}
		for name, v := range imported.Files {
			mem[name[len("storage/riff/"):]] = v
		}
		merged, e := riff.LoadDeck(mem, flashcard.BuiltinDeckID, c.RequestRetention, c.MaximumInterval, c.Weights, time.Now)
		if e != nil || merged.CountCards() != want+1 {
			t.Fatal("foreign destination merge", merged, e)
		}
		request.BlockIDs[blockID] = blockID
		if _, e = l.PlanNativeFlashcardImport(ctx, request); e == nil {
			t.Fatal("mapping escaped imported trees")
		}
	}
	if digest(t, root) != before {
		t.Fatal("archive phase wrote source")
	}
}
func TestNativeFlashcardArchiveRejectsExistingIDsAndReleasedOrCancelledOwner(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	m := open(t, root, flashOptions(root))
	l := getDocLease(t, m, pageA)
	raw, e := os.ReadFile(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.PlanNativeFlashcardImport(ctx, NativeFlashcardImportRequest{Documents: []NativeFlashcardImportDocument{{Box: boxID, Path: "/" + docID + ".docj", Data: raw}}}); !errors.Is(e, flashcard.ErrReviewConflict) {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = l.PlanNativeFlashcardExport(cancelled, NativeFlashcardExportRequest{Scope: "tree", ID: docID}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	l.Release()
	if _, e = l.PlanNativeFlashcardImport(ctx, NativeFlashcardImportRequest{}); e == nil {
		t.Fatal("released archive owner")
	}
}
