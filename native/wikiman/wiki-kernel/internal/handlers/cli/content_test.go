package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/handlers/readhttp"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

func TestContentCLIHTTPParityCRUDAndSmallReads(t *testing.T) {
	base := t.TempDir()
	roots := []string{filepath.Join(base, "CLI ?# ' 中文"), filepath.Join(base, "HTTP ?# ' 中文")}
	for _, root := range roots {
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
			t.Fatal(err)
		}
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	defer m.Drain(context.Background())
	id, err := m.OpenStore(context.Background(), roots[1])
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("content-test-token-", 3)
	handler, err := readhttp.NewOperations(m, token)
	if err != nil {
		t.Fatal(err)
	}
	type envelope struct {
		OK     bool
		Result wikiops.ContentResult
		Error  wikiops.ServiceError
	}
	call := func(r wikiops.ContentRequest) envelope {
		t.Helper()
		raw, _ := json.Marshal(r)
		var stdout, stderr bytes.Buffer
		code := RunWithInput([]string{"content", "--root=" + roots[0], "--request=-", "--json"}, bytes.NewReader(raw), &stdout, &stderr)
		request := httptest.NewRequest("POST", "/v1/content", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		var a, b envelope
		if json.Unmarshal(stdout.Bytes(), &a) != nil || json.Unmarshal(recorder.Body.Bytes(), &b) != nil || stderr.Len() != 0 {
			t.Fatal(stdout.String(), stderr.String(), recorder.Body.String())
		}
		if (code == 0) != a.OK || (recorder.Code == 200) != b.OK {
			t.Fatal(code, recorder.Code)
		}
		for _, value := range []*envelope{&a, &b} {
			if value.Result.Receipt != nil {
				value.Result.Receipt.Epoch = ""
				value.Result.Receipt.TreeGeneration = 0
			}
		}
		// Each independent source records its actual UTC deletion time. Compare
		// receipts after verifying each hash against its own tombstone.
		if r.Action == "document.delete" && a.OK && b.OK {
			var records [2]sourcetx.DocumentTombstone
			for i, value := range []*envelope{&a, &b} {
				name := "tombstones/documents/" + r.PageID + "/" + r.BoxID + "/" + r.DocID + ".json"
				raw, err := os.ReadFile(filepath.Join(roots[i], filepath.FromSlash(name)))
				if err != nil || sourcetx.Hash(raw) != value.Result.Hash || value.Result.Receipt.Outcome.Hash != value.Result.Hash {
					t.Fatalf("tombstone for handler %d does not match receipt: %v", i, err)
				}
				records[i], err = sourcetx.DecodeDocumentTombstone(raw, name)
				if err != nil {
					t.Fatal(err)
				}
				records[i].DeletedAt = ""
				value.Result.Hash, value.Result.Receipt.Outcome.Hash, value.Result.Receipt.RequestHash = "", "", ""
			}
			if !reflect.DeepEqual(records[0], records[1]) {
				t.Fatal(records)
			}
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatal("handler mismatch", stdout.String(), recorder.Body.String())
		}
		return a
	}
	page, box, doc, block := "1760000000000000001", "20261019120000-box0002", "20261019120001-doc0005", "20261019120002-par0006"
	baseRequest := wikiops.ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: page, BoxID: box, DocID: doc}
	put := baseRequest
	put.Action = "document.put"
	put.OperationID = "parity-create"
	put.NotebookName = "Notebook"
	put.AST = json.RawMessage(`{"ID":"` + doc + `","Type":"NodeDocument","Future":{"big":900719925474099312345},"Properties":{"title":"Title"},"Children":[]}`)
	put.DryRun = true
	if r := call(put); !r.OK || r.Result.Receipt.Status != "dry_run" {
		t.Fatal(r)
	}
	put.DryRun = false
	if r := call(put); !r.OK {
		t.Fatal(r)
	}
	if r := call(put); !r.OK {
		t.Fatal(r)
	}
	get := baseRequest
	get.Action = "document.get"
	current := call(get).Result
	create := baseRequest
	create.Action = "block.create"
	create.BlockID = block
	create.ParentID = doc
	create.ExpectedHash = current.Hash
	create.OperationID = "parity-block-create"
	create.AST = json.RawMessage(`{"ID":"` + block + `","Type":"NodeParagraph","Opaque":{"n":900719925474099312345},"Children":[{"Type":"NodeText","Data":"Agent text"}]}`)
	if r := call(create); !r.OK {
		t.Fatal(r)
	}
	if r := call(create); !r.OK {
		t.Fatal(r)
	}
	current = call(get).Result
	read := baseRequest
	read.Action = "block.get"
	read.BlockID = block
	read.Format = "text"
	if r := call(read); r.Result.Text != "Agent text\n" {
		t.Fatal(r)
	}
	replace := create
	replace.Action = "block.replace"
	replace.ParentID = ""
	replace.OperationID = "parity-block-replace"
	replace.ExpectedHash = current.Hash
	replace.AST = json.RawMessage(strings.Replace(string(create.AST), "Agent text", "Changed text", 1))
	if r := call(replace); !r.OK {
		t.Fatal(r)
	}
	current = call(get).Result
	remove := baseRequest
	remove.Action = "block.delete"
	remove.BlockID = block
	remove.OperationID = "parity-block-delete"
	remove.ExpectedHash = current.Hash
	if r := call(remove); !r.OK {
		t.Fatal(r)
	}
	if r := call(remove); !r.OK {
		t.Fatal(r)
	}
	asset := wikiops.ContentRequest{ProtocolVersion: 1, Action: "asset.put", StoreID: id, PageID: page, AssetName: "test image.bin", Data: []byte{0, 1, 255}, OperationID: "parity-asset"}
	if r := call(asset); !r.OK {
		t.Fatal(r)
	}
	asset.Action = "asset.get"
	asset.OperationID = ""
	asset.Data = nil
	loaded := call(asset)
	if !loaded.OK {
		t.Fatal(loaded)
	}
	asset.Action = "asset.delete"
	asset.OperationID = "parity-asset-delete"
	asset.ExpectedHash = loaded.Result.Hash
	if r := call(asset); !r.OK {
		t.Fatal(r)
	}
	for _, action := range []string{"document.list", "search", "check"} {
		request := wikiops.ContentRequest{ProtocolVersion: 1, Action: action, StoreID: id, PageID: page}
		if r := call(request); !r.OK {
			t.Fatal(r)
		}
	}
	current = call(get).Result
	put.NotebookName = ""
	put.OperationID = "parity-update"
	put.ExpectedHash = current.Hash
	put.AST = json.RawMessage(strings.Replace(string(put.AST), "Title", "Updated", 1))
	if r := call(put); !r.OK {
		t.Fatal(r)
	}
	current = call(get).Result
	del := baseRequest
	del.Action = "document.delete"
	del.ExpectedHash = current.Hash
	del.OperationID = "parity-doc-delete"
	if r := call(del); !r.OK {
		t.Fatal(r)
	}
	if r := call(del); !r.OK {
		t.Fatal(r)
	}
	get.Format = "text"
	if r := call(get); r.OK || r.Error.Type != "not_found" {
		t.Fatal(r)
	}
}

