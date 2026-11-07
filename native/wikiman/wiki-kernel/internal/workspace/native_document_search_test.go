//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
)

func TestNativeDocumentSearchRealOwnerProjectionNoFlushAndCardServices(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]interface{}
		if err = json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		doc["Properties"].(map[string]interface{})["title"] = "O'Reilly " + page
		raw, _ = json.Marshal(doc)
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
			return []conf.Notebook{{ID: boxID, Name: page, Icon: page + "-icon"}, {ID: "closed", Name: page, Closed: true}}, nil
		}, nil, nil), nil
	}})
	for _, page := range []string{pageA, pageB} {
		ctx := context.Background()
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			t.Fatal(err)
		}
		q := docquery.Request{Keyword: "O'Reilly"}
		got, err := l.NativeSearchDocuments(ctx, q, nil)
		if err != nil || len(got) != 0 {
			t.Fatal("navigation introduced implicit flush", got, err)
		}
		if err = l.FlushNativeMainQueue(ctx); err != nil {
			t.Fatal(err)
		}
		got, err = l.NativeSearchDocuments(ctx, q, nil)
		if err != nil || len(got) != 1 || !strings.Contains(got[0]["hPath"], "O'Reilly "+page) || got[0]["boxIcon"] != page+"-icon" || got[0]["path"] != "/"+docID+".docj" {
			t.Fatal("foreign owner result", got, err)
		}
		got[0]["boxIcon"] = "caller mutation"
		again, err := l.NativeSearchDocuments(ctx, q, nil)
		if err != nil || again[0]["boxIcon"] != page+"-icon" {
			t.Fatal("result alias", again, err)
		}
		q.ExcludeIDs = []string{docID}
		got, err = l.NativeSearchDocuments(ctx, q, nil)
		if err != nil || len(got) != 0 {
			t.Fatal("exclude ignored", got, err)
		}
		q = docquery.Request{Keyword: page, Flashcard: true}
		if _, err = l.NativeSearchDocuments(ctx, q, nil); err == nil {
			t.Fatal("flashcards silently fell back")
		}
		counts := 0
		cards := &docquery.Flashcards{Open: func(context.Context) (bool, error) { return true, nil }, Count: func(_ context.Context, scope, id string) (docquery.Counts, error) {
			if (scope == "box" && id != boxID) || (scope == "root" && id != docID) {
				t.Fatal("foreign count identity", scope, id)
			}
			counts++
			return docquery.Counts{New: 1, Due: 2, Total: 3}, nil
		}}
		got, err = l.NativeSearchDocuments(ctx, q, cards)
		if err != nil || len(got) != 2 || counts != 2 || got[1]["flashcardCount"] != "3" {
			t.Fatal(got, counts, err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err = l.NativeSearchDocuments(cancelled, q, cards); err == nil {
			t.Fatal("cancelled navigation succeeded")
		}
		l.Release()
		if _, err = l.NativeSearchDocuments(ctx, q, cards); err == nil {
			t.Fatal("released navigation succeeded")
		}
	}
	if digest(t, root) != before {
		t.Fatal("navigation wrote source")
	}
}
func TestNativeDocumentSearchRejectsMissingNotebookOwner(t *testing.T) {
	m := open(t, fixture(t), Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(context.Background(), storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if got, err := l.NativeSearchDocuments(context.Background(), docquery.Request{}, nil); err == nil || got != nil {
		t.Fatal("missing notebook service became empty successful store", got, err)
	}
}
