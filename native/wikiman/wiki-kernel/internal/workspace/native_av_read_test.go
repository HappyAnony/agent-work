//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avquery"
)

func TestNativeAVPrimaryViewSourceIsolationUnknownFieldsAndFailures(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const avID = "20261022090000-av00001"
	const missing = "20261022090009-gone001"
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.Replace(string(raw), `"id": "`+blockID+`"`, `"id": "`+blockID+`", "custom-sy-av-view":"first"`, 1))
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		doc := map[string]any{"id": avID, "name": "  " + page + "  ", "spec": 0, "futureRoot": true, "viewID": "second", "views": []any{map[string]any{"id": "first", "name": page, "futureView": true}, map[string]any{"id": "second", "filters": []any{map[string]any{"column": "key", "operator": "contains", "futureFilter": 7}}, "sorts": []any{map[string]any{"column": "key", "order": "ASC", "futureSort": 8}}}}, "keyValues": []any{map[string]any{"key": map[string]any{"id": "key", "type": "block", "futureKey": true}, "futureKV": 42, "values": []any{map[string]any{"id": "bound", "type": "block", "block": map[string]any{"id": blockID, "content": page + " Alpha", "updated": 1}, "futureValue": true}, map[string]any{"id": "detached", "type": "block", "isDetached": true, "block": map[string]any{"content": " " + page + " ALPHA ", "updated": 9}}, map[string]any{"id": "missing", "type": "block", "block": map[string]any{"id": missing, "content": "Alpha", "updated": 999}}}}}}
		raw, e = json.Marshal(doc)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, avID+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		raw, e = msgpack.Marshal(map[string][]string{avID: {docID, blockID, missing, blockID}})
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, "blocks.msgpack"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(nil, map[string]map[int]string{"en_US": {105: page + " fallback"}}, nil), nil
	}})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		primary, e := l.NativeAttributeViewPrimaryValues(ctx, avID, "alpha", 1, 1)
		if e != nil || primary.Name != page || len(primary.DatabaseBlockIDs) != 2 || !strings.Contains(string(primary.KeyValues), "detached") || strings.Contains(string(primary.KeyValues), `"missing"`) {
			t.Fatal(primary, e)
		}
		primary, e = l.NativeAttributeViewPrimaryValues(ctx, avID, "alpha", 2, 1)
		if e != nil || !strings.Contains(string(primary.KeyValues), "futureValue") || !strings.Contains(string(primary.KeyValues), page) || !strings.Contains(string(primary.KeyValues), "futureKV") {
			t.Fatal(primary, e)
		}
		primary, e = l.NativeAttributeViewPrimaryValues(ctx, avID, "alpha", 100, 1)
		if e != nil || strings.Contains(string(primary.KeyValues), `"values"`) {
			t.Fatal(primary, e)
		}
		view, e := l.NativeAttributeViewView(ctx, avID, blockID)
		if e != nil || !strings.Contains(string(view.View), page) || len(view.Filters) != 0 || view.Filters == nil || view.Sorts == nil {
			t.Fatal(view, e)
		}
		view, e = l.NativeAttributeViewView(ctx, avID, missing)
		if e != nil || len(view.Filters) != 1 || len(view.Sorts) != 1 || !strings.Contains(string(view.Filters[0]), "futureFilter") {
			t.Fatal(view, e)
		}
		view.Filters[0][0] = 'x'
		again, e := l.NativeAttributeViewView(ctx, avID, "")
		if e != nil || again.Filters[0][0] != '{' {
			t.Fatal("view result aliases owner", again, e)
		}
		raw, e := l.ReadNativeAttributeView(ctx, avID)
		if e != nil || !strings.Contains(string(raw), "futureRoot") || !strings.Contains(string(raw), `"spec":0`) {
			t.Fatal(string(raw), e)
		}
		raw[0] = 'x'
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if out, e := l.NativeAttributeViewPrimaryValues(canceled, avID, "", 1, 1); !errors.Is(e, context.Canceled) || out.KeyValues != nil {
			t.Fatal(out, e)
		}
		if out, e := l.NativeAttributeViewPrimaryValues(ctx, avID, "", 0, 1); !errors.Is(e, avquery.ErrPrimaryPage) || out.KeyValues != nil {
			t.Fatal(out, e)
		}
		if _, e = l.ReadNativeAttributeView(ctx, "../escape"); e == nil {
			t.Fatal("source escape allowed")
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("AV read repaired/modified source")
	}
}