func TestRecoveryHandlersAdmitHalfPublishedTreeAndRejectWrongStore(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{})
	defer m.Drain(ctx)
	id, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := os.ReadFile(filepath.Join(root, "pages/1760000000000000002/page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var page wikistore.Page
	_ = json.Unmarshal(meta, &page)
	page.PageID = "1760000000000000009"
	page.ParentID = nil
	page.Revision = 1
	meta, _ = json.Marshal(page)
	document := []byte(`{"ID":"` + page.EntryDocument.DocID + `","Type":"NodeDocument","Properties":{"title":"` + page.Name + `"},"Children":[]}`)
	for _, transport := range []string{"CLI", "HTTP"} {
		source, err := sourcetx.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		writer, err := source.Writer(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		tx := sourcetx.Request{OperationID: "recovery-" + transport, Changes: []sourcetx.Change{{Path: "pages/" + page.PageID + "/page.json", Data: meta}, {Path: "pages/" + page.PageID + "/data/" + page.EntryDocument.BoxID + "/" + page.EntryDocument.DocID + ".docj", Data: document}}}
		_, err = writer.Apply(ctx, tx, func(point string) error {
			if point == "published:0" {
				return errors.New("interrupted")
			}
			return nil
		})
		writer.Close()
		if err == nil {
			t.Fatal("fault not reached")
		}
		if _, err := (wikistore.Scanner{}).Scan(ctx, root); err == nil {
			t.Fatal("fixture did not produce a half-invalid tree")
		}
		bad, _ := json.Marshal(wikiops.RecoverRequest{ProtocolVersion: 1, StoreID: "536f1884-2299-4fde-88c1-9da8f26d2aac"})
		var out, stderr bytes.Buffer
		if code := RunWithInput([]string{"recover", "--root=" + root, "--request=-"}, bytes.NewReader(bad), &out, &stderr); code != 2 {
			t.Fatal("wrong store recovered", code)
		}
		if _, err := (wikistore.Scanner{}).Scan(ctx, root); err == nil {
			t.Fatal("wrong identity changed pending source")
		}
		beforePreview := inspectionSnapshot(t, root)
		previewRequest, _ := json.Marshal(wikiops.RecoverRequest{ProtocolVersion: 1, StoreID: id, DryRun: true})
		out.Reset()
		if code := RunWithInput([]string{"recover", "--root=" + root, "--request=-"}, bytes.NewReader(previewRequest), &out, &stderr); code != 0 {
			t.Fatal(code, out.String())
		}
		token := strings.Repeat("recover-preview-", 4)
		handler, _ := readhttp.NewOperations(m, token)
		previewHTTP := httptest.NewRequest("POST", "/v1/recover", bytes.NewReader(previewRequest))
		previewHTTP.Header.Set("Content-Type", "application/json")
		previewHTTP.Header.Set("Authorization", "Bearer "+token)
		recorded := httptest.NewRecorder()
		handler.ServeHTTP(recorded, previewHTTP)
		if recorded.Code != 200 || recorded.Body.String() != out.String() {
			t.Fatal(recorded.Code, recorded.Body.String(), out.String())
		}
		if !reflect.DeepEqual(beforePreview, inspectionSnapshot(t, root)) {
			t.Fatal("preview changed the half-published source")
		}
		wrongPreview, _ := json.Marshal(wikiops.RecoverRequest{ProtocolVersion: 1, StoreID: "536f1884-2299-4fde-88c1-9da8f26d2aac", DryRun: true})
		out.Reset()
		if code := RunWithInput([]string{"recover", "--root=" + root, "--request=-"}, bytes.NewReader(wrongPreview), &out, &stderr); code != 2 {
			t.Fatal(code, out.String())
		}
		request := httptest.NewRequest("POST", "/v1/recover", bytes.NewReader(wrongPreview))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		recorded = httptest.NewRecorder()
		handler.ServeHTTP(recorded, request)
		var localErr, serverErr struct{ Error wikiops.ServiceError }
		if recorded.Code != 400 || json.Unmarshal(out.Bytes(), &localErr) != nil ||
			json.Unmarshal(recorded.Body.Bytes(), &serverErr) != nil || localErr != serverErr ||
			localErr.Error.Type != "invalid_workspace_identity" {
			t.Fatal(recorded.Code, recorded.Body.String(), out.String())
		}
		raw, _ := json.Marshal(wikiops.RecoverRequest{ProtocolVersion: 1, StoreID: id})
		if transport == "CLI" {
			out.Reset()
			if code := RunWithInput([]string{"recover", "--root=" + root, "--request=-"}, bytes.NewReader(raw), &out, &stderr); code != 0 {
				t.Fatal(code, out.String(), stderr.String())
			}
		} else {
			token := strings.Repeat("recover-test-", 4)
			handler, _ := readhttp.NewOperations(m, token)
			request := httptest.NewRequest("POST", "/v1/recover", bytes.NewReader(raw))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != 200 {
				t.Fatal(recorder.Code, recorder.Body.String())
			}
		}
		if _, err := (wikistore.Scanner{}).Scan(ctx, root); err != nil {
			t.Fatal("recovery left invalid source", err)
		}
	}
}
