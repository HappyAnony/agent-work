// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"encoding/json"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"wiki-kernel/conf"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/graphview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

type uiAVRender struct {
	ID, BlockID, ViewID, Query string
	Page, PageSize             int
	GroupPaging                map[string]workspace.NativeAVPaging
}

func prepareUIAVRender(q uiAVRender) (uiCall, error) {
	if !nativeID.MatchString(q.ID) || q.BlockID != "" && !nativeID.MatchString(q.BlockID) {
		return uiCall{}, ErrArgument
	}
	return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
		raw, err := l.ReadNativeAttributeView(ctx, q.ID)
		if err != nil {
			return nil, err
		}
		var a avmodel.AttributeView
		if err = json.Unmarshal(raw, &a); err != nil || a.ID != q.ID {
			return nil, ErrArgument
		}
		if q.ViewID == "" && q.BlockID != "" {
			attrs, e := l.ReadNativeBlockAttributes(ctx, q.BlockID)
			if e != nil {
				return nil, e
			}
			q.ViewID = attrs[avmodel.NodeAttrView]
		}
		if q.ViewID == "" {
			q.ViewID = a.ViewID
		}
		selected, err := a.GetCurrentView(q.ViewID)
		if err != nil {
			return nil, err
		}
		views := make([]map[string]any, 0, len(a.Views))
		for _, view := range a.Views {
			if view == nil {
				return nil, ErrArgument
			}
			views = append(views, map[string]any{"id": view.ID, "icon": view.Icon, "name": view.Name, "desc": view.Desc,
				"hideAttrViewName": view.HideAttrViewName, "type": view.LayoutType, "pageSize": view.PageSize})
		}
		if selected == nil {
			return nil, ErrArgument
		}
		if q.PageSize <= 0 {
			q.PageSize = selected.PageSize
			if q.PageSize <= 0 {
				q.PageSize = 50
			}
		}
		rendered, err := l.NativeAttributeViewRender(ctx, q.ID, selected.ID, q.Query, max(1, q.Page), q.PageSize, q.GroupPaging)
		if err != nil {
			return nil, err
		}
		mirrors, err := l.NativeAttributeViewMirrors(ctx, q.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"name": a.Name, "id": a.ID, "viewID": selected.ID, "viewType": selected.LayoutType,
			"views": views, "view": rendered, "isMirror": len(mirrors) > 1, "sourceHash": sourcetx.Hash(raw)}, nil
	}}, nil
}

type uiGraph[T any] struct {
	ID, K string
	Conf  *T
	ReqID int64
}

func graphUIValue(qID string, value graphview.Result, cfg any, reqID int64) map[string]any {
	out := map[string]any{"box": value.Box, "nodes": value.Nodes, "links": value.Links, "conf": cfg, "reqId": reqID}
	if qID != "" {
		out["id"] = qID
	}
	return out
}

type uiHistorySearch struct {
	Notebook, Query, Op, Created string
	Type, Page                   int
}

func uiHistory(m *workspace.WorkspaceManager, r UIRequest, items bool) uiRoute {
	return uiRouteFor(func(q uiHistorySearch) (uiCall, error) {
		if q.Notebook != "" && !nativeID.MatchString(q.Notebook) || len(q.Query) > 4096 || q.Page > 10000 {
			return uiCall{}, ErrArgument
		}
		if q.Op == "" {
			q.Op = "all"
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			var linked map[string]int
			if q.Type != 2 && (q.Op == "all" || q.Op == "delete") {
				var err error
				linked, err = indexUIDeletedHistory(ctx, m, r, l)
				if err != nil {
					return nil, err
				}
			}
			req := indexdb.HistorySearchRequest{Query: q.Query, Box: q.Notebook, Operation: q.Op, Type: q.Type, Page: max(1, q.Page)}
			if items {
				result, err := l.NativeHistorySearchItems(ctx, req, q.Created)
				items := make([]map[string]string, 0, len(result))
				for _, row := range result {
					parts := strings.Split(row.Path, "/")
					notebook := ""
					if len(parts) > 1 && row.Type != 2 {
						notebook = parts[1]
					}
					item := map[string]string{"title": row.Title, "path": row.Path, "op": row.Op, "notebook": notebook}
					if size := linked[row.Path]; size > 1 {
						item["linkedCount"] = strconv.Itoa(size)
					}
					items = append(items, item)
				}
				return map[string]any{"items": items}, err
			}
			result, err := l.NativeHistorySearch(ctx, req)
			return map[string]any{"histories": result.Created, "pageCount": result.Pages, "totalCount": result.Total}, err
		}}, nil
	})
}

