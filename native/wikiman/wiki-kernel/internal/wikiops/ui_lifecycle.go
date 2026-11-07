// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

type uiLifecycleData struct {
	Notebook, Path, ID, Title, MD, Markdown, ParentID, ToNotebook, ToPath string
	FromPaths, Paths, Sorts                                               []string
	App, Callback, Tags, ClippingHref, HistoryPath                        string
	ListDocTree, WithMath                                                 bool
	Data                                                                  []byte
	ExpectedHash                                                          string
}

type uiLifecycleAction struct {
	action string
	keys   string
}

var uiLifecycleRoutes = map[string]uiLifecycleAction{
	"/api/filetree/createDoc":         {"document.create", "notebook path title md sorts app callback listDocTree"},
	"/api/filetree/createDailyNote":   {"document.create", "notebook app callback"},
	"/api/filetree/createDocWithMd":   {"document.create", "notebook path id parentID markdown app tags clippingHref withMath"},
	"/api/filetree/duplicateDoc":      {"document.create", "id app"},
	"/api/filetree/renameDoc":         {"document.rename", "notebook path title expectedHash"},
	"/api/filetree/removeDoc":         {"document.delete", "notebook path"},
	"/api/filetree/removeDocs":        {"document.delete", "paths"},
	"/api/filetree/moveDocs":          {"document.move", "fromPaths toPath toNotebook callback"},
	"/api/history/rollbackDocHistory": {"document.version.apply", "notebook historyPath"},
	"/api/filetree/changeSort":        {"document.sort", "notebook paths"},
	"/api/import/importSY":            {"document.import", "notebook toPath data"},
}

