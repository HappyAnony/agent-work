package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/sourcetx"
)

func blockDOM(id, text string) string {
	return `<div data-node-id="` + id + `" data-type="NodeParagraph" class="p"><div contenteditable="true">` + text + `</div><div class="protyle-attr" contenteditable="false"></div></div>`
}
func domOperation(action, id, text string) ProtyleOperation {
	data, _ := json.Marshal(blockDOM(id, text))
	return ProtyleOperation{Action: action, ID: id, Data: data}
}
func TestProtyleNativeRenderingAndAtomicOperationsPreserveOpaqueFields(t *testing.T) {
	ctx := context.Background()
	raw := []byte(`{"ID":"` + newDoc + `","Type":"NodeDocument","Opaque":900719925474099312345,"Properties":{"title":"native","custom":{"keep":true}},"Children":[{"ID":"` + newBlock + `","Type":"NodeParagraph","OpaqueBlock":{"n":900719925474099312346},"Properties":{"custom":{"v":true}},"Children":[{"Type":"NodeText","Data":"before"}]}]}`)
	html, err := RenderDocument(ctx, raw, newDoc)
	if err != nil || !strings.Contains(html, "before") || !strings.Contains(html, newBlock) {
		t.Fatal(html, err)
	}
	insert := domOperation("insert", anotherBlock, "second")
	insert.PreviousID = newBlock
	next, err := applyProtyle(ctx, raw, newDoc, []ProtyleOperation{domOperation("update", newBlock, "<strong>after</strong><wbr>"), insert, {Action: "setAttrs", ID: newBlock, Data: json.RawMessage(`"{\"name\":\"named\"}"`)}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(next, []byte("‸")) {
		t.Fatal("selection marker persisted as text")
	}
	for _, value := range []string{"900719925474099312345", "900719925474099312346", `"keep":true`, `"custom":{"v":true}`, "second", "named"} {
		if !bytes.Contains(next, []byte(value)) {
			t.Fatalf("lost %s: %s", value, next)
		}
	}
	html, err = RenderDocument(ctx, next, newDoc)
	if err != nil || !strings.Contains(html, "after") || !strings.Contains(html, "second") {
		t.Fatal(html, err)
	}
	moved, err := applyProtyle(ctx, next, newDoc, []ProtyleOperation{{Action: "move", ID: anotherBlock, ParentID: newDoc}})
	if err != nil {
		t.Fatal(err)
	}
	root, _ := parseNode(moved)
	if root.children[0].id() != anotherBlock {
		t.Fatal("move did not honor first-child position")
	}
	deleted, err := applyProtyle(ctx, moved, newDoc, []ProtyleOperation{{Action: "delete", ID: anotherBlock}})
	if err != nil || bytes.Contains(deleted, []byte(anotherBlock)) {
		t.Fatal(string(deleted), err)
	}
	for _, placement := range []ProtyleOperation{
		{Action: "insert", ID: anotherBlock, Data: insert.Data, PreviousID: newBlock, NextID: newBlock},
		{Action: "insert", ID: anotherBlock, Data: insert.Data, NextID: newBlock, ParentID: newBlock},
	} {
		if _, err := applyProtyle(ctx, raw, newDoc, []ProtyleOperation{placement}); !errors.Is(err, ErrArgument) {
			t.Fatal("ambiguous or foreign-parent placement accepted", err)
		}
	}
}

func TestProtyleContentServiceUsesCASJournalAndRejectsUnsupportedBatchWithoutWriting(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	read := baseContent(id, "document.get")
	read.PageID = testPage
	read.DocID = "20261019090001-doc0001"
	before, err := Content(ctx, m, read)
	if err != nil {
		t.Fatal(err)
	}
	render := read
	render.Action = "document.render"
	view, err := Content(ctx, m, render)
	if err != nil || view.Hash != before.Hash || !strings.Contains(view.HTML, "Synthetic knowledge") {
		t.Fatal(view, err)
	}
	request := read
	request.Action = "document.transactions"
	request.OperationID = "native-protyle-edit"
	request.ExpectedHash = before.Hash
	request.Operations = []ProtyleOperation{domOperation("update", "20261019090002-par0001", "typed <strong>native</strong>")}
	written, err := Content(ctx, m, request)
	if err != nil || written.Receipt == nil || !written.Receipt.SourceCommitted || written.Hash == "" {
		t.Fatal(written, err)
	}
	after, err := Content(ctx, m, read)
	if err != nil || after.Hash != written.Hash {
		t.Fatal(after, err)
	}
	if replay, err := Content(ctx, m, request); err != nil || replay.Receipt.RequestHash != written.Receipt.RequestHash || replay.Hash != written.Hash {
		t.Fatal("replay must not adopt a later file hash", replay, err)
	}
	request.OperationID = "stale-protyle-cas"
	if _, err = Content(ctx, m, request); err == nil {
		t.Fatal("stale CAS overwrote source")
	}
	path := filepath.Join(root, "pages", testPage, "data", box, read.DocID+".docj")
	saved, _ := os.ReadFile(path)
	request.OperationID = "unsupported-batch"
	request.ExpectedHash = after.Hash
	request.Operations = append(request.Operations, ProtyleOperation{Action: "unreviewed-side-effect", ID: newBlock})
	if _, err = Content(ctx, m, request); !errors.Is(err, ErrRichText) {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(path)
	if !bytes.Equal(saved, actual) {
		t.Fatal("failed operation partially committed")
	}
}

func TestDocumentListTitleIsOnlyAReadProjection(t *testing.T) {
	root, m, id := setupPages(t)
	file := filepath.Join(root, "pages", testPage, "data", box, "20261019090001-doc0001.docj")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Content(context.Background(), m, ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, Action: "document.list"})
	if err != nil || len(result.Items) != 1 || result.Items[0].Title != "单文档示例" {
		t.Fatal(result, err)
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("title projection rewrote source", err)
	}
}

func TestProtyleDeleteDoesNotLoseOpaqueMetadataThroughHTMLUndo(t *testing.T) {
	ctx := context.Background()
	for _, extra := range []string{`,"OpaqueBlock":{"future":900719925474099312345}`, `,"Properties":{"custom":{"future":true}}`, `,"Properties":{"custom-note":"retain"}`} {
		raw := []byte(`{"ID":"` + newDoc + `","Type":"NodeDocument","Children":[{"ID":"` + newBlock + `","Type":"NodeParagraph"` + extra + `,"Children":[{"Type":"NodeText","Data":"protected"}]}]}`)
		if _, err := applyProtyle(ctx, raw, newDoc, []ProtyleOperation{{Action: "delete", ID: newBlock}}); !errors.Is(err, ErrRichText) {
			t.Fatal("HTML-only undo cannot restore opaque source metadata", err)
		}
	}
	plain := []byte(`{"ID":"` + newDoc + `","Type":"NodeDocument","OpaqueRoot":900719925474099312345,"Children":[{"ID":"` + newBlock + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"plain"}]}]}`)
	deleted, err := applyProtyle(ctx, plain, newDoc, []ProtyleOperation{{Action: "delete", ID: newBlock}})
	if err != nil {
		t.Fatal(err)
	}
	undo := domOperation("insert", newBlock, "plain")
	undo.ParentID = newDoc
	restored, err := applyProtyle(ctx, deleted, newDoc, []ProtyleOperation{undo})
	if err != nil || !bytes.Contains(restored, []byte("900719925474099312345")) || !bytes.Contains(restored, []byte("plain")) {
		t.Fatal(string(restored), err)
	}
}

func TestPreserveNativeStructuralRemovalRetainsOpaqueParentAndRejectsHiddenDescendants(t *testing.T) {
	for _, hidden := range []string{"", `,"OpaqueChild":{"n":900719925474099312346}`, `,"Properties":{"custom":{"keep":true}}`} {
		old, err := parseNode([]byte(`{"ID":"` + newBlock + `","Type":"NodeList","OpaqueParent":{"n":900719925474099312345},"Children":[{"ID":"` + anotherBlock + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"kept"}]},{"ID":"20261019100002-par0004","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"removed"` + hidden + `}]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		next, err := parseNode(old.raw)
		if err != nil {
			t.Fatal(err)
		}
		next.children = next.children[:1]
		err = preserveNative(old, next, false)
		if hidden != "" {
			if !errors.Is(err, ErrRichText) {
				t.Fatal("lost hidden descendant", err)
			}
			continue
		}
		if err != nil {
			t.Fatal("plain structural undo rejected", err)
		}
		raw, _, err := next.encode()
		if err != nil || !bytes.Contains(raw, []byte("900719925474099312345")) || bytes.Contains(raw, []byte("removed")) {
			t.Fatal(string(raw), err)
		}
	}
}

