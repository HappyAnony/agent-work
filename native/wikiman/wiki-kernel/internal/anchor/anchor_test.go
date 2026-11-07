package anchor

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalDynamicAnchorSetTextTextFallbackAndCache(t *testing.T) {
	ctx := context.Background()
	cache := Cache{Limit: 1}
	node := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: "def", TextMarkBlockRefSubtype: "s"}
	if e := SetText(node, "  new  ", cache.Put); e != nil || node.TextMarkTextContent != "new" || node.TextMarkBlockRefSubtype != "d" {
		t.Fatal(node, e)
	}
	if v, ok := cache.Load("def"); !ok || v != "new" {
		t.Fatal(v, ok)
	}
	cache.Range(func(k, v interface{}) bool { cache.Delete(k); return true })
	if _, ok := cache.Load("def"); ok {
		t.Fatal("Range held lock/deletion failed")
	}
	if e := SetText(node, " ", cache.Put); e != nil || node.TextMarkTextContent != "def" {
		t.Fatal(e, node)
	}
	cache.Close()
	if cache.Put("def", "later") == nil {
		t.Fatal("closed cache accepted put")
	}
	old := &ast.Node{Type: ast.NodeBlockRef}
	old.AppendChild(&ast.Node{Type: ast.NodeBlockRefID, Tokens: []byte("def")})
	if e := SetText(old, "literal", nil); e != nil || old.FirstChild.Next.Type != ast.NodeBlockRefSpace || old.LastChild.Type != ast.NodeBlockRefDynamicText {
		t.Fatal(old, e)
	}
	p := &ast.Node{Type: ast.NodeParagraph}
	p.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("hello\n世界&")})
	s, e := Text(ctx, p, 7, indexdb.ContentServices{})
	if e != nil || s != "hello世界..." {
		t.Fatal(s, e)
	}
	p.SetIALAttr("name", " <named> ")
	s, e = Text(ctx, p, 1, indexdb.ContentServices{})
	if e != nil || s != "&lt;named&gt;" {
		t.Fatal(s, e)
	}
	image := &ast.Node{Type: ast.NodeImage}
	image.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte("alt")})
	blank := &ast.Node{Type: ast.NodeParagraph}
	blank.AppendChild(image)
	s, e = Text(ctx, blank, 96, indexdb.ContentServices{})
	if e != nil || s != "alt" {
		t.Fatal(s, e)
	}
	if _, e = Text(ctx, &ast.Node{Type: ast.NodeAttributeView}, 96, indexdb.ContentServices{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
}
func TestOriginalAnchorRefreshSevenRoundsStaticAndFailures(t *testing.T) {
	ctx := context.Background()
	definitions := map[string]*ast.Node{}
	trees := map[string]*parse.Tree{}
	refs := map[string][]indexdb.RefRecord{}
	for i, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		p := &ast.Node{ID: id, Type: ast.NodeParagraph}
		if i == 0 {
			p.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("seed")})
		} else {
			prev := string(rune('a' + i - 1))
			p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: prev, TextMarkBlockRefSubtype: "d", TextMarkTextContent: "old"})
			refs[prev] = []indexdb.RefRecord{{RootID: id, BlockID: id}}
		}
		root := &ast.Node{Type: ast.NodeDocument, ID: id + "root"}
		root.AppendChild(p)
		trees[id] = &parse.Tree{ID: id, Root: root}
		definitions[id] = p
	}
	var notifications []Change
	stages := 0
	cache := Cache{}
	s := Services{StrictLoad: true, Load: func(ctx context.Context, id string) (*parse.Tree, error) { return trees[id], nil }, References: func(ctx context.Context, n *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error) {
		return refs[n.ID], nil, nil
	}, Text: func(ctx context.Context, n *ast.Node) (string, error) {
		return Text(ctx, n, 96, indexdb.ContentServices{})
	}, Set: func(n *ast.Node, text string) error { return SetText(n, text, cache.Put) }, Embedded: func(*ast.Node) string { return "" }, Notify: func(ctx context.Context, c Change) error { notifications = append(notifications, c); return nil }, IndexRefs: func(context.Context, *parse.Tree) error { return nil }, UpdateAVs: func(context.Context, map[string]*ast.Node) error { return nil }, Stage: func(context.Context, *parse.Tree) error { stages++; return nil }}
	if e := Refresh(ctx, map[string]*ast.Node{"a": definitions["a"]}, map[string]*parse.Tree{}, s); e != nil || stages != 7 || len(notifications) != 7 || definitions["h"].FirstChild.TextMarkTextContent != "seed" || definitions["i"].FirstChild.TextMarkTextContent != "old" {
		t.Fatal(stages, notifications, e)
	}
	n := definitions["b"].FirstChild
	n.TextMarkBlockRefSubtype = "s"
	n.TextMarkTextContent = "static"
	changed, changes, e := Update(ctx, definitions["b"], map[string]*ast.Node{"a": definitions["a"]}, s)
	if e != nil || !changed || len(changes) != 1 || changes[0].Type != "ref-s" || n.TextMarkTextContent != "static" {
		t.Fatal(changes, e)
	}
	want := errors.New("source failure")
	s.Load = func(context.Context, string) (*parse.Tree, error) { return nil, want }
	if e = Refresh(ctx, map[string]*ast.Node{"a": definitions["a"]}, map[string]*parse.Tree{}, s); !errors.Is(e, want) {
		t.Fatal(e)
	}
	if !strings.Contains(notifications[0].Text, "seed") {
		t.Fatal(notifications)
	}
}
