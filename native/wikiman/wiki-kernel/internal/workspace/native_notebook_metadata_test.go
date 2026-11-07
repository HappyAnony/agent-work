//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/notebooks"
)

func TestNativeNotebookMetadataOwnCandidatesWithoutPublication(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		name := filepath.Join(root, "pages", page, "data", boxID, ".wiki/conf.json")
		raw, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		var v conf.BoxConf
		if e = json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		v.Name = page
		raw, _ = json.Marshal(v)
		if e = os.WriteFile(name, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	opts := lifecycleOptions(root)
	provider := opts.ConfigurationBusiness
	opts.ConfigurationBusiness = func(store, page string) (*conf.Business, error) {
		base, e := provider(store, page)
		if e != nil {
			return nil, e
		}
		configuration, e := opts.Configuration(store, page)
		if e != nil {
			return nil, e
		}
		labels := map[string]string{}
		for _, key := range []string{"now", "1s", "xs", "1m", "xm", "1h", "xh", "1d", "xd", "1w", "xw", "1M", "xM", "1y", "2y", "xy", "max", "albl", "blbl"} {
			labels[key] = key
		}
		return conf.NewBusiness(base.Notebooks, nil, nil, map[string]map[string]string{configuration.Lang: labels}), nil
	}
	m := open(t, root, opts)
	ctx := context.Background()
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		defer l.Release()
		value, e := l.NativeNotebookConfiguration(ctx, boxID)
		if e != nil || value.Name != page {
			t.Fatal(value, e)
		}
		info, e := l.NativeNotebookInfo(ctx, boxID)
		if e != nil || info.Name != page || info.DocCount != 1 || info.Size == 0 {
			t.Fatal(info, e)
		}
		plan, e := l.PlanNativeNotebookMetadata(ctx, notebooks.MetadataRequest{Kind: "configuration", ID: boxID, Patch: json.RawMessage(`{"dailyNoteSavePath":" next / day "}`)})
		if e != nil || plan.Configuration == nil || plan.Configuration.Name != page || plan.Configuration.DailyNoteSavePath != "/next/day" || len(plan.Changes) != 1 {
			t.Fatal(plan, e)
		}
		created, e := l.PlanNativeNotebookCreate(ctx, "new candidate", "untitled")
		if e != nil || created.CreatedID == "" || created.Configuration.Name != "new candidate" || len(created.Changes) != 2 {
			t.Fatal(created, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("metadata candidate published source")
	}
}
