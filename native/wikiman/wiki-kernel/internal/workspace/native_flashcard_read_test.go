//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
)

func flashReadOptions(root string) Options {
	o := flashOptions(root)
	o.ConfigurationBusiness = func(_, page string) (*conf.Business, error) {
		raw, e := os.ReadFile("../../../wiki-app/appearance/langs/en_US.json")
		if e != nil {
			return nil, e
		}
		var lang struct {
			Time map[string]string `json:"_time"`
		}
		if e = json.Unmarshal(raw, &lang); e != nil {
			return nil, e
		}
		return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return []conf.Notebook{{ID: boxID, Name: page}}, nil }, map[string]map[int]string{"en_US": {180: "missing " + page}}, nil, map[string]map[string]string{"en_US": lang.Time}), nil
	}
	return o
}
func TestNativeFlashcardPagesProjectionAndDocumentLimitsRealOwners(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	addSecondReviewCard(t, root)
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		json.Unmarshal(raw, &d)
		limit := "0"
		if page == pageB {
			limit = "1"
		}
		d["Properties"].(map[string]interface{})["custom-riff-new-card-limit"] = limit
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, flashReadOptions(root))
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		queueGetDoc(t, l)
		unflushed, e := l.NativeFlashcardPage(ctx, flashcard.PageRequest{Scope: "deck", ID: flashcard.BuiltinDeckID, Page: 1, PageSize: 20})
		if e != nil || unflushed.Blocks[0].RiffCard != nil {
			t.Fatal("read implicitly flushed index", unflushed, e)
		}
		if e = l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		for _, scope := range []string{"deck", "tree", "notebook"} {
			id := flashcard.BuiltinDeckID
			if scope == "tree" {
				id = docID
			}
			if scope == "notebook" {
				id = boxID
			}
			p, e := l.NativeFlashcardPage(ctx, flashcard.PageRequest{Scope: scope, ID: id, Page: 1, PageSize: 1})
			if e != nil || p.Total != 2 || p.PageCount != 2 || len(p.Blocks) != 1 || p.Blocks[0].RiffCardID == "" {
				t.Fatalf("scope=%s page=%+v block=%+v err=%v", scope, p, p.Blocks[0], e)
			}
		}
		p, e := l.NativeFlashcardPage(ctx, flashcard.PageRequest{Scope: "blocks", IDs: []string{blockID, "missing", blockID}})
		if e != nil || len(p.Blocks) != 3 || p.Blocks[1].Content != "missing "+page {
			t.Fatal(p, e)
		}
		owned, e := l.NativeFlashcardPage(ctx, flashcard.PageRequest{Scope: "blocks", IDs: []string{positionInside}})
		if e != nil || !strings.Contains(owned.Blocks[0].Content, page) {
			t.Fatal("foreign SQL projection", owned, e)
		}
		due, e := l.NativeFlashcardDueView(ctx, "", "root", docID, nil)
		want := 0
		if page == pageB {
			want = 1
		}
		if e != nil || len(due.Cards) != want {
			t.Fatal("owner limits ignored", page, due, e)
		}
		if page == pageA {
			file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
			original, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			var d map[string]interface{}
			if e = json.Unmarshal(original, &d); e != nil {
				t.Fatal(e)
			}
			d["Properties"].(map[string]interface{})["custom-riff-new-card-limit"] = "1"
			raw, _ := json.Marshal(d)
			if e = os.WriteFile(file, raw, 0600); e != nil {
				t.Fatal(e)
			}
			fresh, e := l.NativeFlashcardDueView(ctx, "", "root", docID, nil)
			if e != nil || len(fresh.Cards) != 1 {
				t.Fatal("stale document limit", fresh, e)
			}
			if e = os.WriteFile(file, []byte("broken"), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = l.NativeFlashcardDueView(ctx, "", "root", docID, nil); e == nil {
				t.Fatal("corrupt document limit source accepted")
			}
			if e = os.WriteFile(file, original, 0600); e != nil {
				t.Fatal(e)
			}
		}
		if page == pageA {
			all, e := l.NativeFlashcardDueView(ctx, "", "notebook", boxID, nil)
			if e != nil || len(all.Cards) != 2 || len(all.Cards[0].NextDues) != 4 {
				t.Fatal("doc limit leaked into notebook", all, e)
			}
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e = l.NativeFlashcardPage(cancelled, flashcard.PageRequest{Scope: "deck", Page: 1, PageSize: 20}); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		l.Release()
		if _, e = l.NativeFlashcardDueView(ctx, "", "root", docID, nil); e == nil {
			t.Fatal("released view")
		}
	}
	if digest(t, root) != before {
		t.Fatal("read changed source")
	}
}
