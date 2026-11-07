//go:build fts5

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/indexdb"
)

func TestNativeDynamicAnchorCandidatesIsolationNoSourceOrIndexPublication(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		if e = l.QueueNativeReferences(ctx, boxID, docID); e != nil {
			t.Fatal(e)
		}
		if e = l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		plan, e := l.PlanNativeDynamicAnchors(ctx, []string{blockID})
		if e != nil || len(plan.Trees) != 1 || len(plan.Changes) != 1 || len(plan.SourceHashes) != 1 {
			t.Fatal(plan, e)
		}
		source, e := os.ReadFile(filepath.Join(root, "pages", page, "data", boxID, docID+".docj"))
		if e != nil || plan.SourceHashes[docID] != sha256.Sum256(source) {
			t.Fatal("candidate source hash mismatch", e)
		}
		word := "Synthetic knowledge"
		if page == pageB {
			word = "Workspace B knowledge"
		}
		if !strings.Contains(plan.Hints[blockID], word) {
			t.Fatal(plan.Hints)
		}
		if !strings.Contains(plan.Changes[0].Text, word) {
			t.Fatal(plan.Changes)
		}
		refs, e := l.NativeRefsCacheByDefinition(ctx, blockID)
		if e != nil || len(refs) != 1 || refs[0].Content != "fixture paragraph" {
			t.Fatal("candidate published derived ref changes", refs, e)
		}
		plan.Hints[blockID] = "outside"
		plan, e = l.PlanNativeDynamicAnchors(ctx, []string{blockID})
		if e != nil || plan.Hints[blockID] == "outside" {
			t.Fatal("plan aliases context", e)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("anchor planning changed source")
	}
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	raw = []byte(strings.Replace(string(raw), `"id": "`+blockID+`"`, `"id": "`+blockID+`", "custom-avs":"20261022090000-av00001"`, 1))
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	plan, e := l.PlanNativeDynamicAnchors(ctx, []string{blockID})
	if !errors.Is(e, anchor.ErrDependency) || len(plan.Trees) != 0 {
		t.Fatal("unbound AV source effect silently skipped", plan, e)
	}
}

func TestNativeSearchAndTagQuerySameIDsSourceDigestAndRequestIsolation(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		if e = l.QueueNativeTree(ctx, boxID, docID, "index"); e != nil {
			t.Fatal(e)
		}
		if e = l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		request := indexdb.BlockSearchRequest{Query: "knowledge", Page: 1, PageSize: 32}
		r, e := l.SearchNativeBlocks(ctx, request)
		if e != nil || r.MatchedBlocks != 1 {
			t.Fatal(r, e)
		}
		want := "Synthetic"
		if page == pageB {
			want = "Workspace B"
		}
		if !strings.Contains(r.Blocks[0].Content, want) {
			t.Fatal("search selected wrong owner", r)
		}
		if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
			db, e := b.Database(indexdb.Main)
			if e != nil {
				return e
			}
			_, e = db.Exec("INSERT INTO spans(id,block_id,root_id,box,path,content,markdown,type,ial) VALUES(?,?,?,?,?,?,?,?,?)", "same-span", blockID, docID, boxID, "/"+docID+".docj", page+" AGI 标签", page, "tag", "")
			return e
		}); e != nil {
			t.Fatal(e)
		}
		tags, e := l.SearchNativeTags(ctx, "AGI 标签")
		if e != nil || len(tags) != 1 || !strings.Contains(tags[0], page) || !strings.Contains(tags[0], "<mark>") {
			t.Fatal(tags, e)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("search created config/default files or changed source")
	}
}

func TestNativeSearchIgnoreUsesOwnerSourceAndRejectsWrites(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	dir := filepath.Join(root, "pages", pageA, "data", ".wiki")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, "searchignore")
	if e := os.WriteFile(file, []byte("1=1; DELETE FROM blocks; --"), 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	if e = l.QueueNativeTree(ctx, boxID, docID, "index"); e != nil {
		t.Fatal(e)
	}
	if e = l.FlushNativeMainQueue(ctx); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	r := indexdb.BlockSearchRequest{Query: "knowledge", Page: 1, PageSize: 32, Ignore: []string{"1=1"}}
	if _, e = l.SearchNativeBlocks(ctx, r); e == nil {
		t.Fatal("owner ignore write accepted")
	}
	if digest(t, root) != before {
		t.Fatal("failed search changed source")
	}
	if e = os.WriteFile(file, []byte("1=1"), 0600); e != nil {
		t.Fatal(e)
	}
	r.Ignore = []string{"1=0"}
	got, e := l.SearchNativeBlocks(ctx, r)
	if e != nil || got.MatchedBlocks != 1 {
		t.Fatal("caller overrode owner policy or prior query changed index", got, e)
	}
}

func TestNativeAVQueryOriginalRankingViewsAndConfinedSource(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const avID = "20261022090000-av00001"
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var data map[string]interface{}
		if e = json.Unmarshal(raw, &data); e != nil {
			t.Fatal(e)
		}
		n := data["Children"].([]interface{})[0].(map[string]interface{})
		n["Type"] = "NodeAttributeView"
		n["AttributeViewID"] = avID
		delete(n, "Children")
		raw, e = json.Marshal(data)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		doc := map[string]interface{}{"id": avID, "name": page + " AGI Planner", "spec": 0, "future": map[string]string{"keep": "unknown"}, "views": []map[string]string{{"id": "view1", "name": "Main", "type": "table"}}}
		raw, e = json.Marshal(doc)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, avID+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		raw, e = msgpack.Marshal(map[string][]string{avID: {blockID}})
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, "blocks.msgpack"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
			return []conf.Notebook{{ID: boxID, Name: page + " notebook"}}, nil
		}, nil, nil), nil
	}})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		r, e := l.SearchNativeAttributeViews(ctx, "agi plan", nil)
		if e != nil || len(r) != 1 || r[0].AvName != page+" AGI Planner" || r[0].BlockID != blockID || len(r[0].Children) != 1 || r[0].Children[0].ViewLayout != "table" || !strings.HasPrefix(r[0].HPath, page+" notebook/") {
			t.Fatal(r, e)
		}
		r[0].Children[0].ViewName = "outside"
		r, e = l.SearchNativeAttributeViews(ctx, "", []string{avID})
		if e != nil || len(r) != 0 {
			t.Fatal(r, e)
		}
		name, e := l.NativeAttributeViewName(ctx, avID)
		if e != nil || name != page+" AGI Planner" {
			t.Fatal(name, e)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("read-only AV search migrated old Spec or unknown fields")
	}
	file := filepath.Join(root, "pages", pageA, "data", "storage", "av", avID+".json")
	if e := os.Rename(file, file+".saved"); e != nil {
		t.Fatal(e)
	}
	foreign := filepath.Join(t.TempDir(), "foreign.json")
	if e := os.WriteFile(foreign, []byte(`{"name":"foreign"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(foreign, file); e != nil {
		t.Fatal(e)
	}
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	if _, e = l.SearchNativeAttributeViews(ctx, "", nil); !errors.Is(e, ErrDocument) {
		t.Fatal("AV symlink accepted", e)
	}
}
