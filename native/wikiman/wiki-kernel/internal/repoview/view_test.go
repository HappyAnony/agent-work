package repoview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestRepositoryViewOriginalPreviewDiffAndDetachedLogProjection(t *testing.T) {
	ctx := context.Background()
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{Markdown: nativeparser.Defaults()})
	raw := []byte(`{"ID":"20261104210000-repo001","Type":"NodeDocument","Properties":{"id":"20261104210000-repo001","title":"Snapshot title"},"Children":[{"ID":"20261104210001-repo002","Type":"NodeParagraph","Properties":{"id":"20261104210001-repo002","fold":"1"},"Children":[{"Type":"NodeText","Data":"snapshot body"}]}]}`)
	file := File{ID: "file", Path: "/box/doc.docj", Data: raw, Size: int64(len(raw)), Updated: 123}
	log := &Log{ID: "log", Files: []*File{{Path: "a.docj"}, {Path: "b.docj"}, {Path: "a.png"}}, Memo: "kept"}
	ports := Ports{Parser: parser, File: func(context.Context, string) (File, error) { return file, nil }, Logs: func(context.Context, int) ([]*Log, int, int, error) { return []*Log{log}, 1, 1, nil }, Diff: func(context.Context, string, string) (DiffInput, error) {
		return DiffInput{Left: DiffIndex{ID: "left"}, Right: DiffIndex{ID: "right"}, RemovesRight: []*File{&file}}, nil
	}}
	out, e := Run(ctx, Request{Kind: "preview", ID: "file"}, ports)
	if e != nil || out.Preview.Title != "Snapshot title" || out.Preview.Text || !strings.Contains(out.Preview.Content, "snapshot body") || strings.Contains(out.Preview.Content, `contenteditable="true"`) {
		t.Fatal(out, e)
	}
	out, e = Run(ctx, Request{Kind: "diff", Left: "left", Right: "right"}, ports)
	if e != nil || len(out.Diff.AddsLeft) != 1 || out.Diff.AddsLeft[0].Title != "Snapshot title" || len(out.Diff.RemovesRight) != 0 {
		t.Fatal(out, e)
	}
	out, e = Run(ctx, Request{Kind: "logs", Page: 1}, ports)
	if e != nil || len(out.Snapshots) != 1 || out.Snapshots[0].TypesCount[0].Count != 2 || out.Snapshots[0].Files != nil || len(log.Files) != 3 {
		t.Fatal(out, e)
	}
	out.Snapshots[0].Memo = "mutated"
	if log.Memo != "kept" {
		t.Fatal("log pointer escaped")
	}
	ports.File = func(context.Context, string) (File, error) {
		return File{ID: "file", Path: "/box/doc.docj", Data: []byte("corrupt")}, nil
	}
	if out, e = Run(ctx, Request{Kind: "diff"}, ports); e == nil || out.Diff != nil {
		t.Fatal("failed file returned partial diff", out, e)
	}
}

func TestRepositoryViewTextAssetAndFailedCacheDoNotReturnFalsePath(t *testing.T) {
	sentinel := errors.New("cache write failed")
	file := File{Path: "box/doc/assets/image.png", Data: []byte("image")}
	ports := Ports{File: func(context.Context, string) (File, error) { return file, nil }, MIME: func(string) string { return "image/png" }, Displayable: func(string) bool { return true }, StoreAsset: func(context.Context, string, []byte) (string, error) { return "partial", sentinel }}
	out, e := Run(context.Background(), Request{Kind: "preview"}, ports)
	if !errors.Is(e, sentinel) || out.Preview.Content != "" {
		t.Fatal(out, e)
	}
	ports.StoreAsset = func(_ context.Context, name string, raw []byte) (string, error) {
		if name != "assets/image.png" || string(raw) != "image" {
			t.Fatal(name, string(raw))
		}
		return "repo/diff/" + name, nil
	}
	out, e = Run(context.Background(), Request{Kind: "preview"}, ports)
	if e != nil || out.Preview.Content != "repo/diff/assets/image.png" {
		t.Fatal(out, e)
	}
	file.Path = "assets/../outside.png"
	if out, e = Run(context.Background(), Request{Kind: "preview"}, ports); e == nil || out.Preview.Content != "" {
		t.Fatal(out, e)
	}
	file.Path = "settings.json"
	file.Data = []byte(`{"value":1}`)
	ports.MIME = func(string) string { return "application/json" }
	out, e = Run(context.Background(), Request{Kind: "preview"}, ports)
	if e != nil || out.Preview.Content != string(file.Data) || !out.Preview.Text {
		t.Fatal(out, e)
	}
}
