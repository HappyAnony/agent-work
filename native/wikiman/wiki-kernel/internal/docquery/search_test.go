package docquery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func docServices() Services {
	return Services{Notebooks: func(context.Context) ([]conf.Notebook, error) {
		return []conf.Notebook{{ID: "open", Name: "Notebook", Icon: "icon"}, {ID: "closed", Name: "Closed", Closed: true}}, nil
	}, Roots: func(context.Context, []string, []string) ([]indexdb.BlockRecord, error) {
		return []indexdb.BlockRecord{{ID: "root", RootID: "root", Box: "open", Path: "/root.docj", HPath: "/A"}, {ID: "other", Box: "closed"}, {ID: "missing", Box: "unknown"}}, nil
	}}
}
func TestDocumentNavigationOriginalTokenRulesEmptyQueryAndCopies(t *testing.T) {
	s := docServices()
	exclude := []string{"excluded"}
	queries := 0
	s.Roots = func(_ context.Context, words, ids []string) ([]indexdb.BlockRecord, error) {
		queries++
		if !reflect.DeepEqual(words, []string{"Notebook", "needle"}) || !reflect.DeepEqual(ids, exclude) {
			t.Fatal(words, ids)
		}
		words[0] = "changed"
		ids[0] = "changed"
		return []indexdb.BlockRecord{{ID: "r", Box: "open", Path: "/r.docj", HPath: "/A"}, {ID: "closed", Box: "closed"}}, nil
	}
	got, err := Search(context.Background(), Request{Keyword: "  Notebook\tneedle  ", ExcludeIDs: exclude}, s)
	if err != nil || len(got) != 2 || got[0]["hPath"] != "Notebook/" || got[1]["hPath"] != "Notebook/A" || got[1]["boxIcon"] != "icon" || exclude[0] != "excluded" {
		t.Fatal(got, err, exclude)
	}
	s.Roots = func(context.Context, []string, []string) ([]indexdb.BlockRecord, error) { queries++; return nil, nil }
	got, err = Search(context.Background(), Request{Keyword: "Note"}, s)
	if err != nil || len(got) != 0 {
		t.Fatal("notebook match changed to substring", got, err)
	}
	q := queries
	got, err = Search(context.Background(), Request{Keyword: " \n ", ExcludeIDs: []string{"open"}}, s)
	if err != nil || len(got) != 1 || queries != q || got[0]["path"] != "/" {
		t.Fatal("empty query should only list opened notebooks", got, err)
	}
	got[0]["boxIcon"] = "caller mutation"
	again, _ := Search(context.Background(), Request{}, s)
	if again[0]["boxIcon"] != "icon" {
		t.Fatal("result aliases owner")
	}
}
func TestDocumentNavigationFlashcardDeckCountsFilteringAndOrder(t *testing.T) {
	s := docServices()
	order := []string{}
	s.Cards = &Flashcards{Open: func(context.Context) (bool, error) { order = append(order, "open"); return true, nil }, Count: func(_ context.Context, scope, id string) (Counts, error) {
		order = append(order, scope+":"+id)
		return Counts{New: 2, Due: 3, Total: 5}, nil
	}}
	got, err := Search(context.Background(), Request{Keyword: "Notebook", Flashcard: true}, s)
	if err != nil || len(got) != 2 || got[0]["newFlashcardCount"] != "2" || got[1]["dueFlashcardCount"] != "3" || got[1]["flashcardCount"] != "5" || !reflect.DeepEqual(order, []string{"open", "box:open", "root:root"}) {
		t.Fatal(got, order, err)
	}
	s.Cards.Count = func(context.Context, string, string) (Counts, error) { return Counts{}, nil }
	got, err = Search(context.Background(), Request{Keyword: "Notebook", Flashcard: true}, s)
	if err != nil || len(got) != 0 {
		t.Fatal("zero-card results included", got, err)
	}
	s.Cards.Open = func(context.Context) (bool, error) { return false, nil }
	s.Notebooks = func(context.Context) ([]conf.Notebook, error) {
		t.Fatal("missing deck should return before notebooks")
		return nil, nil
	}
	got, err = Search(context.Background(), Request{Flashcard: true}, s)
	if err != nil || !reflect.DeepEqual(got, []map[string]string{}) {
		t.Fatal(got, err)
	}
}
func TestDocumentNavigationFailuresAndLateCancellationNoPartialSuccess(t *testing.T) {
	sentinel := errors.New("owner failed")
	for _, point := range []string{"deck", "notebooks", "roots", "count"} {
		for _, late := range []bool{false, true} {
			t.Run(point+map[bool]string{false: "/error", true: "/cancel"}[late], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := docServices()
				fail := func() error {
					if late {
						cancel()
						return nil
					}
					return sentinel
				}
				s.Cards = &Flashcards{Open: func(context.Context) (bool, error) { return true, nil }, Count: func(context.Context, string, string) (Counts, error) { return Counts{Total: 1}, nil }}
				switch point {
				case "deck":
					s.Cards.Open = func(context.Context) (bool, error) { return true, fail() }
				case "notebooks":
					s.Notebooks = func(context.Context) ([]conf.Notebook, error) {
						return []conf.Notebook{{ID: "open", Name: "Notebook"}}, fail()
					}
				case "roots":
					s.Roots = func(context.Context, []string, []string) ([]indexdb.BlockRecord, error) { return nil, fail() }
				case "count":
					s.Cards.Count = func(context.Context, string, string) (Counts, error) { return Counts{Total: 1}, fail() }
				}
				got, err := Search(ctx, Request{Keyword: "Notebook", Flashcard: true}, s)
				want := sentinel
				if late {
					want = context.Canceled
				}
				if got != nil || !errors.Is(err, want) {
					t.Fatal("partial result escaped", got, err)
				}
			})
		}
	}
}
func TestDocumentNavigationMissingServicesAndNoImplicitFlashcards(t *testing.T) {
	for _, point := range []string{"ctx", "notebooks", "roots", "cards", "open", "count"} {
		s := docServices()
		ctx := context.Background()
		q := Request{}
		switch point {
		case "ctx":
			ctx = nil
		case "notebooks":
			s.Notebooks = nil
		case "roots":
			s.Roots = nil
		case "cards":
			q.Flashcard = true
		case "open":
			q.Flashcard = true
			s.Cards = &Flashcards{Count: func(context.Context, string, string) (Counts, error) { return Counts{}, nil }}
		case "count":
			q.Flashcard = true
			s.Cards = &Flashcards{Open: func(context.Context) (bool, error) { return true, nil }}
		}
		if _, err := Search(ctx, q, s); err == nil {
			t.Fatal("unbound dependency accepted", point)
		}
	}
}
