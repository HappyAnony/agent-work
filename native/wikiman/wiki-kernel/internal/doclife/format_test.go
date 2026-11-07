package doclife

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"strings"
	"testing"
	"time"
	"wiki-kernel/internal/nativeparser"
)

func TestFormatOriginalTypographyPreservesIdentityAndParser(t *testing.T) {
	p := nativeparser.NewLute(nativeparser.Defaults())
	p.SetAutoSpace(false)
	tree := parse.Parse("", []byte("中文English\n\n{: id=\"20261104120001-format1\"}\n"), p.ParseOptions)
	tree.ID = "20261104120000-format0"
	tree.Root.ID = tree.ID
	tree.Root.SetIALAttr("id", tree.ID)
	tree.Root.SetIALAttr("title", "Kept")
	tree.Root.SetIALAttr("icon", "book")
	tree.Box = "box"
	tree.Path = "/source.docj"
	got, e := Format(context.Background(), tree, p, TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now})
	if e != nil {
		t.Fatal(e)
	}
	md := string(render.NewFormatRenderer(got, p.RenderOptions).Render())
	if !strings.Contains(md, "中文 English") || got.ID != tree.ID || got.Root.ID != tree.ID || got.Root.IALAttr("title") != "Kept" || got.Root.IALAttr("icon") != "book" || got.Path != tree.Path || got.Root.FirstChild.ID != "20261104120001-format1" {
		t.Fatal(md, got)
	}
	if p.RenderOptions.AutoSpace {
		t.Fatal("format mutated borrowed parser")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Format(ctx, tree, p, TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
