//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func TestNativeHistoricalAVOwnerIsolationAndNoPublication(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	id := "20261029150000-deepav1"
	for _, page := range []string{pageA, pageB} {
		writeLayoutFixture(t, root, page, layoutFixture(id, page, av.CoverFromNone))
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
	stamp := time.Now().Unix()
	created := strconv.FormatInt(stamp, 10)
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		defer l.Release()
		a := layoutFixture(id, "Historical "+page, av.CoverFromNone)
		name := filepath.Join(l.workspace.paths.RuntimeDir, "history", time.Unix(stamp, 0).Format("2006-01-02-150405")+"-update", "storage", "av", id+".json")
		if e = os.MkdirAll(filepath.Dir(name), 0700); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(a)
		if e = os.WriteFile(name, raw, 0600); e != nil {
			t.Fatal(e)
		}
		q := NativeHistoricalAVRequest{Mode: "history", Created: created, AVID: id, ViewID: "gallery", Page: 1, PageSize: -1}
		out, e := l.NativeHistoricalAttributeView(ctx, q)
		var v struct {
			Name  string
			Cards []json.RawMessage
		}
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(out, &v); e != nil || v.Name != "Historical "+page || len(v.Cards) != 2 {
			t.Fatal(string(out), e)
		}
		if e = os.Remove(name); e != nil {
			t.Fatal(e)
		}
		out, e = l.NativeHistoricalAttributeView(ctx, q)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(out, &v); e != nil || v.Name != page {
			t.Fatal(string(out), e)
		}
		if e = os.WriteFile(name, []byte("broken"), 0600); e != nil {
			t.Fatal(e)
		}
		out, e = l.NativeHistoricalAttributeView(ctx, q)
		if e == nil || len(out) > 0 {
			t.Fatal("corruption hidden", string(out), e)
		}
		q.Created = "0"
		out, e = l.NativeHistoricalAttributeView(ctx, q)
		if e != nil || len(out) > 0 {
			t.Fatal("unselected prefix", string(out), e)
		}
		q.Created = "invalid"
		if _, e = l.NativeHistoricalAttributeView(ctx, q); e == nil {
			t.Fatal("invalid timestamp")
		}
	}
	if digest(t, root) != before {
		t.Fatal("historical preview published source")
	}
}
