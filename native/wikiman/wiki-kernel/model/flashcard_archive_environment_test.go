package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
)

func archiveModelTree(id string) *parse.Tree {
	n := &ast.Node{ID: id, Type: ast.NodeDocument}
	n.SetIALAttr("id", id)
	return &parse.Tree{ID: id, Root: n}
}
func TestFlashcardArchiveEnvironmentPropagatesOwnerAndRefusesFallback(t *testing.T) {
	ctx := context.WithValue(context.Background(), "archive-owner", "owner-a")
	check := func(ctx context.Context) {
		if ctx.Value("archive-owner") != "owner-a" {
			t.Fatal("lost owner")
		}
	}
	writes, imports := 0, 0
	id := "20261028050000-doc0001"
	services := flashcard.ArchiveServices{Settings: *conf.NewFlashcard(), Now: time.Now, NewID: ast.NewNodeID, Cards: func(ctx context.Context, _ []string) ([]riff.Card, error) { check(ctx); return nil, nil }, Destination: func(ctx context.Context) (*riff.Deck, error) { check(ctx); return nil, nil }, WriteExport: func(ctx context.Context, p flashcard.ArchivePlan) error { check(ctx); writes++; return nil }, ApplyImport: func(ctx context.Context, p flashcard.ArchivePlan) error { check(ctx); imports++; return nil }}
	e := &flashcardEnvironment{gate: newAttributeMutationGate(), archive: services}
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx, flashcard: e}}
	if _, err := tx.exportFlashcardArchive([]*parse.Tree{archiveModelTree(id)}, "/must-not-use-other-owner"); err != nil || writes != 1 {
		t.Fatal(err, writes)
	}
	if _, err := tx.importFlashcardArchive(nil, nil, []*parse.Tree{archiveModelTree(id)}); err != nil || imports != 1 {
		t.Fatal(err, imports)
	}
	tx.environment.flashcard = nil
	if _, err := tx.exportFlashcardArchive(nil, ""); !errors.Is(err, flashcard.ErrDependency) {
		t.Fatal("legacy fallback", err)
	}
	tx.environment.flashcard = e
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	tx.environment.ctx = cancelled
	if _, err := tx.importFlashcardArchive(nil, nil, nil); !errors.Is(err, context.Canceled) || imports != 1 {
		t.Fatal(err, imports)
	}
}
func TestFlashcardArchiveExtractionReaderRejectsSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	if files, e := readLegacyFlashcardArchive(context.Background(), root); e != nil || len(files) != 0 {
		t.Fatal(files, e)
	}
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(root, "storage")); e != nil {
		t.Fatal(e)
	}
	if _, e := readLegacyFlashcardArchive(context.Background(), root); e == nil {
		t.Fatal("archive storage symlink accepted")
	}
	if e := os.Remove(filepath.Join(root, "storage")); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "storage", "riff")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(outside, "card")
	if e := os.WriteFile(file, []byte("secret synthetic bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(file, filepath.Join(dir, builtinDeckID+".cards")); e != nil {
		t.Fatal(e)
	}
	if _, e := readLegacyFlashcardArchive(context.Background(), root); e == nil {
		t.Fatal("archive card symlink accepted")
	}
}
