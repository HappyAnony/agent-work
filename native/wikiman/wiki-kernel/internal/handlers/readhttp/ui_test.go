package readhttp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func TestR06OriginalUIProviderScopeAndPayload(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	t.Cleanup(func() { _ = m.Drain(ctx) })
	store, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	session, _ := NewSession(m, strings.Repeat("synthetic-", 4))
	defer session.Close()
	generation := session.BindStore(root, store)
	const a, b = "1760000000000000002", "1760000000000000004"
	const box, doc, block = "20261019090000-box0001", "20261019090001-doc0001", "20261019090002-par0001"
	fixture := filepath.Join(root, "pages", a, "data", box, doc, "assets")
	if err = os.MkdirAll(fixture, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fixture, "child.txt"), []byte("owned asset"), 0600); err != nil {
		t.Fatal(err)
	}
	seed := wikiops.ContentRequest{ProtocolVersion: 1, StoreID: store, PageID: b, Action: "document.put", BoxID: box, DocID: doc,
		OperationID: "r06-second-page", NotebookName: "Second notebook",
		AST: json.RawMessage(`{"Type":"NodeDocument","ID":"` + doc + `","Properties":{"title":"Second"},"Children":[{"Type":"NodeParagraph","ID":"` + block + `","Children":[{"Type":"NodeText","Data":"Second page body"},{"Type":"NodeTextMark","TextMarkType":"tag","TextMarkTextContent":"Alpha"},{"Type":"NodeTextMark","TextMarkType":"tag","TextMarkTextContent":"Zulu"}]}]}`)}
	if _, err = wikiops.Content(ctx, m, seed); err != nil {
		t.Fatal(err)
	}
	send := func(method, route, ticket, origin string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://127.0.0.1"+route, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Wiki-Session", generation)
		if ticket == "" {
			r.Header.Set("Authorization", "Bearer "+session.token)
		} else {
			r.Header.Set("X-Wiki-Editor", ticket)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		session.ServeHTTP(rec, r)
		return rec
	}
	open := func(page, mode string) EditorView {
		rec := send("POST", "/v1/editor", "", "", editorRequest{ProtocolVersion: 1, StoreID: store, PageID: page, Action: "open", Mode: mode})
		var v struct{ Result EditorView }
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &v) != nil || v.Result.ViewID == "" {
			t.Fatal(rec.Code, rec.Body.String())
		}
		return v.Result
	}
	first, second := open(a, "docxSingle"), open(b, "wikiDesktop")
	query := func(view EditorView, endpoint, page string, args map[string]any) *httptest.ResponseRecorder {
		return send("POST", "/v1/ui", view.Ticket, "http://127.0.0.1", map[string]any{
			"protocolVersion": 1, "storeId": store, "pageId": page, "endpoint": endpoint, "data": args,
		})
	}
	for _, pair := range []struct {
		view EditorView
		text string
	}{{first, "Synthetic knowledge"}, {second, "Second page body"}} {
		rec := query(pair.view, "/api/filetree/getDoc", pair.view.PageID, map[string]any{"id": doc, "size": 100})
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), pair.text) || !strings.Contains(rec.Body.String(), pair.view.ViewID) {
			t.Fatal(rec.Code, rec.Body.String())
		}
		cfg := query(pair.view, "/api/system/getConf", pair.view.PageID, map[string]any{})
		if cfg.Code != 200 || strings.Contains(cfg.Body.String(), "accessAuthCode") ||
			strings.Contains(cfg.Body.String(), root) || strings.Contains(cfg.Body.String(), "singleNotebookRootDocID") != (pair.view.Mode == "docxSingle") {
			t.Fatal(cfg.Code, cfg.Body.String())
		}
	}
	args := map[string]any{"id": block}
	if rec := query(first, "/api/block/getBlockDOM", a, args); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Synthetic knowledge") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/block/getBlockSiblingID", a, args); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"parent":"`+doc+`"`) {
		t.Fatal("original sibling response", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/block/checkBlockFold", a, args); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"isRoot":false`) {
		t.Fatal("original fold response", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/block/getTreeStat", a, map[string]any{"id": doc, "reqId": 12}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"reqId":12`) || !strings.Contains(rec.Body.String(), `"wordCount":`) {
		t.Fatal("original statistics response", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/block/getBlockDOM", b, args); rec.Code != 401 {
		t.Fatal("cross-page UI request", rec.Code)
	}
	if rec := query(EditorView{Ticket: "forged"}, "/api/system/getConf", a, map[string]any{}); rec.Code != 401 {
		t.Fatal("forged UI view", rec.Code)
	}
	if rec := query(EditorView{}, "/api/system/getConf", a, map[string]any{}); rec.Code != 401 {
		t.Fatal("host credential accepted as a view", rec.Code)
	}
	if rec := query(first, "/api/filetree/listDocsByPath", a, map[string]any{"notebook": box, "path": "/"}); rec.Code != 401 {
		t.Fatal("single mode listed the notebook", rec.Code)
	}
	templates := filepath.Join(root, "pages", a, "data", "templates")
	if err = os.MkdirAll(templates, 0700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"entry.md":      "Single template witness .action{.title} .action{(getBlock \"" + block + "\").Content}",
		"query.md":      `.action{range querySQL "SELECT content FROM blocks"} .action{.content} .action{end}`,
		"outside.md":    `.action{querySQL "DELETE FROM blocks"}`,
		"outside-id.md": `.action{(getBlock "20261106090000-outside").Content}`,
	} {
		if err = os.WriteFile(filepath.Join(templates, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	entryFile := filepath.Join(root, "pages", a, "data", box, doc+".docj")
	sibling := filepath.Join(filepath.Dir(entryFile), "20261106090000-outside.docj")
	if err = os.WriteFile(sibling, []byte(`{"Type":"NodeDocument","ID":"20261106090000-outside","Properties":{"title":"Sibling secret"},"Children":[{"Type":"NodeParagraph","ID":"20261106090001-outside","Children":[{"Type":"NodeText","Data":"Sibling secret body"}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	entryBefore, err := os.ReadFile(entryFile)
	if err != nil {
		t.Fatal(err)
	}
	if rec := query(first, "/api/search/searchTemplate", a, map[string]any{"k": "entry"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "entry.md") {
		t.Fatal("single template list", rec.Code, rec.Body.String())
	}
	for _, preview := range []bool{true, false} {
		rec := query(first, "/api/template/render", a, map[string]any{"id": doc, "path": "templates/entry.md", "preview": preview})
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Single template witness") || !strings.Contains(rec.Body.String(), "Synthetic knowledge") {
			t.Fatal("single template", rec.Code, rec.Body.String())
		}
	}
	if rec := query(first, "/api/template/render", a, map[string]any{"id": doc, "path": "templates/query.md", "preview": true}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "Synthetic knowledge") || strings.Contains(rec.Body.String(), "Sibling secret") ||
		strings.Contains(rec.Body.String(), "Second page body") {
		t.Fatal("single template query saw content outside entry", rec.Code, rec.Body.String())
	}
	for _, name := range []string{"outside.md", "outside-id.md"} {
		if rec := query(first, "/api/template/render", a, map[string]any{"id": doc, "path": "templates/" + name, "preview": true}); rec.Code == 200 {
			t.Fatal("single template escaped entry scope", rec.Body.String())
		}
	}
	if rec := query(first, "/api/template/render", a, map[string]any{"id": block, "path": "templates/entry.md", "preview": true}); rec.Code != 401 {
		t.Fatal("single template accepted a non-entry target", rec.Code, rec.Body.String())
	}
	if entryAfter, e := os.ReadFile(entryFile); e != nil || !bytes.Equal(entryBefore, entryAfter) {
		t.Fatal("template preview or rendering published the source", e)
	}
	for _, tc := range []struct {
		id, root, keyword, witness string
		status                     int
	}{
		{doc, doc, "Synthetic", block, 200},
		{doc, doc, "Sibling", `"blocks":[]`, 200},
		{doc, doc, "Second", `"blocks":[]`, 200},
		{"20261106090000-outside", doc, "Synthetic", block, 200},
		{doc, "20261106090000-outside", "Synthetic", "", 401},
	} {
		rec := query(first, "/api/search/searchRefBlock", a, map[string]any{
			"id": tc.id, "rootID": tc.root, "k": tc.keyword, "beforeLen": 24,
		})
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.witness) ||
			strings.Contains(rec.Body.String(), "Sibling secret") || strings.Contains(rec.Body.String(), "Second page body") {
			t.Fatal("single reference candidates", tc, rec.Code, rec.Body.String())
		}
	}
	if rec := query(second, "/api/filetree/listDocsByPath", b, map[string]any{"notebook": box, "path": "/"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Second") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	tagTree := map[string]any{"sort": 1, "ignoreMaxListHint": true, "app": "test"}
	if rec := query(first, "/api/tag/getTag", a, tagTree); rec.Code != 401 {
		t.Fatal("single mode received page-wide tags", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/tag/getTag", b, tagTree); rec.Code != 200 ||
		strings.Index(rec.Body.String(), `"name":"Zulu"`) < 0 ||
		strings.Index(rec.Body.String(), `"name":"Alpha"`) <= strings.Index(rec.Body.String(), `"name":"Zulu"`) {
		t.Fatal("scoped tag tree order", rec.Code, rec.Body.String())
	}
	embed := map[string]any{"embedBlockID": doc, "includeIDs": []string{block}, "headingMode": 0}
	if rec := query(second, "/api/search/getEmbedBlock", b, embed); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "Second page body") || strings.Contains(rec.Body.String(), "Synthetic knowledge") {
		t.Fatal("owned embed projection", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/search/getEmbedBlock", a, embed); rec.Code != 401 {
		t.Fatal("single-document embed permission escaped", rec.Code, rec.Body.String())
	}
	queryEmbed := map[string]any{"embedBlockID": doc, "stmt": "SELECT * FROM blocks", "excludeIDs": []string{doc}, "breadcrumb": true}
	if rec := query(first, "/api/search/searchEmbedBlock", a, queryEmbed); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "Synthetic knowledge") || strings.Contains(rec.Body.String(), "Sibling secret") ||
		strings.Contains(rec.Body.String(), "Second page body") {
		t.Fatal("single SQL embed escaped the entry projection", rec.Code, rec.Body.String())
	}
	queryEmbed["stmt"] = "DELETE FROM blocks"
	if rec := query(first, "/api/search/searchEmbedBlock", a, queryEmbed); rec.Code == 200 {
		t.Fatal("single SQL embed accepted a write", rec.Body.String())
	}
	if after, e := os.ReadFile(entryFile); e != nil || !bytes.Equal(entryBefore, after) {
		t.Fatal("single SQL embed changed entry source", e)
	}
	if rec := query(second, "/api/file/getFile", b, map[string]any{"path": "/etc/passwd"}); rec.Code == 200 || !strings.Contains(rec.Body.String(), "ui_endpoint_unavailable") {
		t.Fatal("ambient source route", rec.Code, rec.Body.String())
	}
	for _, endpoint := range []string{
		"/api/bazaar/installBazaarPlugin", "/api/ai/chatGPT",
		"/api/account/login", "/api/sync/performBootSync",
	} {
		for _, view := range []EditorView{first, second} {
			rec := query(view, endpoint, view.PageID, map[string]any{})
			denial := "ui_endpoint_unavailable"
			if strings.HasPrefix(endpoint, "/api/account/") || strings.HasPrefix(endpoint, "/api/sync/") {
				denial = "unauthorized"
			}
			if rec.Code == 200 || !strings.Contains(rec.Body.String(), denial) {
				t.Fatal("unconfigured provider reachable from editor", endpoint, rec.Code, rec.Body.String())
			}
		}
	}
	get := seed
	get.Action, get.OperationID, get.NotebookName, get.AST = "document.get", "", "", nil
	current, err := wikiops.Content(ctx, m, get)
	if err != nil {
		t.Fatal(err)
	}
	edit := map[string]any{"boxId": box, "docId": doc, "operationId": "r06-original-edit", "expectedHash": current.Hash,
		"transactions": []any{
			map[string]any{"doOperations": []any{map[string]string{"action": "setAttrs", "id": block, "data": `{"custom-r06":"one"}`}}},
			map[string]any{"doOperations": []any{map[string]string{"action": "setAttrs", "id": block, "data": `{"custom-r06-second":"two"}`}}},
		},
	}
	if rec := query(first, "/api/transactions", a, edit); rec.Code != 401 {
		t.Fatal("single view received page-wide mutation capability", rec.Code)
	}
	if rec := query(second, "/api/transactions", b, edit); rec.Code != 200 || !strings.Contains(rec.Body.String(), "r06-original-edit") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	applied, err := wikiops.Content(ctx, m, get)
	if err != nil || !strings.Contains(string(applied.AST), "custom-r06-second") {
		t.Fatal(string(applied.AST), err)
	}
	history := get
	history.Action, history.OperationID, history.HistoryID, history.ExpectedHash = "document.history", "r06-original-undo", "r06-original-edit", applied.Hash
	if value, err := wikiops.Content(ctx, m, history); err != nil || value.Hash != current.Hash {
		t.Fatal("exact R04 history after original UI batch", value.Hash, err)
	}
	cfg := query(second, "/api/system/getConf", b, map[string]any{})
	var scoped struct {
		Result struct {
			Data struct {
				Conf struct{ FileTree map[string]any }
			}
		}
	}
	if err = json.Unmarshal(cfg.Body.Bytes(), &scoped); err != nil || scoped.Result.Data.Conf.FileTree == nil {
		t.Fatal(cfg.Body.String(), err)
	}
	options := scoped.Result.Data.Conf.FileTree
	options["maxListCount"] = 27
	if rec := query(second, "/api/setting/setFiletree", b, options); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if cfg = query(second, "/api/system/getConf", b, map[string]any{}); !strings.Contains(cfg.Body.String(), `"maxListCount":27`) {
		t.Fatal("new preference did not load", cfg.Body.String())
	}
	if cfg = query(first, "/api/system/getConf", a, map[string]any{}); strings.Contains(cfg.Body.String(), `"maxListCount":27`) {
		t.Fatal("workspace preference escaped its page", cfg.Body.String())
	}
	options["openFilesUseCurrentTab"] = !options["openFilesUseCurrentTab"].(bool)
	if rec := query(second, "/api/setting/setFiletree", b, options); rec.Code != 401 {
		t.Fatal("scoped preference edited device field", rec.Code, rec.Body.String())
	}
	clipboard := query(second, "/api/lute/html2BlockDOM", b, map[string]any{"dom": "<p><strong>Scoped clipboard</strong></p>"})
	if clipboard.Code != 200 || !strings.Contains(clipboard.Body.String(), "Scoped clipboard") {
		t.Fatal(clipboard.Code, clipboard.Body.String())
	}
	file := query(second, "/api/lute/html2BlockDOM", b, map[string]any{"dom": `<img src="file:///etc/passwd">`})
	if file.Code == 200 && strings.Contains(file.Body.String(), "file:///etc/passwd") {
		t.Fatal("clipboard retained a host file path", file.Body.String())
	}
	asset := wikiops.ContentRequest{ProtocolVersion: 1, StoreID: store, PageID: b, Action: "asset.put",
		OperationID: "r06-annotation-asset", AssetName: "test.pdf", Data: []byte("fixture only")}
	if _, err = wikiops.Content(ctx, m, asset); err != nil {
		t.Fatal(err)
	}
	if rec := query(second, "/api/asset/setFileAnnotation", b, map[string]any{"path": "assets/test.pdf.sya", "data": `{"example":{"page":1}}`}); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/asset/getFileAnnotation", b, map[string]any{"path": "assets/test.pdf.sya"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "example") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/asset/getFileAnnotation", a, map[string]any{"path": "assets/test.pdf.sya"}); rec.Code == 200 {
		t.Fatal("annotation escaped its page", rec.Body.String())
	}

	const empty = "1760000000000000001"
	emptyView := open(empty, "wikiDesktop")
	created := query(emptyView, "/api/notebook/createNotebook", empty, map[string]any{"name": "Explicit notebook"})
	var notebook struct {
		Result struct {
			Data struct {
				Notebook struct{ ID string }
			}
		}
	}
	if created.Code != 200 || json.Unmarshal(created.Body.Bytes(), &notebook) != nil || notebook.Result.Data.Notebook.ID == "" {
		t.Fatal("empty page notebook creation", created.Code, created.Body.String())
	}
	newBox := notebook.Result.Data.Notebook.ID
	const newDocument = "20261019092000-uid0001"
	explicit := query(emptyView, "/api/filetree/createDoc", empty, map[string]any{"notebook": newBox,
		"path": "/" + newDocument + ".docj", "title": "Explicit document", "md": "A scoped note."})
	if explicit.Code != 200 || !strings.Contains(explicit.Body.String(), newDocument) {
		t.Fatal("explicit empty-page document", explicit.Code, explicit.Body.String())
	}
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	archiveEntry, err := zipWriter.Create("Export/" + doc + ".docj")
	if err != nil {
		t.Fatal(err)
	}
	archiveDoc := []byte(`{"ID":"` + doc + `","Type":"NodeDocument","Spec":"1","Properties":{"title":"Archive fixture"},"Children":[{"ID":"` +
		block + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"Imported body"}]}]}`)
	if _, err = archiveEntry.Write(archiveDoc); err != nil {
		t.Fatal(err)
	}
	if err = zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	importArgs, err := json.Marshal(map[string]any{"notebook": newBox, "toPath": "/" + newDocument + ".docj", "data": archive.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	importDoc := func(view EditorView, page string) *httptest.ResponseRecorder {
		return send("POST", "/v1/ui", view.Ticket, "http://127.0.0.1", wikiops.UIRequest{
			ProtocolVersion: 1, StoreID: store, PageID: page, Endpoint: "/api/import/importSY",
			OperationID: "r06-ui-archive", Data: importArgs,
		})
	}
	if rec := importDoc(first, a); rec.Code != 401 {
		t.Fatal("single-document import escaped its entry", rec.Code, rec.Body.String())
	}
	if rec := importDoc(emptyView, empty); rec.Code != 200 {
		t.Fatal("original import", rec.Code, rec.Body.String())
	}
	imported, err := os.ReadDir(filepath.Join(root, "pages", empty, "data", newBox, newDocument))
	if err != nil || len(imported) != 1 || !strings.HasSuffix(imported[0].Name(), ".docj") {
		t.Fatal("archive did not land beneath its owned parent", imported, err)
	}
	var markdown bytes.Buffer
	markdownZIP := zip.NewWriter(&markdown)
	for name, contents := range map[string]string{
		"Root.md":           "# Markdown root\n\n[Child](Notes/Child.md#Child heading) [Attachment](Notes/fixture.txt)",
		"Notes/Child.md":    "# Child heading\n\nNested Markdown text",
		"Notes/fixture.txt": "Markdown attachment",
	} {
		entry, e := markdownZIP.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = entry.Write([]byte(contents)); e != nil {
			t.Fatal(e)
		}
	}
	if err = markdownZIP.Close(); err != nil {
		t.Fatal(err)
	}
	markdownImport := func(view EditorView, page, endpoint, operation string, fields map[string]any) *httptest.ResponseRecorder {
		args, e := json.Marshal(fields)
		if e != nil {
			t.Fatal(e)
		}
		return send("POST", "/v1/ui", view.Ticket, "http://127.0.0.1", wikiops.UIRequest{
			ProtocolVersion: 1, StoreID: store, PageID: page, Endpoint: endpoint, OperationID: operation, Data: args,
		})
	}
	zipArgs := map[string]any{"notebook": newBox, "toPath": "/" + newDocument + ".docj", "data": markdown.Bytes(), "name": "Nested.zip"}
	if rec := markdownImport(first, a, "/api/import/importZipMd", "r06-markdown-zip", zipArgs); rec.Code != 401 {
		t.Fatal("single document accepted Markdown ZIP", rec.Code, rec.Body.String())
	}
	for range 2 {
		if rec := markdownImport(emptyView, empty, "/api/import/importZipMd", "r06-markdown-zip", zipArgs); rec.Code != 200 {
			t.Fatal("Markdown ZIP import", rec.Code, rec.Body.String())
		}
	}
	markdownRoot := filepath.Join(root, "pages", empty, "data")
	docs, err := filepath.Glob(filepath.Join(markdownRoot, newBox, newDocument, "*", "*.docj"))
	if err != nil || len(docs) != 2 {
		t.Fatal("Markdown ZIP did not create precisely the document and folder", docs, err)
	}
	var body []byte
	for _, file := range docs {
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		if bytes.Contains(raw, []byte("Markdown root")) {
			body = raw
		}
	}
	if !bytes.Contains(body, []byte("siyuan://blocks/")) || !bytes.Contains(body, []byte("fixture.txt")) {
		t.Fatal("Markdown links did not resolve into the private import", string(body))
	}
	assets, err := filepath.Glob(filepath.Join(markdownRoot, "assets", "*-fixture.txt"))
	if err != nil || len(assets) != 1 {
		t.Fatal("Markdown attachment publication", assets, err)
	}
	if rec := markdownImport(emptyView, empty, "/api/import/importStdMd", "r06-markdown-local", map[string]any{
		"notebook": newBox, "toPath": "/", "files": map[string][]byte{"Picked.md": []byte("Local picker Markdown")},
	}); rec.Code != 200 {
		t.Fatal("Markdown file picker import", rec.Code, rec.Body.String())
	}
	if rec := markdownImport(emptyView, empty, "/api/import/importStdMd", "r06-markdown-traversal", map[string]any{
		"notebook": newBox, "toPath": "/", "files": map[string][]byte{"../escape.md": []byte("invalid")},
	}); rec.Code == 200 {
		t.Fatal("Markdown accepted path traversal")
	}
	paths := []string{"/20261019092001-uid0002.docj", "/20261019092002-uid0003.docj"}
	for _, name := range paths {
		if rec := query(emptyView, "/api/filetree/createDoc", empty, map[string]any{"notebook": newBox, "path": name, "title": "Batch fixture"}); rec.Code != 200 {
			t.Fatal("seed batch fixture", rec.Code, rec.Body.String())
		}
	}
	move := map[string]any{"fromPaths": []string{paths[0], "/20261019092003-absent1.docj"}, "toNotebook": newBox, "toPath": "/" + newDocument + ".docj"}
	if rec := query(emptyView, "/api/filetree/moveDocs", empty, move); rec.Code == 200 {
		t.Fatal("batch with missing source was accepted")
	}
	base := filepath.Join(root, "pages", empty, "data", newBox)
	for _, name := range paths {
		if _, err = os.Stat(filepath.Join(base, name)); err != nil {
			t.Fatal("failed batch moved a selected document", err)
		}
	}
	move["fromPaths"] = paths
	if rec := query(emptyView, "/api/filetree/moveDocs", empty, move); rec.Code != 200 {
		t.Fatal("original batch move", rec.Code, rec.Body.String())
	}
	for _, name := range paths {
		if _, err = os.Stat(filepath.Join(base, newDocument, name)); err != nil {
			t.Fatal("selected document not moved", err)
		}
	}
	selected := []string{"/" + newDocument + paths[0], "/" + newDocument + paths[1]}
	if rec := query(emptyView, "/api/filetree/removeDocs", empty, map[string]any{"paths": selected}); rec.Code != 200 {
		t.Fatal("original batch delete", rec.Code, rec.Body.String())
	}
	for i, name := range selected {
		if _, err = os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
			t.Fatal("batch delete left a selected document", err)
		}
		id := strings.TrimSuffix(strings.TrimPrefix(paths[i], "/"), ".docj")
		listed, err := wikiops.Content(ctx, m, wikiops.ContentRequest{ProtocolVersion: 1, StoreID: store, PageID: empty,
			Action: "document.deleted", BoxID: newBox})
		if err != nil || !slices.ContainsFunc(listed.Items, func(item wikiops.ContentItem) bool { return item.DocID == id }) {
			t.Fatal("batch delete did not retain individual recovery records", listed, err)
		}
	}
	if rec := query(second, "/api/storage/setLocalStorageVal", b, map[string]any{"app": "fixture", "key": "local-codelang", "val": "rust"}); rec.Code != 200 {
		t.Fatal("scoped local write", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/storage/getLocalStorage", b, map[string]any{}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"local-codelang":"rust"`) {
		t.Fatal("owned local read", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/storage/getLocalStorage", a, map[string]any{}); rec.Code != 200 || strings.Contains(rec.Body.String(), "rust") {
		t.Fatal("local preference leaked across pages", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/system/setUILayout", b, map[string]any{"layout": map[string]any{"r06Layout": true}}); rec.Code != 200 {
		t.Fatal("original layout write", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/system/getConf", b, map[string]any{}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "r06Layout") {
		t.Fatal("original layout reload", rec.Code, rec.Body.String())
	}
	if rec := query(emptyView, "/api/system/getConf", empty, map[string]any{}); rec.Code != 200 || strings.Contains(rec.Body.String(), "r06Layout") {
		t.Fatal("layout escaped its page", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/system/setAppearanceMode", a, map[string]any{"mode": 1}); rec.Code != 401 {
		t.Fatal("single view changed device preferences", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/system/setAppearanceMode", b, map[string]any{"mode": 1}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"mode":1`) {
		t.Fatal("original device mode write", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/setting/setKeymap", b, map[string]any{"data": map[string]any{"editor": map[string]any{"general": map[string]any{"undo": map[string]string{"custom": "⌘K", "default": "⌘Z"}}}}}); rec.Code != 200 {
		t.Fatal("original keymap write", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/system/getConf", a, map[string]any{}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"mode":1`) ||
		!strings.Contains(rec.Body.String(), "⌘K") || strings.Contains(rec.Body.String(), "r06Layout") {
		t.Fatal("device settings and view layout owners differ", rec.Code, rec.Body.String())
	}
	desktopEntry := open(a, "wikiDesktop")
	position := func(view EditorView, page, id string, top int) *httptest.ResponseRecorder {
		return query(view, "/api/storage/setLocalStorageVal", page, map[string]any{
			"key": "local-fileposition", "entryID": id, "val": map[string]any{"rootId": id, "scrollTop": top},
		})
	}
	if rec := position(desktopEntry, a, doc, 123); rec.Code != 200 {
		t.Fatal("desktop scroll preference", rec.Code, rec.Body.String())
	}
	if rec := position(first, a, doc, 456); rec.Code != 200 {
		t.Fatal("single scroll preference", rec.Code, rec.Body.String())
	}
	if rec := position(first, a, newDocument, 666); rec.Code == 200 {
		t.Fatal("single view stored a foreign document position")
	}
	if rec := query(desktopEntry, "/api/storage/setLocalStorageVal", a, map[string]any{
		"key": "local-dialogposition", "val": map[string]any{"search": map[string]any{"width": 420, "height": 360, "top": 80, "left": 40}},
	}); rec.Code != 200 {
		t.Fatal("dialog position", rec.Code, rec.Body.String())
	}
	reopened := open(a, "wikiDesktop")
	if rec := query(reopened, "/api/storage/getLocalStorage", a, map[string]any{}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"scrollTop":123`) || !strings.Contains(rec.Body.String(), `"width":420`) ||
		strings.Contains(rec.Body.String(), `"scrollTop":456`) {
		t.Fatal("desktop positions did not survive reopen in their own mode", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/storage/getLocalStorage", a, map[string]any{}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"scrollTop":456`) || strings.Contains(rec.Body.String(), `"width":420`) {
		t.Fatal("single position crossed mode", rec.Code, rec.Body.String())
	}
	tabLayout := `{"version":1,"focused":1,"root":{"direction":"horizontal","ratio":55,"children":[` +
		`{"active":0,"tabs":[{"boxId":"` + box + `","docId":"` + doc + `","pinned":false}]},` +
		`{"active":0,"tabs":[{"boxId":"` + box + `","docId":"` + doc + `","pinned":true}]}]}}`
	writeTabs := func(view EditorView, page, value string) *httptest.ResponseRecorder {
		return query(view, "/api/storage/setLocalStorageVal", page, map[string]any{"key": "local-desktop-tabs", "val": json.RawMessage(value)})
	}
	if rec := writeTabs(second, b, tabLayout); rec.Code != 200 {
		t.Fatal("desktop tab layout write", rec.Code, rec.Body.String())
	}
	if rec := writeTabs(second, b, strings.Replace(tabLayout, `"ratio":55`, `"ratio":99`, 1)); rec.Code == 200 {
		t.Fatal("invalid split ratio was accepted")
	}
	if rec := writeTabs(first, a, tabLayout); rec.Code == 200 {
		t.Fatal("single view stored desktop tabs")
	}
	if rec := query(open(b, "wikiDesktop"), "/api/storage/getLocalStorage", b, map[string]any{}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"local-desktop-tabs":`) || !strings.Contains(rec.Body.String(), `"ratio":55`) {
		t.Fatal("desktop tab layout did not survive reopen", rec.Code, rec.Body.String())
	}
	if rec := query(reopened, "/api/storage/getLocalStorage", a, map[string]any{}); rec.Code != 200 ||
		strings.Contains(rec.Body.String(), `"local-desktop-tabs":`) {
		t.Fatal("desktop tab layout crossed pages", rec.Code, rec.Body.String())
	}

	const conversionDoc = "20261019092300-cnv0001"
	const pivot = "20261019092301-par0001"
	const heading = "20261019092302-hdg0001"
	const bodyBlock = "20261019092303-par0002"
	const list = "20261019092304-lst0001"
	const listItem = "20261019092305-lit0001"
	const listBody = "20261019092306-par0003"
	structure := wikiops.ContentRequest{ProtocolVersion: 1, StoreID: store, PageID: empty, BoxID: newBox, DocID: conversionDoc,
		OperationID: "r06-structure-fixture", Action: "document.put", AST: json.RawMessage(`{"ID":"` + conversionDoc +
			`","Type":"NodeDocument","Spec":"1","Properties":{"title":"Conversions"},"Children":[
			{"ID":"` + pivot + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"Pivot"}]},
			{"ID":"` + list + `","Type":"NodeList","ListData":{"Typ":0,"Tight":true,"BulletChar":42,"Marker":"Kg=="},"Children":[
			{"ID":"` + listItem + `","Type":"NodeListItem","ListData":{"Typ":0,"Tight":true,"BulletChar":42,"Marker":"Kg=="},"Children":[
			{"ID":"` + listBody + `","Type":"NodeParagraph","Children":[{"Type":"NodeText","Data":"List body"}]}]}]},
			{"ID":"` + heading + `","Type":"NodeHeading","HeadingLevel":2,"Children":[{"Type":"NodeText","Data":"Heading title"}]},
			{"ID":"` + bodyBlock + `","Type":"NodeParagraph","R06Opaque":900719925474099312345,"Children":[{"Type":"NodeText","Data":"Heading body"}]}]}`)}
	if _, err = wikiops.Content(ctx, m, structure); err != nil {
		t.Fatal(err)
	}
	if rec := query(emptyView, "/api/filetree/heading2Doc", empty, map[string]any{"srcHeadingID": heading, "targetNoteBook": newBox, "targetPath": "/"}); rec.Code != 200 {
		t.Fatal("original heading conversion", rec.Code, rec.Body.String())
	}
	headingPath := filepath.Join(base, heading+".docj")
	converted, err := os.ReadFile(headingPath)
	if err != nil || !bytes.Contains(converted, []byte(bodyBlock)) || !bytes.Contains(converted, []byte("900719925474099312345")) {
		t.Fatal("converted heading lost block identity or opaque data", err, string(converted))
	}
	if rec := query(emptyView, "/api/filetree/doc2Heading", empty, map[string]any{"srcID": heading, "targetID": pivot, "after": true}); rec.Code != 200 {
		t.Fatal("original document conversion", rec.Code, rec.Body.String())
	}
	if _, err = os.Stat(headingPath); !os.IsNotExist(err) {
		t.Fatal("converted source document still exists", err)
	}
	if rec := query(emptyView, "/api/filetree/li2Doc", empty, map[string]any{"srcListItemID": listItem, "targetNoteBook": newBox, "targetPath": "/"}); rec.Code != 200 {
		t.Fatal("original list item conversion", rec.Code, rec.Body.String())
	}
	converted, err = os.ReadFile(filepath.Join(base, listItem+".docj"))
	if err != nil || !bytes.Contains(converted, []byte(listBody)) {
		t.Fatal("converted list item lost its block identity", err, string(converted))
	}

	notebookAsset := filepath.Join(base, "assets", "retained.txt")
	if err = os.MkdirAll(filepath.Dir(notebookAsset), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(notebookAsset, []byte("notebook asset"), 0600); err != nil {
		t.Fatal(err)
	}
	if rec := query(first, "/api/notebook/removeNotebook", a, map[string]any{"notebook": box}); rec.Code != 401 {
		t.Fatal("single view removed a notebook", rec.Code, rec.Body.String())
	}
	if rec := query(open(a, "wikiDesktop"), "/api/notebook/removeNotebook", a, map[string]any{"notebook": box}); rec.Code == 200 {
		t.Fatal("removed entry notebook", rec.Body.String())
	}
	if rec := query(emptyView, "/api/notebook/removeNotebook", empty, map[string]any{"notebook": newBox, "callback": "mountRemove"}); rec.Code != 200 {
		t.Fatal("original notebook remove", rec.Code, rec.Body.String())
	}
	if _, err = os.Stat(filepath.Join(base, newDocument+".docj")); !os.IsNotExist(err) {
		t.Fatal("notebook document still active", err)
	}
	if retained, e := os.ReadFile(filepath.Join(markdownRoot, "assets", "retained.txt")); e != nil || string(retained) != "notebook asset" {
		t.Fatal("notebook attachment was not retained", e)
	}
	if rec := query(emptyView, "/api/history/getNotebookHistory", empty, map[string]any{}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), newBox) || !strings.Contains(rec.Body.String(), "Explicit notebook") {
		t.Fatal("original notebook history", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/history/getNotebookHistory", b, map[string]any{}); rec.Code != 200 || strings.Contains(rec.Body.String(), newBox) {
		t.Fatal("notebook history crossed pages", rec.Code, rec.Body.String())
	}
	if rec := query(emptyView, "/api/history/rollbackNotebookHistory", empty, map[string]any{"historyPath": newBox}); rec.Code != 200 {
		t.Fatal("original notebook restore", rec.Code, rec.Body.String())
	}
	if restored, e := os.ReadFile(filepath.Join(base, listItem+".docj")); e != nil || !bytes.Equal(restored, converted) {
		t.Fatal("notebook restore changed document bytes", e)
	}

	const avID = "20261019092400-av00001"
	const avView = "20261019092401-view001"
	const avPrimary = "20261019092402-key0001"
	const avText = "20261019092403-key0002"
	rules := []*av.ViewFilter{{Column: avText, Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "R06 default value"}}}}
	database := &av.AttributeView{Spec: 4, ID: avID, Name: "R06 scoped AV", ViewID: avView,
		KeyValues: []*av.KeyValues{{Key: &av.Key{ID: avPrimary, Type: av.KeyTypeBlock}}, {Key: &av.Key{ID: avText, Type: av.KeyTypeText}}},
		Views: []*av.View{{ID: avView, Name: "R06 table", LayoutType: av.LayoutTypeTable, PageSize: 20, Filters: rules,
			Sorts: []*av.ViewSort{{Column: avText, Order: av.SortOrderDesc}},
			Table: &av.LayoutTable{BaseLayout: &av.BaseLayout{}, Columns: []*av.ViewTableColumn{
				{BaseField: &av.BaseField{ID: avPrimary}}, {BaseField: &av.BaseField{ID: avText}},
			}}}},
	}
	avFolder := filepath.Join(root, "pages", b, "data", "storage", "av")
	if err = os.MkdirAll(avFolder, 0700); err != nil {
		t.Fatal(err)
	}
	avPath := filepath.Join(avFolder, avID+".json")
	avData, err := json.Marshal(database)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(avPath, avData, 0600); err != nil {
		t.Fatal(err)
	}
	if rec := query(second, "/api/av/getAttributeViewFilterSort", b, map[string]any{"id": avID, "blockID": block}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "R06 default value") || !strings.Contains(rec.Body.String(), `"sorts":[{`) {
		t.Fatal("AV filter/sort shape", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/av/getAttributeViewAddingBlockDefaultValues", b, map[string]any{"avID": avID, "viewID": avView}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "R06 default value") {
		t.Fatal("AV insertion preview", rec.Code, rec.Body.String())
	}
	if rec := query(second, "/api/av/renderAttributeView", b, map[string]any{"id": avID, "viewID": avView}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"viewType":"table"`) {
		t.Fatal("AV render shape", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/av/renderAttributeView", a, map[string]any{"id": avID}); rec.Code != 401 {
		t.Fatal("AV render crossed view scope", rec.Code, rec.Body.String())
	}
	if raw, e := os.ReadFile(avPath); e != nil || !bytes.Equal(raw, avData) {
		t.Fatal("AV preview mutated source", e)
	}

	deckResult := query(second, "/api/riff/createRiffDeck", b, map[string]any{"name": "R06 original deck"})
	var deck struct {
		Result struct{ Data struct{ ID string } }
	}
	if deckResult.Code != 200 || json.Unmarshal(deckResult.Body.Bytes(), &deck) != nil || deck.Result.Data.ID == "" {
		t.Fatal("original deck create", deckResult.Code, deckResult.Body.String())
	}
	deckID := deck.Result.Data.ID
	card := map[string]any{"deckID": deckID, "blockIDs": []string{block}}
	if rec := query(second, "/api/riff/addRiffCards", b, card); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"size":1`) {
		t.Fatal("original card add", rec.Code, rec.Body.String())
	}
	if rec := query(first, "/api/riff/addRiffCards", a, card); rec.Code != 401 {
		t.Fatal("single view altered another deck", rec.Code, rec.Body.String())
	}
	dueEndpoint := "/api/riff/getRiffDueCards"
	due := query(second, dueEndpoint, b, map[string]any{"deckID": deckID})
	var dueCards struct {
		Result struct {
			Data struct{ Cards []struct{ CardID string } }
		}
	}
	if due.Code != 200 || json.Unmarshal(due.Body.Bytes(), &dueCards) != nil || len(dueCards.Result.Data.Cards) != 1 {
		t.Fatal("original due cards", due.Code, due.Body.String())
	}
	cardID := dueCards.Result.Data.Cards[0].CardID
	reviewedCards := []map[string]string{{"cardID": cardID}}
	reviewInput := map[string]any{"deckID": deckID, "cardID": cardID, "rating": -3, "reviewedCards": reviewedCards}
	if rec := query(second, "/api/riff/skipReviewRiffCard", b, reviewInput); rec.Code != 200 {
		t.Fatal("original skip review", rec.Code, rec.Body.String())
	}
	dueInput := map[string]any{"deckID": deckID, "reviewedCards": reviewedCards}
	if rec := query(second, dueEndpoint, b, dueInput); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"cards":[]`) {
		t.Fatal("skipped card still in owned round", rec.Code, rec.Body.String())
	}
	reviewView := open(b, "wikiDesktop")
	if rec := query(reviewView, dueEndpoint, b, dueInput); rec.Code != 200 || !strings.Contains(rec.Body.String(), cardID) {
		t.Fatal("skip escaped to another view", rec.Code, rec.Body.String())
	}
	reviewInput["rating"] = 3
	if rec := query(reviewView, "/api/riff/reviewRiffCard", b, reviewInput); rec.Code != 200 {
		t.Fatal("original card review", rec.Code, rec.Body.String())
	}
	if rec := query(reviewView, dueEndpoint, b, map[string]any{"deckID": deckID}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"cards":[]`) {
		t.Fatal("reviewed card still due", rec.Code, rec.Body.String())
	}
	reviewLogs, e := filepath.Glob(filepath.Join(root, "pages", b, "data", "storage", "riff", "logs", "*.msgpack"))
	if e != nil || len(reviewLogs) != 1 {
		t.Fatal("review did not persist its log", reviewLogs, e)
	}
	if rec := query(second, "/api/riff/removeRiffCards", b, card); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"size":0`) {
		t.Fatal("original card removal", rec.Code, rec.Body.String())
	}

	resource := "/v1/resource?storeId=" + store + "&pageId=" + a + "&name=child.txt&boxId=" + box + "&docId=" + doc
	if rec := send("GET", resource, first.Ticket, "", nil); rec.Code != 200 || rec.Body.String() != "owned asset" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := send("GET", resource, second.Ticket, "", nil); rec.Code != 401 {
		t.Fatal("foreign resource served", rec.Code)
	}
	if rec := send("POST", "/v1/editor", "", "", editorRequest{ProtocolVersion: 1, StoreID: store, Action: "close", Ticket: first.Ticket}); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := query(first, "/api/block/getBlockDOM", a, args); rec.Code != 401 {
		t.Fatal("retired UI view", rec.Code)
	}
}