func TestOpaqueDeleteRejectsWholeNativeBatchWithoutWriting(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	r := baseContent(id, "document.put")
	r.OperationID = "seed-opaque-delete"
	r.NotebookName = "Protected"
	r.AST = []byte(`{"ID":"` + newDoc + `","Type":"NodeDocument","Children":[{"ID":"` + newBlock + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"before"}]},{"ID":"` + anotherBlock + `","Type":"NodeParagraph","Opaque":{"big":900719925474099312345},"Children":[{"Type":"NodeText","Data":"protected"}]}]}`)
	if _, err := Content(ctx, m, r); err != nil {
		t.Fatal(err)
	}
	r = baseContent(id, "document.get")
	read, err := Content(ctx, m, r)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "pages", emptyPage, "data", box, newDoc+".docj")
	before, _ := os.ReadFile(file)
	r.Action = "document.transactions"
	r.ExpectedHash = read.Hash
	r.OperationID = "opaque-delete-must-not-commit"
	r.Operations = []ProtyleOperation{domOperation("update", newBlock, "would change"), {Action: "delete", ID: anotherBlock}}
	if _, err := Content(ctx, m, r); !errors.Is(err, ErrRichText) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("failed guarded delete committed an earlier operation")
	}
	r.Action = "block.delete"
	r.BlockID = anotherBlock
	r.Operations = nil
	r.OperationID = "explicit-native-delete-opaque"
	result, err := Content(ctx, m, r)
	if err != nil || result.Receipt == nil || !result.Receipt.SourceCommitted {
		t.Fatal("explicit native delete must remain supported", result, err)
	}
}

func TestAnonymousStructuralEditCannotMoveHiddenMetadataToShiftedNeighbour(t *testing.T) {
	old, err := parseNode([]byte(`{"ID":"` + newBlock + `","Type":"NodeTable","Children":[{"Type":"NodeTableRow","Opaque":{"owner":"first"},"Children":[{"Type":"NodeTableCell","Children":[{"Type":"NodeText","Data":"first"}]}]},{"Type":"NodeTableRow","Children":[{"Type":"NodeTableCell","Children":[{"Type":"NodeText","Data":"second"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	next, err := parseNode(old.raw)
	if err != nil {
		t.Fatal(err)
	}
	next.children = next.children[1:]
	for _, history := range []bool{false, true} {
		if err := preserveNative(old, next, history); !errors.Is(err, ErrRichText) {
			t.Fatal("opaque first row could be reassigned to second row", history, err)
		}
	}
}

func TestNativeHistoryDeleteAndRestoreAreLosslessAndCASProtected(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	r := baseContent(id, "document.put")
	r.OperationID, r.NotebookName = "history-seed", "History"
	r.AST = []byte(" \n" + `{"ID":"` + newDoc + `","Type":"NodeDocument","Opaque":900719925474099312345,"Children":[{"ID":"` + newBlock + `","Type":"NodeParagraph","Properties":{"custom":{"keep":true}},"OpaqueBlock":900719925474099312346,"Children":[{"Type":"NodeText","Data":"restore me"}]}]}` + "\n ")
	if _, err := Content(ctx, m, r); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "pages", emptyPage, "data", box, newDoc+".docj")
	before, _ := os.ReadFile(file)
	read, err := Content(ctx, m, baseContent(id, "document.get"))
	if err != nil {
		t.Fatal(err)
	}
	r = baseContent(id, "document.transactions")
	r.ExpectedHash, r.OperationID, r.NativeHistory = read.Hash, "history-delete", true
	r.Operations = []ProtyleOperation{{Action: "delete", ID: newBlock}}
	result, err := Content(ctx, m, r)
	if err != nil || result.Receipt == nil || !result.Receipt.SourceCommitted {
		t.Fatal(result, err)
	}
	after, _ := os.ReadFile(file)
	if result.HistoryID != r.OperationID || result.Hash != sourcetx.Hash(after) || bytes.Contains(after, []byte(newBlock)) {
		t.Fatal("snapshot is not exact committed bytes")
	}
	// Only the opaque history handle crosses the transport boundary.
	wire, _ := json.Marshal(result)
	var decoded ContentResult
	if json.Unmarshal(wire, &decoded) != nil || decoded.HistoryID != r.OperationID || bytes.Contains(wire, []byte("historyBefore")) || bytes.Contains(wire, []byte("historyAfter")) {
		t.Fatal("wire snapshot lost native bytes")
	}
	restore := baseContent(id, "document.history")
	restore.OperationID, restore.ExpectedHash, restore.HistoryID = "history-undo", result.Hash, decoded.HistoryID
	if _, err := Content(ctx, m, restore); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(file)
	if !bytes.Equal(actual, before) {
		t.Fatal("undo lost source bytes")
	}
	restore.OperationID = "history-stale-undo"
	if _, err := Content(ctx, m, restore); err == nil {
		t.Fatal("stale history overwrote newer source")
	}
	restore.OperationID, restore.ExpectedHash, restore.Redo = "history-redo", read.Hash, true
	if _, err := Content(ctx, m, restore); err != nil {
		t.Fatal(err)
	}
	actual, _ = os.ReadFile(file)
	if !bytes.Equal(actual, after) {
		t.Fatal("redo differs from committed snapshot")
	}
	r = baseContent(id, "document.restore")
	r.OperationID, r.ExpectedHash = "invalid-history", result.Hash
	for _, data := range [][]byte{nil, []byte(`{"Type":"NodeDocument","ID":"wrong"}`), bytes.Repeat([]byte("x"), (4<<20)+1)} {
		r.Data = data
		if _, err := Content(ctx, m, r); err == nil {
			t.Fatal("invalid/oversized restore accepted")
		}
	}
	actual, _ = os.ReadFile(file)
	if !bytes.Equal(actual, after) {
		t.Fatal("rejected restore wrote source")
	}
}

func TestNativeHistoryStructuralRemovalPreservesSurvivorsAndRestoresExactSource(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	removedID := "20261019100002-par0004"
	r := baseContent(id, "document.put")
	r.OperationID, r.NotebookName = "structural-history-seed", "History"
	r.AST = []byte(" \n" + `{"ID":"` + newDoc + `","Type":"NodeDocument","Children":[{"ID":"` + newBlock + `","Type":"NodeBlockquote","OpaqueParent":900719925474099312345,"Children":[{"ID":"` + removedID + `","Type":"NodeParagraph","OpaqueRemoved":900719925474099312346,"Children":[{"Type":"NodeText","Data":"removed","HiddenInline":{"keep":true}}]},{"ID":"` + anotherBlock + `","Type":"NodeParagraph","OpaqueSurvivor":{"owner":"survivor"},"Properties":{"custom":{"keep":true}},"Children":[{"Type":"NodeText","Data":"kept"}]}]}]}` + "\n ")
	if _, err := Content(ctx, m, r); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "pages", emptyPage, "data", box, newDoc+".docj")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := parseNode(before)
	if err != nil {
		t.Fatal(err)
	}
	projected.children[0].children = projected.children[0].children[1:]
	projected.children[0].dirty, projected.dirty = true, true
	projection, _, err := projected.encode()
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderDocument(ctx, projection, newDoc)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(html)
	read, err := Content(ctx, m, baseContent(id, "document.get"))
	if err != nil {
		t.Fatal(err)
	}
	r = baseContent(id, "document.transactions")
	r.OperationID, r.ExpectedHash = "structural-html-reject", read.Hash
	r.Operations = []ProtyleOperation{{Action: "update", ID: newBlock, Data: data}}
	if _, err := Content(ctx, m, r); !errors.Is(err, ErrRichText) {
		t.Fatal("HTML-only undo cannot restore removed hidden subtree", err)
	}
	actual, _ := os.ReadFile(file)
	if !bytes.Equal(actual, before) {
		t.Fatal("rejected structural edit wrote source")
	}
	r.OperationID, r.NativeHistory = "structural-native-history", true
	result, err := Content(ctx, m, r)
	if err != nil || result.Receipt == nil || !result.Receipt.SourceCommitted {
		t.Fatal(result, err)
	}
	after, _ := os.ReadFile(file)
	if result.HistoryID != r.OperationID || result.Hash != sourcetx.Hash(after) {
		t.Fatal("structural snapshots are not exact committed bytes")
	}
	for _, retained := range []string{"900719925474099312345", `"OpaqueSurvivor":{"owner":"survivor"}`, `"custom":{"keep":true}`, anotherBlock} {
		if !bytes.Contains(after, []byte(retained)) {
			t.Fatalf("surviving metadata lost: %s: %s", retained, after)
		}
	}
	for _, removed := range []string{removedID, "OpaqueRemoved", "HiddenInline"} {
		if bytes.Contains(after, []byte(removed)) {
			t.Fatal("removed subtree metadata shifted to survivor", removed)
		}
	}
	restore := baseContent(id, "document.history")
	restore.OperationID, restore.ExpectedHash, restore.HistoryID = "structural-undo", result.Hash, result.HistoryID
	if _, err := Content(ctx, m, restore); err != nil {
		t.Fatal(err)
	}
	actual, _ = os.ReadFile(file)
	if !bytes.Equal(actual, before) {
		t.Fatal("structural undo lost native bytes")
	}
	restore.OperationID, restore.ExpectedHash, restore.Redo = "structural-redo", read.Hash, true
	if _, err := Content(ctx, m, restore); err != nil {
		t.Fatal(err)
	}
	actual, _ = os.ReadFile(file)
	if !bytes.Equal(actual, after) {
		t.Fatal("structural redo lost native bytes")
	}
}

func TestNativeHistoryUsesJournalForLargeSource(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	r := baseContent(id, "document.put")
	r.OperationID, r.NotebookName = "history-large-seed", "Limits"
	r.AST = []byte(`{"ID":"` + newDoc + `","Type":"NodeDocument","Padding":"` + strings.Repeat("x", 4<<20) + `","Children":[{"ID":"` + newBlock + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"before"}]}]}`)
	if _, err := Content(ctx, m, r); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "pages", emptyPage, "data", box, newDoc+".docj")
	before, err := os.ReadFile(file)
	if err != nil || len(before) <= 4<<20 {
		t.Fatal(len(before), err)
	}
	r = baseContent(id, "document.transactions")
	r.OperationID, r.ExpectedHash, r.NativeHistory = "history-large-edit", sourcetx.Hash(before), true
	r.Operations = []ProtyleOperation{domOperation("update", newBlock, "after")}
	result, err := Content(ctx, m, r)
	if err != nil || result.HistoryID != r.OperationID || !result.OriginalIndexFresh {
		t.Fatal(result, err)
	}
	after, err := os.ReadFile(file)
	if err != nil || len(after) <= 4<<20 || bytes.Equal(before, after) {
		t.Fatal(len(after), err)
	}
	wire, _ := json.Marshal(result)
	if len(wire) > 4096 {
		t.Fatal("large image leaked into result", len(wire))
	}
	history := baseContent(id, "document.history")
	history.OperationID, history.ExpectedHash, history.HistoryID = "history-large-undo", result.Hash, result.HistoryID
	if _, err = Content(ctx, m, history); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, before) {
		t.Fatal("undo did not restore exact large source")
	}
	history.OperationID, history.ExpectedHash, history.Redo = "history-large-redo", sourcetx.Hash(before), true
	if _, err = Content(ctx, m, history); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(file)
	if !bytes.Equal(got, after) {
		t.Fatal("redo did not restore exact large source")
	}
}
