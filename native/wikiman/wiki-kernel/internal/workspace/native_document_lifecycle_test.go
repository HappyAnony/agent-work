//go:build fts5

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
)

const lifecycleNew = "20261027230003-new0001"

func TestNativeDocumentLifecycleRealOwnersAllCommandsAndSourceUnchanged(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	before := digest(t, root)
	for _, l := range []*Lease{a, b} {
		p, e := l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "create", Box: boxID, Path: "/" + lifecycleNew + ".docj", Title: "Created", Markdown: "Hello"})
		if e != nil || p.Tree == nil || p.Tree.Root.IALAttr("title") != "Created" {
			t.Fatal(p, e)
		}
		p, e = l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "rename", Box: boxID, Path: "/" + docID + ".docj", Title: "Renamed"})
		if e != nil || len(p.Changes) != 2 || p.Changes[0].From.HPath != "/Title "+l.PageID() || p.Changes[1].Tree.HPath != "/Renamed/Child" {
			t.Fatal(p, e)
		}
		raw, e := os.ReadFile(filepath.Join(root, "pages", l.PageID(), "data", boxID, docID+".docj"))
		if e != nil || p.SourceHashes["data/"+boxID+"/"+docID+".docj"] != sha256.Sum256(raw) {
			t.Fatal("candidate source hash mismatch", e)
		}
		p, e = l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "move", Paths: []string{"/" + docID + "/" + infoChild + ".docj", "/" + docID + ".docj"}, ToBox: boxID, ToPath: "/" + getDocOther + ".docj"})
		if e != nil || len(p.Changes) != 2 || p.Changes[1].Tree.Path != "/"+getDocOther+"/"+docID+"/"+infoChild+".docj" || p.Changes[1].Tree.HPath != "/DocTerm/Title "+l.PageID()+"/Child" {
			t.Fatal(p, e)
		}
		p, e = l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "delete", Box: boxID, Path: "/" + docID + ".docj"})
		if e != nil || len(p.Changes) != 2 || len(p.IDs) != 2 || !strings.Contains(strings.Join(p.Effects, ","), "av-unbind") {
			t.Fatal(p, e)
		}
		if l.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("lifecycle plan flushed main queue")
		}
	}
	if digest(t, root) != before {
		t.Fatal("lifecycle candidates wrote source")
	}
}

func TestNativeDocumentLifecycleRestoreRealHistoryDuplicateAndScope(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		raw, e := os.ReadFile(filepath.Join(root, "pages", l.PageID(), "data", boxID, docID+".docj"))
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		d["Children"] = append(d["Children"].([]interface{}), map[string]interface{}{"ID": getDocDefinition, "Type": "NodeParagraph", "Properties": map[string]string{"id": getDocDefinition}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": "Conflict"}}})
		raw, _ = json.Marshal(d)
		file := filepath.Join(l.workspace.paths.RuntimeDir, "history", "stamp", boxID, docID+".docj")
		if e = os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		before := digest(t, root)
		p, e := l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "restore", Box: boxID, HistoryPath: "stamp/" + boxID + "/" + docID + ".docj"})
		if e != nil || p.Tree == nil || p.Tree.Root.IALAttr("title") != "Title "+l.PageID() || p.Tree.Root.LastChild.ID == getDocDefinition || p.Working == nil || len(p.Effects) != 10 {
			t.Fatal(p, e)
		}
		if digest(t, root) != before {
			t.Fatal("restore candidate committed source/history")
		}
		for _, bad := range []string{"../outside.docj", "/tmp/outside.docj"} {
			if _, e = l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "restore", Box: boxID, HistoryPath: bad}); e == nil {
				t.Fatal("foreign history accepted")
			}
		}
		link := filepath.Join(l.workspace.paths.RuntimeDir, "history", lifecycleNew+".docj")
		if e = os.Symlink(file, link); e != nil {
			t.Fatal(e)
		}
		if _, e = l.PlanNativeDocumentLifecycle(ctx, doclife.Request{Action: "restore", Box: boxID, HistoryPath: lifecycleNew + ".docj"}); e == nil {
			t.Fatal("history symlink accepted")
		}
	}
}

func TestNativeDocumentLifecycleCancelBadSourceReleaseAndConcurrent(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	var wg sync.WaitGroup
	q := doclife.Request{Action: "rename", Box: boxID, Path: "/" + docID + ".docj", Title: "Candidate"}
	for _, l := range []*Lease{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := l.PlanNativeDocumentLifecycle(ctx, q)
			if e != nil || p.Changes[0].From.HPath != "/Title "+l.PageID() {
				t.Error(p, e)
			}
		}()
	}
	wg.Wait()
	c, stop := context.WithCancel(ctx)
	stop()
	if p, e := a.PlanNativeDocumentLifecycle(c, q); !errors.Is(e, context.Canceled) || p.Tree != nil {
		t.Fatal(p, e)
	}
	<-a.workspace.resources.gate
	c, stop = context.WithTimeout(ctx, 10*time.Millisecond)
	_, e := a.PlanNativeDocumentLifecycle(c, q)
	stop()
	a.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(root, "pages", pageA, "data", boxID, docID, infoChild+".docj")); e != nil {
		t.Fatal(e)
	}
	// A discovered child disappearing after registry creation is not a
	// successful partial rename. Corrupt the still-discovered root as well.
	if e = os.WriteFile(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if p, e := a.PlanNativeDocumentLifecycle(ctx, q); e == nil || p.Tree != nil {
		t.Fatal("bad source fallback", p, e)
	}
	a.Release()
	if _, e = a.PlanNativeDocumentLifecycle(ctx, q); e == nil {
		t.Fatal("released owner accepted")
	}
}

func lifecycleOptions(root string) Options {
	o := positionOptions(root)
	provider := o.Configuration
	o.Configuration = func(store, page string) (conf.AppData, error) {
		c, e := provider(store, page)
		c.FileTree = conf.NewFileTree()
		return c, e
	}
	return o
}
