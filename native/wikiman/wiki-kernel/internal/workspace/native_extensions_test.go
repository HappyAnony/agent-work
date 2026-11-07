//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/extensionprefs"
)

func TestNativeExtensionsActualTwoOwnersAndSourcePlans(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		dir := filepath.Join(root, "pages", page, "data", "plugins", "demo")
		if e := os.MkdirAll(filepath.Join(dir, "i18n"), 0700); e != nil {
			t.Fatal(e)
		}
		for n, raw := range map[string]string{"plugin.json": `{"name":"demo","backends":["all"],"frontends":["desktop"]}`, "index.js": "JS " + page, "i18n/en_US.json": `{"page":"` + page + `"}`} {
			if e := os.WriteFile(filepath.Join(dir, n), []byte(raw), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(_, _ string) (conf.AppData, error) {
		v := conf.NewBazaar()
		v.Trust = true
		return conf.AppData{Bazaar: v, Lang: "en_US"}, nil
	}})
	for _, l := range []*Lease{getDocLease(t, m, pageA), getDocLease(t, m, pageB)} {
		p, e := l.PlanNativeExtensions(context.Background(), extensionprefs.Request{Action: "petal.set", Name: "demo", Enabled: true, Frontend: "desktop"})
		if e != nil {
			t.Fatal(e)
		}
		var petal extensionprefs.Petal
		if e = json.Unmarshal(p.Value, &petal); e != nil || petal.JS != "JS "+l.PageID() || petal.I18n["page"] != l.PageID() {
			t.Fatal(petal, e)
		}
		if len(p.SourceWrites["storage/petal/petals.json"]) == 0 {
			t.Fatal("missing source intent")
		}
		p, e = l.PlanNativeExtensions(context.Background(), extensionprefs.Request{Action: "snippet.set", Value: []byte(`[{"name":"candidate","type":"js"}]`)})
		if e != nil || len(p.SourceWrites["snippets/conf.json"]) == 0 {
			t.Fatal(p, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("extension plan modified source")
	}
}
