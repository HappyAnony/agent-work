//go:build fts5

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func originalProjectionConfiguration(string, string) (conf.AppData, error) {
	return conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport(), Search: conf.NewSearch()}, nil
}

func TestOriginalNativeProjectionQueuesReadActualIsolatedSource(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	nested := filepath.Join(root, "pages", pageB, "data", boxID, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "pages", pageB, "data", boxID, docID+".docj"), filepath.Join(nested, docID+".docj")); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	for lease, owner := range map[*Lease]string{a: "A", b: "B"} {
		path := "/" + docID + ".docj"
		if owner == "B" {
			path = "/nested" + path
		}
		if err = lease.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, _ := bundle.Database(indexdb.Main)
			for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
				if _, err := db.Exec("INSERT INTO "+table+"(id,parent_id,root_id,box,path,content) VALUES(?,?,?,?,?,'old')", blockID, owner+"parent", docID, boxID, path); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err = lease.QueueNativeIndexNode(ctx, boxID, docID, blockID); err != nil {
			t.Fatal(err)
		}
		if err = lease.QueueNativeReferences(ctx, boxID, docID); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan IndexEvent, 4)
	sub, err := a.SubscribeIndexEvents(ctx, func(e IndexEvent) {
		if e.Kind == "main-index-flushed" {
			events <- e
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	task, err := a.StartNativeMainFlush(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"index_node", "update_refs"} {
		select {
		case e := <-events:
			if e.Action != action || e.PageID != pageA {
				t.Fatal(e)
			}
		case <-time.After(time.Second):
			t.Fatal("no actual projection flush", action)
		}
	}
	task.Cancel()
	if err = task.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = b.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.Main)
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM refs").Scan(&count)
		if count != 0 {
			t.Error("A flush drained B")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = b.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	for lease, owner := range map[*Lease]string{a: "A", b: "B"} {
		if err = lease.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, _ := bundle.Database(indexdb.Main)
			for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
				var content string
				if err := db.QueryRow("SELECT content FROM "+table+" WHERE id=?", blockID).Scan(&content); err != nil {
					return err
				}
				expected := "Synthetic knowledge"
				if owner == "B" {
					expected = "Workspace B knowledge"
				}
				if !strings.Contains(content, expected) {
					t.Errorf("wrong content owner %s: %s", owner, content)
				}
			}
			var parent, path, defPath, root string
			if err := db.QueryRow("SELECT def_block_parent_id,path,def_block_path,def_block_root_id FROM refs WHERE def_block_id=?", blockID).Scan(&parent, &path, &defPath, &root); err != nil {
				return err
			}
			expected := "/" + docID + ".docj"
			if owner == "B" {
				expected = "/nested" + expected
			}
			// Registry source metadata, not the deliberately different FTS cache parent.
			if parent != docID || path != expected || defPath != expected || root != docID {
				t.Errorf("crossed reference owner %s: %s %s %s %s", owner, parent, path, defPath, root)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if digest(t, root) != before {
		t.Fatal("derived projection wrote source")
	}
	if err = a.QueueNativeIndexNode(ctx, boxID, docID, "20261019099999-missing"); !errors.Is(err, ErrDocument) {
		t.Fatal(err)
	}
}

func TestNativeContentDependenciesFailBeforeQueueAdmission(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// A real source image, not a fake projector callback; source remains untouched
	// by both failed and successful derived operations below.
	replaced := strings.Replace(string(raw), `{"Type": "NodeText", "Data": "Synthetic knowledge: offline Wiki and Agent fixture."}`, `{"Type":"NodeImage","Children":[{"Type":"NodeLinkText","Data":"caption"},{"Type":"NodeLinkDest","Data":"assets/a.png"}]}`, 1)
	if replaced == string(raw) {
		t.Fatal("fixture replacement failed")
	}
	if err = os.WriteFile(file, []byte(replaced), 0600); err != nil {
		t.Fatal(err)
	}
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	before := digest(t, root)
	if err = l.QueueNativeIndexNode(ctx, boxID, docID, blockID); !errors.Is(err, indexdb.ErrProjection) {
		t.Fatal("missing OCR was silently ignored", err)
	}
	if l.workspace.resources.contentQueue.Pending() != 0 {
		t.Fatal("failed projection entered queue")
	}
	l.Release()
	good := open(t, root, Options{Configuration: originalProjectionConfiguration, NativeContent: func(store, page string) (indexdb.ContentServices, error) {
		if store != storeID || page != pageA {
			t.Fatal(store, page)
		}
		return indexdb.ContentServices{AssetText: func(context.Context, string) (string, error) { return "owned OCR", nil }}, nil
	}})
	l, err = good.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.Main)
		for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
			if _, err := db.Exec("INSERT INTO "+table+"(id,content) VALUES(?,'old')", blockID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = l.QueueNativeIndexNode(ctx, boxID, docID, blockID); err != nil {
		t.Fatal(err)
	}
	pending := l.workspace.resources.contentQueue.Pending()
	if pending != 1 {
		t.Fatal(pending)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.Main)
		for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
			var content string
			if err := db.QueryRow("SELECT content FROM "+table+" WHERE id=?", blockID).Scan(&content); err != nil {
				return err
			}
			if !strings.Contains(content, "owned OCR") {
				t.Errorf("OCR not indexed into %s: %s", table, content)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if digest(t, root) != before {
		t.Fatal("projection modified source")
	}
}

func TestNativeProjectionProviderFailureDoesNotPublishContext(t *testing.T) {
	ctx := context.Background()
	m := open(t, fixture(t), Options{NativeContent: func(string, string) (indexdb.ContentServices, error) {
		return indexdb.ContentServices{}, errors.New("synthetic provider failure")
	}})
	if _, err := m.Acquire(ctx, storeID, pageA); err == nil {
		t.Fatal("provider failure ignored")
	}
	if m.Stats().Workspaces != 0 || m.Stats().Leases != 0 {
		t.Fatal(m.Stats())
	}
}
