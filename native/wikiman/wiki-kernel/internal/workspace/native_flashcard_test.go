//go:build fts5

package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
)

type flashMemory map[string][]byte

func (m flashMemory) Read(p string) ([]byte, error) {
	b, ok := m[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return slices.Clone(b), nil
}
func (m flashMemory) Write(p string, b []byte) error { m[p] = slices.Clone(b); return nil }

const flashCardID = "20261027000000-card001"

func flashOptions(root string) Options {
	opts := lifecycleOptions(root)
	opts.ConfigurationBusiness = treeOptions(root).ConfigurationBusiness
	old := opts.Configuration
	opts.Configuration = func(store, page string) (conf.AppData, error) {
		c, e := old(store, page)
		c.Flashcard = conf.NewFlashcard()
		c.Lang = "en_US"
		return c, e
	}
	return opts
}
func flashSource(t *testing.T) string {
	t.Helper()
	root := positionSource(t)
	settings := conf.NewFlashcard()
	for _, page := range []string{pageA, pageB} {
		// infoSource deliberately includes a directory named counted.docj for
		// statistics tests. Tree listing correctly rejects that malformed source.
		if e := os.Remove(filepath.Join(root, "pages", page, "data", boxID, docID, "counted.docj")); e != nil {
			t.Fatal(e)
		}
		m := flashMemory{}
		d, e := riff.LoadDeck(m, flashcard.BuiltinDeckID, settings.RequestRetention, settings.MaximumInterval, settings.Weights, time.Now)
		if e != nil {
			t.Fatal(e)
		}
		d.Name = page
		d.AddCard(flashCardID, blockID)
		if page == pageB {
			d.GetCard(flashCardID).SetDue(time.Now().Add(24 * time.Hour))
		}
		if e = d.Save(); e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "riff")
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		for p, b := range m {
			if e = os.WriteFile(filepath.Join(dir, p), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	return root
}
func TestNativeFlashcardRealOwnersCountCandidatesAndLifecycle(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	before := digest(t, root)
	m := open(t, root, flashOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		queueGetDoc(t, l)
		ids, e := l.NativeFlashcardBlockIDs(ctx, "root", docID)
		if e != nil || !slices.Contains(ids, blockID) || !slices.Contains(ids, infoChild) {
			t.Fatal(ids, e)
		}
		tree, e := l.NativeFlashcardBlockIDs(ctx, "tree", docID)
		if e != nil || len(tree) == 0 || slices.Contains(tree, infoChild) {
			t.Fatal(tree, e)
		}
		cards, e := l.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil)
		if e != nil || len(cards) != 1 || cards[0].BlockID() != blockID {
			t.Fatal(cards, e)
		}
		got, e := l.NativeSearchDocuments(ctx, docquery.Request{Keyword: "", Flashcard: true}, nil)
		wantDue := "1"
		if l.PageID() == pageB {
			wantDue = "0"
		}
		if e != nil || len(got) != 1 || got[0]["flashcardCount"] != "1" || got[0]["dueFlashcardCount"] != wantDue {
			t.Fatal("foreign deck count", got, e)
		}
		listed, total, e := l.NativeListDocumentTree(ctx, doctree.Request{Box: boxID, Path: "/", Max: 20, Flashcard: true}, nil)
		if e != nil || total != 1 || len(listed) != 1 || listed[0].ID != docID || listed[0].FlashcardCount != 1 {
			t.Fatal("real owner tree counts", listed, total, e)
		}
		due, e := l.NativeDueFlashcards(ctx, flashcard.BuiltinDeckID, "root", docID, nil)
		want := 1
		if l.PageID() == pageB {
			want = 0
		}
		if e != nil || len(due.Cards) != want {
			t.Fatal(due, e)
		}
		changes, e := l.PlanNativeFlashcard(ctx, flashcard.Edit{Action: "rename", DeckID: flashcard.BuiltinDeckID, Name: l.PageID() + " renamed"})
		if e != nil || len(changes) != 1 || len(changes[0].Before) == 0 || len(changes[0].After) == 0 {
			t.Fatal(changes, e)
		}
		if _, e = l.PlanNativeFlashcard(ctx, flashcard.Edit{Action: "add", DeckID: flashcard.BuiltinDeckID, CardID: "20261027000001-card002", BlockID: "20261027000001-missing"}); e == nil {
			t.Fatal("foreign block accepted")
		}
	}
	if e := a.SkipNativeFlashcard(ctx, flashcard.BuiltinDeckID, flashCardID); e != nil {
		t.Fatal(e)
	}
	due, e := a.NativeDueFlashcards(ctx, flashcard.BuiltinDeckID, "box", boxID, []string{"round-in-progress"})
	if e != nil || len(due.Cards) != 0 {
		t.Fatal(due, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = a.NativeFlashcardDecks(cancelled); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	state := a.workspace.resources.flashcards
	a.Release()
	b.Release()
	if _, e = a.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil); e == nil {
		t.Fatal("released lease read")
	}
	if e = m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = state.Decks(ctx); !errors.Is(e, flashcard.ErrClosed) {
		t.Fatal("owner failed to close decks", e)
	}
	if digest(t, root) != before {
		t.Fatal("candidate or read wrote source")
	}
}

func TestNativeFlashcardReloadAndSymlinkRefusal(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	m := open(t, root, flashOptions(root))
	l := getDocLease(t, m, pageA)
	_, e := l.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil)
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(root, "pages", pageA, "data", "storage", "riff", flashcard.BuiltinDeckID+".cards")
	future := filepath.Join(root, "pages", pageB, "data", "storage", "riff", flashcard.BuiltinDeckID+".cards")
	b, e := os.ReadFile(future)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, b, 0600); e != nil {
		t.Fatal(e)
	}
	due, e := l.NativeDueFlashcards(ctx, flashcard.BuiltinDeckID, "root", docID, nil)
	if e != nil || len(due.Cards) != 0 {
		t.Fatal("stale cards", due, e)
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(future, file); e != nil {
		t.Fatal(e)
	}
	if _, e = l.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil); e == nil {
		t.Fatal("foreign source symlink accepted")
	}
}
