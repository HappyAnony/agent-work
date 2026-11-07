package notebooks

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/configtypes"
)

const good = "20261027090000-good001"
const missing = "20261027090001-miss001"
const broken = "20261027090002-bad0001"

func fixture(t *testing.T) (string, *os.Root) {
	t.Helper()
	dir := t.TempDir()
	r, e := os.OpenRoot(dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	return dir, r
}
func put(t *testing.T, dir, name, value string) {
	t.Helper()
	file := filepath.Join(dir, name)
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file, []byte(value), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestNotebookScanIsReadOnlyAndPreservesOriginalProjection(t *testing.T) {
	dir, r := fixture(t)
	source := `{"name":"Alpha10","closed":false,"sort":2,"sortMode":5,"icon":"custom.png","extension":{"keep":true}}`
	put(t, dir, good+"/.siyuan/conf.json", source)
	put(t, dir, broken+"/.siyuan/conf.json", "bad-json")
	os.Mkdir(filepath.Join(dir, missing), 0700)
	os.Mkdir(filepath.Join(dir, "20210808180117-czj9bvb"), 0700)
	p, e := Scan(context.Background(), r, ".siyuan", configtypes.SortModeNameASC)
	if e != nil || len(p.Notebooks) != 2 || len(p.Repairs) != 2 || p.Notebooks[0].ID != good || p.Notebooks[0].Icon != "custom.png" {
		t.Fatal(p, e)
	}
	if _, e = os.Stat(filepath.Join(dir, missing, ".siyuan")); !os.IsNotExist(e) {
		t.Fatal("read repaired missing metadata")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, broken, ".siyuan/conf.json"))
	if string(raw) != "bad-json" {
		t.Fatal("read deleted corruption")
	}
	raw, _ = os.ReadFile(filepath.Join(dir, good, ".siyuan/conf.json"))
	if string(raw) != source {
		t.Fatal("read rewrote extensions")
	}
}
func TestNotebookExplicitRepairChecksPreimagesAndOriginalTwoPass(t *testing.T) {
	dir, r := fixture(t)
	os.Mkdir(filepath.Join(dir, missing), 0700)
	put(t, dir, broken+"/.siyuan/conf.json", "bad-json")
	ctx := context.Background()
	p, e := Scan(ctx, r, ".siyuan", 0)
	if e != nil {
		t.Fatal(e)
	}
	if e = Apply(ctx, r, p.Repairs, nil); !errors.Is(e, ErrPermission) {
		t.Fatal(e)
	}
	events := 0
	after := func(ctx context.Context, id string) error {
		events++
		raw, e := os.ReadFile(filepath.Join(dir, id, ".siyuan/conf.json"))
		if e != nil || !bytes.Contains(raw, []byte("Untitled")) {
			t.Fatal("unindex preceded save", e)
		}
		return nil
	}
	if e = Apply(ctx, r, p.Repairs, after); e != nil || events != 1 {
		t.Fatal(e, events)
	}
	if _, e = os.Stat(filepath.Join(dir, broken, ".siyuan/conf.json")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	p, e = Scan(ctx, r, ".siyuan", 0)
	if e != nil || len(p.Notebooks) != 2 || len(p.Repairs) != 1 || !p.Repairs[0].Missing {
		t.Fatal(p, e)
	}
	put(t, dir, broken+"/.siyuan/conf.json", `{"name":"external"}`)
	if e = Apply(ctx, r, p.Repairs, after); !errors.Is(e, ErrChanged) {
		t.Fatal("overwrote concurrent repair", e)
	}
}
func TestNotebookPathLinksAmbiguityAndFailedEffects(t *testing.T) {
	dir, r := fixture(t)
	outside := t.TempDir()
	put(t, dir, good+"/.wiki/conf.json", `{"name":"canonical"}`)
	put(t, dir, good+"/.siyuan/conf.json", `{"name":"legacy"}`)
	if _, e := Scan(context.Background(), r, "auto", 0); !errors.Is(e, ErrPath) {
		t.Fatal(e)
	}
	os.RemoveAll(filepath.Join(dir, good, ".siyuan"))
	os.RemoveAll(filepath.Join(dir, good, ".wiki"))
	if e := os.Symlink(outside, filepath.Join(dir, good, ".wiki")); e != nil {
		t.Fatal(e)
	}
	if _, e := Scan(context.Background(), r, "auto", 0); !errors.Is(e, ErrPath) {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(dir, good, ".wiki"))
	p, e := Scan(context.Background(), r, "auto", 0)
	if e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("unindex failed")
	if e = Apply(context.Background(), r, p.Repairs, func(context.Context, string) error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, good, ".wiki/conf.json")); e != nil {
		t.Fatal("post-save failure was falsely rolled back", e)
	}
}
func TestNotebookCancellationCorruptSnapshotAndNaturalOrder(t *testing.T) {
	dir, r := fixture(t)
	put(t, dir, good+"/.wiki/conf.json", `{"name":"Page10"}`)
	put(t, dir, broken+"/.wiki/conf.json", `{"name":"Page2"}`)
	p, e := Scan(context.Background(), r, "auto", configtypes.SortModeAlphanumASC)
	if e != nil || p.Notebooks[0].ID != broken {
		t.Fatal(p, e)
	}
	put(t, dir, broken+"/.wiki/conf.json", "bad")
	p, e = Scan(context.Background(), r, "auto", 0)
	if e != nil {
		t.Fatal(e)
	}
	put(t, dir, broken+"/.wiki/conf.json", `{"name":"new"}`)
	if e = Apply(context.Background(), r, p.Repairs, nil); !errors.Is(e, ErrChanged) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Scan(ctx, r, "auto", 0); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
