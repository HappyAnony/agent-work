//go:build fts5

package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wiki-kernel/internal/sourcetx"
)

func TestR05DeletionPreservesExactSubtreeAndRestoresWithCAS(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		v, e := Content(ctx, m, r)
		if e != nil {
			t.Fatal(r.Action, e)
		}
		return v
	}
	put := baseContent(id, "document.put")
	put.NotebookName, put.OperationID, put.AST = "Restorable", "r05-parent", documentAST(newDoc, newBlock, "Root", "Root text")
	call(put)
	child := baseContent(id, "document.put")
	child.DocID, child.ParentDocID, child.OperationID = "20261019100003-doc0003", newDoc, "r05-child"
	child.AST = documentAST(child.DocID, "20261019100004-par0004", "Child", "Child text")
	call(child)
	data := filepath.Join(root, "pages", emptyPage, "data", box)
	parentFile := filepath.Join(data, newDoc+".docj")
	childFile := filepath.Join(data, newDoc, child.DocID+".docj")
	before, _ := os.ReadFile(parentFile)
	beforeChild, _ := os.ReadFile(childFile)
	assetFile := filepath.Join(data, newDoc, "assets", "local.txt")
	if e := os.MkdirAll(filepath.Dir(assetFile), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(assetFile, []byte("local content"), 0600); e != nil {
		t.Fatal(e)
	}
	del := baseContent(id, "document.delete")
	del.OperationID, del.ExpectedHash, del.Cascade = "r05-delete", sourcetx.Hash(before), true
	preview := del
	preview.DryRun = true
	if v := call(preview); v.Receipt.SourceCommitted {
		t.Fatal(v)
	}
	if v := call(del); !v.Receipt.SourceCommitted || !v.OriginalIndexFresh {
		t.Fatal(v)
	}
	for _, f := range []string{parentFile, childFile, assetFile} {
		if _, e := os.Stat(f); !errors.Is(e, os.ErrNotExist) {
			t.Fatal(f, e)
		}
	}
	shared := filepath.Join(root, "pages", emptyPage, "data", "assets", "local.txt")
	if v, e := os.ReadFile(shared); e != nil || string(v) != "local content" {
		t.Fatal(string(v), e)
	}
	list := baseContent(id, "document.deleted")
	list.DocID = ""
	deleted := call(list)
	if len(deleted.Tombstones) != 1 || len(deleted.Tombstones[0].Files) != 3 {
		t.Fatal(deleted)
	}
	tomb := deleted.Tombstones[0]
	if v, e := os.ReadFile(filepath.Join(root, tomb.ArchivePath(box+"/"+newDoc+".docj"))); e != nil || !bytes.Equal(v, before) {
		t.Fatal(string(v), e)
	}
	restore := baseContent(id, "document.recover")
	restore.OperationID, restore.ExpectedHash = "r05-recover", deleted.Items[0].Hash
	conflict := baseContent(id, "document.put")
	conflict.DocID, conflict.OperationID = "20261019100005-doc0004", "r05-conflicting-block"
	conflict.AST = documentAST(conflict.DocID, newBlock, "Conflicting", "Block")
	call(conflict)
	if _, e := Content(ctx, m, restore); !errors.Is(e, sourcetx.ErrConflict) {
		t.Fatal(e)
	}
	if e := os.Remove(filepath.Join(data, conflict.DocID+".docj")); e != nil {
		t.Fatal(e)
	}
	recovered := call(restore)
	if retry := call(restore); !reflect.DeepEqual(recovered.Receipt, retry.Receipt) {
		t.Fatal(retry)
	}
	if v, _ := os.ReadFile(parentFile); !bytes.Equal(v, before) {
		t.Fatal("root bytes changed")
	}
	if v, _ := os.ReadFile(childFile); !bytes.Equal(v, beforeChild) {
		t.Fatal("child bytes changed")
	}
	if v, _ := os.ReadFile(assetFile); string(v) != "local content" {
		t.Fatal("asset bytes changed")
	}
	if v := call(list); v.Total != 0 {
		t.Fatal(v)
	}
}

