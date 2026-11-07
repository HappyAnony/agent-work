// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"strings"

	"wiki-kernel/internal/assetquery"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
)

type uiSearch struct {
	Query, Type                     string
	Paths                           []string
	Types                           map[string]bool
	Method, OrderBy, Page, PageSize int
	GroupBy                         int
	ReqID                           int64
}

func prepareUIBlockSearch(q uiSearch) (uiCall, error) {
	request := indexdb.BlockSearchRequest{Query: q.Query, Types: q.Types, Method: q.Method,
		OrderBy: q.OrderBy, Page: max(1, q.Page), PageSize: q.PageSize}
	if request.PageSize <= 0 {
		request.PageSize = 32
	}
	for _, p := range q.Paths {
		box, rest, _ := strings.Cut(strings.TrimSpace(p), "/")
		if box != "" {
			if !nativeID.MatchString(box) {
				return uiCall{}, ErrArgument
			}
			request.Boxes = append(request.Boxes, box)
		}
		if rest != "" {
			request.Paths = append(request.Paths, "/"+rest)
		}
	}
	return uiCall{action: "search", pageWide: true, options: readSearch{Request: request, GroupBy: q.GroupBy}}, nil
}

func uiAVRead(action string, wrapper string) uiRoute {
	return uiRouteFor(func(q uiID) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: action, pageWide: true, options: readID{q.ID}, project: func(value any) any {
			if wrapper != "" {
				return map[string]any{wrapper: value}
			}
			return value
		}}, nil
	})
}

func uiAVKeys() uiRoute {
	return uiRouteFor(func(q struct {
		AvID   string
		KeyIDs []string
	}) (uiCall, error) {
		return uiCall{action: "av.keys", pageWide: true, options: readAV{ID: q.AvID, Kind: "ids", IDs: q.KeyIDs}}, nil
	})
}

