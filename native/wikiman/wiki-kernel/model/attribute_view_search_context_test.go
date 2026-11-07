//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/conf"
	"wiki-kernel/filesys"
	"wiki-kernel/util"
)

func TestAttributeViewSearchActualSourceMirrorAndFailure(t *testing.T) {
	c, r, _, file, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	id, block, viewID := "20261105100000-views01", "20261105100001-block01", "20261105100002-view001"
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var document map[string]interface{}
	json.Unmarshal(raw, &document)
	document["Children"] = append(document["Children"].([]interface{}), map[string]interface{}{"ID": block, "Type": "NodeAttributeView", "AttributeViewID": id, "AttributeViewType": "table", "Properties": map[string]string{"id": block}})
	raw, _ = json.Marshal(document)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	candidate := &av.AttributeView{ID: id, Spec: 4, Name: "Owned AV", ViewID: viewID, Views: []*av.View{{ID: viewID, Name: "Owner table", LayoutType: av.LayoutTypeTable}}}
	source, _ := json.Marshal(candidate)
	name := archiveWrite(t, util.DataDir, "storage/av/"+id+".json", source)
	relations, _ := msgpack.Marshal(map[string][]string{id: {"20261105100003-missing", block}})
	archiveWrite(t, util.DataDir, "storage/av/blocks.msgpack", relations)
	box := filepath.Base(filepath.Dir(file))
	tree, e := filesys.LoadTree(box, "/"+filepath.Base(file), util.NewLute())
	if e != nil {
		t.Fatal(e)
	}
	if e = r.registry.Upsert(context.Background(), tree); e != nil {
		t.Fatal(e)
	}
	if e = r.queue.Upsert(context.Background(), tree); e != nil {
		t.Fatal(e)
	}
	if e = r.queue.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(file)
	c.persistenceReadOnly = true
	out, e := SearchAttributeViewContext(context.Background(), "owned", nil)
	if e != nil || len(out) != 1 || out[0].AvID != id || out[0].BlockID != block || len(out[0].Children) != 1 || out[0].Children[0].ViewID != viewID {
		t.Fatal(out, e)
	}
	out, e = SearchAttributeViewContext(context.Background(), "", []string{id})
	if e != nil || len(out) != 0 {
		t.Fatal(out, e)
	}
	after, _ := os.ReadFile(name)
	docAfter, _ := os.ReadFile(file)
	if !bytes.Equal(after, source) || !bytes.Equal(docAfter, before) {
		t.Fatal("read-only search changed source")
	}
	if e = os.WriteFile(name, []byte("malformed"), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e = SearchAttributeViewContext(context.Background(), "owned", nil); e == nil || out != nil {
		t.Fatal("selected malformed AV ignored", out, e)
	}
}
