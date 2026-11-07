//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"wiki-kernel/util"
)

func conversionImageFixture(t *testing.T, typ string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.RGBA{R: 180, G: 30, B: 20, A: 255})
		}
	}
	var output bytes.Buffer
	var err error
	if typ == "image/png" {
		err = png.Encode(&output, img)
	} else {
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 100})
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
func conversionNode(uri string) *ast.Node {
	parent := &ast.Node{Type: ast.NodeImage}
	parent.SetIALAttr("style", "width: 24px;")
	parent.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte("picture")})
	n := &ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(uri)}
	parent.AppendChild(n)
	return n
}
func TestImageConversionPNGJPEGSVGAndOriginalNodeAttributes(t *testing.T) {
	for _, typ := range []string{"image/png", "image/jpeg", "image/svg+xml"} {
		t.Run(typ, func(t *testing.T) {
			a, _, _, _ := markdownAssetsFixture(t, false)
			raw := []byte(`<svg width="3" height="4"><rect width="3" height="4"/></svg>`)
			if typ != "image/svg+xml" {
				raw = conversionImageFixture(t, typ)
			}
			payload := base64.StdEncoding.EncodeToString(raw)
			if len(payload) > 32 {
				payload = payload[:32] + "%0a" + payload[32:]
			}
			uri := "data:" + typ + ";base64," + payload
			n := conversionNode(uri)
			if err := a.base64Image(n, uri, "assets"); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(n.TokensStr(), "assets/picture-") || n.Parent.IALAttr("style") != "width: 24px;" {
				t.Fatal(n.TokensStr(), n.Parent.KramdownIAL)
			}
			data, err := a.target.Data.ReadFile(n.TokensStr())
			if err != nil {
				t.Fatal(err)
			}
			if typ == "image/svg+xml" {
				if !bytes.Equal(data, raw) {
					t.Fatal("SVG bytes changed")
				}
			} else {
				img, kind, err := image.Decode(bytes.NewReader(data))
				if err != nil || img.Bounds() != image.Rect(0, 0, 3, 4) || typ == "image/png" && kind != "png" || typ == "image/jpeg" && kind != "jpeg" {
					t.Fatal(kind, err)
				}
			}
			if _, err := os.Stat(filepath.Join(util.TempDir, "base64")); !os.IsNotExist(err) {
				t.Fatal("shared base64 directory used", err)
			}
		})
	}
}
func TestImageConversionFailureUnsupportedTypeAndNoPrematureNodeMutation(t *testing.T) {
	for _, kind := range []string{"bad-base64", "bad-png", "bad-jpeg", "unsupported", "nil-node", "wrong-node", "encoding-guard", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			a, _, _, _ := markdownAssetsFixture(t, false)
			uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(conversionImageFixture(t, "image/png"))
			switch kind {
			case "bad-base64":
				uri = "data:image/png;base64,%%%%"
			case "bad-png":
				uri = "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("invalid PNG"))
			case "bad-jpeg":
				uri = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("invalid JPEG"))
			case "unsupported":
				uri = "data:image/gif;base64," + base64.StdEncoding.EncodeToString([]byte("retained"))
			case "readonly":
				owner := a.authority.binding.scope.commit.r.owner
				owner.m.Lock()
				owner.persistenceReadOnly = true
				owner.m.Unlock()
				defer func() { owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
			}
			n := conversionNode(uri)
			if kind == "nil-node" {
				n = nil
			}
			if kind == "wrong-node" {
				n = &ast.Node{Type: ast.NodeText, Tokens: []byte(uri)}
			}
			sentinel := errors.New("encoding owner failure")
			if kind == "encoding-guard" {
				calls := 0
				a.extraGuard = func() error {
					calls++
					if calls == 4 {
						return sentinel
					}
					return nil
				}
			}
			err := a.base64Image(n, uri, "assets")
			if kind == "unsupported" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("conversion unexpectedly succeeded")
			}
			if kind == "encoding-guard" && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if n != nil && n.TokensStr() != uri {
				t.Fatal("failed/unsupported conversion changed node")
			}
		})
	}
}
func TestImageConversionRawSVGOriginalExtractionAndUnmatchedBlock(t *testing.T) {
	a, _, _, _ := markdownAssetsFixture(t, false)
	svg := `<svg width="2"><path d="M0 0"/></svg>`
	n := &ast.Node{Type: ast.NodeHTMLBlock, Tokens: []byte(`<pre class="svg">` + svg + `</pre>`)}
	if err := a.htmlBlockSVG(n, "assets"); err != nil {
		t.Fatal(err)
	}
	imageNode := n.ChildByType(ast.NodeImage)
	if n.Type != ast.NodeParagraph || imageNode == nil {
		t.Fatal(n.Type)
	}
	dest := imageNode.ChildByType(ast.NodeLinkDest)
	data, err := a.target.Data.ReadFile(dest.TokensStr())
	if err != nil || string(data) != svg {
		t.Fatal(string(data), err)
	}
	unchanged := &ast.Node{Type: ast.NodeHTMLBlock, Tokens: []byte("unmatched block")}
	if err := a.htmlBlockSVG(unchanged, "assets"); err != nil || unchanged.Type != ast.NodeHTMLBlock {
		t.Fatal(err)
	}
}
func TestImageConversionHTMLHandlersCancellationAndFailureResults(t *testing.T) {
	_, _, _, _ = markdownAssetsFixture(t, false)
	engine := lute.New()
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(conversionImageFixture(t, "image/png"))
	md, _, err := HTML2Markdown(`<p>Original paragraph</p><img alt="picture" src="`+uri+`">`, engine, context.Background())
	if err != nil || !strings.Contains(md, "Original paragraph") || !strings.Contains(md, "assets/picture-") {
		t.Fatal(md, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if md, math, err := HTML2Markdown("<p>canceled</p>", engine, ctx); !errors.Is(err, context.Canceled) || md != "" || math {
		t.Fatal(md, math, err)
	}
	if tree, math, err := HTML2TreeWithContext(ctx, "<p>canceled</p>", engine); !errors.Is(err, context.Canceled) || tree != nil || math {
		t.Fatal(tree, math, err)
	}
	if tree, _, err := HTML2TreeWithContext(context.Background(), `<img src="data:image/png;base64,%%%%">`, engine); err == nil || tree != nil {
		t.Fatal(tree, err)
	}
	if tree, _, err := HTML2TreeWithContext(context.Background(), "plain", nil); err == nil || tree != nil {
		t.Fatal(tree, err)
	}
}
func TestImageConversionCapturedRootCloseFailureIsReturned(t *testing.T) {
	a, _, _, _ := markdownAssetsFixture(t, false)
	e := a.authority
	sentinel := errors.New("image conversion root close failed")
	e.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), sentinel) }
	if err := e.withImageConversionAssets(func(owned *markdownImportAssets) error { return owned.guard() }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
func TestImageConversionActualMalformedThenIndependentMarkdownImport(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	owner := m.archive.binding.scope.commit.r.owner
	file := filepath.Join(folder, "bad-image.md")
	if err := os.WriteFile(file, []byte("![bad](data:image/png;base64,%%%%)"), 0600); err != nil {
		t.Fatal(err)
	}
	owner.syncRuntime.same.Store(7)
	if err := m.importFromLocalPath(box, file, "/", "", true); err == nil || owner.syncRuntime.same.Load() != 7 {
		t.Fatal(err)
	}
	if m.trees != nil || m.links != nil {
		t.Fatal("failed candidates retained")
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(conversionImageFixture(t, "image/png"))
	if err := os.WriteFile(file, []byte("![good]("+uri+")"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := captureMarkdownImportPreparation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = next.importFromLocalPath(box, file, "/", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(util.TempDir, "base64")); !os.IsNotExist(err) {
		t.Fatal("shared temp remains", err)
	}
}
func TestImageConversionConcurrentOwnersPublishDistinctNames(t *testing.T) {
	a, _, _, _ := markdownAssetsFixture(t, false)
	uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg/>`))
	const count = 6
	results := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := conversionNode(uri)
			err := a.base64Image(n, uri, "assets")
			errs <- err
			results <- n.TokensStr()
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	names := map[string]bool{}
	for name := range results {
		if names[name] {
			t.Fatal("shared output name", name)
		}
		names[name] = true
		data, err := a.target.Data.ReadFile(name)
		if err != nil || string(data) != "<svg/>" {
			t.Fatal(err)
		}
	}
}

func TestImageConversionTextMarkLinkKeepsTextAndUsesConvertedHref(t *testing.T) {
	a, _, _, _ := markdownAssetsFixture(t, false)
	uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg/>`))
	n := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "a", TextMarkAHref: uri, TextMarkTextContent: "retained text"}
	if err := a.base64Image(n, uri, "assets"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.TextMarkAHref, "assets/image-") || n.TextMarkTextContent != "retained text" {
		t.Fatal(n.TextMarkAHref, n.TextMarkTextContent)
	}
	data, err := a.target.Data.ReadFile(n.TextMarkAHref)
	if err != nil || string(data) != "<svg/>" {
		t.Fatal(err)
	}
}