func runUILifecycle(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView, route uiLifecycleAction) (any, error) {
	if view.Mode == "docxSingle" && route.action != "document.rename" && route.action != "document.version.apply" {
		return nil, ErrUnauthorized
	}
	var fields map[string]json.RawMessage
	if err := Decode(bytes.NewReader(r.Data), &fields); err != nil || fields == nil {
		return nil, ErrArgument
	}
	for key := range fields {
		if !strings.Contains(" "+route.keys+" ", " "+key+" ") {
			return nil, ErrArgument
		}
	}
	var input uiLifecycleData
	if json.Unmarshal(r.Data, &input) != nil || len(input.Sorts) > 0 {
		return nil, ErrArgument
	}
	if r.Endpoint == "/api/history/rollbackDocHistory" && strings.HasPrefix(input.HistoryPath, deletedUIHistory+"/") {
		return uiRecoverDeletedHistory(ctx, m, r, view, input)
	}
	if r.Endpoint == "/api/filetree/removeDocs" || r.Endpoint == "/api/filetree/moveDocs" && len(input.FromPaths) > 1 {
		return nil, runUIBatchLifecycle(ctx, m, r, input, route.action)
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, Action: route.action,
		BoxID: input.Notebook, Title: input.Title, OperationID: "ui-" + ast.NewNodeID()}
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		l, e := m.Acquire(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		defer l.Release()
		cfg, e := l.Configuration(c)
		if e != nil {
			return e
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		prefix := "pages/" + r.PageID + "/data/"
		locate := func(box, name string) (string, string, error) {
			rel := strings.TrimPrefix(name, "/")
			if !nativeID.MatchString(box) || name != "/"+rel || !documentRelativePath(rel) {
				return "", "", ErrArgument
			}
			id := strings.TrimSuffix(path.Base(rel), ".docj")
			file, raw, e := m.DocumentSource(c, r.StoreID, r.PageID, box, id)
			if e != nil {
				return "", "", e
			}
			if file != prefix+box+"/"+rel {
				return "", "", sourcetx.ErrConflict
			}
			return id, sourcetx.Hash(raw), nil
		}
		switch r.Endpoint {
		case "/api/import/importSY":
			if len(input.Data) == 0 || len(input.Data) > 8<<20 || r.OperationID == "" {
				return ErrArgument
			}
			boxes, err := l.ConfigurationNotebooks(c)
			if err != nil {
				return err
			}
			found := false
			for _, box := range boxes {
				found = found || box.ID == request.BoxID && !box.Closed
			}
			if !found {
				return fs.ErrNotExist
			}
			request.Data, request.OperationID = input.Data, r.OperationID
			if input.ToPath != "/" {
				request.ParentDocID, _, e = locate(request.BoxID, input.ToPath)
			}
		case "/api/filetree/createDailyNote":
			request.CreationMode = "daily"
		case "/api/filetree/createDocWithMd":
			request.CreationMode, request.DocID, request.ParentDocID = "markdown", input.ID, input.ParentID
			request.HPath, request.Markdown = input.Path, input.Markdown
			request.Tags, request.ClippingHref, request.WithMath = input.Tags, input.ClippingHref, input.WithMath
		case "/api/filetree/createDoc":
			rel := strings.TrimPrefix(input.Path, "/")
			if input.Path != "/"+rel || !documentRelativePath(rel) {
				return ErrArgument
			}
			request.DocID, request.Markdown = strings.TrimSuffix(path.Base(rel), ".docj"), input.MD
			if parent := path.Dir(rel); parent != "." {
				request.ParentDocID, _, e = locate(request.BoxID, "/"+parent+".docj")
			}
		case "/api/history/rollbackDocHistory":
			request.DocID = strings.TrimSuffix(path.Base(input.HistoryPath), ".docj")
			before, err := l.NativeDocumentVersion(c, request.BoxID, request.DocID, input.HistoryPath)
			if err != nil {
				return err
			}
			request.LegacyHistoryPath, request.VersionHash = input.HistoryPath, sourcetx.Hash(before)
			_, raw, err := m.DocumentSource(c, r.StoreID, r.PageID, request.BoxID, request.DocID)
			if err == nil {
				request.ExpectedHash = sourcetx.Hash(raw)
			} else if errors.Is(err, fs.ErrNotExist) && view.Mode == "wikiDesktop" {
				parts := strings.Split(input.HistoryPath, "/")
				if len(parts) > 3 {
					request.ParentDocID = parts[len(parts)-2]
				}
			} else {
				return err
			}
		case "/api/filetree/duplicateDoc":
			if _, e = l.PrepareNativeReads(c, false); e != nil {
				return e
			}
			loc, e := l.NativeBlockLocation(c, input.ID)
			if e != nil {
				return e
			}
			if loc["rootID"] != input.ID {
				return ErrArgument
			}
			request.CreationMode, request.BoxID, request.DocID = "duplicate", loc["box"], input.ID
		case "/api/filetree/changeSort":
			for _, rel := range input.Paths {
				id, _, e := locate(request.BoxID, rel)
				if e != nil {
					return e
				}
				request.Order = append(request.Order, id)
			}
			sort, e := m.SourceFile(c, r.StoreID, prefix+request.BoxID+"/.wiki/sort.json")
			if e == nil {
				request.ExpectedHash = sourcetx.Hash(sort)
			} else if !errors.Is(e, fs.ErrNotExist) {
				return e
			}
		case "/api/filetree/moveDocs":
			if len(input.FromPaths) != 1 {
				return ErrUIEndpoint
			}
			from := input.FromPaths[0]
			// An ancestor ID may be the first component of a relative path.
			// Look up the root ID by its registered location when needed.
			id := strings.TrimSuffix(path.Base(from), ".docj")
			if _, e = l.PrepareNativeReads(c, false); e != nil {
				return e
			}
			loc, e := l.NativeBlockLocation(c, id)
			if e != nil || loc["rootID"] != id {
				return ErrArgument
			}
			if from != loc["path"] && strings.TrimPrefix(from, "/") != loc["box"]+loc["path"] {
				return sourcetx.ErrConflict
			}
			request.BoxID, request.ToBoxID = loc["box"], input.ToNotebook
			request.DocID, request.ExpectedHash, e = locate(request.BoxID, loc["path"])
			if e != nil {
				return e
			}
			if input.ToPath != "/" {
				parent, _, err := locate(request.ToBoxID, input.ToPath)
				if err != nil {
					return err
				}
				request.ParentDocID = parent
			}
		default:
			request.DocID, request.ExpectedHash, e = locate(request.BoxID, input.Path)
			request.Cascade = route.action == "document.delete"
			if e == nil && r.Endpoint == "/api/filetree/renameDoc" && r.OperationID != "" {
				if input.ExpectedHash == "" || input.ExpectedHash != request.ExpectedHash {
					return sourcetx.ErrConflict
				}
				request.OperationID = r.OperationID
			}
		}
		if e != nil {
			return e
		}
		tree, e := m.Tree(c, r.StoreID)
		if e != nil {
			return e
		}
		page, ok := tree.Get(r.PageID)
		if !ok || page.DeletedAt != nil {
			return fs.ErrNotExist
		}
		if view.Mode == "docxSingle" && (page.EntryDocument == nil || *page.EntryDocument != *view.Entry ||
			request.BoxID != view.Entry.BoxID || request.DocID != view.Entry.DocID) {
			return ErrUnauthorized
		}
		request.ExpectedTreeDigest = tree.SourceDigest()
		if page.EntryDocument != nil && page.EntryDocument.BoxID == request.BoxID && page.EntryDocument.DocID == request.DocID &&
			(route.action == "document.rename" || route.action == "document.move" || route.action == "document.version.apply") {
			raw, e := m.SourceFile(c, r.StoreID, "pages/"+r.PageID+"/page.json")
			if e != nil {
				return e
			}
			request.ExpectedPageHash = sourcetx.Hash(raw)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result, err := Content(ctx, m, request)
	if err != nil {
		return nil, err
	}
	switch r.Endpoint {
	case "/api/filetree/createDocWithMd":
		return result.DocID, nil
	case "/api/filetree/duplicateDoc":
		options, _ := json.Marshal(docquery.PathRequest{Mode: "id", ID: result.DocID})
		value, err := Query(ctx, m, QueryRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, Action: "document.paths", Options: options})
		if err != nil {
			return nil, err
		}
		location := value.Value.(docquery.PathResult)
		return map[string]any{"id": result.DocID, "notebook": location.Box, "path": location.Path, "hPath": location.HPath}, nil
	case "/api/filetree/createDailyNote", "/api/filetree/createDoc":
		return map[string]string{"id": result.DocID}, nil
	}
	return nil, nil
}
