//go:build fts5

package workspace

import (
	"bytes"
	"context"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	stdDraw "image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/ocr"
)

// Opt-in only: tests never discover/install an OCR executable or alter device config.
func TestOwnedOCRExplicitInstalledEngineEndToEnd(t *testing.T) {
	binary := os.Getenv("WIKI_TEST_OCR_BINARY")
	if binary == "" {
		t.Skip("explicit OCR executable not supplied")
	}
	large := image.NewRGBA(image.Rect(0, 0, 1600, 240))
	stdDraw.Draw(large, large.Bounds(), image.NewUniform(color.White), image.Point{}, stdDraw.Src)
	typeface, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(typeface, &opentype.FaceOptions{Size: 48, DPI: 120, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	d := font.Drawer{Dst: large, Src: image.NewUniform(color.Black), Face: face, Dot: fixed.P(30, 140)}
	d.DrawString("WIKI KNOWLEDGE")
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, large); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "temporary fixture")
	if err := os.WriteFile(filepath.Join(root, "pages", pageA, "data", "assets", "a.png"), encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	opt := ocr.Defaults()
	opt.Candidates = []string{binary}
	opt.Languages = []string{"eng"}
	if err = l.ConfigureNativeOCR(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.RunNativeOCR(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeOCRCache(ctx); err != nil {
		t.Fatal(err)
	}
	block, found, err := l.NativeBlock(ctx, blockID)
	if err != nil || !found || !strings.Contains(block.Content, "WIKI KNOWLEDGE") {
		t.Fatal(block.Content, found, err)
	}
	if before != digest(t, root) {
		t.Fatal("real OCR changed source")
	}
	t.Logf("explicit installed engine %s, recognized fixture and persisted page-local cache", l.workspace.resources.ocr.engine.Version)
}
