package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/sourcetx"
)

const newDoc = "20261019100000-doc0002"
const newBlock = "20261019100001-par0002"
const anotherBlock = "20261019100002-par0003"
const box = "20261019090000-box0001"
const emptyPage = "1760000000000000001"

func documentAST(doc, block, title, text string) json.RawMessage {
	return json.RawMessage(`{"ID":"` + doc + `","Type":"NodeDocument","Opaque":{"big":900719925474099312345},"Properties":{"title":"` + title + `","custom":{"keep":true}},"Children":[{"ID":"` + block + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"` + text + `"}]}]}`)
}
func baseContent(id, action string) ContentRequest {
	return ContentRequest{ProtocolVersion: 1, Action: action, StoreID: id, PageID: emptyPage, BoxID: box, DocID: newDoc}
}
func TestContentDocumentBlockAssetsLifecycleAndReplay(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		result, err := Content(ctx, m, r)
		if err != nil {
			t.Fatal(r.Action, result, err)
		}
		return result
	}
	put := baseContent(id, "document.put")
	put.AST = documentAST(newDoc, newBlock, "New document", "Hello knowledge")
	put.NotebookName = "New notebook"
	put.OperationID = "create-document"
	put.DryRun = true
	before := snapshotFiles(t, root)
	preview := call(put)
	if preview.Receipt.SourceCommitted || preview.Receipt.Status != "dry_run" || !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("dry-run wrote source/runtime", preview)
	}
	put.DryRun = false
	committed := call(put)
	if !committed.Receipt.SourceCommitted || !committed.Receipt.IndexFresh {
		t.Fatal(committed)
	}
	if retry := call(put); !reflect.DeepEqual(committed.Receipt, retry.Receipt) {
		t.Fatal("create replay", retry)
	}
	get := baseContent(id, "document.get")
	current := call(get)
	if !strings.Contains(string(current.AST), "900719925474099312345") {
		t.Fatal("lost unknown integer")
	}
	get.Format = "text"
	if text := call(get).Text; text != "Hello knowledge\n" {
		t.Fatal(text)
	}
	get.Format = ""
	block := baseContent(id, "block.create")
	block.OperationID = "insert-block"
	block.BlockID = anotherBlock
	block.ParentID = newDoc
	block.ExpectedHash = current.Hash
	block.AST = json.RawMessage(`{"ID":"` + anotherBlock + `","Type":"NodeParagraph","Unknown":{"n":900719925474099312345},"Children":[{"Type":"NodeText","Data":"Inserted"}]}`)
	created := call(block)
	if replay := call(block); !reflect.DeepEqual(created.Receipt, replay.Receipt) {
		t.Fatal("block retry")
	}
	current = call(get)
	move := baseContent(id, "block.move")
	move.BlockID = anotherBlock
	move.ParentID = newDoc
	move.BeforeID = newBlock
	move.ExpectedHash = current.Hash
	move.OperationID = "move-block"
	call(move)
	current = call(get)
	if strings.Index(string(current.AST), anotherBlock) > strings.Index(string(current.AST), newBlock) {
		t.Fatal("block not moved")
	}
	if !strings.Contains(string(current.AST), "900719925474099312345") {
		t.Fatal("unknown field stripped by ancestor edit")
	}
	replace := baseContent(id, "block.replace")
	replace.BlockID = anotherBlock
	replace.ExpectedHash = current.Hash
	replace.OperationID = "replace-block"
	replace.AST = json.RawMessage(`{"ID":"` + anotherBlock + `","Type":"NodeParagraph","Future":"preserved","Children":[{"Type":"NodeText","Data":"Replacement"}]}`)
	call(replace)
	current = call(get)
	readBlock := baseContent(id, "block.get")
	readBlock.BlockID = anotherBlock
	readBlock.Format = "text"
	if value := call(readBlock).Text; value != "Replacement\n" {
		t.Fatal(value)
	}
	remove := baseContent(id, "block.delete")
	remove.BlockID = anotherBlock
	remove.ExpectedHash = current.Hash
	remove.OperationID = "delete-block"
	call(remove)
	if _, err := Content(ctx, m, readBlock); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deleted block readable", err)
	}
	asset := ContentRequest{ProtocolVersion: 1, Action: "asset.put", StoreID: id, PageID: emptyPage, AssetName: "images/空格 image.bin", Data: []byte{0, 1, 2, 255}, OperationID: "create-asset"}
	assetReceipt := call(asset)
	if replay := call(asset); !reflect.DeepEqual(replay.Receipt, assetReceipt.Receipt) {
		t.Fatal("asset replay")
	}
	assetRead := ContentRequest{ProtocolVersion: 1, Action: "asset.get", StoreID: id, PageID: emptyPage, AssetName: asset.AssetName}
	loaded := call(assetRead)
	if !reflect.DeepEqual(loaded.Data, asset.Data) {
		t.Fatal(loaded)
	}
	asset.OperationID = "update-asset"
	asset.ExpectedHash = loaded.Hash
	asset.Data = []byte("updated")
	call(asset)
	loaded = call(assetRead)
	asset.Action = "asset.delete"
	asset.OperationID = "delete-asset"
	asset.ExpectedHash = loaded.Hash
	asset.Data = nil
	deleted := call(asset)
	if replay := call(asset); !reflect.DeepEqual(deleted.Receipt, replay.Receipt) {
		t.Fatal("delete replay")
	}
	if _, err := Content(ctx, m, assetRead); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	current = call(get)
	stale := put
	stale.NotebookName = ""
	stale.ExpectedHash = current.Hash
	stale.OperationID = "replace-doc"
	stale.AST = documentAST(newDoc, newBlock, "New title", "Changed knowledge")
	call(stale)
	stale.OperationID = "stale-doc"
	if _, err := Content(ctx, m, stale); !errors.Is(err, sourcetx.ErrConflict) {
		t.Fatal("stale write accepted", err)
	}
	list := ContentRequest{ProtocolVersion: 1, Action: "search", StoreID: id, PageID: emptyPage, Query: "CHANGED"}
	if found := call(list); found.Total != 1 {
		t.Fatal(found)
	}
	check := list
	check.Action = "check"
	check.Query = ""
	if result := call(check); len(result.Diagnostics) != 0 {
		t.Fatal(result)
	}
	current = call(get)
	del := baseContent(id, "document.delete")
	del.OperationID = "delete-doc"
	del.ExpectedHash = current.Hash
	deleted = call(del)
	if retry := call(del); !reflect.DeepEqual(retry.Receipt, deleted.Receipt) {
		t.Fatal("document delete replay")
	}
	if _, err := Content(ctx, m, get); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestDocumentMovesDescendantsAndCreatesDocxAtomically(t *testing.T) {
	_, m, id := setupPages(t)
	ctx := context.Background()
	call := func(r ContentRequest) ContentResult {
		t.Helper()
		result, err := Content(ctx, m, r)
		if err != nil {
			t.Fatal(r.Action, result, err)
		}
		return result
	}
	put := baseContent(id, "document.put")
	put.AST = documentAST(newDoc, newBlock, "Parent", "parent")
	put.NotebookName = "Notebook"
	put.OperationID = "parent-create"
	call(put)
	childID := "20261019100003-doc0003"
	childBlock := "20261019100004-par0004"
	child := baseContent(id, "document.put")
	child.DocID = childID
	child.ParentDocID = newDoc
	child.OperationID = "child-create"
	child.AST = documentAST(childID, childBlock, "Child", "child")
	call(child)
	otherID := "20261019100005-doc0004"
	other := baseContent(id, "document.put")
	other.DocID = otherID
	other.OperationID = "other-create"
	other.AST = documentAST(otherID, "20261019100006-par0005", "Other", "other")
	call(other)
	sourceGet := baseContent(id, "document.get")
	targetGet := sourceGet
	targetGet.DocID = otherID
	sourceDoc, targetDoc := call(sourceGet), call(targetGet)
	blockMove := baseContent(id, "block.move")
	blockMove.BlockID = newBlock
	blockMove.ParentID = otherID
	blockMove.TargetDocID = otherID
	blockMove.ExpectedHash = sourceDoc.Hash
	blockMove.TargetExpectedHash = targetDoc.Hash
	blockMove.OperationID = "cross-document-block"
	call(blockMove)
	if moved := call(targetGet); !strings.Contains(string(moved.AST), newBlock) {
		t.Fatal("cross-document block lost")
	}
	if moved := call(sourceGet); strings.Contains(string(moved.AST), newBlock) {
		t.Fatal("source block not removed")
	}
	current := call(baseContent(id, "document.get"))
	move := baseContent(id, "document.move")
	move.ParentDocID = otherID
	move.ExpectedHash = current.Hash
	move.OperationID = "move-document-subtree"
	receipt := call(move)
	if retry := call(move); !reflect.DeepEqual(retry.Receipt, receipt.Receipt) {
		t.Fatal("move replay")
	}
	list := ContentRequest{ProtocolVersion: 1, Action: "document.list", StoreID: id, PageID: emptyPage, Limit: 1}
	page := call(list)
	if page.Total != 3 || page.NextCursor == "" {
		t.Fatal(page)
	}
	list.Cursor = page.NextCursor
	if next := call(list); len(next.Items) != 1 || next.Items[0].Path == page.Items[0].Path {
		t.Fatal(next)
	}
	all := list
	all.Cursor = ""
	all.Limit = 100
	var childPath string
	for _, item := range call(all).Items {
		if item.DocID == childID {
			childPath = item.Path
		}
	}
	if !strings.Contains(childPath, otherID+"/"+newDoc+"/") {
		t.Fatal("descendant not moved", childPath)
	}
	current = call(baseContent(id, "document.get"))
	del := baseContent(id, "document.delete")
	del.ExpectedHash = current.Hash
	del.OperationID = "delete-subtree"
	if _, err := Content(ctx, m, del); !errors.Is(err, ErrArgument) {
		t.Fatal("implicit cascade", err)
	}
	del.Cascade = true
	call(del)
	if _, err := Content(ctx, m, list); !errors.Is(err, sourcetx.ErrCursor) {
		t.Fatal("stale content cursor", err)
	}
	// Entry page + document + notebook are created under one candidate overlay.
	template := pageGet(t, m, id, testPage).Page
	template.PageID = "1760000000000000006"
	template.Name = "Atomic docx"
	template.Revision = 1
	template.ParentID = nil
	put = baseContent(id, "document.put")
	put.PageID = template.PageID
	put.DocID = template.EntryDocument.DocID
	put.BoxID = template.EntryDocument.BoxID
	put.AST = documentAST(put.DocID, newBlock, template.Name, "new entry")
	put.NotebookName = "Docx notebook"
	put.PageUpdate = &PageUpdate{Page: template}
	put.OperationID = "create-docx"
	call(put)
	if p := pageGet(t, m, id, put.PageID); p.Page.Name != template.Name || p.EntryHash == "" {
		t.Fatal(p)
	}
	bad := put
	bad.OperationID = "invalid-entry-title"
	bad.PageUpdate = nil
	bad.NotebookName = ""
	bad.ExpectedHash = sourcetx.Hash(put.AST)
	bad.AST = documentAST(put.DocID, newBlock, "Unsynced title", "new entry")
	if _, err := Content(ctx, m, bad); !errors.Is(err, ErrArgument) {
		t.Fatal("entry title drift allowed", err)
	}
	foreign := put
	foreign.OperationID = "mismatched-docx-entry"
	foreign.PageUpdate = &PageUpdate{Page: template}
	otherEntry := *template.EntryDocument
	otherEntry.DocID = otherID
	foreign.PageUpdate.Page.EntryDocument = &otherEntry
	if _, err := Content(ctx, m, foreign); !errors.Is(err, ErrArgument) {
		t.Fatal("updated another docx entry without its AST/title", err)
	}
}

func TestContentRejectsTraversalUnknownFieldsAndInvalidBlockMoves(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	for _, name := range []string{"../store.json", "/absolute", ".local/locks", "assets/../bad", "x\\y", "doc.docj"} {
		_, err := Content(ctx, m, ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, Action: "asset.put", AssetName: name, OperationID: "bad-asset", Data: []byte("bad")})
		if !errors.Is(err, ErrArgument) {
			t.Fatal(name, err)
		}
	}
	before := snapshotFiles(t, root)
	for _, raw := range []string{`{"protocolVersion":1,"storeId":"x","pageId":"1760000000000000001","action":"document.get","root":"/etc"}`, `{"protocolVersion":1,"storeId":"x","pageId":"1760000000000000001","action":"asset.get","assetName":"a","assetName":"b"}`} {
		if _, err := DecodeContentRequest(strings.NewReader(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("invalid input wrote files")
	}
	if err := os.Symlink(filepath.Join(root, "store.json"), filepath.Join(root, "pages", testPage, "data", "assets", "foreign.bin")); err != nil {
		t.Fatal(err)
	}
	_, err := Content(ctx, m, ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, Action: "asset.get", AssetName: "foreign.bin"})
	if err == nil {
		t.Fatal("symlink asset read")
	}
}

func TestContentDomainReceiptSurvivesInterruptedPublicationAndRecovery(t *testing.T) {
	for _, point := range []string{"published:0", "committed"} {
		t.Run(point, func(t *testing.T) {
			root, m, id := setupPages(t)
			ctx := context.Background()
			request := baseContent(id, "document.put")
			request.OperationID = "recover-domain-retry"
			request.AST = documentAST(newDoc, newBlock, "Recover", "text")
			canonical, _ := json.Marshal(request)
			source, err := sourcetx.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			writer, err := source.Writer(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			tx := sourcetx.Request{OperationID: request.OperationID, ClientHash: sourcetx.Hash(canonical), Changes: []sourcetx.Change{{Path: "pages/" + emptyPage + "/data/" + box + "/" + newDoc + ".docj", Data: request.AST}}}
			_, err = writer.Apply(ctx, tx, func(at string) error {
				if at == point {
					return errors.New("interrupted")
				}
				return nil
			})
			writer.Close()
			if err == nil {
				t.Fatal("fault not reached")
			}
			if _, err := Content(ctx, m, request); !errors.Is(err, sourcetx.ErrRecovery) {
				t.Fatal("pending operation admitted", err)
			}
			if _, err := RecoverRoot(ctx, root, RecoverRequest{ProtocolVersion: 1, StoreID: id}); err != nil {
				t.Fatal(err)
			}
			before := snapshotFiles(t, root)
			result, err := Content(ctx, m, request)
			if err != nil || result.Receipt == nil || result.Receipt.ClientHash != tx.ClientHash || result.Receipt.SourceCommitted != (point == "committed") {
				t.Fatal(result, err)
			}
			if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
				t.Fatal("recovered retry applied twice")
			}
			request.OperationID = ""
			request.Action = "document.get"
			request.AST = nil
			if _, err := Content(ctx, m, request); (err == nil) != (point == "committed") {
				t.Fatal("recovery source outcome", err)
			}
		})
	}
}