var uiProjectionRoutes = map[string]uiRoute{
	"/api/lute/html2BlockDOM":     uiRouteFor(prepareUIClipboard),
	"/api/av/renderAttributeView": uiRouteFor(prepareUIAVRender),
	"/api/graph/resetGraph": uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{pageWide: true, run: func(context.Context, *workspace.Lease) (any, error) {
			return map[string]any{"conf": conf.NewGlobalGraph()}, nil
		}}, nil
	}),
	"/api/graph/resetLocalGraph": uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{pageWide: true, run: func(context.Context, *workspace.Lease) (any, error) {
			return map[string]any{"conf": conf.NewLocalGraph()}, nil
		}}, nil
	}),
	"/api/graph/getGraph": uiRouteFor(func(q uiGraph[conf.GlobalGraph]) (uiCall, error) {
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			if q.Conf == nil {
				cfg, err := l.Configuration(ctx)
				if err != nil || cfg.Graph == nil {
					return nil, ErrArgument
				}
				q.Conf = cfg.Graph.Global
			}
			value, err := l.NativeGraphWithOptions(ctx, "", q.K, q.Conf, nil)
			return graphUIValue("", value, q.Conf, q.ReqID), err
		}}, nil
	}),
	"/api/graph/getLocalGraph": uiRouteFor(func(q uiGraph[conf.LocalGraph]) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			if q.Conf == nil {
				cfg, err := l.Configuration(ctx)
				if err != nil || cfg.Graph == nil {
					return nil, ErrArgument
				}
				q.Conf = cfg.Graph.Local
			}
			value, err := l.NativeGraphWithOptions(ctx, q.ID, q.K, nil, q.Conf)
			return graphUIValue(q.ID, value, q.Conf, q.ReqID), err
		}}, nil
	}),
	"/api/history/getDocHistoryContent": uiRouteFor(func(q struct {
		HistoryPath, K string
		Highlight      *bool
	}) (uiCall, error) {
		parts := strings.Split(q.HistoryPath, "/")
		if len(parts) < 3 || !fs.ValidPath(q.HistoryPath) || len(q.K) > 4096 {
			return uiCall{}, ErrArgument
		}
		doc := strings.TrimSuffix(path.Base(q.HistoryPath), ".docj")
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			if _, err := l.NativeDocumentVersion(ctx, parts[1], doc, q.HistoryPath); err != nil {
				return nil, err
			}
			out, err := l.NativeRenderHistory(ctx, q.HistoryPath, q.K, q.Highlight == nil || *q.Highlight)
			return map[string]any{"id": out.ID, "rootID": out.RootID, "content": out.Content, "isLargeDoc": out.Large}, err
		}}, nil
	}),
}

func uiTemplateRender(entry *wikistore.EntryDocument) uiRoute {
	return uiRouteFor(func(q struct {
		Path, ID string
		Preview  bool
	}) (uiCall, error) {
		templatePath := strings.TrimPrefix(q.Path, "/")
		if !nativeID.MatchString(q.ID) || !fs.ValidPath(templatePath) ||
			!strings.HasPrefix(templatePath, "templates/") || !strings.EqualFold(path.Ext(templatePath), ".md") {
			return uiCall{}, ErrArgument
		}
		if entry != nil && q.ID != entry.DocID {
			return uiCall{}, ErrUnauthorized
		}
		return uiCall{run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			out, err := l.PlanNativeTemplate(ctx, workspace.NativeTemplateRequest{Action: "render", Path: q.Path, ID: q.ID, Preview: q.Preview, Entry: entry})
			if err != nil {
				return nil, err
			}
			if len(out.AttributeViews) > 0 || len(out.SourceWrites) > 0 {
				return nil, ErrUIEndpoint
			}
			return map[string]any{"path": q.Path, "content": out.DOM}, nil
		}}, nil
	})
}
