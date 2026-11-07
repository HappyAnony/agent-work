package reminder

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"time"
)

func TestReminderOriginalContainerContentDateAndCandidate(t *testing.T) {
	doc := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	q := &ast.Node{Type: ast.NodeBlockquote, ID: "quote"}
	n := &ast.Node{Type: ast.NodeParagraph, ID: "leaf"}
	doc.AppendChild(q)
	q.AppendChild(n)
	n.SetIALAttr(Attribute, "20261028120000")
	tree := &parse.Tree{Root: doc}
	attrs := map[string]string{Attribute: "old-cache", "keep": "yes"}
	loc := time.FixedZone("fixture", 8*3600)
	s := Services{Location: loc, Language: func(i int) string { return "notice %s" }, Content: func(_ context.Context, node *ast.Node) (string, error) {
		if node != n {
			t.Fatal("wrong container leaf")
		}
		return strings.Repeat("中", 127) + "\u200b" + "discarded", nil
	}}
	p, e := Prepare(context.Background(), "quote", "20261029123000", tree, attrs, s)
	if e != nil {
		t.Fatal(e)
	}
	if p.TargetID != "leaf" || p.Payload.ID != "quote" || p.Payload.Content != strings.Repeat("中", 127) || p.Payload.Milliseconds != time.Date(2026, 10, 29, 12, 30, 0, 0, loc).UnixMilli() || p.NoticeMilliseconds != 5000 || p.Notice != "notice 2026-10-29 12:30" {
		t.Fatal(p)
	}
	if attrs[Attribute] != "old-cache" || n.IALAttr(Attribute) != "20261028120000" {
		t.Fatal("preparation mutated source/cache")
	}
	p, e = Prepare(context.Background(), "quote", "0", tree, attrs, s)
	if e != nil || p.Payload.Milliseconds != 0 || p.NoticeMilliseconds != 3000 || p.Notice != "notice 2026-10-28 12:00" {
		t.Fatal(p, e)
	}
	if _, ok := p.Attrs[Attribute]; ok {
		t.Fatal("cancel retained attr")
	}
	if p.Attrs["keep"] != "yes" || attrs[Attribute] != "old-cache" {
		t.Fatal("cache ownership changed")
	}
}
func TestReminderPrepareRejectsMissingDependenciesBadDatesAndCancellation(t *testing.T) {
	s := Services{Location: time.UTC, Language: func(int) string { return "%s" }, Content: func(context.Context, *ast.Node) (string, error) { return "", nil }}
	if _, e := Prepare(context.Background(), "missing", "0", nil, nil, s); !errors.Is(e, ErrMissing) {
		t.Fatal(e)
	}
	if _, e := Prepare(context.Background(), "id", "not a date", nil, nil, s); e == nil {
		t.Fatal("bad time accepted")
	}
	if _, e := Prepare(context.Background(), "id", "0", nil, nil, Services{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Prepare(ctx, "id", "0", nil, nil, s); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
