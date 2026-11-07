package doctree

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/docquery"
)

const first = "20261026090000-doc0001"
const second = "20261026090001-doc0002"
const hidden = "20261026090002-doc0003"

func treeServices() Services {
	return Services{Settings: func(context.Context, string) (int, int, error) {
		return configtypes.SortModeFileTree, configtypes.SortModeNameASC, nil
	},
		List: func(context.Context, string) ([]Entry, error) {
			return []Entry{{Path: "/" + first + ".docj", Name: first + ".docj", Size: 10}, {Path: "/" + first + "/", Name: first, Directory: true}, {Path: "/" + second + ".docj", Name: second + ".docj", Size: 20}, {Path: "/" + hidden + ".docj", Name: hidden + ".docj"}, {Path: "/invalid.docj", Name: "invalid.docj"}, {Path: "/other/", Name: "other", Directory: true}, {Path: "/20261026100000-orphan1/", Name: "20261026100000-orphan1", Directory: true}}, nil
		},
		Stat: func(_ context.Context, p string) (*Entry, error) {
			if p == "/"+first+".docj" {
				return &Entry{Path: p, Name: first + ".docj", Size: 10}, nil
			}
			return nil, nil
		},
		IsDir: func(_ context.Context, p string) (bool, error) { return p == "/"+first, nil },
		IAL: func(_ context.Context, p string) (map[string]string, error) {
			if strings.Contains(p, "visible") {
				return map[string]string{}, nil
			}
			if strings.Contains(p, "hidden-child") {
				return map[string]string{"custom-hidden": "true"}, nil
			}
			id := strings.TrimSuffix(strings.TrimPrefix(p, "/"), ".docj")
			m := map[string]string{"id": id}
			if id == hidden {
				m["custom-hidden"] = "true"
			}
			return m, nil
		},
		Children: func(context.Context, string) ([]Entry, error) {
			return []Entry{{Name: "visible.docj"}, {Name: "hidden-child.docj"}, {Name: "note.txt"}}, nil
		},
		Project: func(_ context.Context, f Entry, a map[string]string) (*File, error) {
			name := "B.docj"
			if a["id"] == second {
				name = "a.docj"
			}
			v := int64(f.Size)
			return &File{ID: a["id"], Path: f.Path, Name: name, Size: uint64(f.Size), Mtime: v, CTime: v}, nil
		},
		References: func(context.Context) (map[string]int, error) { return map[string]int{first: 4, second: 2}, nil }, Sorts: func(_ context.Context, files []*File) error {
			for _, f := range files {
				f.Sort = 0
			}
			return nil
		},
	}
}
func TestDocumentTreeMergeHiddenTotalsAndOriginalSortModes(t *testing.T) {
	s := treeServices()
	ctx := context.Background()
	q := Request{Sort: configtypes.SortModeUnassigned, Max: 1}
	got, total, err := List(ctx, q, s)
	if err != nil || total != 2 || len(got) != 1 || got[0].ID != second {
		t.Fatal(got, total, err)
	}
	for mode := 0; mode <= 14; mode++ {
		q.Sort = mode
		q.Max = 10
		got, total, err = List(ctx, q, s)
		if err != nil || total != 2 {
			t.Fatal(mode, total, err)
		}
		want := first
		switch mode {
		case 0, 3, 5, 6, 7, 10, 12, 13:
			want = second
		}
		if got[0].ID != want {
			t.Fatal("sort mode", mode, got[0].ID, want)
		}
		for _, f := range got {
			if f.ID == first && (f.SubFileCount != 1 || f.Count != 4) {
				t.Fatal("merged parent/child count", f)
			}
		}
	}
	q.ShowHidden = true
	q.Sort = configtypes.SortModeFileTree
	q.ShowHidden = false
	q.Max = 10
	got, total, err = List(ctx, q, s)
	if err != nil || total != 2 || got[0].ID != first {
		t.Fatal("unknown/no-sort mode reordered original entries", got, err)
	}
	q.ShowHidden = true
	got, total, err = List(ctx, q, s)
	if err != nil || total != 3 {
		t.Fatal(got, total, err)
	}
	q.Max = 0
	got, total, err = List(ctx, q, s)
	if err != nil || len(got) != 0 || total != 3 {
		t.Fatal("total must be before truncate", got, total, err)
	}
}
func TestDocumentTreeCardsFallbackAndOwnerCacheCopies(t *testing.T) {
	s := treeServices()
	cached := &File{ID: first, Name: "Cached"}
	s.Project = func(context.Context, Entry, map[string]string) (*File, error) { return cached, nil }
	s.Cards = &docquery.Flashcards{Open: func(context.Context) (bool, error) { return true, nil }, Count: func(_ context.Context, scope, id string) (docquery.Counts, error) {
		if scope != "root" {
			t.Fatal(scope)
		}
		if id == second {
			return docquery.Counts{}, nil
		}
		return docquery.Counts{New: 1, Due: 2, Total: 3}, nil
	}}
	got, total, err := List(context.Background(), Request{Sort: 6, Max: 10, Flashcard: true}, s)
	if err != nil || total != 1 || got[0].FlashcardCount != 3 || cached.Count != 0 || cached.SubFileCount != 0 {
		t.Fatal(got, total, cached, err)
	}
	s.Cards.Open = func(context.Context) (bool, error) { return false, nil }
	s.Settings = func(context.Context, string) (int, int, error) {
		t.Fatal("missing deck must return before box")
		return 0, 0, nil
	}
	got, total, err = List(context.Background(), Request{Max: 10, Flashcard: true}, s)
	if err != nil || total != 0 || !reflect.DeepEqual(got, []*File{}) {
		t.Fatal(got, total, err)
	}
}
func TestDocumentTreeErrorsLateCancelAndLegacyChildPolicy(t *testing.T) {
	sentinel := errors.New("owner error")
	for _, point := range []string{"settings", "list", "stat", "ial", "children", "dir", "project", "refs", "sorts", "cards"} {
		for _, late := range []bool{false, true} {
			t.Run(point+map[bool]string{false: "/error", true: "/cancel"}[late], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := treeServices()
				bad := func() error {
					if late {
						cancel()
						return nil
					}
					return sentinel
				}
				switch point {
				case "settings":
					s.Settings = func(context.Context, string) (int, int, error) { return 0, 0, bad() }
				case "list":
					s.List = func(context.Context, string) ([]Entry, error) { return nil, bad() }
				case "stat":
					s.Stat = func(context.Context, string) (*Entry, error) { return nil, bad() }
				case "ial":
					s.IAL = func(context.Context, string) (map[string]string, error) { return nil, bad() }
				case "children":
					s.Children = func(context.Context, string) ([]Entry, error) { return nil, bad() }
				case "dir":
					s.IsDir = func(context.Context, string) (bool, error) { return false, bad() }
				case "project":
					s.Project = func(context.Context, Entry, map[string]string) (*File, error) { return nil, bad() }
				case "refs":
					s.References = func(context.Context) (map[string]int, error) { return nil, bad() }
				case "sorts":
					s.Sorts = func(context.Context, []*File) error { return bad() }
				case "cards":
					s.Cards = &docquery.Flashcards{Open: func(context.Context) (bool, error) { return true, nil }, Count: func(context.Context, string, string) (docquery.Counts, error) {
						return docquery.Counts{Total: 1}, bad()
					}}
				}
				got, total, err := List(ctx, Request{Sort: 6, Max: 10, Flashcard: point == "cards"}, s)
				want := sentinel
				if late {
					want = context.Canceled
				}
				if got != nil || total != 0 || !errors.Is(err, want) {
					t.Fatal(got, total, err)
				}
			})
		}
	}
	s := treeServices()
	s.Legacy = true
	s.Children = func(context.Context, string) ([]Entry, error) { return nil, sentinel }
	if _, _, err := List(context.Background(), Request{Max: 10}, s); err != nil {
		t.Fatal("legacy child read error policy", err)
	}
	if _, _, err := List(context.Background(), Request{Max: -1}, s); err == nil {
		t.Fatal("negative limit accepted")
	}
	s.Settings = nil
	if _, _, err := List(context.Background(), Request{}, s); err == nil {
		t.Fatal("missing dependency accepted")
	}
}
func TestDocumentTreeMetadataFieldsAndCancellation(t *testing.T) {
	attrs := map[string]string{"id": first, "title": "标题", "updated": "20261026100000", "icon": "icon", "name": "name", "alias": "alias", "memo": "memo", "bookmark": "bookmark"}
	got, err := Metadata(context.Background(), Entry{Path: "/x.docj", Size: 2048}, attrs, time.UTC, func(time.Time) (string, error) { return "relative", nil })
	if err != nil || got.Name != "标题.docj" || got.Size != 2048 || got.Name1 != "name" || got.HSize == "" || !strings.HasSuffix(got.HMtime, ", relative") || got.Mtime <= got.CTime || got.Bookmark != "bookmark" {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if got, err = Metadata(ctx, Entry{}, attrs, time.UTC, func(time.Time) (string, error) { cancel(); return "", nil }); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(got, err)
	}
	if _, err = Metadata(context.Background(), Entry{}, map[string]string{}, time.UTC, func(time.Time) (string, error) { return "", nil }); err == nil {
		t.Fatal("bad ID accepted")
	}
}
