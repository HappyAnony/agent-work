//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestR03ReadProjectionActualSourceIsolationInvalidationAtomicRebuild(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{ReadDefaults: true})
	before := digest(t, root)
	leases := []*Lease{}
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		defer l.Release()
		leases = append(leases, l)
		p, e := l.PrepareNativeReads(ctx, false)
		if e != nil || !p.Rebuilt || p.Documents != 1 {
			t.Fatal(page, p, e)
		}
		same, e := l.PrepareNativeReads(ctx, false)
		if e != nil || same.Rebuilt || same.SourceHash != p.SourceHash {
			t.Fatal("unstable index", same, e)
		}
		r, e := l.SearchNativeBlocks(ctx, indexdb.BlockSearchRequest{Query: "knowledge", Page: 1, PageSize: 20})
		if e != nil || r.MatchedBlocks != 1 {
			t.Fatal(r, e)
		}
		want := "Synthetic"
		if page == pageB {
			want = "Workspace B"
		}
		if !strings.Contains(r.Blocks[0].Content, want) {
			t.Fatal("wrong owner", r)
		}
	}
	if digest(t, root) != before {
		t.Fatal("read created source files")
	}
	a, b := leases[0], leases[1]
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	changed := []byte(strings.ReplaceAll(string(raw), "Synthetic knowledge", "ChangedIndexTerm"))
	if e = os.WriteFile(file, changed, 0600); e != nil {
		t.Fatal(e)
	}
	if p, e := a.PrepareNativeReads(ctx, false); e != nil || !p.Rebuilt {
		t.Fatal(p, e)
	}
	for _, l := range []*Lease{a, b} {
		r, e := l.SearchNativeBlocks(ctx, indexdb.BlockSearchRequest{Query: "ChangedIndexTerm", Page: 1, PageSize: 20})
		if e != nil || (r.MatchedBlocks == 1) != (l == a) {
			t.Fatal("stale/cross-page", r, e)
		}
	}
	if e = os.WriteFile(file, []byte(`{"Type":`), 0600); e != nil {
		t.Fatal(e)
	}
	if p, e := a.PrepareNativeReads(ctx, false); e == nil || p.SourceHash != "" {
		t.Fatal("corrupt source published", p, e)
	}
	if e = os.WriteFile(file, changed, 0600); e != nil {
		t.Fatal(e)
	}
	p, e := a.PrepareNativeReads(ctx, true)
	if e != nil || !p.Rebuilt {
		t.Fatal("recovery", p, e)
	}
	extraDoc := "20261105091000-extra01"
	extraBlock := "20261105091001-extra02"
	extraFile := filepath.Join(filepath.Dir(file), extraDoc+".docj")
	extra := []byte(`{"ID":"` + extraDoc + `","Type":"NodeDocument","Properties":{"id":"` + extraDoc + `","title":"extra"},"Children":[{"ID":"` + extraBlock + `","Type":"NodeParagraph","Properties":{"id":"` + extraBlock + `"},"Children":[{"Type":"NodeText","Data":"extra"}]}]}`)
	if e = os.WriteFile(extraFile, extra, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = a.PrepareNativeReads(ctx, false); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(extraFile); e != nil {
		t.Fatal(e)
	}
	if _, e = a.PrepareNativeReads(ctx, false); e != nil {
		t.Fatal(e)
	}
	row, found, e := a.NativeBlock(ctx, extraDoc)
	if e != nil || found {
		t.Fatal("deleted row survived", row, e)
	}

	// A replacement context reconstructs disposable indexes, never source.
	a.Release()
	b.Release()
	if e = m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	m = open(t, root, Options{ReadDefaults: true})
	a, e = m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Release()
	if p, e = a.PrepareNativeReads(ctx, false); e != nil || !p.Rebuilt {
		t.Fatal("lost index not rebuilt", p, e)
	}
}