func TestR05DocxRenameAndVersionRestoreUseOneCAS(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	page := pageGet(t, m, id, testPage).Page
	q := ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, BoxID: page.EntryDocument.BoxID, DocID: page.EntryDocument.DocID, Action: "document.get"}
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		v, e := Content(ctx, m, r)
		if e != nil {
			t.Fatal(r.Action, e)
		}
		return v
	}
	before := call(q)
	meta := filepath.Join(root, "pages", testPage, "page.json")
	raw, _ := os.ReadFile(meta)
	rename := q
	rename.Action, rename.OperationID, rename.ExpectedHash, rename.Title = "document.rename", "r05-docx-rename", before.Hash, "New / title"
	if _, e := Content(ctx, m, rename); !errors.Is(e, sourcetx.ErrConflict) {
		t.Fatal("missing metadata CAS", e)
	}
	rename.ExpectedPageHash = sourcetx.Hash(raw)
	renamed := call(rename)
	after := call(q)
	if renamed.Hash != after.Hash || !strings.Contains(string(after.AST), "New  title") {
		t.Fatal(renamed, string(after.AST))
	}
	current := pageGet(t, m, id, testPage).Page
	if current.Name != "New  title" || current.Revision != page.Revision+1 {
		t.Fatal(current)
	}
	get := q
	get.Action, get.VersionID = "document.version.get", rename.OperationID
	if v := call(get); !bytes.Equal(v.AST, before.AST) {
		t.Fatal("version lost source bytes")
	}
	restore := q
	raw, _ = os.ReadFile(meta)
	restore.Action, restore.VersionID, restore.OperationID, restore.ExpectedHash, restore.ExpectedPageHash = "document.version.apply", rename.OperationID, "r05-version-apply", after.Hash, sourcetx.Hash(raw)
	call(restore)
	if restored := call(q); !bytes.Equal(restored.AST, before.AST) {
		t.Fatal("restore did not preserve bytes")
	}
	current = pageGet(t, m, id, testPage).Page
	if current.Name != page.Name || current.Revision != page.Revision+2 {
		t.Fatal(current)
	}
}

func TestR05OriginalCreateDuplicateSortAndAssetRename(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		v, e := Content(ctx, m, r)
		if e != nil {
			t.Fatal(r.Action, e)
		}
		return v
	}
	seed := baseContent(id, "document.put")
	seed.NotebookName, seed.OperationID, seed.AST = "Original", "r05-create-seed", documentAST(newDoc, newBlock, "Seed", "Seed")
	call(seed)
	create := baseContent(id, "document.create")
	create.DocID, create.OperationID, create.Title, create.Markdown = "20261019100003-doc0003", "r05-create-markdown", "Markdown title", "**Native** [file](assets/synthetic.txt)"
	first := call(create)
	if first.DocID != create.DocID || first.Hash == "" {
		t.Fatal(first)
	}
	copy := baseContent(id, "document.create")
	copy.CreationMode, copy.OperationID = "duplicate", "r05-create-copy"
	copied := call(copy)
	if copied.DocID == copy.DocID || copied.DocID == "" {
		t.Fatal(copied)
	}
	get := baseContent(id, "document.get")
	get.DocID = copied.DocID
	v := call(get)
	if !bytes.Contains(v.AST, []byte("custom")) {
		t.Fatal("duplicate lost unknown property", string(v.AST))
	}
	order := baseContent(id, "document.sort")
	order.DocID, order.OperationID, order.Order = "", "r05-sort", []string{copied.DocID, create.DocID, newDoc}
	if raw, e := os.ReadFile(filepath.Join(root, "pages", emptyPage, "data", box, ".wiki", "sort.json")); e == nil {
		order.ExpectedHash = sourcetx.Hash(raw)
	}
	call(order)
	put := ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: emptyPage, Action: "asset.put", AssetName: "synthetic.txt", OperationID: "r05-asset-put", Data: []byte("file contents")}
	call(put)
	asset := put
	asset.Action, asset.OperationID, asset.ExpectedHash, asset.Data = "asset.delete", "r05-asset-delete", sourcetx.Hash(put.Data), nil
	if _, e := Content(ctx, m, asset); !errors.Is(e, sourcetx.ErrConflict) {
		t.Fatal(e)
	}
	asset.Action, asset.OperationID, asset.NewAssetName = "asset.rename", "r05-asset-rename", "renamed"
	renamed := call(asset)
	if !strings.HasPrefix(renamed.AssetName, "renamed") || renamed.Hash != sourcetx.Hash(put.Data) {
		t.Fatal(renamed)
	}
	get.DocID = create.DocID
	if v := call(get); !bytes.Contains(v.AST, []byte(renamed.AssetName)) {
		t.Fatal("reference not updated", string(v.AST))
	}
}

