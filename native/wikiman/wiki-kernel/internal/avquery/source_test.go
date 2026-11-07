package avquery

import (
	"context"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexrepair"
)

func TestSourceCatalogCheckedManifestAndMissingDirectory(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	root, e := os.OpenRoot(folder)
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	source := indexrepair.Source{Data: root}
	catalog, e := ReadSourceCatalog(ctx, source)
	if e != nil || catalog == nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Join(folder, "storage/av"), 0700); e != nil {
		t.Fatal(e)
	}
	if e = catalog.Confirm(ctx, source); !errors.Is(e, indexrepair.ErrSourceChanged) {
		t.Fatal("late missing root not checked", e)
	}
	id := "20261105100000-views01"
	relations, _ := msgpack.Marshal(map[string][]string{id: {"20261105100001-block01"}})
	root.WriteFile("storage/av/blocks.msgpack", relations, 0600)
	root.WriteFile("storage/av/"+id+".json", []byte(`{"id":"`+id+`","name":"Scoped"}`), 0600)
	catalog, e = ReadSourceCatalog(ctx, source)
	if e != nil || len(catalog.Entries) != 1 || catalog.Entries[0].Name != "Scoped" {
		t.Fatal(catalog, e)
	}
	root.WriteFile("storage/av/added.txt", []byte("late"), 0600)
	if e = catalog.Confirm(ctx, source); !errors.Is(e, indexrepair.ErrSourceChanged) {
		t.Fatal("late entry ignored", e)
	}
	root.Remove("storage/av/added.txt")
	root.WriteFile("storage/av/"+id+".json", []byte("broken"), 0600)
	if out, e := ReadSourceCatalog(ctx, source); e == nil || out != nil {
		t.Fatal(out, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = ReadSourceCatalog(canceled, source); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
