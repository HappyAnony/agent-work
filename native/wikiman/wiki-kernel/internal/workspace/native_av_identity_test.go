package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativeAVIdentitiesOwnerIsolationReadOnlyAndSourceFailures(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261026150000-av00001"
	for _, page := range []string{pageA, pageB} {
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		doc := map[string]interface{}{"id": id, "spec": 0, "unknownExtension": page, "keyValues": []interface{}{map[string]interface{}{"key": map[string]string{"id": "primary", "type": "block"}, "values": []interface{}{map[string]interface{}{"blockID": page + "-first", "block": map[string]string{"id": blockID}}, map[string]interface{}{"blockID": page + "-last", "isDetached": true, "block": map[string]string{"id": blockID}}, map[string]interface{}{"blockID": page + "-empty", "isDetached": true, "block": map[string]string{}}}}}}
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, id+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := NewManager(Options{})
	store, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Drain(ctx); err != nil {
			t.Error(err)
		}
	})
	for _, page := range []string{pageA, pageB} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(ctx, store, page)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Release()
			file := filepath.Join(root, "pages", page, "data", "storage", "av", id+".json")
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			ret, err := l.NativeAttributeViewItemIDs(ctx, id, []string{blockID, "missing", ""})
			want := map[string]string{blockID: page + "-last", "missing": "", "": page + "-empty"}
			if err != nil || !reflect.DeepEqual(ret, want) {
				t.Fatal(ret, err)
			}
			ret, err = l.NativeAttributeViewBoundBlockIDs(ctx, id, []string{page + "-first", page + "-empty", "missing"})
			want = map[string]string{page + "-first": blockID, page + "-empty": "", "missing": ""}
			if err != nil || !reflect.DeepEqual(ret, want) {
				t.Fatal(ret, err)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if ret, err = l.NativeAttributeViewItemIDs(cancelled, id, nil); err == nil || ret != nil {
				t.Fatal("cancel ignored", ret, err)
			}
			if ret, err = l.NativeAttributeViewBoundBlockIDs(ctx, "../outside", nil); err == nil || ret != nil {
				t.Fatal("invalid path accepted")
			}
			after, err := os.ReadFile(file)
			if err != nil || string(before) != string(after) {
				t.Fatal("source changed")
			}
			if err = os.WriteFile(file, []byte(`{"id":"wrong"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if ret, err = l.NativeAttributeViewItemIDs(ctx, id, nil); err == nil || ret != nil {
				t.Fatal("wrong source ID accepted", ret, err)
			}
			if err = os.WriteFile(file, []byte(`{broken`), 0600); err != nil {
				t.Fatal(err)
			}
			if ret, err = l.NativeAttributeViewBoundBlockIDs(ctx, id, nil); err == nil || ret != nil {
				t.Fatal("bad JSON accepted")
			}
			if err = os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if ret, err = l.NativeAttributeViewItemIDs(ctx, id, nil); err == nil || ret != nil {
				t.Fatal("missing source accepted")
			}
		})
	}
}
