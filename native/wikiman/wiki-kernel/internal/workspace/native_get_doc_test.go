//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

const getDocOther = "20261027170000-other01"
const getDocDefinition = "20261027170001-def0001"
const getDocAV = "20261027170100-av00001"

func getDocSource(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		base := filepath.Join(root, "pages", page, "data")
		file := filepath.Join(base, boxID, docID+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		d["Properties"].(map[string]interface{})["title"] = "Title " + page
		p := d["Children"].([]interface{})[0].(map[string]interface{})
		p["Children"] = []interface{}{map[string]string{"Type": "NodeText", "Data": "SharedNeedle O'Reilly OwnerTermA OwnerTermB ExtraTerm DropTerm AliasTerm DocTerm fixture paragraph"}}
		p["Properties"].(map[string]interface{})["custom-avs"] = getDocAV
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		term := "OwnerTermA"
		if page == pageB {
			term = "OwnerTermB"
		}
		d = map[string]interface{}{"ID": getDocOther, "Type": "NodeDocument", "Properties": map[string]string{"id": getDocOther, "title": "DocTerm"}, "Children": []interface{}{map[string]interface{}{"ID": getDocDefinition, "Type": "NodeParagraph", "Properties": map[string]string{"id": getDocDefinition, "name": term, "alias": "AliasTerm"}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": term}}}}}
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(filepath.Join(base, boxID, getDocOther+".docj"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if e = os.MkdirAll(filepath.Join(base, "storage", "av"), 0700); e != nil {
			t.Fatal(e)
		}
		raw, _ = json.Marshal(map[string]interface{}{"id": getDocAV, "name": page + " database", "spec": 4, "views": []interface{}{}, "keyValues": []interface{}{}})
		if e = os.WriteFile(filepath.Join(base, "storage", "av", getDocAV+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return root
}
func getDocOptions() Options {
	return Options{Configuration: func(store, page string) (conf.AppData, error) {
		c, e := originalProjectionConfiguration(store, page)
		c.Editor.VirtualBlockRef = true
		c.Editor.VirtualBlockRefInclude = "ExtraTerm,DropTerm"
		c.Editor.VirtualBlockRefExclude = "/Drop.*/"
		c.Search.VirtualRefName = true
		c.Search.VirtualRefAlias = false
		c.Search.VirtualRefAnchor = false
		c.Search.VirtualRefDoc = false
		c.Search.CaseSensitive = page == pageB
		return c, e
	}}
}
func getDocLease(t *testing.T, m *WorkspaceManager, page string) *Lease {
	t.Helper()
	l, e := m.Acquire(context.Background(), storeID, page)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Release)
	return l
}
func queueGetDoc(t *testing.T, l *Lease) {
	t.Helper()
	for _, id := range []string{docID, getDocOther} {
		if e := l.QueueNativeTree(context.Background(), boxID, id, "index"); e != nil {
			t.Fatal(e)
		}
	}
}

func TestNativeGetDocActualOwnersHighlightsVirtualAVAndRecent(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	before := digest(t, root)
	m := open(t, root, getDocOptions())
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	for _, l := range []*Lease{a, b} {
		for _, method := range []int{0, 1, 3} {
			r, e := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100, Query: "SharedNeedle", QueryMethod: method, QueryTypes: map[string]bool{"paragraph": true}, Highlight: true})
			if e != nil || !strings.Contains(r.DOM, "search-mark") || !strings.Contains(r.DOM, `refcount="1"`) || !strings.Contains(r.DOM, l.PageID()+" database") || r.RootID != docID || r.Box != boxID || !reflect.DeepEqual(r.Keywords, []string{"SharedNeedle"}) {
				t.Fatal(l.PageID(), method, r, e)
			}
		}
		r, e := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100})
		owner, other := "OwnerTermA", "OwnerTermB"
		if l == b {
			owner, other = other, owner
		}
		if e != nil || !strings.Contains(r.DOM, `data-type="virtual-block-ref">`+owner+`</span>`) || strings.Contains(r.DOM, `data-type="virtual-block-ref">`+other+`</span>`) || !strings.Contains(r.DOM, `data-type="virtual-block-ref">ExtraTerm</span>`) || strings.Contains(r.DOM, `data-type="virtual-block-ref">DropTerm</span>`) {
			t.Fatal("virtual isolation", l.PageID(), r.DOM, e)
		}
		back, err := l.NativeGetDoc(ctx, docview.Request{ID: blockID, Size: 100, Backlink: true, OriginalRefs: map[string]string{blockID: blockID}})
		if err != nil || !back.BacklinkExpand || !strings.Contains(back.DOM, blockID) {
			t.Fatal("native backlink", back, err)
		}
		r, e = l.NativeGetDoc(ctx, docview.Request{ID: blockID, Query: "O'Reilly", QueryTypes: map[string]bool{"paragraph": true}, Highlight: true, Size: 100})
		if e != nil || !strings.Contains(r.DOM, "search-mark") {
			t.Fatal("quoted highlight", r, e)
		}
		r, e = l.NativeGetDoc(ctx, docview.Request{ID: blockID, Query: "sharedneedle", QueryTypes: map[string]bool{"paragraph": true}, Highlight: true, Size: 100})
		if e != nil || (strings.Contains(r.DOM, "search-mark") != (l == a)) {
			t.Fatal("owner case", r, e)
		}
		r, e = l.NativeGetDoc(ctx, docview.Request{ID: blockID, Query: "SharedNeedle", QueryTypes: map[string]bool{"paragraph": true}, Highlight: false, Size: 100})
		if e != nil || strings.Contains(r.DOM, "search-mark") || len(r.Keywords) == 0 {
			t.Fatal("highlight disabled", r, e)
		}
		docs, e := l.NativeRecentDocs(ctx)
		if e != nil || len(docs) != 1 || docs[0].RootID != docID || docs[0].Title != "Title "+l.PageID() || docs[0].ClosedAt != 0 {
			t.Fatal(docs, e)
		}
		if l == a && b.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("A drained B index queue")
		}
	}
	if b.workspace.resources.contentQueue.Pending() != 0 {
		t.Fatal("pending index work")
	}
	if digest(t, root) != before {
		t.Fatal("GetDoc modified authoritative source")
	}
	a.Release()
	b.Release()
	if _, e := m.EvictIdle(time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	reopened := getDocLease(t, m, pageA)
	docs, e := reopened.NativeRecentDocs(ctx)
	if e != nil || len(docs) != 1 || docs[0].Title != "Title "+pageA {
		t.Fatal("recent did not survive reopen", docs, e)
	}
}

func TestNativeGetDocCancelFailuresAndRecentCorruption(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	l := getDocLease(t, m, pageA)
	queueGetDoc(t, l)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r, e := l.NativeGetDoc(canceled, docview.Request{ID: docID}); !errors.Is(e, context.Canceled) || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal(r, e)
	}
	if _, e := os.Stat(filepath.Join(root, recentPath(l.workspace))); !os.IsNotExist(e) {
		t.Fatal("canceled read wrote recent", e)
	}
	if r, e := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100, Query: "[", QueryMethod: 3}); e == nil || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal("invalid regex accepted", r, e)
	}
	if _, e := os.Stat(filepath.Join(root, recentPath(l.workspace))); !os.IsNotExist(e) {
		t.Fatal("failed read wrote recent", e)
	}
	if _, e := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); e != nil {
		t.Fatal(e)
	}
	recentFile := filepath.Join(root, recentPath(l.workspace))
	recentBefore, _ := os.ReadFile(recentFile)
	<-l.workspace.resources.gate
	deadline, cancelDeadline := context.WithTimeout(ctx, 10*time.Millisecond)
	r, e := l.NativeGetDoc(deadline, docview.Request{ID: docID, Size: 100})
	cancelDeadline()
	l.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal("blocked admission not canceled", r, e)
	}
	after, _ := os.ReadFile(recentFile)
	if string(after) != string(recentBefore) {
		t.Fatal("canceled admission wrote recent")
	}
	avPath := filepath.Join(root, "pages", pageA, "data", "storage", "av", getDocAV+".json")
	if e = os.Remove(avPath); e != nil {
		t.Fatal(e)
	}
	if r, e = l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); e == nil || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal("missing AV fallback", r, e)
	}
	if e = os.WriteFile(recentFile, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = l.NativeRecentDocs(ctx); e == nil {
		t.Fatal("corrupt recent silently replaced")
	}
	if raw, _ := os.ReadFile(recentFile); string(raw) != "broken" {
		t.Fatal("corrupt state modified")
	}
	l.Release()
	if _, e = l.NativeGetDoc(ctx, docview.Request{ID: docID}); e == nil {
		t.Fatal("released owner accepted")
	}
}

