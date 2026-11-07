package model

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func TestDocumentSearchEnvironmentOwnerQueryCountsAndSourceUnchanged(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newBookmarkHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		if err = l.QueueNativeTree(h.ctx, settingsBox, nameDoc, "index"); err != nil {
			t.Fatal(err)
		}
		if err = l.FlushNativeMainQueue(h.ctx); err != nil {
			t.Fatal(err)
		}
		e := &documentSearchEnvironment{ctx: context.Background(), services: docquery.Services{Notebooks: func(ctx context.Context) ([]conf.Notebook, error) {
			return []conf.Notebook{{ID: settingsBox, Name: page, Icon: "owner"}}, ctx.Err()
		}, Roots: func(ctx context.Context, words, exclude []string) (ret []indexdb.BlockRecord, err error) {
			err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
				db, e := b.Database(indexdb.Main)
				if e != nil {
					return e
				}
				ret, e = indexdb.SearchDocumentRoots(ctx, db, *conf.NewSearch(), words, exclude)
				return e
			})
			return
		}, Cards: &docquery.Flashcards{Open: func(context.Context) (bool, error) { return true, nil }, Count: func(context.Context, string, string) (docquery.Counts, error) {
			return docquery.Counts{New: 1, Total: 2}, nil
		}}}}
		h.env.documentSearch = e
		got, err := h.tx.searchDocuments(docquery.Request{Keyword: "Second owner", Flashcard: true})
		if err != nil || len(got) != 1 || got[0]["hPath"] != page+"/Second owner document" || got[0]["newFlashcardCount"] != "1" {
			t.Fatal(got, err)
		}
		got, err = h.tx.searchDocuments(docquery.Request{Keyword: "Second", ExcludeIDs: []string{nameDoc}})
		if err != nil || !reflect.DeepEqual(got, []map[string]string{}) {
			t.Fatal(got, err)
		}
		h.cancel()
		if got, err = h.tx.searchDocuments(docquery.Request{}); got != nil || !errors.Is(err, context.Canceled) {
			t.Fatal("outer context ignored", got, err)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}
func TestDocumentSearchEnvironmentNoImplicitGlobalServices(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, err := tx.searchDocuments(docquery.Request{}); err == nil {
		t.Fatal("missing environment fell back")
	}
	tx.environment.documentSearch = &documentSearchEnvironment{ctx: context.Background()}
	if _, err := tx.searchDocuments(docquery.Request{}); err == nil {
		t.Fatal("missing services accepted")
	}
}
