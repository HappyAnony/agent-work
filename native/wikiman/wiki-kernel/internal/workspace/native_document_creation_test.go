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
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/doclife"
)

func creationSettings(t *testing.T, root, page, save, tpl string) {
	t.Helper()
	base := filepath.Join(root, "pages", page, "data")
	dir := filepath.Join(base, boxID, ".siyuan")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	cfg := conf.NewBoxConf()
	cfg.DailyNoteSavePath = save
	cfg.DailyNoteTemplatePath = tpl
	raw, _ := json.Marshal(cfg)
	if e := os.WriteFile(filepath.Join(dir, "conf.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if tpl != "" {
		if e := os.MkdirAll(filepath.Join(base, "templates"), 0700); e != nil {
			t.Fatal(e)
		}
		content := "Owner " + page + " / .action{.title} / .action{.id}\n\n{: id=\"20261027221000-templ01\" type=\"doc\" name=\"Template Name\" custom-owner=\"" + page + "\" title=\"Ignore template title\" tags=\"not copied\"}\n"
		if e := os.WriteFile(filepath.Join(base, "templates", tpl), []byte(content), 0600); e != nil {
			t.Fatal(e)
		}
	}
}

func TestNativePeripheralCreationMarkdownMultiLevelAndNoPublication(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	before := digest(t, root)
	for _, l := range []*Lease{a, b} {
		q := doclife.CreationRequest{Mode: "markdown", Box: boxID, HPath: "/Title " + l.PageID() + "/Clips/First", ID: lifecycleNew, Markdown: "Clipped **text** $x$", Tags: " one， two ,,three ", WithMath: true, ClippingHref: "https://ld246.com/article/test"}
		p, e := l.PlanNativeDocumentCreation(ctx, q)
		if e != nil || p.Result.Tree == nil || len(p.Trees) != 2 || p.Result.ID != lifecycleNew || !strings.HasPrefix(p.Result.Path, "/"+docID+"/") || p.Result.Tree.Root.IALAttr("tags") != "one,two,three" || p.Result.Tree.HPath != "/Title "+l.PageID()+"/Clips/First" {
			t.Fatal(p, e)
		}
		raw, e := os.ReadFile(filepath.Join(root, "pages", l.PageID(), "data", boxID, docID+".docj"))
		if e != nil || p.SourceHashes["data/"+boxID+"/"+docID+".docj"] != sha256.Sum256(raw) {
			t.Fatal("source preimage mismatch", e)
		}
		if l.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("creation flushed main queue")
		}
	}
	if digest(t, root) != before {
		t.Fatal("creation candidate changed source")
	}
}

func TestNativePeripheralCreationDailyActualConfigTemplateAndReuse(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	for _, page := range []string{pageA, pageB} {
		creationSettings(t, root, page, "/Daily/{{now | date \"2006-01-02\"}}", "/daily.md")
	}
	m := open(t, root, lifecycleOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	before := digest(t, root)
	for _, l := range []*Lease{a, b} {
		queueGetDoc(t, l)
		if e := l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		file := filepath.Join(root, "pages", l.PageID(), "data", "templates", "daily.md")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.Replace(string(raw), "\n\n{: id=", "\n.action{range queryBlocks \"SELECT * FROM blocks WHERE id='"+getDocDefinition+"'\"}.action{.Content}.action{end}\n\n{: id=", 1))
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before = digest(t, root)
	for _, l := range []*Lease{a, b} {
		p, e := l.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID})
		if e != nil || p.Result.Existed || len(p.Trees) != 2 || p.Result.Tree.Root.IALAttr("custom-owner") != l.PageID() || p.Result.Tree.Root.IALAttr("name") != "Template Name" || p.Result.Tree.Root.IALAttr("tags") != "" || !strings.Contains(p.Result.Tree.Root.Text(), "Owner "+l.PageID()) || p.Result.Tree.Root.IALAttr(doclife.DailyNoteAttrPrefix+time.Now().Format("20060102")) == "" {
			t.Fatal(p, e)
		}
		if len(p.SourceHashes) < 2 {
			t.Fatal("configuration/template hashes missing")
		}
		term := "OwnerTermA"
		if l == b {
			term = "OwnerTermB"
		}
		if !strings.Contains(p.Result.Tree.Root.Text(), term) {
			t.Fatal("template SQL borrowed another owner", p.Result.Tree.Root.Text())
		}
	}
	if digest(t, root) != before {
		t.Fatal("daily template candidate saved source")
	}
	confPath := filepath.Join(root, "pages", pageB, "data", boxID, ".siyuan", "conf.json")
	if e := os.Remove(confPath); e != nil {
		t.Fatal(e)
	}
	defaultBefore := digest(t, root)
	defaultPlan, e := b.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID})
	if e != nil || !strings.HasPrefix(defaultPlan.Result.Tree.HPath, "/daily note/") {
		t.Fatal(defaultPlan, e)
	}
	if _, e = os.Stat(confPath); !os.IsNotExist(e) || digest(t, root) != defaultBefore {
		t.Fatal("default config read created source", e)
	}
	creationSettings(t, root, pageA, "/Title "+pageA, "must-not-read.md")
	if e := os.Remove(filepath.Join(root, "pages", pageA, "data", "templates", "must-not-read.md")); e != nil {
		t.Fatal(e)
	}
	p, e := a.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID})
	if e != nil || !p.Result.Existed || p.Result.ID != docID || len(p.Trees) != 1 {
		t.Fatal(p, e)
	}
	creationSettings(t, root, pageA, "/NewDaily", "missing.md")
	if e = os.Remove(filepath.Join(root, "pages", pageA, "data", "templates", "missing.md")); e != nil {
		t.Fatal(e)
	}
	before = digest(t, root)
	if p, e = a.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID}); e == nil || p.Trees != nil {
		t.Fatal("missing template silently accepted", p, e)
	}
	if digest(t, root) != before {
		t.Fatal("failed daily preparation wrote source")
	}
}

