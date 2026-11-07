package docquery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func pathServices() PathServices {
	return PathServices{
		LoadPath: func(_ context.Context, box, p string) (Location, error) {
			return Location{Path: p, Box: box, HPath: "/Parent/Child &amp; title"}, nil
		},
		LoadID: func(context.Context, string) (Location, error) {
			return Location{Path: "/root/child.docj", Box: "box", HPath: "/Parent/Child &amp; title"}, nil
		},
		BoxName: func(context.Context, string) (string, bool, error) { return "Notebook", true, nil },
		BoxesByPaths: func(context.Context, []string) (map[string]*conf.Notebook, error) {
			return map[string]*conf.Notebook{"/actual.docj": {ID: "box", Name: "Notebook"}, "/missing-box.docj": nil, "/missing-tree.docj": {ID: "box", Name: "Notebook"}}, nil
		},
		BlockByPath: func(_ context.Context, p string) (*indexdb.BlockTreeRecord, error) {
			if p == "/missing-tree.docj" {
				return nil, nil
			}
			return &indexdb.BlockTreeRecord{HPath: "/Indexed &amp; title"}, nil
		},
		Roots: func(context.Context, string, string) ([]string, error) {
			return []string{"second", "first", "second"}, nil
		},
		MissingBox: func() error { return ErrPathBoxMissing },
	}
}
func TestDocumentPathsSixModesAndOriginalShapes(t *testing.T) {
	ctx := context.Background()
	s := pathServices()
	for _, mode := range []string{"path", "id", "location", "full"} {
		r, err := Paths(ctx, PathRequest{Mode: mode, Box: "box", Path: "/x.docj", ID: "child"}, s)
		if err != nil {
			t.Fatal(err)
		}
		want := "/Parent/Child &amp; title"
		if mode == "full" {
			want = "Notebook" + want
		}
		if r.HPath != want || r.Box != "box" {
			t.Fatal(mode, r)
		}
	}
	r, err := Paths(ctx, PathRequest{Mode: "path", Path: "/"}, PathServices{})
	if err != nil || r.HPath != "/" {
		t.Fatal("root shortcut", r, err)
	}
	warns := 0
	s.Warn = func(string, string) { warns++ }
	r, err = Paths(ctx, PathRequest{Mode: "batch", Paths: []string{"/alias/actual.docj", "/actual.docj"}}, s)
	if err != nil || !reflect.DeepEqual(r.Paths, []string{"Notebook/Indexed & title"}) || warns != 2 {
		t.Fatal(r, warns, err)
	}
	r, err = Paths(ctx, PathRequest{Mode: "ids"}, s)
	if err != nil || !reflect.DeepEqual(r.IDs, []string{"second", "first"}) {
		t.Fatal(r, err)
	}
	s.Roots = func(context.Context, string, string) ([]string, error) { return nil, nil }
	r, err = Paths(ctx, PathRequest{Mode: "ids"}, s)
	if err != nil || !reflect.DeepEqual(r.IDs, []string{}) {
		t.Fatal("empty IDs shape", r, err)
	}
	s.BoxesByPaths = func(context.Context, []string) (map[string]*conf.Notebook, error) { return nil, nil }
	r, err = Paths(ctx, PathRequest{Mode: "batch"}, s)
	if err != nil || r.Paths != nil {
		t.Fatal("empty batch must remain nil", r, err)
	}
}
func TestDocumentPathsErrorsLateCancellationAndMissingBox(t *testing.T) {
	sentinel := errors.New("owner failed")
	for _, point := range []string{"path", "id", "box", "boxes", "block", "roots"} {
		for _, late := range []bool{false, true} {
			t.Run(point+map[bool]string{false: "/error", true: "/cancel"}[late], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := pathServices()
				bad := func() error {
					if late {
						cancel()
						return nil
					}
					return sentinel
				}
				q := PathRequest{Mode: "path", Path: "/x.docj"}
				switch point {
				case "path":
					s.LoadPath = func(context.Context, string, string) (Location, error) { return Location{HPath: "partial"}, bad() }
				case "id":
					q.Mode = "id"
					s.LoadID = func(context.Context, string) (Location, error) { return Location{HPath: "partial"}, bad() }
				case "box":
					q.Mode = "full"
					s.BoxName = func(context.Context, string) (string, bool, error) { return "partial", true, bad() }
				case "boxes":
					q.Mode = "batch"
					s.BoxesByPaths = func(context.Context, []string) (map[string]*conf.Notebook, error) { return nil, bad() }
				case "block":
					q.Mode = "batch"
					s.BlockByPath = func(context.Context, string) (*indexdb.BlockTreeRecord, error) {
						return &indexdb.BlockTreeRecord{HPath: "partial"}, bad()
					}
				case "roots":
					q.Mode = "ids"
					s.Roots = func(context.Context, string, string) ([]string, error) { return []string{"partial"}, bad() }
				}
				r, err := Paths(ctx, q, s)
				want := sentinel
				if late {
					want = context.Canceled
				}
				if !errors.Is(err, want) || !reflect.DeepEqual(r, PathResult{}) {
					t.Fatal(r, err)
				}
			})
		}
	}
	s := pathServices()
	s.BoxName = func(context.Context, string) (string, bool, error) { return "", false, nil }
	s.MissingBox = func() error { return nil }
	if _, err := Paths(context.Background(), PathRequest{Mode: "full"}, s); !errors.Is(err, ErrPathBoxMissing) {
		t.Fatal(err)
	}
}
func TestDocumentPathsInputCopiesAndDependencyGuards(t *testing.T) {
	s := pathServices()
	input := []string{"original"}
	s.BoxesByPaths = func(_ context.Context, p []string) (map[string]*conf.Notebook, error) {
		p[0] = "changed"
		return nil, nil
	}
	if _, err := Paths(context.Background(), PathRequest{Mode: "batch", Paths: input}, s); err != nil || input[0] != "original" {
		t.Fatal(input, err)
	}
	for _, mode := range []string{"path", "id", "location", "full", "batch", "ids", "bad-mode"} {
		if _, err := Paths(context.Background(), PathRequest{Mode: mode, Path: "/x.docj"}, PathServices{}); err == nil {
			t.Fatal("unbound mode accepted", mode)
		}
	}
	if _, err := Paths(nil, PathRequest{Mode: "path", Path: "/"}, s); err == nil {
		t.Fatal("nil owner accepted")
	}
}
