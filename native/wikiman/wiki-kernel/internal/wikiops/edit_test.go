//go:build fts5

package wikiops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/sourcetx"
)

func TestR04EditIndexFailureRetryKeepsReceiptAndCommittedVersion(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	const doc = "20261019090001-doc0001"
	const block = "20261019090002-par0001"
	name := box + "/" + doc + ".docj"
	file := filepath.Join(root, "pages", testPage, "data", name)
	before, _ := os.ReadFile(file)
	r := EditRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, OperationID: "r04-retry-index", ExpectedSources: map[string]string{name: sourcetx.Hash(before)}, Operations: []*editcore.Operation{{Action: "update", ID: block, Data: blockDOM(block, "r04 retry one")}}}
	fault := errors.New("native index failure")
	committed, e := editOriginal(ctx, m, r, "", func() error { return fault })
	if !errors.Is(e, fault) || !errors.Is(e, sourcetx.ErrIndex) || committed.IndexFresh || committed.Receipt == nil || !committed.Receipt.SourceCommitted || len(committed.Notices) != 0 {
		t.Fatal(committed, e)
	}
	raw, _ := os.ReadFile(file)
	if bytes.Equal(raw, before) || sourcetx.Hash(raw) != committed.Hashes[name] {
		t.Fatal("source receipt mismatch")
	}
	retry, e := Edit(ctx, m, r)
	if e != nil || !retry.Replay || !retry.IndexFresh || retry.Receipt.RequestHash != committed.Receipt.RequestHash || retry.Hashes[name] != committed.Hashes[name] {
		t.Fatal(retry, e)
	}
	later := r
	later.OperationID = "r04-later-edit"
	later.ExpectedSources = map[string]string{name: sourcetx.Hash(raw)}
	later.Operations = []*editcore.Operation{{Action: "update", ID: block, Data: blockDOM(block, "r04 retry later")}}
	next, e := Edit(ctx, m, later)
	if e != nil {
		t.Fatal(e)
	}
	retry, e = Edit(ctx, m, r)
	current, _ := os.ReadFile(file)
	if e != nil || retry.Hashes[name] != committed.Hashes[name] || next.Hashes[name] != sourcetx.Hash(current) || bytes.Equal(current, raw) {
		t.Fatal(retry, next, e)
	}
	// A document ticket cannot undo a host operation even if it touched this doc.
	undo := ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, BoxID: box, DocID: doc, Action: "document.history", OperationID: "r04-editor-forgery", HistoryID: later.OperationID, ExpectedHash: next.Hashes[name]}
	if _, e = Content(ctx, m, undo); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("editor could reverse host edit", e)
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, current) {
		t.Fatal("forged history changed source")
	}
	request := EditRequest{ProtocolVersion: 1, StoreID: id, PageID: testPage, OperationID: "r04-host-undo", ExpectedSources: map[string]string{name: next.Hashes[name]}, History: &EditHistory{OperationID: later.OperationID}}
	undone, e := Edit(ctx, m, request)
	if e != nil || !undone.IndexFresh || undone.HistoryID != "" || undone.Hashes[name] != retry.Hashes[name] {
		t.Fatal(undone, e)
	}
	got, _ = os.ReadFile(file)
	if !bytes.Equal(got, raw) {
		t.Fatal("host undo did not restore exact source")
	}
	request.OperationID, request.History.Redo = "r04-host-redo", true
	request.ExpectedSources = map[string]string{name: undone.Hashes[name]}
	redone, e := Edit(ctx, m, request)
	got, _ = os.ReadFile(file)
	if e != nil || !redone.IndexFresh || !bytes.Equal(got, current) {
		t.Fatal(redone, e)
	}

}

func TestR04EditorHistoryRetainsMoreThanEightExactSteps(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	r := baseContent(id, "document.put")
	r.OperationID, r.NotebookName, r.AST = "r04-steps-seed", "Steps", documentAST(newDoc, newBlock, "Steps", "Initial")
	if _, e := Content(ctx, m, r); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(root, "pages", emptyPage, "data", box, newDoc+".docj")
	read := func() []byte {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	frames := [][]byte{read()}
	ids := []string{}
	for i := range 10 {
		r := baseContent(id, "document.transactions")
		r.OperationID, r.ExpectedHash, r.NativeHistory = fmt.Sprintf("r04-steps-edit-%d", i), sourcetx.Hash(frames[i]), true
		r.Operations = []ProtyleOperation{domOperation("update", newBlock, fmt.Sprintf("Step %d", i))}
		result, e := Content(ctx, m, r)
		if e != nil || result.HistoryID != r.OperationID || !result.OriginalIndexFresh {
			t.Fatal(i, result, e)
		}
		frames = append(frames, read())
		ids = append(ids, result.HistoryID)
	}
	for _, redo := range []bool{false, true} {
		for step := range len(ids) {
			i, from, to := len(ids)-1-step, len(ids)-step, len(ids)-1-step
			if redo {
				i, from, to = step, step, step+1
			}
			r := baseContent(id, "document.history")
			r.OperationID, r.HistoryID, r.ExpectedHash, r.Redo = fmt.Sprintf("r04-steps-%t-%d", redo, i), ids[i], sourcetx.Hash(frames[from]), redo
			result, e := Content(ctx, m, r)
			if e != nil || !result.OriginalIndexFresh || result.Hash != sourcetx.Hash(frames[to]) || !bytes.Equal(read(), frames[to]) {
				t.Fatal(redo, i, result, e)
			}
		}
	}
}