func TestNativeGetDocConcurrentOwnersAndNoStaleVirtualDictionary(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	var wg sync.WaitGroup
	for _, l := range []*Lease{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				r, e := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100})
				if e != nil || !strings.Contains(r.DOM, l.PageID()+" database") {
					t.Error(r, e)
					return
				}
			}
		}()
	}
	wg.Wait()
	// A derived-index change must be visible on the next request, even for the
	// same root ID. The source remains unchanged; this only simulates new index
	// work already accepted by the normal index queue.
	if e := a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		_, e = db.ExecContext(ctx, "UPDATE blocks SET name=? WHERE id=?", "OwnerTermB", getDocDefinition)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	r, e := a.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100})
	if e != nil || strings.Contains(r.DOM, `data-type="virtual-block-ref">OwnerTermA</span>`) || !strings.Contains(r.DOM, `data-type="virtual-block-ref">OwnerTermB</span>`) {
		t.Fatal("stale virtual dictionary", r.DOM, e)
	}
}

func TestNativeRecentBoundOrderDurabilityAndUnsafePath(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	l := getDocLease(t, m, pageA)
	err := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		for i := 0; i < 40; i++ {
			id := fmt.Sprintf("202610271800%02d-doc0001", i)
			if e := saveNativeRecent(ctx, l.workspace, docview.Recent{ID: id, Title: fmt.Sprint(i)}); e != nil {
				return e
			}
		}
		return saveNativeRecent(ctx, l.workspace, docview.Recent{ID: "20261027180020-doc0001", Title: "latest"})
	})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := l.NativeRecentDocs(ctx)
	if err != nil || len(docs) != 32 || docs[0].Title != "latest" || docs[1].Title != "39" {
		t.Fatal(docs, err)
	}
	other := getDocLease(t, m, pageB)
	if docs, err = other.NativeRecentDocs(ctx); err != nil || len(docs) != 0 {
		t.Fatal("cross-owner recent", docs, err)
	}
	queueGetDoc(t, l)
	file := filepath.Join(root, recentPath(l.workspace))
	if err = os.WriteFile(file, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if r, e := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); e == nil || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal("corrupt recent did not reject read", r, e)
	}
	if raw, _ := os.ReadFile(file); string(raw) != "broken" {
		t.Fatal("corrupt recent repaired")
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign.json")
	if err = os.WriteFile(foreign, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(foreign, file); err != nil {
		t.Fatal(err)
	}
	if _, err = l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); err == nil {
		t.Fatal("symlink accepted")
	}
	if raw, _ := os.ReadFile(foreign); string(raw) != "untouched" {
		t.Fatal("foreign recent overwritten")
	}
}