var uiExtraRoutes = map[string]uiRoute{
	"/api/block/getHeadingDeleteTransaction": uiHeadingOperations("delete"),
	"/api/block/getHeadingInsertTransaction": uiHeadingOperations("insert"),
	"/api/block/getHeadingLevelTransaction":  uiHeadingOperations("level"),
	"/api/block/getHeadingChildrenDOM": uiRouteFor(func(q struct {
		ID             string
		RemoveFoldAttr *bool
	}) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "block.read", blocks: []string{q.ID}, options: docquery.BlocksRequest{Mode: "heading-dom", ID: q.ID,
			RemoveFold: q.RemoveFoldAttr == nil || *q.RemoveFoldAttr}, project: func(value any) any { return value.(docquery.BlocksResult).Text }}, nil
	}),
	"/api/block/getBlockSiblingID":   uiBlockRelations("sibling"),
	"/api/block/getBlockRelevantIDs": uiBlockRelations("relevant"),
	"/api/block/checkBlockFold":      uiBlockFold("state"),
	"/api/block/getUnfoldedParentID": uiBlockFold("unfolded-parent"),
	"/api/block/getContentWordCount": uiStatistics("content"),
	"/api/block/getBlocksWordCount":  uiStatistics("blocks"),
	"/api/block/getTreeStat":         uiStatistics("tree"),
	"/api/block/getDOMText": uiRouteFor(func(q struct{ DOM string }) (uiCall, error) {
		return uiCall{action: "block.position", options: docquery.PositionRequest{Mode: "dom-text", Content: q.DOM},
			project: func(value any) any { return value.(docquery.PositionResult).Text }}, nil
	}),
	"/api/search/fullTextSearchBlock": uiRouteFor(prepareUIBlockSearch),
	"/api/search/fullTextSearchAssetContent": uiRouteFor(func(q uiSearch) (uiCall, error) {
		return uiCall{action: "assets.search", pageWide: true, options: assetquery.Request{Query: q.Query, Types: q.Types,
			Method: q.Method, OrderBy: q.OrderBy, Page: max(1, q.Page), PageSize: q.PageSize}, project: func(value any) any {
			v := value.(assetquery.Result)
			return map[string]any{"assetContents": v.Contents, "matchedAssetCount": v.Matched, "pageCount": v.PageCount}
		}}, nil
	}),
	"/api/search/getAssetContent": uiRouteFor(func(q readContent) (uiCall, error) {
		return uiCall{action: "assets.content", pageWide: true, options: q}, nil
	}),
	"/api/search/searchAsset": uiRouteFor(func(q struct {
		K    string
		Exts []string
	}) (uiCall, error) {
		return uiCall{action: "assets.names", pageWide: true, options: readAsset{Keyword: q.K, Extensions: q.Exts}}, nil
	}),
	"/api/block/checkBlockExist": uiRouteFor(func(q uiID) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "block.get", options: readID{q.ID}, blocks: []string{q.ID}, project: func(value any) any {
			return value.(*blockview.Block) != nil
		}}, nil
	}),
	"/api/filetree/getHPathByPath": uiRouteFor(func(q struct{ Notebook, Path string }) (uiCall, error) {
		return uiCall{action: "document.paths", pageWide: true, options: docquery.PathRequest{Mode: "path", Box: q.Notebook, Path: q.Path}, project: func(value any) any {
			return value.(docquery.PathResult).HPath
		}}, nil
	}),
	"/api/filetree/getHPathsByPaths": uiRouteFor(func(q struct{ Paths []string }) (uiCall, error) {
		return uiCall{action: "document.paths", pageWide: true, options: docquery.PathRequest{Mode: "batch", Paths: q.Paths}, project: func(value any) any {
			return value.(docquery.PathResult).Paths
		}}, nil
	}),
	"/api/block/getBlocksIndexes": uiRouteFor(func(q readIDs) (uiCall, error) {
		return uiCall{action: "block.position", options: docquery.PositionRequest{Mode: "indexes", IDs: q.IDs}, blocks: q.IDs, project: func(value any) any {
			return value.(docquery.PositionResult).Indexes
		}}, nil
	}),
	"/api/av/getAttributeView":                  uiAVRead("av.get", "av"),
	"/api/av/getAttributeViewKeysByID":          uiAVKeys(),
	"/api/av/getAttributeViewKeysByAvID":        uiAVKeys(),
	"/api/av/getAttributeViewKeys":              uiReadID("block.avkeys", true),
	"/api/av/searchAttributeViewNonRelationKey": uiAVSearchKeys("nonrelation"),
	"/api/av/searchAttributeViewRelationKey":    uiAVSearchKeys("relation"),
	"/api/av/searchAttributeViewRollupDestKeys": uiAVSearchKeys("rollupdest"),
	"/api/av/searchAttributeView": uiRouteFor(func(q struct {
		Keyword  string
		Excludes []string
	}) (uiCall, error) {
		return uiCall{action: "av.search", pageWide: true, options: struct {
			Keyword    string
			ExcludeIDs []string
		}{q.Keyword, q.Excludes}, project: func(value any) any { return map[string]any{"results": value} }}, nil
	}),
	"/api/av/getAttributeViewPrimaryKeyValues": uiRouteFor(func(q struct {
		ID, Keyword    string
		Page, PageSize int
	}) (uiCall, error) {
		return uiCall{action: "av.primary", pageWide: true, options: readAV{ID: q.ID, Query: q.Keyword, Page: max(1, q.Page), PageSize: q.PageSize}, project: func(value any) any {
			v := value.(workspace.NativeAVPrimary)
			return map[string]any{"name": v.Name, "blockIDs": v.DatabaseBlockIDs, "rows": v.KeyValues}
		}}, nil
	}),
	"/api/av/getAttributeViewFilterSort": uiRouteFor(func(q struct{ ID, BlockID string }) (uiCall, error) {
		if !nativeID.MatchString(q.ID) || q.BlockID != "" && !nativeID.MatchString(q.BlockID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			view, err := l.NativeAttributeViewView(ctx, q.ID, q.BlockID)
			return map[string]any{"filters": view.Filters, "sorts": view.Sorts}, err
		}}, nil
	}),
	"/api/av/getAttributeViewAddingBlockDefaultValues": uiRouteFor(func(q struct {
		AvID, ViewID, GroupID, PreviousID, AddingBlockID string
	}) (uiCall, error) {
		if !nativeID.MatchString(q.AvID) || len(q.GroupID) > 1024 {
			return uiCall{}, ErrArgument
		}
		for _, id := range []string{q.ViewID, q.PreviousID, q.AddingBlockID} {
			if id != "" && !nativeID.MatchString(id) {
				return uiCall{}, ErrArgument
			}
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			values, err := l.NativeAttributeViewDefaults(ctx, q.AvID, q.ViewID, q.GroupID, q.PreviousID, q.AddingBlockID)
			return map[string]any{"values": values}, err
		}}, nil
	}),
	"/api/av/getCurrentAttrViewImages": uiRouteFor(func(q struct {
		ID, ViewID, Query string
	}) (uiCall, error) {
		return uiCall{action: "av.images", pageWide: true, options: readAV{ID: q.ID, ViewID: q.ViewID, Query: q.Query}}, nil
	}),
}