func TestR10CreateDocReferenceCommitsBothSourcesOrNeither(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	put := baseContent(id, "document.put")
	put.NotebookName, put.OperationID, put.AST = "Original", "r10-reference-seed", documentAST(newDoc, newBlock, "Parent", "Before")
	if _, err := Content(ctx, m, put); err != nil {
		t.Fatal(err)
	}
	const child = "20261106193001-r10doc1"
	dir := filepath.Join(root, "pages", emptyPage, "data", box)
	parentFile := filepath.Join(dir, newDoc+".docj")
	childFile := filepath.Join(dir, newDoc, child+".docj")
	before, err := os.ReadFile(parentFile)
	if err != nil {
		t.Fatal(err)
	}
	block := `<div data-type="NodeParagraph" data-node-id="` + newBlock + `"><div contenteditable="true">`
	data := map[string]string{"boxId": box, "docId": newDoc, "blockId": newBlock, "expectedHash": sourcetx.Hash(before),
		"notebook": box, "parentId": newDoc, "id": child, "title": "Child", "blockDOM": block + "Unrelated</div></div>"}
	request := func() UIRequest {
		t.Helper()
		payload, _ := json.Marshal(data)
		wire, _ := json.Marshal(UIRequest{ProtocolVersion: 1, StoreID: id, PageID: emptyPage, Endpoint: uiCreateReferenceEndpoint,
			OperationID: "r10-create-reference", Data: payload})
		r, e := DecodeUIRequest(bytes.NewReader(wire))
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	view := UIView{Mode: "wikiDesktop"}
	if _, e := UI(ctx, m, request(), view); !errors.Is(e, ErrArgument) {
		t.Fatal("accepted an edit without the new reference", e)
	}
	data["blockDOM"] = block + `Before <span data-type="block-ref" data-subtype="d" data-id="` + child + `">Child</span></div></div>`
	data["expectedHash"] = strings.Repeat("0", 64)
	if _, e := UI(ctx, m, request(), view); !errors.Is(e, sourcetx.ErrConflict) {
		t.Fatal("accepted a stale parent hash", e)
	}
	if after, e := os.ReadFile(parentFile); e != nil || !bytes.Equal(before, after) {
		t.Fatal("failed attempt changed the parent", e)
	}
	if _, e := os.Stat(childFile); !os.IsNotExist(e) {
		t.Fatal("failed attempt published the child", e)
	}
	data["expectedHash"] = sourcetx.Hash(before)
	r := request()
	for range 2 {
		if result, e := UI(ctx, m, r, view); e != nil || result.(map[string]any)["id"] != child {
			t.Fatal(result, e)
		}
	}
	if after, e := os.ReadFile(parentFile); e != nil || bytes.Count(after, []byte(child)) != 1 {
		t.Fatal("parent reference missing or duplicated on retry", e, string(after))
	}
	if raw, e := os.ReadFile(childFile); e != nil || !bytes.Contains(raw, []byte(child)) {
		t.Fatal("child source missing", e)
	}
}

func TestR05HistoryReadsOriginalRuntimeAndRestoresOwnedBytes(t *testing.T) {
	_, m, id := setupPages(t)
	ctx := context.Background()
	put := baseContent(id, "document.put")
	put.NotebookName, put.OperationID, put.AST = "History", "r05-history-seed", documentAST(newDoc, newBlock, "Current", "Present")
	if _, e := Content(ctx, m, put); e != nil {
		t.Fatal(e)
	}
	l, e := m.Acquire(ctx, id, emptyPage)
	if e != nil {
		t.Fatal(e)
	}
	paths, e := l.RuntimePaths(ctx)
	l.Release()
	if e != nil {
		t.Fatal(e)
	}
	name := "2026-11-05-010203-update/" + box + "/" + newDoc + ".docj"
	file := filepath.Join(paths.RuntimeDir, "history", name)
	previous := documentAST(newDoc, newBlock, "Older", "Historical bytes")
	if e = os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, previous, 0600); e != nil {
		t.Fatal(e)
	}
	list := baseContent(id, "document.version.list")
	rows, e := Content(ctx, m, list)
	if e != nil || len(rows.Items) != 1 || rows.Items[0].LegacyHistoryPath != name || rows.Items[0].Hash != sourcetx.Hash(previous) {
		t.Fatal(rows, e)
	}
	get := baseContent(id, "document.version.get")
	get.LegacyHistoryPath = name
	value, e := Content(ctx, m, get)
	if e != nil || !bytes.Equal(value.AST, previous) {
		t.Fatal(value, e)
	}
	get.LegacyHistoryPath = "2026-11-05-010203-update/20261019090000-box0002/" + newDoc + ".docj"
	if _, e = Content(ctx, m, get); e == nil {
		t.Fatal("history path escaped box")
	}
	apply := baseContent(id, "document.version.apply")
	apply.OperationID, apply.LegacyHistoryPath, apply.VersionHash, apply.ExpectedHash = "r05-apply-legacy", name, sourcetx.Hash(previous), sourcetx.Hash(put.AST)
	if v, e := Content(ctx, m, apply); e != nil || !v.OriginalIndexFresh || v.Hash != sourcetx.Hash(previous) {
		t.Fatal(v, e)
	}
	if v, e := Content(ctx, m, baseContent(id, "document.get")); e != nil || !bytes.Equal(v.AST, previous) {
		t.Fatal(v, e)
	}
}