func TestR03ReadProjectionCanceledUnknownFieldsAndConcurrentOwners(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{ReadDefaults: true})
	ctx := context.Background()
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]json.RawMessage
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	d["Future"] = json.RawMessage(`{"big":900719925474099312345}`)
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if p, e := l.PrepareNativeReads(canceled, false); !errors.Is(e, context.Canceled) || p.SourceHash != "" {
		t.Fatal(p, e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := l.PrepareNativeReads(ctx, false); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if digest(t, root) != before {
		t.Fatal("read normalized unknown source")
	}
	attrs, e := l.ReadNativeBlockAttributes(ctx, docID)
	if e != nil || attrs["title"] == "" {
		t.Fatal(attrs, e)
	}
	if e = os.Symlink(file, filepath.Join(root, "pages", pageA, "data", "evil")); e != nil {
		t.Fatal(e)
	}
	if _, e = l.PrepareNativeReads(ctx, false); e == nil {
		t.Fatal("source symlink accepted")
	}
}

func TestR03MissingDerivedTablesRebuildAndPreserveOtherQueues(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{ReadDefaults: true})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	p, e := l.PrepareNativeReads(ctx, false)
	if e != nil {
		t.Fatal(e)
	}
	history := indexdb.HistoryEntry{ID: "history", Type: 0, Title: "kept"}
	if e = l.QueueNativeHistories(ctx, []indexdb.HistoryEntry{history}); e != nil {
		t.Fatal(e)
	}
	asset := indexdb.AssetContentEntry{ID: "asset", Path: "assets/a.txt", Content: "kept"}
	if e = l.QueueNativeAssetContents(ctx, []indexdb.AssetContentEntry{asset}); e != nil {
		t.Fatal(e)
	}
	if e = l.FlushNativeAssetContents(ctx); e != nil {
		t.Fatal(e)
	}
	if e = l.FlushNativeHistories(ctx); e != nil {
		t.Fatal(e)
	}
	if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		_, e = db.ExecContext(ctx, "DROP TABLE blocks_fts_case_insensitive")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	rebuilt, e := l.PrepareNativeReads(ctx, false)
	if e != nil || !rebuilt.Rebuilt || rebuilt.SourceHash != p.SourceHash {
		t.Fatal(rebuilt, e)
	}
	if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		for _, kind := range []indexdb.Kind{indexdb.History, indexdb.Assets} {
			db, e := b.Database(kind)
			if e != nil {
				return e
			}
			name := "histories_fts_case_insensitive"
			if kind == indexdb.Assets {
				name = "asset_contents_fts_case_insensitive"
			}
			var n int
			if e = db.QueryRowContext(ctx, "SELECT count(*) FROM "+name).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				t.Fatalf("rebuild dropped %v rows: %d", kind, n)
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}

func TestR03ProjectionRefusesSourceChangedDuringActualProjection(t *testing.T) {
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]any
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	first := d["Children"].([]any)[0].(map[string]any)
	first["Children"] = []any{map[string]any{"Type": "NodeImage", "Children": []any{map[string]string{"Type": "NodeLinkText", "Data": "image"}, map[string]string{"Type": "NodeLinkDest", "Data": "assets/owned.png"}}}}
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	asset := filepath.Join(root, "pages", pageA, "data", "assets", "owned.png")
	if e = os.MkdirAll(filepath.Dir(asset), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(asset, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	m := open(t, root, Options{ReadDefaults: true, NativeContent: func(string, string) (indexdb.ContentServices, error) {
		return indexdb.ContentServices{AssetText: func(ctx context.Context, _ string) (string, error) {
			once.Do(func() { close(entered) })
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-release:
				return "", nil
			}
		}}, nil
	}})
	l, e := m.Acquire(context.Background(), storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	done := make(chan error, 1)
	go func() {
		p, e := l.PrepareNativeReads(context.Background(), false)
		if p.SourceHash != "" {
			t.Error("failed projection returned successful source hash")
		}
		done <- e
	}()
	<-entered
	if e = os.WriteFile(asset, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-done; !errors.Is(e, errWorkspaceDirectorySourceChanged) {
		t.Fatal("concurrent source change not rejected", e)
	}
	if l.workspace.resources.readProjectionHash != "" {
		t.Fatal("failed candidate marked ready")
	}
	if _, e = l.PrepareNativeReads(context.Background(), false); e != nil {
		t.Fatal("retry failed", e)
	}
}
