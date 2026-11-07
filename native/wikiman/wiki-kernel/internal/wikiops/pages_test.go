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

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

func TestStoreContentCheckResolvesCrossPageReferencesWithoutWriting(t *testing.T) {
	root, m, id := setupPages(t)
	ctx := context.Background()
	const sourcePage = "1760000000000000001"
	const otherPage = "1760000000000000004"
	const target = "20261019090002-par0001"
	put := func(page, child, op string) {
		t.Helper()
		_, err := Content(ctx, m, ContentRequest{
			ProtocolVersion: 1, StoreID: id, PageID: page, Action: "document.put",
			OperationID: op, BoxID: "20261019090000-box0001", DocID: "20261106090001-r10test", NotebookName: "Check fixture",
			AST: json.RawMessage(`{"ID":"20261106090001-r10test","Type":"NodeDocument","Properties":{"title":"Check fixture"},"Children":[` + child + `]}`),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	put(sourcePage, `{"ID":"20261106090002-r10test","Type":"NodeParagraph","Children":[{"Type":"NodeTextMark","TextMarkType":"block-ref","TextMarkBlockRefID":"`+target+`","TextMarkBlockRefSubtype":"d","TextMarkTextContent":"Target"}]}`, "check-source")
	check := func(want string) {
		t.Helper()
		before := snapshotFiles(t, root)
		result, err := Pages(ctx, m, PageRequest{ProtocolVersion: 1, StoreID: id, Action: "check", Scope: "content"})
		if want == "" {
			if err != nil {
				t.Fatal(result.Diagnostics, err)
			}
		} else {
			found := false
			for _, d := range result.Diagnostics {
				found = found || d.PageID == sourcePage && d.Code == want+target
			}
			if !errors.Is(err, docast.ErrDocument) || !found {
				t.Fatal(result.Diagnostics, err)
			}
		}
		if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
			t.Fatal("content check changed source or runtime files")
		}
	}
	check("")
	put(otherPage, `{"ID":"`+target+`","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"Duplicate target"}]}`, "check-duplicate")
	check("ambiguous_block_reference:")
	for _, page := range []string{otherPage, testPage} {
		v := pageGet(t, m, id, page)
		deleted := "2026-11-06T09:30:00Z"
		v.Page.DeletedAt = &deleted
		v.Page.UpdatedAt = deleted
		v.Page.Revision++
		_, err := Pages(ctx, m, PageRequest{
			ProtocolVersion: 1, StoreID: id, Action: "apply", OperationID: "check-deleted-" + page,
			Updates: []PageUpdate{{Page: v.Page, ExpectedHash: v.Hash, EntryExpectedHash: v.EntryHash}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if page == otherPage {
			check("")
		}
	}
	check("unresolved_block_reference:")
}

const testPage = "1760000000000000002"
const testDoc = "pages/" + testPage + "/data/20261019090000-box0001/20261019090001-doc0001.docj"

func setupPages(t *testing.T) (string, *workspace.WorkspaceManager, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "wiki ' ?#%& 中文")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(root, os.DirFS("../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	id, err := m.OpenStore(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Drain(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return root, m, id
}
func pageGet(t *testing.T, m *workspace.WorkspaceManager, id, page string) PageView {
	t.Helper()
	r, err := Pages(context.Background(), m, PageRequest{ProtocolVersion: 1, Action: "get", StoreID: id, PageID: page})
	if err != nil || len(r.Pages) != 1 {
		t.Fatal(r, err)
	}
	return r.Pages[0]
}
func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, name)
		result[relative] = sourcetx.Hash(raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPageDryRunRenamePreservesUnknownASTAndDeduplicates(t *testing.T) {
	root, m, id := setupPages(t)
	name := filepath.Join(root, filepath.FromSlash(testDoc))
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately opaque fields and an integer not exactly representable by
	// float64 must survive the service's title-only edit.
	raw = []byte(strings.Replace(string(raw), `"Spec": "1",`, `"Spec": "1", "Opaque": {"number":900719925474099312345,"future":[{"X":true}]},`, 1))
	if err := os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	before := pageGet(t, m, id, testPage)
	update := before.Page
	update.Name = "Renamed 未知字段"
	update.Revision++
	update.UpdatedAt = "2026-10-19T02:00:00Z"
	req := PageRequest{ProtocolVersion: 1, Action: "apply", StoreID: id, OperationID: "page-rename-test", DryRun: true, Updates: []PageUpdate{{Page: update, ExpectedHash: before.Hash, EntryExpectedHash: before.EntryHash}}}
	files := snapshotFiles(t, root)
	r, err := Pages(context.Background(), m, req)
	if err != nil || r.Receipt == nil || r.Receipt.Status != "dry_run" || r.Receipt.SourceCommitted {
		t.Fatal(r, err)
	}
	if !reflect.DeepEqual(files, snapshotFiles(t, root)) {
		t.Fatal("preview changed files, journals or index")
	}
	invalid := req
	invalid.Updates = append([]PageUpdate(nil), req.Updates...)
	invalid.Updates[0].Page.ParentID = &invalid.Updates[0].Page.PageID
	if _, err := Pages(context.Background(), m, invalid); err == nil {
		t.Fatal("preview accepted a cycle")
	}
	if !reflect.DeepEqual(files, snapshotFiles(t, root)) {
		t.Fatal("invalid preview changed files")
	}
	req.DryRun = false
	r, err = Pages(context.Background(), m, req)
	if err != nil || !r.Receipt.SourceCommitted || !r.Receipt.IndexFresh {
		t.Fatal(r, err)
	}
	after := pageGet(t, m, id, testPage)
	if after.Page.Name != update.Name || after.Page.Revision != update.Revision {
		t.Fatal(after)
	}
	changed, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var a, b map[string]json.RawMessage
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal(changed, &b) != nil {
		t.Fatal("invalid AST")
	}
	var properties map[string]json.RawMessage
	if json.Unmarshal(b["Properties"], &properties) != nil || string(properties["title"]) != `"Renamed 未知字段"` {
		t.Fatal(string(changed))
	}
	delete(a, "Properties")
	delete(b, "Properties")
	canonicalA, _ := json.Marshal(a)
	canonicalB, _ := json.Marshal(b)
	if string(canonicalA) != string(canonicalB) {
		t.Fatal("lost unknown AST", string(canonicalA), string(canonicalB))
	}
	if !strings.Contains(string(changed), "900719925474099312345") {
		t.Fatal("rounded unknown number")
	}
	r2, err := Pages(context.Background(), m, req)
	if err != nil || !reflect.DeepEqual(r.Receipt, r2.Receipt) {
		t.Fatal("idempotent retry", r2, err)
	}
	req.OperationID = "stale-version"
	if _, err := Pages(context.Background(), m, req); !errors.Is(err, sourcetx.ErrConflict) {
		t.Fatal("stale CAS accepted", err)
	}
	indexed, err := Pages(context.Background(), m, PageRequest{ProtocolVersion: 1, Action: "index", StoreID: id, Scope: "all"})
	if err != nil || indexed.Index == nil || !indexed.Index.ContentFresh || !indexed.Index.TreeFresh {
		t.Fatal(indexed, err)
	}
}

func TestPagesCreateMoveSoftDeleteRestoreAndPagination(t *testing.T) {
	_, m, id := setupPages(t)
	ctx := context.Background()
	list := PageRequest{ProtocolVersion: 1, Action: "list", StoreID: id, Limit: 1}
	first, err := Pages(ctx, m, list)
	if err != nil || first.Total != 4 || len(first.Pages) != 1 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	list.Cursor = first.NextCursor
	next, err := Pages(ctx, m, list)
	if err != nil || len(next.Pages) != 1 || next.Pages[0].Page.PageID == first.Pages[0].Page.PageID {
		t.Fatal(next, err)
	}
	page := wikistore.Page{SchemaVersion: 1, PageID: "1760000000000000005", Name: "New Wiki", Rank: "a1", Type: "wiki", Revision: 1, CreatedAt: "2026-10-19T02:00:00Z", UpdatedAt: "2026-10-19T02:00:00Z", Ext: json.RawMessage(`{"opaque":true}`)}
	apply := func(op string, p wikistore.Page, hash string) {
		t.Helper()
		r, err := Pages(ctx, m, PageRequest{ProtocolVersion: 1, Action: "apply", StoreID: id, OperationID: op, Updates: []PageUpdate{{Page: p, ExpectedHash: hash}}})
		if err != nil || !r.Receipt.SourceCommitted {
			t.Fatal(r, err)
		}
	}
	apply("create-page", page, "")
	if _, err := Pages(ctx, m, list); !errors.Is(err, sourcetx.ErrCursor) {
		t.Fatal("old cursor accepted", err)
	}
	current := pageGet(t, m, id, page.PageID)
	parent := "1760000000000000001"
	page.ParentID = &parent
	page.Revision++
	apply("move-page", page, current.Hash)
	current = pageGet(t, m, id, page.PageID)
	if !strings.Contains(current.Path, "New Wiki") || current.Page.ParentID == nil {
		t.Fatal(current)
	}
	now := "2026-10-19T03:00:00Z"
	page.DeletedAt = &now
	page.Revision++
	apply("delete-page", page, current.Hash)
	filtered, err := Pages(ctx, m, PageRequest{ProtocolVersion: 1, Action: "list", StoreID: id, Query: "new wiki"})
	if err != nil || filtered.Total != 0 {
		t.Fatal(filtered, err)
	}
	current = pageGet(t, m, id, page.PageID)
	page.DeletedAt = nil
	page.Revision++
	apply("restore-page", page, current.Hash)
	filtered, err = Pages(ctx, m, PageRequest{ProtocolVersion: 1, Action: "list", StoreID: id, Query: "NEW WIKI"})
	if err != nil || filtered.Total != 1 {
		t.Fatal(filtered, err)
	}
}

func TestPageRequestStrictSchemaAndIrrelevantOptions(t *testing.T) {
	for _, raw := range []string{
		`{"protocolVersion":1,"action":"list","storeId":"x","storeId":"y"}`,
		`{"protocolVersion":1,"action":"list","storeId":"x","root":"/tmp"}`,
		`{"protocolVersion":1,"action":"list","storeId":"x","dryRun":true}`,
		`{"protocolVersion":1,"action":"list","storeId":"x","updates":[] } {}`,
		`{"protocolVersion":2,"action":"list","storeId":"x"}`,
		`{"protocolVersion":1,"action":"index","storeId":"x","scope":"bad"}`,
		`null`,
	} {
		if _, err := DecodePageRequest(strings.NewReader(raw)); !errors.Is(err, ErrArgument) {
			t.Fatal(raw, err)
		}
	}
}

func TestPagesRejectForeignStoreIdentityAfterRegistration(t *testing.T) {
	root, m, id := setupPages(t)
	name := filepath.Join(root, "store.json")
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), id, "536f1884-2299-4fde-88c1-9da8f26d2aac", 1))
	if err := os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = Pages(context.Background(), m, PageRequest{ProtocolVersion: 1, Action: "list", StoreID: id})
	if !errors.Is(err, sourcetx.ErrIdentity) {
		t.Fatal("read another store under registered identity", err)
	}
}