func TestR05DeleteAndVersionRestoreAttributeViewEffects(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	const avID = "20261105090001-attr001"
	const mirror = "20261105090002-mirr001"
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		v, e := Content(ctx, m, r)
		if e != nil {
			t.Fatal(r.Action, e)
		}
		return v
	}
	put := baseContent(id, "document.put")
	put.NotebookName, put.OperationID = "Database", "r05-av-seed"
	put.AST = []byte(`{"ID":"` + newDoc + `","Type":"NodeDocument","Spec":"1","Properties":{"title":"Database"},"Children":[{"ID":"` + mirror + `","Type":"NodeAttributeView","AttributeViewID":"` + avID + `","AttributeViewType":"table","Properties":{"id":"` + mirror + `"}}]}`)
	call(put)
	dir := filepath.Join(root, "pages", emptyPage, "data", "storage", "av")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	view := filepath.Join(dir, avID+".json")
	before := []byte(`{"spec":4,"id":"` + avID + `","name":"Original","viewID":"view","keyValues":[{"key":{"id":"primary","name":"Name","type":"block"}}],"views":[{"id":"view","name":"Table","type":"table","table":{"id":"table-layout","columns":[{"id":"primary"}]}}]}`)
	if e := os.WriteFile(view, before, 0600); e != nil {
		t.Fatal(e)
	}
	registry, e := msgpack.Marshal(map[string][]string{avID: {mirror}})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "blocks.msgpack"), registry, 0600); e != nil {
		t.Fatal(e)
	}
	save := baseContent(id, "document.version.save")
	save.OperationID, save.ExpectedHash = "r05-db-version", sourcetx.Hash(put.AST)
	call(save)
	changed := bytes.ReplaceAll(before, []byte("Original"), []byte("Later"))
	if e = os.WriteFile(view, changed, 0600); e != nil {
		t.Fatal(e)
	}
	restore := baseContent(id, "document.version.apply")
	restore.OperationID, restore.VersionID, restore.ExpectedHash = "r05-db-restore", save.OperationID, save.ExpectedHash
	if _, e = Content(ctx, m, restore); !errors.Is(e, sourcetx.ErrConflict) {
		t.Fatal("AV overwrite without CAS", e)
	}
	restore.ExpectedAVHashes = map[string]string{avID: sourcetx.Hash(changed)}
	call(restore)
	if data, _ := os.ReadFile(view); !bytes.Equal(data, before) {
		t.Fatal("historical AV bytes were lost")
	}
	del := baseContent(id, "document.delete")
	del.OperationID, del.ExpectedHash = "r05-av-delete", sourcetx.Hash(put.AST)
	call(del)
	var state map[string][]string
	data, _ := os.ReadFile(filepath.Join(dir, "blocks.msgpack"))
	if e = msgpack.Unmarshal(data, &state); e != nil || len(state[avID]) != 0 {
		t.Fatal(state, e)
	}
	list := baseContent(id, "document.deleted")
	list.DocID = ""
	deleted := call(list)
	if len(deleted.Tombstones) != 1 || len(deleted.Tombstones[0].Effects) != 1 {
		t.Fatal(deleted)
	}
	recoverDoc := baseContent(id, "document.recover")
	recoverDoc.OperationID, recoverDoc.ExpectedHash = "r05-av-recover", deleted.Items[0].Hash
	call(recoverDoc)
	if data, _ = os.ReadFile(filepath.Join(dir, "blocks.msgpack")); !bytes.Equal(data, registry) {
		t.Fatal("AV registry preimage not recovered")
	}
}