func TestNativePeripheralCreationTemplateAVCloneFromActualSource(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	for _, page := range []string{pageA, pageB} {
		creationSettings(t, root, page, "/Template AV", "av.md")
		base := filepath.Join(root, "pages", page, "data")
		a := &av.AttributeView{ID: getDocAV, Name: page + " template database", Spec: 4, ViewID: "view", KeyIDs: []string{"primary"}, KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Type: av.KeyTypeBlock}}}, Views: []*av.View{{ID: "view", Name: "Table", LayoutType: av.LayoutTypeTable, PageSize: 32, Table: &av.LayoutTable{BaseLayout: &av.BaseLayout{}, Columns: []*av.ViewTableColumn{{BaseField: &av.BaseField{ID: "primary"}}}}}}}
		raw, e := json.Marshal(a)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(base, "storage", "av", getDocAV+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		content := "<div data-type=\"NodeAttributeView\" data-av-id=\"" + getDocAV + "\" data-av-type=\"table\"></div>\n"
		if e = os.WriteFile(filepath.Join(base, "templates", "av.md"), []byte(content), 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := open(t, root, lifecycleOptions(root))
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		p, e := l.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID})
		if e != nil || len(p.AttributeViews) != 1 {
			t.Fatal(p, e)
		}
		for id, a := range p.AttributeViews {
			if id == getDocAV || a.Name != page+" template database" || len(a.KeyValues[0].Values) != 0 || a.KeyValues[0].Key.ID == "primary" {
				t.Fatal(a)
			}
		}
	}
	if digest(t, root) != before {
		t.Fatal("template AV candidate published source")
	}
}

func TestNativePeripheralCreationDuplicateRealSourceUnboundAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
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
		d["Children"] = append(d["Children"].([]interface{}), map[string]interface{}{"ID": "20261027223000-mirror1", "Type": "NodeAttributeView", "AttributeViewID": getDocAV, "Properties": map[string]string{"id": "20261027223000-mirror1"}})
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := open(t, root, lifecycleOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	before := digest(t, root)
	var wg sync.WaitGroup
	for _, l := range []*Lease{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := l.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "duplicate", ID: docID, Box: boxID})
			if e != nil || p.Result.Tree == nil || p.Result.ID == docID || !strings.HasPrefix(p.Result.Tree.Root.IALAttr("title"), "Title "+l.PageID()+" (Duplicated ") || p.Result.Tree.Root.IALAttr("custom-avs") != "" || p.Result.Tree.Root.FirstChild.IALAttr("custom-avs") != "" || len(p.Trees) != 1 {
				t.Error(p, e)
			}
			mirror := false
			for _, effect := range p.Effects {
				if effect.Kind == "mirror" && effect.Target == getDocAV && effect.ID != "20261027223000-mirror1" {
					mirror = true
				}
			}
			if !mirror {
				t.Error("real AV mirror candidate missing", p.Effects)
			}
		}()
	}
	wg.Wait()
	if digest(t, root) != before {
		t.Fatal("duplicate candidate changed source")
	}
}

func TestNativePeripheralCreationCancellationBadSourcesAndRelease(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, lifecycleOptions(root))
	l := getDocLease(t, m, pageA)
	q := doclife.CreationRequest{Mode: "duplicate", ID: docID, Box: boxID}
	c, stop := context.WithCancel(ctx)
	stop()
	if p, e := l.PlanNativeDocumentCreation(c, q); !errors.Is(e, context.Canceled) || p.Trees != nil {
		t.Fatal(p, e)
	}
	<-l.workspace.resources.gate
	c, stop = context.WithTimeout(ctx, 10*time.Millisecond)
	_, e := l.PlanNativeDocumentCreation(c, q)
	stop()
	l.workspace.resources.gate <- struct{}{}
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	creationSettings(t, root, pageA, "/New", "daily.md")
	link := filepath.Join(root, "pages", pageA, "data", "templates", "linked.md")
	if e = os.Symlink(filepath.Join(root, "pages", pageB, "data", boxID, docID+".docj"), link); e != nil {
		t.Fatal(e)
	}
	creationSettings(t, root, pageA, "/New", "")
	raw, _ := json.Marshal(map[string]string{"dailyNoteSavePath": "/New", "dailyNoteTemplatePath": "linked.md"})
	if e = os.WriteFile(filepath.Join(root, "pages", pageA, "data", boxID, ".siyuan", "conf.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if p, e := l.PlanNativeDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID}); e == nil || p.Trees != nil {
		t.Fatal("foreign template symlink accepted", p, e)
	}
	l.Release()
	if _, e = l.PlanNativeDocumentCreation(ctx, q); e == nil {
		t.Fatal("released owner accepted")
	}
}
