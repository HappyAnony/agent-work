package assetextract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/xuri/excelize/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == WorkerArgument {
		var q Request
		if e := json.NewDecoder(os.Stdin).Decode(&q); e != nil {
			os.Exit(2)
		}
		if string(q.Data) == "test-owned-cancel" {
			os.WriteFile("started", []byte(strconv.Itoa(os.Getpid())), 0600)
			time.Sleep(time.Hour)
			os.Exit(2)
		}
		raw, _ := json.Marshal(q)
		if e := Serve(bytes.NewReader(raw), os.Stdout); e != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func device(t *testing.T) (Binding, string) {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	temp := t.TempDir()
	run, e := Bind(exe, temp)
	if e != nil {
		t.Fatal(e)
	}
	return run, temp
}
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, s := range files {
		f, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte(s)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestExtractionActualOfficeUTF8AndCleanup(t *testing.T) {
	run, temp := device(t)
	ctx := context.Background()
	x := excelize.NewFile()
	x.SetCellValue("Sheet1", "A1", "First")
	x.NewSheet("Second")
	x.SetCellValue("Second", "A1", "Second")
	b, e := x.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	x.Close()
	for _, q := range []Request{
		{Kind: "text", Data: []byte(" exact\ntext ")},
		{Kind: "docx", Data: zipBytes(t, map[string]string{"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`, "word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>First text</w:t></w:r></w:p></w:body></w:document>`})},
		{Kind: "pptx", Data: zipBytes(t, map[string]string{"[Content_Types].xml":`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>`,"ppt/slides/slide1.xml": `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:p><a:r><a:t>First text</a:t></a:r></a:p></p:sld>`})},
		{Kind: "xlsx", Data: b.Bytes()},
	} {
		got, e := run(ctx, q)
		if e != nil || got.Skipped {
			t.Fatal(q.Kind, got, e)
		}
		if q.Kind == "text" && got.Content != string(q.Data) || q.Kind != "text" && !strings.Contains(got.Content, "First") {
			t.Fatal(q.Kind, got)
		}
		if q.Kind == "xlsx" && got.Content != "First Second" {
			t.Fatal("sheet order changed", got)
		}
	}
	got, e := run(ctx, Request{Kind: "text", Data: []byte{0xff}})
	if e != nil || !got.Skipped {
		t.Fatal(got, e)
	}
	if _, e = run(ctx, Request{Kind: "docx", Data: []byte("corrupt")}); e == nil {
		t.Fatal("converter error swallowed")
	}
	if entries, e := os.ReadDir(temp); e != nil || len(entries) != 0 {
		t.Fatal("temporary copies leaked", entries, e)
	}
}
func TestExtractionCancellationJoinsRealChildAndCleanup(t *testing.T) {
	run, temp := device(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := run(ctx, Request{Kind: "docx", Data: []byte("test-owned-cancel")}); done <- e }()
	deadline := time.Now().Add(10 * time.Second)
	started := ""
	for started == "" && time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(temp, "asset-extract-*", "started"))
		if len(matches) > 0 {
			started = matches[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if started == "" {
		cancel()
		<-done
		t.Fatal("child did not start")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("actual child did not join")
	}
	if entries, e := os.ReadDir(temp); e != nil || len(entries) != 0 {
		t.Fatal("cleanup not joined", entries, e)
	}
}
