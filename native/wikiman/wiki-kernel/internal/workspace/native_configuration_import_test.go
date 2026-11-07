//go:build fts5

package workspace

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
)

func TestNativeConfigurationImportUsesActualOwnerAndNoPublication(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		v := conf.AppData{Search: conf.NewSearch(), System: conf.NewSystemForDevice(page, "device", "version"), UserData: "private-" + page}
		if page == pageA {
			v.Search.Limit = 51
		} else {
			v.Search.Limit = 52
		}
		return v, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for l, page := range map[*Lease]string{a: pageA, b: pageB} {
		v, e := l.PlanNativeConfigurationImport(context.Background(), []byte(`{"search":{"limit":77},"system":{"id":"foreign","dataDir":"/foreign","networkProxy":{}},"userData":"foreign"}`))
		if e != nil || v.Setting.Data.UserData != "private-"+page || v.Setting.Data.System.ID != page || v.Setting.Data.System.DataDir == "/foreign" || v.Patches[conf.WorkspacePreference].Set["search.limit"] == nil {
			t.Fatal(v, e)
		}
		actual, e := l.Configuration(context.Background())
		if e != nil || actual.Search.Limit == 77 {
			t.Fatal("candidate published", actual, e)
		}
		for _, patch := range v.Patches {
			if patch.Set["userData"] != nil || patch.Set["system.dataDir"] != nil {
				t.Fatal("foreign authority patch")
			}
		}
	}
	if digest(t, root) != before {
		t.Fatal("import preview wrote source")
	}
	a.Release()
	if _, e := a.PlanNativeConfigurationImport(context.Background(), []byte(`{"search":{}}`)); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestNativeNotebookRepairCandidatesRetainPerOwnerPreimages(t *testing.T) {
	root := fixture(t)
	extra := "20261027095000-miss001"
	for _, page := range []string{pageA, pageB} {
		if e := os.Mkdir(filepath.Join(root, "pages", page, "data", extra), 0700); e != nil {
			t.Fatal(e)
		}
	}
	brokenFile := filepath.Join(root, "pages", pageB, "data", boxID, ".wiki/conf.json")
	raw := []byte("broken-original-metadata")
	if e := os.WriteFile(brokenFile, raw, 0600); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	m := open(t, root, Options{})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	pa, e := a.PlanNativeNotebookRepairs(context.Background())
	if e != nil || len(pa.Repairs) != 1 || !pa.Repairs[0].Missing || pa.Repairs[0].Path != extra+"/.wiki/conf.json" || len(pa.Notebooks) != 2 {
		t.Fatal(pa, e)
	}
	pb, e := b.PlanNativeNotebookRepairs(context.Background())
	if e != nil || len(pb.Repairs) != 2 || len(pb.Notebooks) != 1 {
		t.Fatal(pb, e)
	}
	matched := false
	for _, r := range pb.Repairs {
		if r.ID == boxID {
			matched = true
			if string(r.Before) != string(raw) || r.Hash != sha256.Sum256(raw) || r.Missing {
				t.Fatal(r)
			}
		}
	}
	if !matched {
		t.Fatal("missing corrupt preimage")
	}
	if digest(t, root) != before {
		t.Fatal("scan repaired source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = a.PlanNativeNotebookRepairs(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	a.Release()
	b.Release()
	if e = m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
}
