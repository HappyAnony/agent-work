package workspace

import (
	"context"
	"slices"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

// As in the original navigation query, no implicit index flush is performed.
// Optional flashcard services must belong to this owner and must not reenter its
// index gate. Nil uses the actual owner deck store.
func (l *Lease) NativeSearchDocuments(ctx context.Context, q docquery.Request, cards *docquery.Flashcards) (ret []map[string]string, err error) {
	q.ExcludeIDs = slices.Clone(q.ExcludeIDs)
	if cards != nil {
		copy := *cards
		cards = &copy
	}
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r := l.workspace.resources
		if q.Flashcard && cards == nil {
			cards = r.navigationFlashcards(l.workspace)
		}
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		settings := *r.configuration.Search
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = docquery.Search(ctx, q, docquery.Services{Notebooks: r.configurationBusiness.Notebooks, Roots: func(ctx context.Context, words, exclude []string) ([]indexdb.BlockRecord, error) {
			return indexdb.SearchDocumentRoots(ctx, db, settings, words, exclude)
		}, Cards: cards})
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
