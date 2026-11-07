package doclife

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"reflect"
	"strings"
	"testing"
)

const rootID = "20261027230000-root001"
const childID = "20261027230001-child01"
const targetID = "20261027230002-target1"
const newID = "20261027230003-new0001"
const blockID = "20261027230004-block01"

func lifecycleFixture() (Services, map[string]Location) {
	locs := map[string]Location{rootID: {Box: "box", Path: "/" + rootID + ".docj", HPath: "/Original", ID: rootID, Document: true}, childID: {Box: "box", Path: "/" + rootID + "/" + childID + ".docj", HPath: "/Original/Child", ID: childID, Document: true}, targetID: {Box: "other", Path: "/" + targetID + ".docj", HPath: "/Target", ID: targetID, Document: true}}
	tree := func(loc Location) *parse.Tree {
		root := &ast.Node{ID: loc.ID, Type: ast.NodeDocument}
		root.SetIALAttr("title", path.Base(loc.HPath))
		root.SetIALAttr("custom-hidden", "true")
		p := &ast.Node{ID: blockID, Type: ast.NodeParagraph}
		p.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("Body")})
		root.AppendChild(p)
		return &parse.Tree{ID: loc.ID, Root: root, Box: loc.Box, Path: loc.Path, HPath: loc.HPath}
	}
	s := Services{Box: func(context.Context, string) (bool, error) { return true, nil }, Lookup: func(_ context.Context, id string) (Location, bool, error) { v, ok := locs[id]; return v, ok, nil }, Load: func(_ context.Context, box, p string) (*parse.Tree, error) {
		for _, v := range locs {
			if v.Box == box && v.Path == p {
				return tree(v), nil
			}
		}
		return nil, ErrMissing
	}, Children: func(_ context.Context, box, p string) (ret []Location, e error) {
		for _, v := range locs {
			if v.Box == box && strings.HasPrefix(v.Path, strings.TrimSuffix(p, ".docj")+"/") {
				ret = append(ret, v)
			}
		}
		return
	}, Exists: func(context.Context, string, string) (bool, error) { return false, nil }, Parser: func(context.Context) (*lute.Lute, error) { return lute.New(), nil }, Language: func(code int) string { return fmt.Sprint("language-", code) }, NewID: func() string { return newID }, Now: func() string { return "20261027235959" }, History: func(context.Context, string) (*parse.Tree, error) { return tree(locs[rootID]), nil }, ExistingIDs: func(_ context.Context, ids []string, exclude string) (map[string]bool, error) {
		return map[string]bool{}, nil
	}, Apply: func(context.Context, *Plan) error { return nil }}
	return s, locs
}

func TestDocumentLifecycleCreateAndRenameOriginalRules(t *testing.T) {
	s, _ := lifecycleFixture()
	ctx := context.Background()
	p, e := Prepare(ctx, Request{Action: "create", Box: "box", Path: "/" + newID + ".docj", Title: " A/👨‍👩‍👧 ", Markdown: "Hello **World**"}, s)
	if e != nil || p.Tree == nil || p.Tree.ID != newID || p.Tree.Root.IALAttr("title") != "A👨‍👩‍👧" || p.Tree.Root.Spec != "1" || p.Tree.HPath != "/A👨‍👩‍👧" || p.Tree.Root.FirstChild == nil || !reflect.DeepEqual(p.Effects, []string{"mkdir", "create-transaction", "flush", "sort"}) {
		t.Fatal(p, e)
	}
	p, e = Prepare(ctx, Request{Action: "create-dom", Box: "box", Path: "/" + rootID + "/" + newID + ".docj", Title: "/", DOM: ""}, s)
	if e != nil || p.Tree.HPath != "/Original/language-16" || p.Tree.Root.FirstChild.ID != newID {
		t.Fatal(p, e)
	}
	p, e = Prepare(ctx, Request{Action: "rename", Box: "box", Path: "/" + rootID + ".docj", Title: " New/Name "}, s)
	if e != nil || len(p.Changes) != 2 || p.Tree.HPath != "/NewName" || p.Changes[1].Tree.HPath != "/NewName/Child" || p.Changes[1].From.HPath != "/Original/Child" || p.Tree.Root.IALAttr("updated") != s.Now() {
		t.Fatal(p, e)
	}
	p, e = Prepare(ctx, Request{Action: "rename", Box: "box", Path: "/" + rootID + ".docj", Title: "Original"}, s)
	if e != nil || !p.Noop {
		t.Fatal(p, e)
	}
	// Rename's empty fallback uses the original unescaped language reader;
	// create continues using the escaped public language reader.
	s.PlainLanguage = func(int) string { return "Raw & Title" }
	p, e = Prepare(ctx, Request{Action: "rename", Box: "box", Path: "/" + rootID + ".docj", Title: ""}, s)
	if e != nil || p.Tree.Root.IALAttr("title") != "Raw & Title" {
		t.Fatal(p, e)
	}
	for _, action := range []string{"create", "rename"} {
		_, e = Prepare(ctx, Request{Action: action, Box: "box", Path: "/" + rootID + ".docj", Title: strings.Repeat("界", 513)}, s)
		if e == nil || e.Error() != "language-106" {
			t.Fatal(e)
		}
	}
}

