// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"wiki-kernel/conf"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

var ErrUIEndpoint = errors.New("ui_endpoint_unavailable")

type UIRequest struct {
	ProtocolVersion int             `json:"protocolVersion"`
	StoreID         string          `json:"storeId"`
	PageID          string          `json:"pageId"`
	Endpoint        string          `json:"endpoint"`
	Data            json.RawMessage `json:"data,omitempty"`
	OperationID     string          `json:"operationId,omitempty"`
}

type UIResult struct {
	StoreID   string `json:"storeId"`
	PageID    string `json:"pageId"`
	ViewID    string `json:"viewId"`
	SessionID string `json:"sessionId"`
	Data      any    `json:"data"`
}

type UIView struct {
	Mode       string
	ViewID     string
	Entry      *wikistore.EntryDocument
	Flashcards *UIFlashcardState
	Themes     []conf.AppearanceThemeRecord
}

type uiCall struct {
	action   string
	options  any
	blocks   []string
	box      string
	pageWide bool
	project  func(any) any
	run      func(context.Context, *workspace.Lease) (any, error)
}

type uiRoute func(json.RawMessage) (uiCall, error)

func DecodeUIRequest(in io.Reader) (UIRequest, error) {
	var request UIRequest
	if err := Decode(in, &request); err != nil {
		return request, err
	}
	limit := 1 << 20
	if request.Endpoint == "/api/import/importSY" || uiMarkdownImport(request.Endpoint) {
		limit = 12 << 20
	} else if request.Endpoint != "/api/filetree/renameDoc" && request.Endpoint != uiCreateReferenceEndpoint &&
		request.Endpoint != uiDocVersionsEndpoint && request.Endpoint != uiTagRenameEndpoint &&
		request.Endpoint != uiPluginStorageEndpoint && request.OperationID != "" {
		return request, ErrArgument
	}
	if request.ProtocolVersion != 1 || request.StoreID == "" || !pageIdentity.MatchString(request.PageID) ||
		!strings.HasPrefix(request.Endpoint, "/api/") || len(request.Endpoint) > 128 || len(request.Data) > limit {
		return request, ErrArgument
	}
	return request, nil
}

func uiRouteFor[T any](prepare func(T) (uiCall, error)) uiRoute {
	return func(raw json.RawMessage) (uiCall, error) {
		var q T
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			raw = []byte("{}")
		}
		if raw[0] != '{' {
			return uiCall{}, ErrArgument
		}
		if err := Decode(bytes.NewReader(raw), &q); err != nil {
			return uiCall{}, err
		}
		return prepare(q)
	}
}

