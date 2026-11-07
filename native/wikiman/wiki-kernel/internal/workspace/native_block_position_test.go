//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

const positionH1 = "20261027200000-head001"
const positionSB = "20261027200001-super01"
const positionH2 = "20261027200002-head002"
const positionInside = "20261027200003-par0001"
const positionAfter = "20261027200004-par0002"
const positionAV = "20261027200005-avnode1"
const positionRef = "20261019090004-ref0001"

func positionSource(t *testing.T) string {
	t.Helper()
	root := infoSource(t)
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		node := func(id, typ, text string) map[string]interface{} {
			n := map[string]interface{}{"ID": id, "Type": typ, "Properties": map[string]string{"id": id}}
			if text != "" {
				n["Children"] = []interface{}{map[string]string{"Type": "NodeText", "Data": text}}
			}
			return n
		}
		h := node(positionH1, "NodeHeading", "Heading & "+page)
		h["HeadingLevel"] = 1
		h["Properties"].(map[string]string)["name"] = "Named & " + page
		h2 := node(positionH2, "NodeHeading", "Nested "+page)
		h2["HeadingLevel"] = 2
		sb := node(positionSB, "NodeSuperBlock", "")
		sb["Children"] = []interface{}{h2, node(positionInside, "NodeParagraph", "Inside "+page)}
		a := node(positionAV, "NodeAttributeView", "")
		a["AttributeViewID"] = getDocAV
		d["Children"] = append(append([]interface{}{h}, d["Children"].([]interface{})...), sb, node(positionAfter, "NodeParagraph", "After "+page), a)
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return root
}
func positionOptions(root string) Options {
	opts := getDocOptions()
	opts.NativeContent = bookmarkViewOptions(root).NativeContent
	opts.ConfigurationBusiness = func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
			return []conf.Notebook{{ID: boxID, Name: "Notebook & " + page}}, nil
		}, nil, nil), nil
	}
	return opts
}

func TestNativeBlockPositionRealOwnersIndexesAndBreadcrumbs(t *testing.T) {
	ctx := context.Background()
	root := positionSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	for _, l := range []*Lease{a, b} {
		for _, tc := range []struct {
			id    string
			index int
		}{{blockID, 2}, {positionRef, 4}, {positionInside, 5}, {docID, 7}, {"missing", 0}} {
			index, e := l.NativeBlockIndex(ctx, tc.id)
			if e != nil || index != tc.index {
				t.Fatal(tc, index, e)
			}
		}
		indexes, e := l.NativeBlocksIndexes(ctx, []string{positionRef, positionInside, getDocDefinition, docID, positionRef})
		if e != nil || !reflect.DeepEqual(indexes, map[string]int{positionRef: 4, positionInside: 5, getDocDefinition: 0, docID: 0}) {
			t.Fatal(indexes, e)
		}
		paths, e := l.NativeBlockBreadcrumb(ctx, positionAfter, nil, false, 0)
		if e != nil || len(paths) != 4 || paths[0].Name != "Notebook &amp; "+l.PageID()+"/Title "+l.PageID() || paths[1].ID != positionH1 || paths[1].Name != "Heading &amp; "+l.PageID() || paths[2].ID != positionH2 || paths[3].Name != "" {
			t.Fatal(paths, e)
		}
		paths, e = l.NativeBlockBreadcrumb(ctx, positionRef, nil, false, 0)
		if e != nil || len(paths) != 3 || paths[2].ID != infoItem || paths[2].SubType != "u" {
			t.Fatal(paths, e)
		}
		paths, e = l.NativeBlockBreadcrumb(ctx, positionH1, nil, true, 2)
		if e != nil || paths[len(paths)-1].Name != "Named &amp; "+l.PageID() {
			t.Fatal(paths, e)
		}
		paths[0].Name = "changed"
		again, e := l.NativeBlockBreadcrumb(ctx, positionH1, nil, true, 0)
		if e != nil || again[0].Name == "changed" || again[len(again)-1].Name != "" {
			t.Fatal(again, e)
		}
		r, e := l.NativeBlockPosition(ctx, docquery.PositionRequest{Mode: "dom-text", Content: `<div data-node-id="` + positionAV + `" data-type="NodeAttributeView" data-av-id="` + getDocAV + `"></div>`})
		if e != nil || r.Text != l.PageID()+" database" {
			t.Fatal("AV text owner", r, e)
		}
		if l.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("position read flushed main queue")
		}
	}
	if digest(t, root) != before {
		t.Fatal("position/breadcrumb wrote source")
	}
}

func TestNativeBlockPositionOwnRegistryFailuresAndCancellation(t *testing.T) {
	ctx := context.Background()
	root := positionSource(t)
	m := open(t, root, positionOptions(root))
	l := getDocLease(t, m, pageA)
	if _, e := l.NativeBlockIndex(ctx, blockID); e != nil {
		t.Fatal(e)
	}
	if e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		_, e = db.ExecContext(ctx, "UPDATE blocktrees SET hpath=? WHERE id=?", "/Owned registry", docID)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	paths, e := l.NativeBlockBreadcrumb(ctx, positionAfter, nil, false, 0)
	if e != nil || paths[0].Name != "Notebook &amp; "+pageA+"/Owned registry" {
		t.Fatal(paths, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r, e := l.NativeBlockPosition(canceled, docquery.PositionRequest{Mode: "indexes"}); !errors.Is(e, context.Canceled) || !reflect.DeepEqual(r, docquery.PositionResult{}) {
		t.Fatal(r, e)
	}
	<-l.workspace.resources.gate
	deadline, cancelDeadline := context.WithTimeout(ctx, 10*time.Millisecond)
	paths, e = l.NativeBlockBreadcrumb(deadline, blockID, nil, false, 0)
	cancelDeadline()
	l.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) || paths != nil {
		t.Fatal(paths, e)
	}
	if e = os.Remove(filepath.Join(root, "pages", pageA, "data", "storage", "av", getDocAV+".json")); e != nil {
		t.Fatal(e)
	}
	if paths, e = l.NativeBlockBreadcrumb(ctx, positionAV, nil, false, 0); e == nil || paths != nil {
		t.Fatal("missing AV fallback", paths, e)
	}
	if _, e = l.NativeBlockIndex(ctx, positionAV); e != nil {
		t.Fatal("index unnecessarily depends on AV", e)
	}
	l.Release()
	if _, e = l.NativeBlocksIndexes(ctx, nil); e == nil {
		t.Fatal("released owner accepted")
	}
}

func TestNativeBlockPositionConcurrentOwnersAndRequiredNotebook(t *testing.T) {
	ctx := context.Background()
	root := positionSource(t)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	var wg sync.WaitGroup
	for _, l := range []*Lease{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				paths, e := l.NativeBlockBreadcrumb(ctx, positionAfter, nil, false, 0)
				if e != nil || !strings.Contains(paths[0].Name, l.PageID()) {
					t.Error(paths, e)
					return
				}
			}
		}()
	}
	wg.Wait()
	unbound := open(t, positionSource(t), Options{Configuration: originalProjectionConfiguration})
	l := getDocLease(t, unbound, pageA)
	if _, e := l.NativeBlockIndex(ctx, blockID); e != nil {
		t.Fatal(e)
	}
	if p, e := l.NativeBlockBreadcrumb(ctx, blockID, nil, false, 0); !errors.Is(e, conf.ErrBusinessUnbound) || p != nil {
		t.Fatal("notebook fallback", p, e)
	}
}