func TestR06BatchDeleteSharedAttributeViewRecoversTogether(t *testing.T) {
	root, manager, store := setupPages(t)
	ctx := context.Background()
	const second = "20261105090013-doc0013"
	const avID = "20261105090010-attr010"
	mirrors := []string{"20261105090011-mirr011", "20261105090012-mirr012"}
	ids := []string{newDoc, second}
	var bodies [][]byte
	for i, id := range ids {
		put := baseContent(store, "document.put")
		put.DocID, put.OperationID = id, "r06-av-seed-"+id
		if i == 0 {
			put.NotebookName = "Shared database"
		}
		put.AST = []byte(`{"ID":"` + id + `","Type":"NodeDocument","Spec":"1","Properties":{"title":"Database mirror"},"Children":[{"ID":"` + mirrors[i] + `","Type":"NodeAttributeView","AttributeViewID":"` + avID + `","AttributeViewType":"table","Properties":{"id":"` + mirrors[i] + `"}}]}`)
		if _, err := Content(ctx, manager, put); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, put.AST)
	}
	data := filepath.Join(root, "pages", emptyPage, "data")
	dir := filepath.Join(data, "storage", "av")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	view := []byte(`{"spec":4,"id":"` + avID + `","name":"Shared","viewID":"view","keyValues":[{"key":{"id":"primary","name":"Name","type":"block"}}],"views":[{"id":"view","name":"Table","type":"table","table":{"id":"table-layout","columns":[{"id":"primary"}]}}]}`)
	if err := os.WriteFile(filepath.Join(dir, avID+".json"), view, 0600); err != nil {
		t.Fatal(err)
	}
	registry, err := msgpack.Marshal(map[string][]string{avID: mirrors})
	if err != nil {
		t.Fatal(err)
	}
	registryFile := filepath.Join(dir, "blocks.msgpack")
	if err = os.WriteFile(registryFile, registry, 0600); err != nil {
		t.Fatal(err)
	}
	selection, _ := json.Marshal(map[string][]string{"paths": {"/" + ids[0] + ".docj", "/" + ids[1] + ".docj"}})
	_, err = UI(ctx, manager, UIRequest{ProtocolVersion: 1, StoreID: store, PageID: emptyPage, Endpoint: "/api/filetree/removeDocs", Data: selection}, UIView{Mode: "wikiDesktop"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(data, box, id+".docj")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal(id, err)
		}
	}
	after, err := os.ReadFile(registryFile)
	var bindings map[string][]string
	if err != nil || msgpack.Unmarshal(after, &bindings) != nil || len(bindings[avID]) != 0 {
		t.Fatal(bindings, err)
	}
	record := sourcetx.DocumentTombstone{PageID: emptyPage, BoxID: box, DocID: second}.RecordPath()
	raw, err := os.ReadFile(filepath.Join(root, record))
	if err != nil {
		t.Fatal(err)
	}
	tomb, err := sourcetx.DecodeDocumentTombstone(raw, record)
	if err != nil || len(tomb.Linked) != 2 {
		t.Fatal(tomb, err)
	}
	history := func(endpoint string, body any) any {
		t.Helper()
		raw, _ := json.Marshal(body)
		got, err := UI(ctx, manager, UIRequest{ProtocolVersion: 1, StoreID: store, PageID: emptyPage,
			Endpoint: "/api/history/" + endpoint, Data: raw}, UIView{Mode: "wikiDesktop"})
		if err != nil {
			t.Fatal(endpoint, err)
		}
		return got
	}
	query := map[string]any{"op": "delete", "type": 1, "notebook": box, "query": "Database mirror", "page": 1}
	created := history("searchHistory", query).(map[string]any)["histories"].([]string)
	if len(created) != 1 {
		t.Fatal(created)
	}
	query["created"] = created[0]
	items := history("getHistoryItems", query).(map[string]any)["items"].([]map[string]string)
	var historyPath string
	for _, item := range items {
		if strings.HasSuffix(item["path"], second+".docj") && item["linkedCount"] == "2" {
			historyPath = item["path"]
		}
	}
	if len(items) != 2 || historyPath == "" {
		t.Fatal(items)
	}
	render := history("getDocHistoryContent", map[string]string{"historyPath": historyPath}).(map[string]any)
	if render["rootID"] != second {
		t.Fatal(render)
	}
	history("rollbackDocHistory", map[string]string{"historyPath": historyPath, "notebook": box})
	for i, id := range ids {
		if value, err := os.ReadFile(filepath.Join(data, box, id+".docj")); err != nil || !bytes.Equal(value, bodies[i]) {
			t.Fatal(id, err, string(value))
		}
	}
	if value, err := os.ReadFile(registryFile); err != nil || !bytes.Equal(value, registry) {
		t.Fatal("AV registry changed", err)
	}
	if items := history("getHistoryItems", query).(map[string]any)["items"].([]map[string]string); len(items) != 0 {
		t.Fatal("restored records were still present", items)
	}
	notebook, _ := json.Marshal(map[string]string{"notebook": box})
	if _, err = UI(ctx, manager, UIRequest{ProtocolVersion: 1, StoreID: store, PageID: emptyPage,
		Endpoint: "/api/notebook/removeNotebook", Data: notebook}, UIView{Mode: "wikiDesktop"}); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(registryFile)
	if err != nil || msgpack.Unmarshal(after, &bindings) != nil || len(bindings[avID]) != 0 {
		t.Fatal(bindings, err)
	}
	history("rollbackNotebookHistory", map[string]string{"historyPath": box})
	if value, err := os.ReadFile(registryFile); err != nil || !bytes.Equal(value, registry) {
		t.Fatal("notebook recovery changed the AV registry", err)
	}
}
