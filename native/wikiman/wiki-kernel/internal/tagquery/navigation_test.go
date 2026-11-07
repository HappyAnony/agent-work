package tagquery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestNavigationOriginalOrderDuplicatesMissingAndDetachedProjection(t *testing.T) {
	row := &indexdb.BlockRecord{ID: "a", RootID: "doc", Content: "<value>", HPath: "Document", Type: "p", IAL: "{: name=\"value\"}"}
	spans := []indexdb.SpanRecord{{BlockID: "a", Content: "tag"}, {BlockID: "z", Content: "tag"}, {BlockID: "a", Content: "tag"}, {BlockID: "a", Content: "other"}}
	got, err := Navigate(context.Background(), "two words", 17, true, NavigationServices{
		Spans: func(_ context.Context, q string, limit int) ([]indexdb.SpanRecord, error) {
			if q != "two words" || limit != 17 {
				t.Fatal(q, limit)
			}
			return spans, nil
		},
		Blocks: func(_ context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
			if !reflect.DeepEqual(ids, []string{"z", "a"}) {
				t.Fatal(ids)
			}
			return []*indexdb.BlockRecord{row, nil}, nil
		},
	})
	if err != nil || len(got["tag"]) != 3 || got["tag"][1] != nil || got["tag"][0].Type != "NodeParagraph" || got["tag"][0].HPath != "/Document" {
		t.Fatal(got, err)
	}
	got["tag"][0].Content = "caller changed"
	got["tag"][0].IAL["name"] = "caller changed"
	if got["tag"][2].Content == "caller changed" || got["other"][0].IAL["name"] == "caller changed" || row.Content != "<value>" {
		t.Fatal("projection aliases source/sibling")
	}
}

func TestNavigationEmptyAndLateFailuresNoPartial(t *testing.T) {
	for _, scenario := range []string{"empty", "no-spans", "no-blocks", "query-error", "blocks-error", "late-query", "late-blocks", "cancelled", "nil-context"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			s := NavigationServices{Spans: func(context.Context, string, int) ([]indexdb.SpanRecord, error) {
				if scenario == "query-error" {
					return nil, errors.New("query")
				}
				if scenario == "late-query" {
					cancel()
				}
				return nil, nil
			}, Blocks: func(context.Context, []string) ([]*indexdb.BlockRecord, error) {
				calls++
				if scenario == "blocks-error" {
					return nil, errors.New("blocks")
				}
				if scenario == "late-blocks" {
					cancel()
				}
				return nil, nil
			}}
			switch scenario {
			case "no-spans":
				s.Spans = nil
			case "no-blocks":
				s.Blocks = nil
			case "cancelled":
				cancel()
			case "nil-context":
				ctx = nil
			}
			got, err := Navigate(ctx, "", 1, false, s)
			if scenario == "empty" {
				if err != nil || got == nil || len(got) != 0 || calls != 1 {
					t.Fatal(got, err, calls)
				}
			} else if err == nil || got != nil {
				t.Fatal("partial success", got, err)
			}
			if scenario == "late-query" && calls != 0 {
				t.Fatal("read after cancel")
			}
		})
	}
}
