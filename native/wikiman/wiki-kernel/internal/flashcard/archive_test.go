package flashcard

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

const importedDoc = "20261028010000-doc0001"
const importedBlock = "20261028010000-block01"

func importTree() *parse.Tree {
	root := &ast.Node{ID: importedDoc, Type: ast.NodeDocument}
	root.SetIALAttr("id", root.ID)
	n := &ast.Node{ID: importedBlock, Type: ast.NodeParagraph}
	n.SetIALAttr("id", n.ID)
	n.SetIALAttr("custom-riff-decks", BuiltinDeckID+","+otherDeck)
	n.SetIALAttr("custom-extension", "keep")
	root.AppendChild(n)
	return &parse.Tree{ID: root.ID, Root: root, Box: mutationBox, Path: "/" + root.ID + ".docj"}
}
func archiveSource(t *testing.T) (map[string][]byte, ArchiveServices, *riff.Deck) {
	t.Helper()
	c := *conf.NewFlashcard()
	now := time.Now().Add(-time.Hour)
	clock := func() time.Time { return now }
	d, e := NewDeck(BuiltinDeckID, "source", c, clock)
	if e != nil {
		t.Fatal(e)
	}
	d.AddCard(cardID, blockID)
	d.Review(cardID, riff.Good)
	s := ArchiveServices{Settings: c, Now: clock, NewID: func() string { return "20261028010000-card001" }, Cards: func(_ context.Context, ids []string) ([]riff.Card, error) { return d.GetCardsByBlockIDs(ids), nil }, Destination: func(context.Context) (*riff.Deck, error) { return nil, nil }}
	p, e := PrepareExport(context.Background(), []*parse.Tree{mutationTree()}, s)
	if e != nil {
		t.Fatal(e)
	}
	return p.Files, s, d
}
func TestFlashcardArchiveRoundtripRemapsAndResetsLearning(t *testing.T) {
	ctx := context.Background()
	files, s, source := archiveSource(t)
	payload := memory{}
	for p, b := range files {
		payload[p[len(archiveBase):]] = b
	}
	d, e := riff.LoadDeck(payload, BuiltinDeckID, s.Settings.RequestRetention, s.Settings.MaximumInterval, s.Settings.Weights, s.Now)
	if e != nil || d.CountCards() != 1 || d.GetCard(cardID).GetReps() != 0 || source.GetCard(cardID).GetReps() != 1 {
		t.Fatal(d, e)
	}
	tree := importTree()
	p, e := PrepareImport(ctx, files, map[string]string{blockID: importedBlock}, []*parse.Tree{tree}, s)
	if e != nil || p.Count != 1 || len(p.Files) != 2 || tree.Root.FirstChild.IALAttr("custom-riff-decks") != BuiltinDeckID || tree.Root.FirstChild.IALAttr("custom-extension") != "keep" {
		t.Fatal(p, e)
	}
	data := memory{}
	for path, b := range p.Files {
		data[path[len(archiveBase):]] = b
	}
	d, e = riff.LoadDeck(data, BuiltinDeckID, s.Settings.RequestRetention, s.Settings.MaximumInterval, s.Settings.Weights, s.Now)
	if e != nil || d.GetCard(cardID) != nil || len(d.GetCardsByBlockID(importedBlock)) != 1 || d.GetCardsByBlockID(importedBlock)[0].GetReps() != 0 {
		t.Fatal(d, e)
	}
	p, e = PrepareImport(ctx, files, map[string]string{}, []*parse.Tree{importTree()}, s)
	if e != nil || p.Count != 0 || p.Skipped != 1 || len(p.Files) != 0 {
		t.Fatal("unmapped card was imported", p, e)
	}
}
func TestFlashcardArchiveValidationCancellationAndPublicationFailure(t *testing.T) {
	ctx := context.Background()
	files, s, _ := archiveSource(t)
	for _, bad := range []map[string][]byte{{archiveBase + BuiltinDeckID + ".deck": files[archiveBase+BuiltinDeckID+".deck"]}, {"../escape": []byte("bad")}, {archiveBase + BuiltinDeckID + ".deck": nil, archiveBase + BuiltinDeckID + ".cards": nil}} {
		if _, e := PrepareImport(ctx, bad, map[string]string{blockID: importedBlock}, []*parse.Tree{importTree()}, s); e == nil {
			t.Fatal("bad archive accepted")
		}
	}
	if _, e := PrepareImport(ctx, files, map[string]string{blockID: mutationBlock2}, []*parse.Tree{importTree()}, s); !errors.Is(e, ErrInvalid) {
		t.Fatal("foreign mapping accepted", e)
	}
	failure := errors.New("archive stage failed")
	s.ApplyImport = func(context.Context, ArchivePlan) error { return failure }
	if _, e := ExecuteImport(ctx, files, map[string]string{blockID: importedBlock}, []*parse.Tree{importTree()}, s); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	s.Cards = func(context.Context, []string) ([]riff.Card, error) { cancel(); return nil, nil }
	writes := 0
	s.WriteExport = func(context.Context, ArchivePlan) error { writes++; return nil }
	if _, e := ExecuteExport(cancelled, []*parse.Tree{mutationTree()}, s); !errors.Is(e, context.Canceled) || writes != 0 {
		t.Fatal(e, writes)
	}
}
func TestFlashcardArchiveEmptyExportAndExistingDestinationStayDetached(t *testing.T) {
	ctx := context.Background()
	files, s, _ := archiveSource(t)
	dest, e := NewDeck(BuiltinDeckID, "destination", *conf.NewFlashcard(), time.Now)
	if e != nil {
		t.Fatal(e)
	}
	dest.AddCard("20261028020000-card001", mutationBlock2)
	dest.Review("20261028020000-card001", riff.Easy)
	s.Destination = func(context.Context) (*riff.Deck, error) { return dest, nil }
	p, e := PrepareImport(ctx, files, map[string]string{blockID: importedBlock}, []*parse.Tree{importTree()}, s)
	if e != nil || p.Count != 1 || dest.CountCards() != 1 || dest.GetCard("20261028020000-card001").GetReps() != 1 {
		t.Fatal(p, e)
	}
	s.Cards = func(context.Context, []string) ([]riff.Card, error) { return nil, nil }
	p, e = PrepareExport(ctx, []*parse.Tree{mutationTree()}, s)
	if e != nil || len(p.Files) != 0 {
		t.Fatal(p, e)
	}
}
