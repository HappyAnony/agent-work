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
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/ocr"
)

func ownedOCRBinary(t *testing.T, slow bool) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "tesseract fixture")
	tail := `IFS= read -r content < "$3"
printf 'level\ttext\n5\t%s\n' "$content"
`
	if slow {
		tail = "printf started > '" + strings.ReplaceAll(name+".started", "'", "'\\''") + "'\nexec sleep 30\n"
	}
	script := `#!/bin/sh
case "$1" in
 --version) printf 'tesseract fixture-1\n';exit 0;;
 --list-langs) printf 'List of available languages:\neng\nchi_sim\n';exit 0;;
esac
` + tail
	if err := os.WriteFile(name, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return name
}
func ownedAssets(t *testing.T, root, page, text string) {
	t.Helper()
	treeImageFixture(t, root, page)
	dir := filepath.Join(root, "pages", page, "data", "assets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestOwnedOCRActualProcessBackgroundFTSLocalPersistenceAndIsolation(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "owner A recognized")
	ownedAssets(t, root, pageB, "owner B recognized")
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
	opt := ocr.Defaults()
	opt.Candidates = []string{ownedOCRBinary(t, false)}
	for _, l := range []*Lease{a, b} {
		if err = l.ConfigureNativeOCR(ctx, opt); err != nil {
			t.Fatal(err)
		}
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			t.Fatal(err)
		}
		if err = l.FlushNativeMainQueue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan IndexEvent, 10)
	sub, err := a.SubscribeIndexEvents(ctx, func(e IndexEvent) {
		if e.Action == "index_node" && e.Kind == "main-index-flushed" {
			events <- e
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	task, err := a.StartNativeOCRBackground(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("OCR did not refresh original node index")
	}
	// Let this accepted callback finish its local-cache flush before stopping the ticker.
	deadline := time.Now().Add(3 * time.Second)
	cacheFile := filepath.Join(root, filepath.FromSlash(ocrCachePath(pageA)))
	for time.Now().Before(deadline) {
		if _, e := os.Stat(cacheFile); e == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	task.Cancel()
	if err = task.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = a.FlushNativeOCRCache(ctx); err != nil {
		t.Fatal(err)
	}
	av, ok, err := a.NativeBlock(ctx, blockID)
	if err != nil || !ok || !strings.Contains(av.Content, "owner A recognized") {
		t.Fatal(av, ok, err)
	}
	bv, ok, err := b.NativeBlock(ctx, blockID)
	if err != nil || !ok || strings.Contains(bv.Content, "recognized") {
		t.Fatal("A job updated B", bv, ok, err)
	}
	if _, e := os.Stat(filepath.Join(root, filepath.FromSlash(ocrCachePath(pageB)))); !os.IsNotExist(e) {
		t.Fatal("A persisted B", e)
	}
	snap, err := a.NativeAssets(ctx)
	if err != nil || len(snap) == 0 {
		t.Fatal(snap, err)
	}
	delete(snap, "assets/a.png")
	snap, _ = a.NativeAssets(ctx)
	if _, ok = snap["assets/a.png"]; !ok {
		t.Fatal("asset snapshot aliases owner")
	}
	if err = b.RunNativeOCR(ctx); err != nil {
		t.Fatal(err)
	}
	if err = b.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	bv, _, err = b.NativeBlock(ctx, blockID)
	if err != nil || !strings.Contains(bv.Content, "owner B recognized") {
		t.Fatal(bv, err)
	}
	// Reopening the same page's explicit OCR service loads local output; source
	// bytes and legacy assets/ocr-texts.json are never overwritten.
	if err = a.ConfigureNativeOCR(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if a.workspace.resources.ocr.texts["assets/a.png"].Text != " owner A recognized" {
		t.Fatal("local OCR cache not loaded")
	}
	if digest(t, root) != before {
		t.Fatal("background OCR changed authoritative source")
	}
}

func TestOwnedOCRCancellationJoinsActualChildAndCorruptCacheIsPreserved(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "image")
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	opt := ocr.Defaults()
	opt.Candidates = []string{ownedOCRBinary(t, true)}
	opt.Timeout = 10 * time.Second
	if err = a.ConfigureNativeOCR(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if err = a.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	task, err := a.StartTask(ctx, func(ctx context.Context, l *Lease) error { return l.RunNativeOCR(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	started := opt.Candidates[0] + ".started"
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, e := os.Stat(started); e == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, e := os.Stat(started); e != nil {
		task.Cancel()
		t.Fatal("actual child never started", e)
	}
	readCtx, readCancel := context.WithTimeout(ctx, time.Second)
	if _, e := a.ReadDocument(readCtx, boxID, docID); e != nil {
		readCancel()
		task.Cancel()
		t.Fatal("OCR held Context read gate", e)
	}
	readCancel()
	task.Cancel()
	wait, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err = task.Wait(wait); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(a.workspace.resources.ocr.nodes) == 0 {
		t.Fatal("failed OCR acknowledged nodes")
	}
	file := filepath.Join(root, filepath.FromSlash(ocrCachePath(pageA)))
	if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("broken JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := a.workspace.resources.ocr
	opt.Enabled = false
	if err = a.ConfigureNativeOCR(ctx, opt); err == nil {
		t.Fatal("corrupted local cache ignored")
	}
	if a.workspace.resources.ocr != previous {
		t.Fatal("failed configure replaced live owner")
	}
	raw, err := os.ReadFile(file)
	if err != nil || string(raw) != "broken JSON" {
		t.Fatal(string(raw), err)
	}
}

func TestOwnedOCRDropsStaleSnapshotAndRetriesWithoutBlockingReader(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "before recognition")
	dir := t.TempDir()
	binary := filepath.Join(dir, "tesseract")
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	quote := func(p string) string { return "'" + strings.ReplaceAll(p, "'", "'\\''") + "'" }
	script := `#!/bin/sh
case "$1" in
 --version) printf 'tesseract fixture-1\n';exit 0;;
 --list-langs) printf 'List:\neng\n';exit 0;;
esac
printf started > ` + quote(started) + `
while [ ! -f ` + quote(release) + ` ]; do sleep 0.01; done
IFS= read -r content < "$3"
printf 'level\ttext\n5\t%s\n' "$content"
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	opt := ocr.Defaults()
	opt.Candidates = []string{binary}
	if err = l.ConfigureNativeOCR(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	task, err := l.StartTask(ctx, func(ctx context.Context, l *Lease) error { return l.RunNativeOCR(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, e := os.Stat(started); e == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, e := os.Stat(started); e != nil {
		task.Cancel()
		t.Fatal("child did not start", e)
	}
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	if _, e := l.ReadDocument(readCtx, boxID, docID); e != nil {
		cancel()
		task.Cancel()
		t.Fatal("background command blocked reader", e)
	}
	cancel()
	if err = os.WriteFile(filepath.Join(root, "pages", pageA, "data", "assets", "a.png"), []byte("after recognition"), 0600); err != nil {
		task.Cancel()
		t.Fatal(err)
	}
	if err = os.WriteFile(release, nil, 0600); err != nil {
		task.Cancel()
		t.Fatal(err)
	}
	if err = task.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if len(l.workspace.resources.ocr.texts) != 0 || len(l.workspace.resources.ocr.nodes) != 1 {
		t.Fatal("stale OCR was accepted or refresh lost")
	}
	if err = l.RunNativeOCR(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	block, _, err := l.NativeBlock(ctx, blockID)
	if err != nil || !strings.Contains(block.Content, "after recognition") || strings.Contains(block.Content, "before recognition") {
		t.Fatal(block.Content, err)
	}
}

func TestOwnedBlockTreeSourceSnapshotPhysicalParentsAndRollback(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
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
	before := digest(t, root)
	if err = a.RebuildNativeBlockTree(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := a.NativeBlockTrees(ctx, indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil || len(rows) != 4 {
		t.Fatal(rows, err)
	}
	other, err := b.NativeBlockTrees(ctx, indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil || len(other) != 0 {
		t.Fatal("A populated B", other, err)
	}
	if err = a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.BlockTree)
		_, e := db.Exec("CREATE TRIGGER fail_tree BEFORE INSERT ON blocktrees BEGIN SELECT RAISE(ABORT,'injected'); END")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = a.RebuildNativeBlockTree(ctx); err == nil {
		t.Fatal("snapshot failure ignored")
	}
	after, err := a.NativeBlockTrees(ctx, indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil || len(after) != 4 {
		t.Fatal("partial registry", after, err)
	}
	if digest(t, root) != before {
		t.Fatal("registry changed source")
	}
}

func TestOwnedBlockTreeResolvesUnflushedOtherDocumentWithPhysicalParent(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const otherDoc = "20261022010000-doc0002"
	const otherBlock = "20261022010001-par0002"
	const heading = "20261022010002-hea0001"
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(raw), `"TextMarkBlockRefID": "`+blockID+`"`, `"TextMarkBlockRefID": "`+otherBlock+`"`, 1)
	if changed == string(raw) {
		t.Fatal("ref fixture replacement failed")
	}
	if err = os.WriteFile(file, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	extra := `{"ID":"` + otherDoc + `","Spec":"1","Type":"NodeDocument","Properties":{"id":"` + otherDoc + `","title":"Other"},"Children":[{"ID":"` + heading + `","Type":"NodeHeading","HeadingLevel":1,"Properties":{"id":"` + heading + `"},"Children":[{"Type":"NodeText","Data":"Heading"}]},{"ID":"` + otherBlock + `","Type":"NodeParagraph","Properties":{"id":"` + otherBlock + `"},"Children":[{"Type":"NodeText","Data":"not FTS-indexed"}]}]}`
	if err = os.WriteFile(filepath.Join(filepath.Dir(file), otherDoc+".docj"), []byte(extra), 0600); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, _ := b.Database(indexdb.Main)
		var parent, defRoot string
		e := db.QueryRow("SELECT def_block_parent_id,def_block_root_id FROM refs").Scan(&parent, &defRoot)
		if e != nil || parent != otherDoc || defRoot != otherDoc {
			t.Fatal("definition read main/global/logical-heading metadata", parent, defRoot, e)
		}
		var count int
		if e = db.QueryRow("SELECT COUNT(*) FROM blocks WHERE root_id=?", otherDoc).Scan(&count); e != nil || count != 0 {
			t.Fatal("oracle document was FTS indexed", count, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := l.NativeBlockTrees(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: otherBlock})
	if err != nil || len(rows) != 1 || rows[0].ParentID != otherDoc {
		t.Fatal(rows, err)
	}
	if digest(t, root) != before {
		t.Fatal("source registry wrote source")
	}
}

func TestOwnedOCREightImageBatchesRetainPendingNodeAndCleanMissingText(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "first image")
	dir := filepath.Join(root, "pages", pageA, "data", "assets")
	for _, name := range []string{"b.png", "c.png", "d.png", "e.png", "f.png", "g.png", "h.png", "z.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+" recognized"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	docFile := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(docFile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(docFile, []byte(strings.ReplaceAll(string(raw), "assets/a.png", "assets/z.png")), 0600); err != nil {
		t.Fatal(err)
	}
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	opts := ocr.Defaults()
	opts.Candidates = []string{ownedOCRBinary(t, false)}
	if err = l.ConfigureNativeOCR(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.RunNativeOCR(ctx); err != nil {
		t.Fatal(err)
	}
	state := l.workspace.resources.ocr
	if len(state.texts) != 8 || len(state.nodes) != 1 {
		t.Fatal("eight-image batch lost pending refresh", len(state.texts), len(state.nodes))
	}
	if err = l.RunNativeOCR(ctx); err != nil {
		t.Fatal(err)
	}
	if len(state.texts) != 9 || len(state.nodes) != 0 {
		t.Fatal(len(state.texts), len(state.nodes))
	}
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	block, found, err := l.NativeBlock(ctx, blockID)
	if err != nil || !found || !strings.Contains(block.Content, "z.png recognized") {
		t.Fatal(block, err)
	}
	if err = os.Remove(filepath.Join(dir, "z.png")); err != nil {
		t.Fatal(err)
	}
	if err = l.RunNativeOCR(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.texts["assets/z.png"]; ok {
		t.Fatal("missing asset OCR not cleaned")
	}
	if err = os.MkdirAll(filepath.Join(root, ".local", "workspaces", pageA, "temp"), 0700); err != nil {
		t.Fatal(err)
	}
	cacheFile := filepath.Join(root, filepath.FromSlash(ocrCachePath(pageA)))
	outside := filepath.Join(t.TempDir(), "foreign")
	if err = os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, cacheFile); err != nil {
		t.Fatal(err)
	}
	if err = l.FlushNativeOCRCache(ctx); !errors.Is(err, ErrDocument) {
		t.Fatal("cache symlink accepted", err)
	}
	foreign, err := os.ReadFile(outside)
	if err != nil || string(foreign) != "keep" || !state.dirty {
		t.Fatal(string(foreign), state.dirty, err)
	}
}

func TestOwnedAssetHashFallbackFreshnessAndPeriodicCatalogArePageBound(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	ownedAssets(t, root, pageA, "hash fixture")
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
	opt := ocr.Defaults()
	opt.Enabled = false
	if err = a.ConfigureNativeOCR(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if err = a.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeMainQueue(ctx); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err = a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.Main)
		return db.QueryRow("SELECT hash FROM assets WHERE path='assets/a.png'").Scan(&hash)
	}); err != nil {
		t.Fatal(err)
	}
	if err = a.ReloadNativeAssets(ctx); err != nil {
		t.Fatal(err)
	} // Drop the hash cache, retain original SQL fallback.
	value, found, err := a.NativeAssetByHash(ctx, hash)
	if err != nil || !found || value.Path != "assets/a.png" {
		t.Fatal(value, found, err)
	}
	if value, found, err = b.NativeAssetByHash(ctx, hash); err != nil || found {
		t.Fatal("A hash escaped to B", value, found, err)
	}
	image := filepath.Join(root, "pages", pageA, "data", "assets", "a.png")
	if err = os.WriteFile(image, []byte("changed contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err = a.NativeAssetByHash(ctx, hash); err != nil || found {
		t.Fatal("stale hash cache/SQL returned old contents", found, err)
	}
	task, err := a.StartNativeAssetRefresh(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(filepath.Dir(image), "new.png")
	if err = os.WriteFile(extra, []byte("new"), 0600); err != nil {
		task.Cancel()
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		snapshot, e := a.NativeAssets(ctx)
		if e != nil {
			task.Cancel()
			t.Fatal(e)
		}
		if _, seen = snapshot["assets/new.png"]; seen {
			break
		}
		time.Sleep(time.Millisecond)
	}
	task.Cancel()
	if err = task.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("owned catalog task did not refresh")
	}
	snapshot, err := b.NativeAssets(ctx)
	if err != nil || len(snapshot) != 0 {
		t.Fatal("A refresh loaded B", snapshot, err)
	}
}
