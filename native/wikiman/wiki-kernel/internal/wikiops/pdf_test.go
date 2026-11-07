package wikiops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPDFConversionIsOptionalScopedAndUsesFixedProtocol(t *testing.T) {
	if _, err := convertPDF(context.Background(), "<p>read</p>"); !errors.Is(err, ErrConverterUnavailable) {
		t.Fatal(err)
	}
	if runtime.GOOS != "darwin" {
		t.Skip("native macOS PDF helper adapter")
	}
	root, m, id := setupPages(t)
	tools := t.TempDir()
	helper := filepath.Join(tools, "wiki-pdf ' trusted")
	script := "#!/bin/sh\n[ \"$#\" = 1 ] && [ \"$1\" = --html-stdin ] && [ \"$HOME\" = \"$TMPDIR\" ] || exit 9\nprintf '%s' '%PDF-1.3\n012345678901234567890123456789012345678901234567890123456789\n%%EOF'\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r := baseContent(id, "document.export")
	r.PageID = testPage
	r.DocID = "20261019090001-doc0001"
	r.Format = "pdf"
	file := filepath.Join(root, "pages", testPage, "data", box, r.DocID+".docj")
	before, _ := os.ReadFile(file)
	ctx := WithPDF(context.Background(), helper, root)
	result, err := Content(ctx, m, r)
	if err != nil || result.MediaType != "application/pdf" || result.HTML != "" || !bytes.HasPrefix(result.Data, []byte("%PDF-")) || result.Hash == "" {
		t.Fatal(result, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("PDF conversion wrote source")
	}
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf 'not-a-pdf'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Content(ctx, m, r); !errors.Is(err, ErrConversion) {
		t.Fatal("invalid helper output accepted", err)
	}
	archive := filepath.Join(tools, "bad.zip")
	_ = os.WriteFile(archive, []byte("archive"), 0600)
	if _, err := convertPDF(WithPDF(context.Background(), archive, root), "<p>read</p>"); !errors.Is(err, ErrConverterUnavailable) {
		t.Fatal("PDF archive executable accepted", err)
	}
	r.Format = "docx"
	if _, err := Content(ctx, m, r); !errors.Is(err, ErrConverterUnavailable) {
		t.Fatal("PDF helper was used as Pandoc", err)
	}
	_, err = DecodeContentRequest(strings.NewReader(`{"protocolVersion":1,"action":"document.export","format":"pdf","storeId":"x","pageId":"1760000000000000002","boxId":"20261019090000-box0001","docId":"20261019090001-doc0001","pdfHelper":"/tmp/evil"}`))
	if err == nil {
		t.Fatal("page request selected PDF executable")
	}
}
