//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/pageprefs"
)

func TestNativePreferencesActualOwnerPreviewAndRecentSharedCore(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		p, e := l.PlanNativeStorage(context.Background(), pageprefs.Request{Action: "criteria.set", Value: []byte(`{"name":"saved","k":"owned"}`)})
		if e != nil || len(p.SourceWrites["storage/criteria.json"]) == 0 {
			t.Fatal(p, e)
		}
		p, e = l.PlanNativeStorage(context.Background(), pageprefs.Request{Action: "local.set", Value: []byte(`{"owner":"` + l.PageID() + `"}`)})
		if e != nil {
			t.Fatal(e)
		}
		if _, ok := p.LocalWrites[nativePreferencePath(l.workspace, "local.json")]; !ok {
			t.Fatal("wrong local owner")
		}
		p, e = l.PlanNativeStorage(context.Background(), pageprefs.Request{Action: "recent.record", Value: []byte(`{"rootID":"20261019090001-doc0001","title":"owned"}`)})
		if e != nil {
			t.Fatal(e)
		}
		var recent nativeRecentDocument
		if e = json.Unmarshal(p.LocalWrites[recentPath(l.workspace)], &recent); e != nil || recent.Version != 1 || len(recent.Docs) != 1 {
			t.Fatal(recent, e)
		}
		for name := range p.LocalWrites {
			if _, e := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(e) {
				t.Fatal("preview created private state", name, e)
			}
		}
	}
	if digest(t, root) != before {
		t.Fatal("preview wrote source or local state")
	}
}
