//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/avquery"
)

func TestNativeAVReadFailuresPreserveCorruptSource(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261022090000-av00001"
	dir := filepath.Join(root, "pages", pageA, "data", "storage", "av")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, id+".json")
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	for _, test := range []struct {
		Name, Payload string
		View          bool
	}{{"no views", `{"name":"ok","views":[]}`, true}, {"null view", `{"name":"ok","views":[null]}`, true}, {"null key", `{"name":"ok","keyValues":[{"key":null}]}`, false}, {"missing block", `{"name":"ok","keyValues":[{"key":{"type":"block"},"values":[{"type":"block"}]}]}`, false}, {"wrong value", `{"name":"ok","keyValues":[{"key":{"type":"block"},"values":[{"type":"text","block":{"content":"x"}}]}]}`, false}, {"unbound fallback", `{"name":" ","keyValues":[{"key":{"type":"block"}}]}`, false}, {"invalid JSON", `{"broken":`, false}} {
		t.Run(test.Name, func(t *testing.T) {
			if e = os.WriteFile(file, []byte(test.Payload), 0600); e != nil {
				t.Fatal(e)
			}
			before := digest(t, root)
			if test.View {
				out, e := l.NativeAttributeViewView(ctx, id, "")
				if e == nil || out.View != nil {
					t.Fatal(out, e)
				}
				if test.Name == "no views" && !errors.Is(e, avquery.ErrViewNotFound) {
					t.Fatal(e)
				}
			} else {
				out, e := l.NativeAttributeViewPrimaryValues(ctx, id, "", 1, 16)
				if e == nil || out.KeyValues != nil {
					t.Fatal(out, e)
				}
			}
			if digest(t, root) != before {
				t.Fatal("failed read repaired source")
			}
		})
	}
	raw, _ := json.Marshal(map[string]any{"id": id, "name": "source"})
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(file, file+".original"); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(id+".json.original", file); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	if out, e := l.ReadNativeAttributeView(ctx, id); e == nil || out != nil {
		t.Fatal("symlink AV accepted", out, e)
	}
	if digest(t, root) != before {
		t.Fatal("symlink source changed")
	}
}