func uiHeadingOperations(mode string) uiRoute {
	return uiRouteFor(func(q struct {
		ID    string
		Level int
	}) (uiCall, error) {
		if !nativeID.MatchString(q.ID) || mode == "level" && (q.Level < 1 || q.Level > 6) {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: mode == "insert", blocks: []string{q.ID}, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			planned, err := l.PlanNativeHeadingOperations(ctx, doclife.HeadingOperationsRequest{Mode: mode, ID: q.ID, Level: q.Level})
			if err != nil || !planned.Plan.Changed {
				return nil, err
			}
			convert := func(values []doclife.HeadingOperation) []*editcore.Operation {
				out := make([]*editcore.Operation, 0, len(values))
				for _, v := range values {
					op := &editcore.Operation{Action: v.Action, ID: v.ID, ParentID: v.ParentID, PreviousID: v.PreviousID}
					if v.DOM != "" {
						op.Data = v.DOM
					}
					if v.IgnoreProcess {
						op.Context = map[string]any{"ignoreProcess": "true"}
					}
					out = append(out, op)
				}
				return out
			}
			return map[string]any{"doOperations": convert(planned.Plan.Do), "undoOperations": convert(planned.Plan.Undo)}, nil
		}}, nil
	})
}

func uiAVSearchKeys(kind string) uiRoute {
	return uiRouteFor(func(q struct{ AvID, Keyword string }) (uiCall, error) {
		if !nativeID.MatchString(q.AvID) || len(q.Keyword) > 4096 {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "av.keys", pageWide: true, options: readAV{ID: q.AvID, Kind: kind, Query: q.Keyword},
			project: func(value any) any { return map[string]any{"keys": value} }}, nil
	})
}

func uiBlockRelations(mode string) uiRoute {
	return uiRouteFor(func(q uiID) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "block.relations", options: docquery.RelationRequest{Mode: mode, ID: q.ID}, blocks: []string{q.ID},
			project: func(value any) any {
				ids := value.(docquery.RelationResult).IDs
				if mode == "sibling" {
					return map[string]string{"parent": ids.Parent, "previous": ids.Previous, "next": ids.Next}
				}
				return map[string]string{"parentID": ids.Parent, "previousID": ids.Previous, "nextID": ids.Next}
			}}, nil
	})
}

func uiBlockFold(mode string) uiRoute {
	return uiRouteFor(func(q uiID) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "block.fold", options: docquery.FoldRequest{Mode: mode, ID: q.ID}, blocks: []string{q.ID},
			project: func(value any) any {
				result := value.(docquery.FoldResult)
				if mode == "state" {
					return map[string]bool{"isFolded": result.IsFolded, "isRoot": result.IsRoot}
				}
				return map[string]string{"parentID": result.ParentID}
			}}, nil
	})
}

func uiStatistics(mode string) uiRoute {
	return uiRouteFor(func(q struct {
		ID, Content string
		IDs         []string
		ReqID       *int64
	}) (uiCall, error) {
		ids := q.IDs
		if mode == "tree" {
			ids = []string{q.ID}
		}
		if len(ids) > 1000 {
			return uiCall{}, ErrArgument
		}
		for _, id := range ids {
			if !nativeID.MatchString(id) {
				return uiCall{}, ErrArgument
			}
		}
		return uiCall{action: "document.statistics", options: docquery.StatRequest{Mode: mode, IDs: ids, Content: q.Content}, blocks: ids,
			project: func(value any) any { return map[string]any{"stat": value, "reqId": q.ReqID} }}, nil
	})
}
