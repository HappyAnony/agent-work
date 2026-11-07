package indexdb

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestOriginalReferenceProjectionDedupAnnotationEmbedAndOwners(t *testing.T) {
	ctx := context.Background()
	engine := nativeparser.NewIndex(nativeparser.Defaults())
	const target = "20261019090002-par0001"
	for _, owner := range []string{"A", "B"} {
		root := &ast.Node{Type: ast.NodeDocument, ID: "20261019090001-doc0001"}
		para := &ast.Node{Type: ast.NodeParagraph, ID: "20261019090003-par0002"}
		root.AppendChild(para)
		for range 2 {
			para.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref tag", TextMarkBlockRefID: target, TextMarkTextContent: "anchor", TextMarkBlockRefSubtype: "s"})
		}
		para.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "file-annotation-ref", TextMarkFileAnnotationRefID: "assets/folder/a.pdf/annotation", TextMarkTextContent: "highlight"})
		para.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "file-annotation-ref", TextMarkFileAnnotationRefID: "assets/b.pdf/empty"})
		para.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "file-annotation-ref", TextMarkFileAnnotationRefID: "invalid-no-slash"})
		for i, script := range []string{"SELECT * FROM blocks WHERE id = '" + target + "'", "SELECT * FROM blocks WHERE id != '" + target + "'", "SELECT * FROM blocks WHERE id = '" + target + "' OR id = 'other'", "SELECT * FROM blocks WHERE root_id = '" + target + "'"} {
			n := &ast.Node{Type: ast.NodeBlockQueryEmbed, ID: []string{"20261019090004-emb0001", "20261019090005-emb0002", "20261019090006-emb0003", "20261019090007-emb0004"}[i]}
			n.AppendChild(&ast.Node{Type: ast.NodeBlockQueryEmbedScript, Tokens: []byte(script)})
			root.AppendChild(n)
		}
		tree := &parse.Tree{ID: root.ID, Root: root, Box: "box", Path: "/nested/doc.docj"}
		refs, annotations, err := ProjectReferences(ctx, tree, engine, func(_ context.Context, id string) (BlockLocation, bool, error) {
			if id != target {
				t.Fatal(id)
			}
			return BlockLocation{ParentID: owner + "parent", RootID: owner + "root", Path: "/" + owner + ".docj"}, true, nil
		})
		if err != nil || len(refs) != 3 || len(annotations) != 2 {
			t.Fatal(refs, annotations, err)
		}
		if refs[0].Content != "anchor" || !strings.Contains(refs[0].Markdown, target) || refs[1].Content != "" || refs[1].Markdown != "" || para.FirstChild.TextMarkType != "block-ref tag" {
			t.Fatal(refs, para.FirstChild.TextMarkType)
		}
		for _, r := range refs {
			if r.DefBlockRootID != owner+"root" || r.DefBlockParentID != owner+"parent" || r.DefBlockPath != "/"+owner+".docj" || r.Path != tree.Path {
				t.Fatal(r)
			}
		}
		if annotations[0].FilePath != "assets/folder/a.pdf" || annotations[0].AnnotationID != "annotation" || annotations[0].Content != "highlight" || annotations[1].Content != "assets/b.pdf" {
			t.Fatal(annotations)
		}
		_, _, err = ProjectReferences(ctx, tree, engine, func(context.Context, string) (BlockLocation, bool, error) {
			return BlockLocation{}, false, errors.New("synthetic lookup failure")
		})
		if err == nil || para.FirstChild.TextMarkType != "block-ref tag" {
			t.Fatal(err, para.FirstChild.TextMarkType)
		}
	}
}

func TestOriginalContentProjectionWhitespaceOCRAVAndErrors(t *testing.T) {
	ctx := context.Background()
	for _, owner := range []string{"A", "B"} {
		services := ContentServices{AssetText: func(context.Context, string) (string, error) { return owner + " OCR", nil }, AttributeView: func(_ context.Context, id string, full bool) (string, error) {
			if id != "av" {
				t.Fatal(id)
			}
			if full {
				return owner + " full", nil
			}
			return owner + " name", nil
		}}
		p := &ast.Node{Type: ast.NodeParagraph}
		p.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(" leading ")})
		img := &ast.Node{Type: ast.NodeImage}
		img.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte("caption")})
		img.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/picture.png")})
		img.AppendChild(&ast.Node{Type: ast.NodeLinkTitle, Tokens: []byte("title")})
		p.AppendChild(img)
		text, err := StaticContent(ctx, p, nil, true, false, true, services)
		if err != nil || text != " leading caption "+owner+" OCR title" {
			t.Fatal(text, err)
		}
		withPath, err := StaticContent(ctx, p, nil, true, true, true, services)
		if err != nil || !strings.Contains(withPath, "assets/picture.png") {
			t.Fatal(withPath, err)
		}
		node := &ast.Node{Type: ast.NodeAttributeView, AttributeViewID: "av"}
		full, err := StaticContent(ctx, node, nil, true, true, true, services)
		if err != nil || full != owner+" full" {
			t.Fatal(full, err)
		}
		name, err := StaticContent(ctx, node, nil, true, true, false, services)
		if err != nil || name != owner+" name" {
			t.Fatal(name, err)
		}
		if _, err = StaticContent(ctx, p, nil, true, false, true, ContentServices{}); !errors.Is(err, ErrProjection) {
			t.Fatal("silently dropped OCR", err)
		}
		if _, err = StaticContent(ctx, node, nil, true, false, true, ContentServices{}); !errors.Is(err, ErrProjection) {
			t.Fatal("silently dropped AV", err)
		}
		services.AssetText = func(context.Context, string) (string, error) { return "", errors.New("synthetic OCR failure") }
		if _, err = StaticContent(ctx, p, nil, true, false, true, services); err == nil {
			t.Fatal("OCR error swallowed")
		}
	}
	p := &ast.Node{Type: ast.NodeParagraph}
	p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "tag", TextMarkTextContent: "tag"})
	p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "a", TextMarkTextContent: "link", TextMarkAHref: "https://x/?a=&amp;amp;", TextMarkATitle: "&amp;title"})
	text, err := StaticContent(ctx, p, nil, true, false, true, ContentServices{})
	if err != nil || text != "#tag#link &title https://x/?a=&" {
		t.Fatal(text, err)
	}
	text, err = StaticContent(ctx, p, []string{"NodeTextMark-tag"}, false, false, true, ContentServices{})
	if err != nil || text != "link" {
		t.Fatal(text, err)
	}
	audio := &ast.Node{Type: ast.NodeAudio, Tokens: []byte(`<audio src="assets/a.mp3">`)}
	text, err = StaticContent(ctx, audio, nil, true, true, true, ContentServices{})
	if err != nil || text != "assets/a.mp3 " {
		t.Fatal(text, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = StaticContent(canceled, p, nil, true, false, true, ContentServices{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