func UI(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (data any, err error) {
	if view.Mode != "wikiDesktop" && (view.Mode != "docxSingle" || view.Entry == nil) {
		return nil, ErrUnauthorized
	}
	if r.Endpoint == uiDocVersionsEndpoint {
		return runUIDocVersions(ctx, m, r, view)
	}
	if r.Endpoint == uiTagRenameEndpoint {
		return runUITagRename(ctx, m, r, view)
	}
	if r.Endpoint == "/api/transactions" {
		return runUIEdit(ctx, m, r, view)
	}
	if r.Endpoint == uiCreateReferenceEndpoint {
		return runUICreateDocWithReference(ctx, m, r, view)
	}
	if r.Endpoint == "/api/ai/status" || r.Endpoint == "/api/ai/chatGPT" {
		return runUIAI(ctx, m, r, view)
	}
	if r.Endpoint == "/api/asset/previewExternal" {
		return runUIExternalPreview(ctx, r)
	}
	if r.Endpoint == "/api/asset/submitExternalFrame" {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUIExternalFrameForm(ctx, r)
	}
	if r.Endpoint == "/api/widget/status" || r.Endpoint == "/api/widget/preview" || r.Endpoint == "/api/widget/list" {
		return runUIWidget(ctx, m, r, view)
	}
	if r.Endpoint == "/api/plugin/loadSandboxed" {
		return runUISandboxPlugins(ctx, m, r, view)
	}
	if r.Endpoint == uiPluginStorageEndpoint {
		return runUISandboxPluginStorage(ctx, m, r, view)
	}
	if strings.HasPrefix(r.Endpoint, "/api/account/") || strings.HasPrefix(r.Endpoint, "/api/cloud/") || strings.HasPrefix(r.Endpoint, "/api/sync/") {
		return nil, ErrUnauthorized
	}
	if kind, ok := uiWorkspaceSettings[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return saveUIWorkspaceSetting(ctx, m, r, kind)
	}
	if kind, ok := uiDeviceSettings[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return saveUIDeviceSetting(ctx, m, r, kind, view.Themes)
	}
	if action, ok := uiAssetActions[r.Endpoint]; ok {
		return runUIAsset(ctx, m, r, view, action)
	}
	if format, ok := uiArchiveExports[r.Endpoint]; ok {
		return runUIArchiveExport(ctx, m, r, view, format)
	}
	if action, ok := uiNotebookArchiveRoutes[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUINotebookArchive(ctx, m, r, action)
	}
	if kind, ok := uiNotebookActions[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUINotebook(ctx, m, r, kind)
	}
	if uiMarkdownImport(r.Endpoint) {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return nil, runUIMarkdownImport(ctx, m, r)
	}
	if lifecycle, ok := uiLifecycleRoutes[r.Endpoint]; ok {
		return runUILifecycle(ctx, m, r, view, lifecycle)
	}
	if action, ok := uiStructureActions[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUIStructure(ctx, m, r, action)
	}
	if action, ok := uiFlashcardWrites[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUIFlashcardWrite(ctx, m, r, action)
	}
	if r.Endpoint == "/api/riff/reviewRiffCard" || r.Endpoint == "/api/riff/skipReviewRiffCard" {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUIFlashcardReview(ctx, m, r, view.Flashcards)
	}
	if scope, ok := uiFlashcardDue[r.Endpoint]; ok {
		if view.Mode != "wikiDesktop" {
			return nil, ErrUnauthorized
		}
		return runUIDueCards(ctx, m, r, scope, view.Flashcards)
	}
	prepare, ok := uiRoutes[r.Endpoint]
	if r.Endpoint == "/api/search/searchRefBlock" && view.Mode == "docxSingle" {
		prepare, ok = uiRouteFor(func(q uiReferenceSearch) (uiCall, error) {
			return prepareUIEntryReferenceSearch(*view.Entry, q)
		}), true
	}
	if r.Endpoint == "/api/search/searchEmbedBlock" && view.Mode == "docxSingle" {
		prepare, ok = uiEmbeds(true, view.Entry), true
	}
	if r.Endpoint == "/api/search/searchAsset" && view.Mode == "docxSingle" {
		prepare, ok = uiRouteFor(func(q struct {
			K    string
			Exts []string
		}) (uiCall, error) {
			return uiCall{run: func(ctx context.Context, l *workspace.Lease) (any, error) {
				return l.NativeSearchEntryAssetNames(ctx, q.K, q.Exts, *view.Entry)
			}}, nil
		}), true
	}
	if r.Endpoint == "/api/template/render" {
		var entry *wikistore.EntryDocument
		if view.Mode == "docxSingle" {
			entry = view.Entry
		}
		prepare, ok = uiTemplateRender(entry), true
	}
	if r.Endpoint == "/api/storage/getLocalStorage" || r.Endpoint == "/api/storage/setLocalStorageVal" {
		prepare, ok = uiViewStorage(r.Endpoint, view), true
	}
	if r.Endpoint == "/api/history/searchHistory" || r.Endpoint == "/api/history/getHistoryItems" {
		prepare, ok = uiHistory(m, r, r.Endpoint == "/api/history/getHistoryItems"), true
	}
	if r.Endpoint == "/api/history/getDocHistoryContent" && strings.HasPrefix(uiHistoryPath(r.Data), deletedUIHistory+"/") {
		prepare, ok = uiRenderDeletedHistory(m, r), true
	}
	if !ok {
		prepare, ok = uiExtraRoutes[r.Endpoint]
	}
	if !ok {
		prepare, ok = uiProjectionRoutes[r.Endpoint]
	}
	if !ok {
		prepare, ok = uiStorageRoutes[r.Endpoint]
	}
	if !ok {
		prepare, ok = uiFlashcardRoutes[r.Endpoint]
	}
	if !ok {
		prepare, ok = uiReferenceRoutes[r.Endpoint]
	}
	if !ok {
		return nil, ErrUIEndpoint
	}
	call, err := prepare(r.Data)
	if err != nil {
		return nil, err
	}
	if view.Entry != nil && view.Mode == "docxSingle" && (call.pageWide || call.box != "" && call.box != view.Entry.BoxID) {
		return nil, ErrUnauthorized
	}
	err = m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		query := func(action string, options any) (any, error) {
			var raw json.RawMessage
			if options != nil {
				encoded, e := json.Marshal(options)
				if e != nil {
					return nil, e
				}
				raw = encoded
			}
			result, e := Query(c, m, QueryRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, Action: action, Options: raw})
			return result.Value, e
		}
		if view.Mode == "docxSingle" {
			tree, e := m.Tree(c, r.StoreID)
			if e != nil {
				return e
			}
			page, ok := tree.Get(r.PageID)
			if !ok || page.DeletedAt != nil || page.EntryDocument == nil || *page.EntryDocument != *view.Entry {
				return ErrStaleSession
			}
			for _, id := range call.blocks {
				if id == "" || id == view.Entry.DocID {
					continue
				}
				value, e := query("block.location", readID{id})
				loc, _ := value.(map[string]string)
				if e != nil || loc["rootID"] != view.Entry.DocID || loc["box"] != view.Entry.BoxID {
					return ErrUnauthorized
				}
			}
		}
		if tree, ok := call.options.(doctree.Request); ok && tree.Max == 0 {
			value, e := query("configuration", nil)
			if e != nil {
				return e
			}
			cfg := value.(conf.AppData)
			tree.Max = conf.NewFileTree().MaxListCount
			if cfg.FileTree != nil && cfg.FileTree.MaxListCount > 0 {
				tree.Max = min(cfg.FileTree.MaxListCount, 102400)
			}
			call.options = tree
		}
		if call.run != nil {
			l, e := m.Acquire(c, r.StoreID, r.PageID)
			if e != nil {
				return e
			}
			defer l.Release()
			projection, e := l.PrepareNativeReads(c, false)
			if e != nil {
				return e
			}
			data, err = call.run(c, l)
			if err == nil {
				err = l.VerifyNativeReadSource(c, projection.SourceHash)
			}
		} else {
			data, err = query(call.action, call.options)
		}
		if err != nil {
			return err
		}
		if call.project != nil {
			data = call.project(data)
		}
		if r.Endpoint == "/api/system/getConf" {
			cfg := uiConfiguration(data.(conf.AppData), view)
			if view.Mode == "wikiDesktop" {
				l, e := m.Acquire(c, r.StoreID, r.PageID)
				if e != nil {
					return e
				}
				defer l.Release()
				cfg["conf"].(map[string]any)["uiLayout"] = loadUILayout(c, l)
			}
			data = cfg
		}
		return nil
	})
	return data, err
}

