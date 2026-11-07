//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/ialcache"
	"wiki-kernel/internal/indexdb"
)

func TestNativeFullTreeActualSourceIsolationCachesAndProgress(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(nil, map[string]map[int]string{"en_US": {90: page + " blocks %d/%d %d %s"}}, nil), nil
	}})
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
	messages := make(chan string, 10)
	sub, err := a.SubscribeOriginalSQLProgress(ctx, conf.SQLProgressPolicy{InsertBlocks: true}, func(e IndexEvent, s string) {
		if e.PageID != pageA || e.Kind != "main-index-blocks" || (e.Action == "index" && e.BlockCount != 4) || (e.Action == "upsert" && e.BlockCount != 1) {
			t.Error(e)
		}
		messages <- s
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	for _, l := range []*Lease{a, b} {
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			t.Fatal(err)
		}
	}
	task, err := a.StartNativeMainFlush(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-messages:
		if !strings.HasPrefix(msg, pageA+" blocks 1/1 4 ") {
			t.Fatal(msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no actual tree block progress")
	}
	task.Cancel()
	if err = task.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if b.workspace.resources.contentQueue.Pending() != 1 {
		t.Fatal("A drained B")
	}
	if _, found, err := b.NativeBlock(ctx, blockID); err != nil || found {
		t.Fatal(found, err)
	}
	if err = b.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	av, ok, err := a.NativeBlock(ctx, blockID)
	if err != nil || !ok || !strings.Contains(av.Content, "Synthetic knowledge") {
		t.Fatal(av, ok, err)
	}
	bv, ok, err := b.NativeBlock(ctx, blockID)
	if err != nil || !ok || !strings.Contains(bv.Content, "Workspace B knowledge") {
		t.Fatal(bv, ok, err)
	}
	if av.HPath != "/单文档示例" {
		t.Fatal(av.HPath)
	}
	// A's original reference to a newly indexed sibling resolves without a global tree.
	if err = a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.Main)
		var def string
		e := db.QueryRow("SELECT def_block_root_id FROM refs").Scan(&def)
		if e != nil || def != docID {
			t.Fatal(def, e)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	for _, l := range []*Lease{a, b} {
		if err = l.WithNativeIAL(ctx, func(c *ialcache.Cache) error {
			c.PutDoc(docID, map[string]string{"old": "doc"})
			c.PutBlock(blockID, map[string]string{"old": "block"})
			c.Wait()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A real source edit is made by this synthetic fixture, not by indexing.
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte(strings.ReplaceAll(string(raw), "Synthetic knowledge", "updated knowledge")), 0600); err != nil {
		t.Fatal(err)
	}
	edited := digest(t, root)
	if before == edited {
		t.Fatal("fixture edit missing")
	}
	if err = a.QueueNativeTree(ctx, boxID, docID, "upsert"); err != nil {
		t.Fatal(err)
	}
	// Index/OCR work is not published to caches before SQL commit.
	cached, _, _ := a.NativeBlock(ctx, blockID)
	if cached != av {
		t.Fatal("cache changed at admission")
	}
	if err = a.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := a.NativeBlock(ctx, blockID)
	if err != nil || !strings.Contains(got.Content, "updated knowledge") {
		t.Fatal(got, err)
	}
	other, _, err := b.NativeBlock(ctx, blockID)
	if err != nil || other != bv {
		t.Fatal("B cache changed", other, err)
	}
	if err = a.WithNativeIAL(ctx, func(c *ialcache.Cache) error {
		if c.GetDoc(docID) != nil || c.GetBlock(blockID) != nil {
			t.Fatal("A IAL stale")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = b.WithNativeIAL(ctx, func(c *ialcache.Cache) error {
		if c.GetDoc(docID)["old"] != "doc" || c.GetBlock(blockID)["old"] != "block" {
			t.Fatal("B IAL invalidated")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if digest(t, root) != edited {
		t.Fatal("derived tree indexing changed source")
	}
}

func treeImageFixture(t *testing.T, root, page string) {
	t.Helper()
	file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := "Synthetic knowledge"
	if page == pageB {
		text = "Workspace B knowledge"
	}
	replaced := strings.Replace(string(raw), `{"Type": "NodeText", "Data": "`+text+`: offline Wiki and Agent fixture."}`, `{"Type":"NodeImage","Children":[{"Type":"NodeLinkText","Data":"caption"},{"Type":"NodeLinkDest","Data":"assets/a.png"}]}`, 1)
	if replaced == string(raw) {
		t.Fatal("image fixture was not changed", page)
	}
	raw = []byte(replaced)
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestNativeFullTreeSameAssetAndOCRNamesStayInTheirOwner(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		treeImageFixture(t, root, page)
		dir := filepath.Join(root, "pages", page, "data", "assets")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte(page+" global"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, "pages", pageA, "data", boxID, "assets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte(pageA+" box"), 0600); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, NativeContent: func(_, page string) (indexdb.ContentServices, error) {
		return indexdb.ContentServices{AssetText: func(context.Context, string) (string, error) { return page + " recognized", nil }}, nil
	}, NativeOCR: func(string, string) (func(context.Context, *ast.Node) error, error) {
		return func(ctx context.Context, _ *ast.Node) error { return ctx.Err() }, nil
	}})
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			l.Release()
			t.Fatal(err)
		}
		if err = l.FlushNativeMainQueue(ctx); err != nil {
			l.Release()
			t.Fatal(err)
		}
		content := page + " global"
		if page == pageA {
			content = page + " box"
		}
		want, err := indexdb.AssetEtag(ctx, strings.NewReader(content), int64(len(content)))
		if err != nil {
			t.Fatal(err)
		}
		err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
			db, _ := b.Database(indexdb.Main)
			var hash string
			e := db.QueryRow("SELECT hash FROM assets WHERE path='assets/a.png'").Scan(&hash)
			if e != nil || hash != want {
				t.Fatal(page, hash, want, e)
			}
			return e
		})
		if err != nil {
			l.Release()
			t.Fatal(err)
		}
		row, found, err := l.NativeBlock(ctx, blockID)
		if err != nil || !found || !strings.Contains(row.Content, page+" recognized") {
			l.Release()
			t.Fatal(page, row.Content, err)
		}
		l.Release()
	}
	if before != digest(t, root) {
		t.Fatal("asset/OCR index changed source")
	}
}

func TestNativeFullTreeOCRProviderFailureDoesNotPublishContext(t *testing.T) {
	sentinel := errors.New("synthetic OCR provider failure")
	m := open(t, fixture(t), Options{NativeOCR: func(string, string) (func(context.Context, *ast.Node) error, error) { return nil, sentinel }})
	if lease, err := m.Acquire(context.Background(), storeID, pageA); lease != nil || !errors.Is(err, sentinel) {
		t.Fatal(lease, err)
	}
	if len(m.workspaces) != 0 {
		t.Fatal("failed provider published Context")
	}
}
func TestNativeFullTreeOCRAssetHashFailureRollbackAndRetry(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	treeImageFixture(t, root, pageA)
	dir := filepath.Join(root, "pages", pageA, "data", boxID, "assets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("owner A asset"), 0600); err != nil {
		t.Fatal(err)
	}
	prepared := 0
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, NativeContent: func(_, page string) (indexdb.ContentServices, error) {
		return indexdb.ContentServices{AssetText: func(context.Context, string) (string, error) {
			if prepared == 0 {
				return "", errors.New("OCR not prepared")
			}
			return page + " OCR", nil
		}}, nil
	}, NativeOCR: func(_, page string) (func(context.Context, *ast.Node) error, error) {
		return func(ctx context.Context, n *ast.Node) error {
			if page != pageA {
				t.Fatal(page)
			}
			prepared++
			return ctx.Err()
		}, nil
	}})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	before := digest(t, root)
	if err = a.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if prepared == 0 {
		t.Fatal("OCR preparation not called")
	}
	// Fail after block/FTS/ref/span inserts. No successful batch event or cache survives.
	if err = a.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		_, e := db.Exec("CREATE TRIGGER fail_asset BEFORE INSERT ON assets BEGIN SELECT RAISE(ABORT,'injected'); END")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	events := 0
	sub, err := a.SubscribeIndexEvents(ctx, func(e IndexEvent) {
		if e.Kind == "main-index-blocks" {
			events++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	if err = a.FlushNativeMainQueue(ctx); err == nil {
		t.Fatal("missing failure")
	}
	if events != 0 || a.workspace.resources.contentQueue.Pending() != 1 {
		t.Fatal(events, a.workspace.resources.contentQueue.Pending())
	}
	if _, found, e := a.NativeBlock(ctx, blockID); e != nil || found {
		t.Fatal("partial block cached", found, e)
	}
	if err = a.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive", "spans", "assets", "attributes", "refs", "file_annotation_refs"} {
			var count int
			if e := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); e != nil || count != 0 {
				t.Fatal(table, count, e)
			}
		}
		_, e := db.Exec("DROP TRIGGER fail_asset")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatal(events)
	}
	if err = a.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		var hash string
		if e := db.QueryRow("SELECT hash FROM assets").Scan(&hash); e != nil || hash == "" {
			t.Fatal(hash, e)
		}
		var content string
		if e := db.QueryRow("SELECT content FROM blocks WHERE id=?", blockID).Scan(&content); e != nil || !strings.Contains(content, pageA+" OCR") {
			t.Fatal(content, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if digest(t, root) != before {
		t.Fatal("index mutated source")
	}
}

func TestNativeFullTreeMissingDependenciesAndUnsafeAssetAreNotAdmitted(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	treeImageFixture(t, root, pageA)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if err = a.QueueNativeTree(ctx, boxID, docID, "index"); !errors.Is(err, indexdb.ErrProjection) {
		t.Fatal(err)
	}
	if a.workspace.resources.contentQueue.Pending() != 0 {
		t.Fatal("incomplete projection admitted")
	}
	if _, err = originalAssetHash(ctx, a.workspace, "assets/../escape", boxID, "/doc.docj"); !errors.Is(err, ErrDocument) {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err = os.MkdirAll(filepath.Join(root, "pages", pageA, "data", "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, filepath.Join(root, "pages", pageA, "data", "assets", "link")); err != nil {
		t.Fatal(err)
	}
	if _, err = originalAssetHash(ctx, a.workspace, "assets/link/a", boxID, "/doc.docj"); !errors.Is(err, ErrDocument) {
		t.Fatal(err)
	}
}

func TestNativeFullTreeEmptyOldSourceIsNotRepaired(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw := `{"ID":"` + docID + `","Spec":"0","Type":"NodeDocument","Properties":{"id":"` + docID + `","title":"empty"},"extension":{"keep":true}}`
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if err = a.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = a.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&count)
		if err != nil || count != 1 {
			t.Fatal("empty paragraph synthesized", count, err)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if before != digest(t, root) {
		t.Fatal("old source was repaired or upgraded")
	}
}

func TestNativeFullTreeOCRAdmissionRemainsPinnedUntilCallbackExits(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	treeImageFixture(t, root, pageA)
	entered, release := make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, NativeContent: func(string, string) (indexdb.ContentServices, error) {
		return indexdb.ContentServices{AssetText: func(context.Context, string) (string, error) { return "OCR", nil }}, nil
	}, NativeOCR: func(string, string) (func(context.Context, *ast.Node) error, error) {
		return func(context.Context, *ast.Node) error {
			select {
			case <-entered:
			default:
				close(entered)
			}
			<-release
			return nil
		}, nil
	}})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.QueueNativeTree(ctx, boxID, docID, "index") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		a.Release()
		t.Fatal("no OCR callback")
	}
	// Release the creating request: only enqueueNativeMain's publishing pin and
	// source admission keep the live callback's resources owned now.
	a.Release()
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	err = m.Drain(short)
	cancel()
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		a.Release()
		t.Fatal("drain released live OCR", err)
	}
	if err = <-done; err != nil {
		a.Release()
		t.Fatal(err)
	}
	a.Release()
	if err = m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}