func TestNativeGetDocVirtualConfigurationIgnoreAndMissingDependencies(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	l := getDocLease(t, m, pageA)
	queueGetDoc(t, l)
	if _, err := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); err != nil {
		t.Fatal(err)
	}
	if err := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		c := l.workspace.resources.configuration.Search
		c.VirtualRefAlias = true
		c.VirtualRefDoc = true
		c.VirtualRefAnchor = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r, err := l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"AliasTerm", "DocTerm", "fixture paragraph"} {
		if !strings.Contains(r.DOM, `data-type="virtual-block-ref">`+word+`</span>`) {
			t.Fatal(word, r.DOM)
		}
	}
	base := filepath.Join(root, "pages", pageA, "data", ".wiki")
	if err = os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	// Original ignore files contain SQL predicates. The owner reads them without
	// creating defaults; the reference predicate and block predicate are separate.
	if err = os.WriteFile(filepath.Join(base, "searchignore"), []byte("id != '"+getDocDefinition+"' AND type != 'd'"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(base, "refsearchignore"), []byte("content != 'fixture paragraph'"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"OwnerTermA", "AliasTerm", "DocTerm", "fixture paragraph"} {
		if strings.Contains(r.DOM, `data-type="virtual-block-ref">`+word+`</span>`) {
			t.Fatal("ignore not applied", word, r.DOM)
		}
	}
	if err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		l.workspace.resources.configuration.Editor.VirtualBlockRef = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r, err = l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100})
	if err != nil || strings.Contains(r.DOM, "virtual-block-ref") {
		t.Fatal("stale editor settings", r, err)
	}
	if err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { l.workspace.resources.configuration.Editor = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if r, err = l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); !errors.Is(err, conf.ErrBusinessUnbound) || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal("missing config fallback", r, err)
	}
	if err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { l.workspace.resources.configuration.Editor = conf.NewEditor(); return nil }); err != nil {
		t.Fatal(err)
	}
	avPath := filepath.Join(root, "pages", pageA, "data", "storage", "av", getDocAV+".json")
	raw, e := os.ReadFile(avPath)
	if e != nil {
		t.Fatal(e)
	}
	var avDoc map[string]interface{}
	if e = json.Unmarshal(raw, &avDoc); e != nil {
		t.Fatal(e)
	}
	avDoc["name"] = ""
	raw, _ = json.Marshal(avDoc)
	if e = os.WriteFile(avPath, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if r, err = l.NativeGetDoc(ctx, docview.Request{ID: docID, Size: 100}); !errors.Is(err, conf.ErrBusinessUnbound) || !reflect.DeepEqual(r, docview.Result{}) {
		t.Fatal("missing AV language fallback", r, err)
	}
}
