package assetflow

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func rtfdZip(t *testing.T, names []string, symlink bool) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if symlink {
			h.SetMode(os.ModeSymlink | 0777)
		}
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if name[len(name)-1] != '/' {
			w.Write([]byte("rtfd bytes"))
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestUploadRTFDCompleteManifestAndPrefixFailure(t *testing.T) {
	ctx := context.Background()
	raw := rtfdZip(t, []string{"bundle.rtfd/", "bundle.rtfd/TXT.rtf", "bundle.rtfd/empty/", "z-ignored/other"}, false)
	entries, e := DecodeRTFD(ctx, raw)
	if e != nil || len(entries) != 2 || entries[0].Name != "TXT.rtf" || string(entries[0].Data) != "rtfd bytes" || !entries[1].Directory {
		t.Fatal(entries, e)
	}
	for _, bad := range [][]byte{rtfdZip(t, []string{"../outside"}, false), rtfdZip(t, []string{"bundle/link"}, true), rtfdZip(t, []string{"bundle/file", "bundle/file"}, false), rtfdZip(t, []string{"bundle/file", "bundle/file/child"}, false), []byte("bad zip")} {
		if _, e = DecodeRTFD(ctx, bad); e == nil {
			t.Fatal("unsafe zip accepted")
		}
	}
	selected := []UploadFile{{Name: "ok.txt", Size: 2, Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("ok"))), nil }}, {Name: "bundle.rtfd.zip", Size: int64(len(raw)), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }}}
	writes := 0
	ports := UploadPorts{NewID: func() string { return "20261104100000-asset01" }, ByHash: func(context.Context, string) (string, error) { return "", nil }, Match: func(context.Context, string) (string, error) { return "", nil }, Write: func(context.Context, string, []byte) error { writes++; return nil }, Directory: func(_ context.Context, name string, got []UploadEntry) error {
		if len(got) != 2 {
			t.Fatal(got)
		}
		return errors.New("directory failure")
	}, Remember: func(context.Context, string, string) error { return nil }}
	out, e := Upload(ctx, selected, "assets", false, true, ports)
	if e == nil || writes != 1 || len(out.SuccMap) != 1 || len(out.ErrFiles) != 1 {
		t.Fatal(out, writes, e)
	}
	ports.Directory = func(context.Context, string, []UploadEntry) error { return nil }
	out, e = Upload(ctx, selected[1:], "assets", false, true, ports)
	if e != nil || out.SuccMap["bundle.rtfd"] == nil {
		t.Fatal(out, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	out, e = Upload(canceled, selected, "assets", false, true, ports)
	if !errors.Is(e, context.Canceled) || len(out.SuccMap) != 0 {
		t.Fatal(out, e)
	}
}
