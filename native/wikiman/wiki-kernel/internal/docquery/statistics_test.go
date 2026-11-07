package docquery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/nativeparser"
)

func statFixture() (*parse.Tree, *av.AttributeView, StatServices) {
	root := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	p := &ast.Node{Type: ast.NodeParagraph, ID: "p"}
	p.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("Hello 世界 ")})
	p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "a", TextMarkTextContent: "link"})
	root.AppendChild(p)
	root.AppendChild(&ast.Node{Type: ast.NodeAttributeView, ID: "av", AttributeViewID: "database"})
	tree := &parse.Tree{Root: root, Box: "owner"}
	a := &av.AttributeView{KeyValues: []*av.KeyValues{
		{Key: &av.Key{Type: av.KeyTypeURL}, Values: []*av.Value{{Type: av.KeyTypeURL, URL: &av.ValueURL{Content: "https://example.test"}}}},
		{Key: &av.Key{Type: av.KeyTypeBlock}, Values: []*av.Value{{Type: av.KeyTypeBlock, Block: &av.ValueBlock{Content: "Bound"}}, {Type: av.KeyTypeBlock, IsDetached: true, Block: &av.ValueBlock{Content: "Detached"}}}},
		{Key: &av.Key{Type: av.KeyTypeText}, Values: []*av.Value{{Type: av.KeyTypeText, Text: &av.ValueText{Content: "Text"}}}},
		{Key: &av.Key{Type: av.KeyTypeNumber}, Values: []*av.Value{{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 12.5, IsNotEmpty: true, FormattedContent: "cached"}}}},
		{Key: &av.Key{Type: av.KeyTypeEmail}, Values: []*av.Value{{Type: av.KeyTypeEmail, Email: &av.ValueEmail{Content: "a@b.test"}}}},
		{Key: &av.Key{Type: av.KeyTypePhone}, Values: []*av.Value{{Type: av.KeyTypePhone, Phone: &av.ValuePhone{Content: "123"}}}},
		{Key: &av.Key{Type: av.KeyTypeMAsset}, Values: []*av.Value{{Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Type: av.AssetTypeImage}, {Type: "file"}}}}},
	}}
	s := StatServices{Trees: func(context.Context, []string) (map[string]*parse.Tree, error) {
		return map[string]*parse.Tree{"root": tree, "p": tree}, nil
	}, Parser: func(context.Context) (*lute.Lute, error) {
		return nativeparser.NewEditor(nativeparser.EditorOptions{}), nil
	}, AttributeView: func(context.Context, string) (*av.AttributeView, error) { return a, nil }}
	return tree, a, s
}

func TestDocumentStatisticsOriginalModesAndReadOnlyAV(t *testing.T) {
	ctx := context.Background()
	tree, a, s := statFixture()
	before, _ := json.Marshal(a)
	number := *a.KeyValues[3].Values[0].Number
	number.FormatNumber()
	p, _ := s.Parser(ctx)
	extra, e := ContentStatistics(ctx, "https://example.testBoundDetachedText"+number.FormattedContent+"a@b.test123", p)
	if e != nil {
		t.Fatal(e)
	}
	r, w, l, i, f := tree.Root.Stat()
	want := &StatResult{RuneCount: r + extra.RuneCount, WordCount: w + extra.WordCount, LinkCount: l + 1, ImageCount: i + 1, RefCount: f + 1, BlockCount: 2}
	got, e := Statistics(ctx, StatRequest{Mode: "tree", IDs: []string{"p"}}, s)
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, want, e)
	}
	got, e = Statistics(ctx, StatRequest{Mode: "block", IDs: []string{"root"}}, s)
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, e)
	}
	got, e = Statistics(ctx, StatRequest{Mode: "block", IDs: []string{"p"}}, s)
	if e != nil || got.BlockCount != 1 || got.RefCount != 0 || got.LinkCount != 1 {
		t.Fatal(got, e)
	}
	got, e = Statistics(ctx, StatRequest{Mode: "blocks", IDs: []string{"p", "root", "p", "absent"}}, s)
	pr, pw, _, _, _ := tree.Root.FirstChild.Stat()
	if e != nil || got.BlockCount != 4 || got.RuneCount != r+2*pr || got.WordCount != w+2*pw || got.RefCount != f {
		t.Fatal(got, e)
	}
	got, e = Statistics(ctx, StatRequest{Mode: "content", Content: `<div data-type="NodeParagraph"><div contenteditable="true">你好 Hello</div></div>`}, s)
	if e != nil || got.BlockCount != 0 || got.RuneCount == 0 {
		t.Fatal(got, e)
	}
	got, e = Statistics(ctx, StatRequest{Mode: "blocks"}, s)
	if e != nil || !reflect.DeepEqual(got, &StatResult{}) {
		t.Fatal(got, e)
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) || tree.Root.FirstChild.Box != "" {
		t.Fatal("statistics changed candidates")
	}
}

func TestDocumentStatisticsErrorsCancellationAndMissingModes(t *testing.T) {
	ctx := context.Background()
	_, _, s := statFixture()
	s.AttributeView = func(context.Context, string) (*av.AttributeView, error) { return nil, errors.New("AV") }
	if r, e := Statistics(ctx, StatRequest{Mode: "tree", IDs: []string{"root"}}, s); e == nil || r != nil {
		t.Fatal(r, e)
	}
	s.IgnoreAVErrors = true
	if r, e := Statistics(ctx, StatRequest{Mode: "tree", IDs: []string{"root"}}, s); e != nil || r == nil {
		t.Fatal(r, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r, e := Statistics(canceled, StatRequest{Mode: "blocks"}, s); !errors.Is(e, context.Canceled) || r != nil {
		t.Fatal(r, e)
	}
	late, cancelLate := context.WithCancel(ctx)
	s.Trees = func(context.Context, []string) (map[string]*parse.Tree, error) { cancelLate(); return nil, nil }
	if r, e := Statistics(late, StatRequest{Mode: "blocks"}, s); !errors.Is(e, context.Canceled) || r != nil {
		t.Fatal(r, e)
	}
	if _, e := Statistics(ctx, StatRequest{Mode: "unknown"}, s); e == nil {
		t.Fatal("unknown mode accepted")
	}
	if _, e := Statistics(ctx, StatRequest{Mode: "content"}, StatServices{}); !errors.Is(e, ErrStatDependency) {
		t.Fatal(e)
	}
}
