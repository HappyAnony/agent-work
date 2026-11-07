//go:build fts5

package workspace

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeDataArchiveFixedSourcesAndFailure(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		if e := os.WriteFile(filepath.Join(root, "pages", page, "data/assets/fixture.svg"), []byte(page), 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	ctx := context.Background()
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		var output bytes.Buffer
		if e = l.WriteNativeDataArchive(ctx, "data", &output); e != nil {
			t.Fatal(e)
		}
		z, e := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, entry := range z.File {
			if entry.Name == "data/assets/fixture.svg" {
				r, e := entry.Open()
				if e != nil {
					t.Fatal(e)
				}
				raw, e := io.ReadAll(r)
				r.Close()
				if e != nil || string(raw) != page {
					t.Fatal(string(raw), e)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("source missing")
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		output.Reset()
		if e = l.WriteNativeDataArchive(canceled, "data", &output); !errors.Is(e, context.Canceled) || output.Len() != 0 {
			t.Fatal(e, output.Len())
		}
		if e = l.WriteNativeDataArchive(ctx, "../escape", &output); e == nil {
			t.Fatal("unsafe prefix")
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("source changed")
	}
}