func uiConfiguration(cfg conf.AppData, view UIView) map[string]any {
	// This explicit projection cannot inherit fields containing host paths,
	// credentials, executable snippets or remote-provider configuration.
	editor := cfg.Editor
	if editor != nil {
		editor.AllowHTMLBLockScript = false
		editor.PlantUMLServePath = ""
	}
	appearance := cfg.Appearance
	if appearance == nil {
		appearance = conf.NewAppearance()
	}
	if projected, err := projectUIAppearance(appearance, view.Themes); err == nil {
		appearance = projected
	}
	appearance.Lang = cfg.Lang
	system := map[string]any{"container": "browser"}
	if view.Mode == "docxSingle" {
		system["single"] = true
		system["singleNotebookBoxID"] = view.Entry.BoxID
		system["singleNotebookRootDocID"] = view.Entry.DocID
		system["unmountedBoxes"] = []any{}
		system["ignoredDocs"] = []any{}
	}
	return map[string]any{
		"conf": map[string]any{
			"lang": cfg.Lang, "langs": cfg.Langs, "appearance": appearance,
			"editor": editor, "fileTree": cfg.FileTree, "keymap": cfg.Keymap,
			"search": cfg.Search, "tag": cfg.Tag, "graph": cfg.Graph, "flashcard": cfg.Flashcard,
			"readonly": cfg.ReadOnly, "system": system,
		},
		"isPublish": false, "start": false,
	}
}

type uiDocument struct {
	ID, StartID, EndID, Query      string
	Index, QueryMethod, Mode, Size int
	QueryTypes                     map[string]bool
	OriginalRefBlockIDs            map[string]string
	IsBacklink                     bool
	Highlight                      *bool
	ReqID                          int64
}

func prepareUIDocument(q uiDocument) (uiCall, error) {
	if !nativeID.MatchString(q.ID) || q.Mode < 0 || q.Mode > 4 || q.Size < 0 || q.Size > 102400 ||
		(q.StartID == "") != (q.EndID == "") {
		return uiCall{}, ErrArgument
	}
	if q.Size == 0 {
		q.Size = 102400
	}
	options := docview.Request{ID: q.ID, StartID: q.StartID, EndID: q.EndID, Query: q.Query,
		Index: q.Index, QueryMethod: q.QueryMethod, Mode: q.Mode, Size: q.Size, QueryTypes: q.QueryTypes,
		OriginalRefs: q.OriginalRefBlockIDs, Backlink: q.IsBacklink, Highlight: q.Highlight == nil || *q.Highlight,
		UseDynamicSize: q.StartID != ""}
	return uiCall{action: "document.view", options: options, blocks: []string{q.ID, q.StartID, q.EndID},
		// A backlink view can project another document, even with a local root.
		pageWide: q.IsBacklink || len(q.OriginalRefBlockIDs) > 0,
		project: func(value any) any {
			v := value.(docview.Result)
			return map[string]any{"id": q.ID, "mode": q.Mode, "parentID": v.ParentID, "parent2ID": v.Parent2ID,
				"rootID": v.RootID, "type": v.Type, "content": v.DOM, "blockCount": v.BlockCount,
				"eof": v.EOF, "scroll": v.Scroll, "box": v.Box, "path": v.Path, "isSyncing": false,
				"isBacklinkExpand": v.BacklinkExpand, "keywords": v.Keywords, "reqId": q.ReqID}
		}}, nil
}