func TestDocumentLifecycleMoveDeleteSubtreesAndFiltering(t *testing.T) {
	s, locs := lifecycleFixture()
	ctx := context.Background()
	paths := []string{locs[childID].Path, locs[rootID].Path, locs[rootID].Path}
	original := append([]string{}, paths...)
	p, e := Prepare(ctx, Request{Action: "move", Paths: paths, ToBox: "other", ToPath: locs[targetID].Path}, s)
	if e != nil || len(p.Changes) != 2 || p.Changes[0].Tree.Path != "/"+targetID+"/"+rootID+".docj" || p.Changes[1].Tree.Path != "/"+targetID+"/"+rootID+"/"+childID+".docj" || p.Changes[1].Tree.HPath != "/Target/Original/Child" || p.Changes[0].Tree.Root.IALAttr("custom-hidden") != "" || p.Changes[1].Tree.Root.IALAttr("custom-hidden") != "true" || !reflect.DeepEqual(paths, original) {
		t.Fatal(p, e, paths)
	}
	p, e = Prepare(ctx, Request{Action: "move", Paths: []string{locs[rootID].Path}, ToBox: "box", ToPath: "/"}, s)
	if e != nil || !p.Noop {
		t.Fatal(p, e)
	}
	p, e = Prepare(ctx, Request{Action: "move", Paths: paths, ToBox: "box", ToPath: locs[childID].Path}, s)
	if e != nil || !p.Noop {
		t.Fatal(p, e)
	}
	p, e = Prepare(ctx, Request{Action: "delete", Paths: paths}, s)
	if e != nil || len(p.Changes) != 2 || !reflect.DeepEqual(p.IDs, []string{rootID, childID}) || !strings.Contains(strings.Join(p.Effects, ","), "history-backup,history-av") {
		t.Fatal(p, e)
	}
	_, e = Prepare(ctx, Request{Action: "delete", Box: "foreign", Path: locs[rootID].Path}, s)
	if e == nil {
		t.Fatal("foreign notebook accepted")
	}
}

func TestDocumentLifecycleRestoreDestinationAndDuplicateIDs(t *testing.T) {
	s, locs := lifecycleFixture()
	ctx := context.Background()
	s.ExistingIDs = func(_ context.Context, ids []string, exclude string) (map[string]bool, error) {
		if exclude != rootID {
			t.Fatal(exclude)
		}
		return map[string]bool{blockID: true}, nil
	}
	p, e := Prepare(ctx, Request{Action: "restore", Box: "box", HistoryPath: "stamp/box/" + rootID + ".docj"}, s)
	if e != nil || p.Tree.Root.FirstChild.ID != newID || p.Tree.Path != locs[rootID].Path || p.Working == nil {
		t.Fatal(p, e)
	}
	delete(locs, rootID)
	p, e = Prepare(ctx, Request{Action: "restore", Box: "box", HistoryPath: "stamp/box/" + childID + "/" + rootID + ".docj"}, Services{})
	if !errors.Is(e, ErrDependency) || p.Tree != nil {
		t.Fatal(p, e)
	}
	// Missing working/parent falls back to notebook root, missing history is no-op.
	s, _ = lifecycleFixture()
	s.Lookup = func(context.Context, string) (Location, bool, error) { return Location{}, false, nil }
	p, e = Prepare(ctx, Request{Action: "restore", Box: "box", HistoryPath: "stamp/box/" + rootID + ".docj"}, s)
	if e != nil || p.Tree.Path != "/"+rootID+".docj" || p.Working != nil {
		t.Fatal(p, e)
	}
	s.History = func(context.Context, string) (*parse.Tree, error) { return nil, nil }
	p, e = Prepare(ctx, Request{Action: "restore", Box: "box", HistoryPath: "missing"}, s)
	if e != nil || !p.Noop {
		t.Fatal(p, e)
	}
}

func TestDocumentLifecycleDependenciesFailureCancellationAndApply(t *testing.T) {
	s, locs := lifecycleFixture()
	ctx := context.Background()
	sentinel := errors.New("failure")
	q := Request{Action: "rename", Box: "box", Path: locs[rootID].Path, Title: "Changed"}
	called := false
	s.Apply = func(context.Context, *Plan) error { called = true; return sentinel }
	if p, e := Execute(ctx, q, s); !errors.Is(e, sentinel) || p.Tree != nil || !called {
		t.Fatal(p, e)
	}
	called = false
	s.Children = func(context.Context, string, string) ([]Location, error) { return nil, sentinel }
	if p, e := Execute(ctx, q, s); !errors.Is(e, sentinel) || p.Tree != nil || called {
		t.Fatal(p, e)
	}
	s, _ = lifecycleFixture()
	c, stop := context.WithCancel(ctx)
	s.Load = func(actual context.Context, _, _ string) (*parse.Tree, error) {
		if actual != c {
			t.Fatal("outer context")
		}
		stop()
		return nil, nil
	}
	if p, e := Prepare(c, q, s); !errors.Is(e, context.Canceled) || p.Tree != nil {
		t.Fatal(p, e)
	}
	for _, c := range []context.Context{nil, c} {
		if p, e := Prepare(c, q, s); e == nil || p.Tree != nil {
			t.Fatal(p, e)
		}
	}
}
