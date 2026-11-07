package wikiops

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStaticExportEmbedsOnlyScopedAssetsAndDropsActiveContent(t *testing.T) {
	reads := []string{}
	body := `<p onclick="bad()" style="background:url(https://remote.invalid)">Text<img src="assets/pic%20one.png" onerror="bad()"><a href="assets/report.html">download</a><span data-href="https://example.org/doc">external</span><img src="https://remote.invalid/img"><img src="data:image/svg+xml,evil"><a href="javascript:bad()">bad link</a><script>evil()</script><iframe src="https://remote.invalid"></iframe><svg onload="bad()"></svg><audio src="assets/voice.mp3" autoplay></audio></p>`
	value, err := staticExport(context.Background(), body, `Title <script>`, func(name string) ([]byte, error) { reads = append(reads, name); return []byte(name), nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, blocked := range []string{"onclick=", "onerror=", "javascript:", "<script>", "<iframe", "<svg", "https://remote.invalid", "autoplay", "background:url"} {
		if strings.Contains(value, blocked) {
			t.Fatal("active export", blocked, value)
		}
	}
	for _, expected := range []string{"data:image/png;base64,", "data:application/octet-stream;base64,", "data:audio/mpeg;base64,", `download="report.html"`, `rel="noopener noreferrer"`, "&lt;script&gt;", "Content-Security-Policy"} {
		if !strings.Contains(value, expected) {
			t.Fatal("missing", expected, value)
		}
	}
	if !reflect.DeepEqual(reads, []string{"pic one.png", "report.html", "voice.mp3"}) {
		t.Fatal(reads)
	}
}

func TestStaticExportRejectsMissingUnsafeAndOversizedAssets(t *testing.T) {
	for _, target := range []string{"assets/../other.png", "assets/%2e%2e/other.png", "assets/%zz.png", "assets/a%2fb/../../escape.png"} {
		_, err := staticExport(context.Background(), `<img src="`+target+`">`, "test", func(string) ([]byte, error) { t.Fatal("unsafe read"); return nil, nil })
		if !errors.Is(err, ErrArgument) {
			t.Fatal(target, err)
		}
	}
	if _, err := staticExport(context.Background(), `<img src="assets/missing.png">`, "test", func(string) ([]byte, error) { return nil, fs.ErrNotExist }); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := staticExport(context.Background(), `<img src="assets/large.png">`, "test", func(string) ([]byte, error) { return make([]byte, 8<<20+1), nil }); !errors.Is(err, ErrExportLimit) {
		t.Fatal(err)
	}
	if _, err := staticExport(context.Background(), strings.Repeat(`<img src="assets/repeated.png">`, 4), "test", func(string) ([]byte, error) { return make([]byte, 4<<20), nil }); !errors.Is(err, ErrExportLimit) {
		t.Fatal("repeated embed limit", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := staticExport(ctx, "<p>cancelled</p>", "test", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if ErrorInfo(ErrExportLimit).Type != "export_size_limit" {
		t.Fatal("unstable limit error")
	}
}

func TestDocumentExportReadsNativeSourceWithoutMutationOrLegacyRuntime(t *testing.T) {
	root, m, id := setupPages(t)
	before, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testDoc)))
	if err != nil {
		t.Fatal(err)
	}
	r := ContentRequest{ProtocolVersion: 1, Action: "document.export", StoreID: id, PageID: testPage, BoxID: box, DocID: "20261019090001-doc0001"}
	result, err := Content(context.Background(), m, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.HTML, "Synthetic knowledge") || !hashPattern.MatchString(result.Hash) || result.Receipt != nil {
		t.Fatal(result)
	}
	after, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(testDoc)))
	if string(before) != string(after) {
		t.Fatal("export changed source")
	}
	r.ExpectedHash = strings.Repeat("a", 64)
	if r.Validate() == nil {
		t.Fatal("read accepted mutation fields")
	}
}
