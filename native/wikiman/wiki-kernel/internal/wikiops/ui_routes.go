// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"html"
	"strconv"

	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

type uiID struct {
	ID string
}

func uiReadEmpty(action string, pageWide bool) uiRoute {
	return uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{action: action, pageWide: pageWide}, nil
	})
}

func uiReadID(action string, pageWide bool) uiRoute {
	return uiRouteFor(func(q uiID) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: action, options: readID{q.ID}, blocks: []string{q.ID}, pageWide: pageWide}, nil
	})
}

func uiReadKeyword(action, wrapper string) uiRoute {
	return uiKeyword(action, wrapper, true)
}

func uiKeyword(action, wrapper string, pageWide bool) uiRoute {
	return uiRouteFor(func(q struct{ K string }) (uiCall, error) {
		return uiCall{action: action, options: readKey{q.K}, pageWide: pageWide, project: func(value any) any {
			if wrapper != "" {
				return map[string]any{wrapper: value, "k": q.K}
			}
			return value
		}}, nil
	})
}

func uiPathByID(mode string) uiRoute {
	return uiRouteFor(func(q uiID) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "document.paths", options: docquery.PathRequest{Mode: mode, ID: q.ID}, blocks: []string{q.ID}, project: func(value any) any {
			return value.(docquery.PathResult).HPath
		}}, nil
	})
}

func uiNotebook(action string, info bool) uiRoute {
	return uiRouteFor(func(q struct{ Notebook string }) (uiCall, error) {
		if !nativeID.MatchString(q.Notebook) {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: action, options: readID{q.Notebook}, box: q.Notebook, project: func(value any) any {
			if info {
				return map[string]any{"boxInfo": value}
			}
			cfg := value.(*conf.BoxConf)
			return map[string]any{"box": q.Notebook, "name": cfg.Name, "conf": cfg}
		}}, nil
	})
}

var uiRoutes = map[string]uiRoute{
	"/api/system/getConf":            uiReadEmpty("configuration", false),
	"/api/system/getEmojiConf":       uiReadEmpty("configuration.emojis", false),
	"/api/notebook/getNotebookInfo":  uiNotebook("notebook.info", true),
	"/api/notebook/getNotebookConf":  uiNotebook("configuration.notebook", false),
	"/api/filetree/getDoc":           uiRouteFor(prepareUIDocument),
	"/api/filetree/getHPathByID":     uiPathByID("id"),
	"/api/filetree/getFullHPathByID": uiPathByID("full"),
	"/api/block/getDocInfo":          uiReadID("document.info", false),
	"/api/block/getBlockInfo":        uiReadID("block.location", false),
	"/api/attr/getBlockAttrs":        uiReadID("block.attributes", false),
	"/api/attr/getBookmarkLabels":    uiReadEmpty("bookmarks.labels", true),
	"/api/block/getRefText":          uiReadID("references.text", false),
	"/api/block/getRefIDs":           uiReadID("references.block", true),
	"/api/block/getRecentUpdatedBlocks": uiRouteFor(func(q struct{ ReqID int64 }) (uiCall, error) {
		return uiCall{action: "blocks.recent", pageWide: true, project: func(value any) any {
			return map[string]any{"blocks": value, "reqId": q.ReqID}
		}}, nil
	}),
	"/api/bookmark/getBookmark":  uiReadEmpty("bookmarks", true),
	"/api/search/searchTemplate": uiKeyword("templates.search", "blocks", false),
	"/api/search/searchWidget":   uiReadKeyword("widgets.search", "blocks"),
	"/api/search/searchTag":      uiReadKeyword("tags.search", "tags"),
	"/api/tag/getTag": uiRouteFor(func(q struct {
		App               string
		Sort              *int32
		IgnoreMaxListHint bool
	}) (uiCall, error) {
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			var sort *int
			if q.Sort != nil {
				mode := int(*q.Sort)
				sort = &mode
			}
			tree, err := l.BuildNativeTagsWithSort(ctx, q.IgnoreMaxListHint, sort)
			return tree.Tags, err
		}}, nil
	}),
	"/api/av/getMirrorDatabaseBlocks": uiReadID("av.mirrors", true),
	"/api/storage/getCriteria": uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{action: "storage", options: readStorage{Action: "criteria.get"}, pageWide: true}, nil
	}),
	"/api/outline/getDocOutline": uiRouteFor(func(q readOutline) (uiCall, error) {
		return uiCall{action: "outline", options: q, blocks: []string{q.ID}}, nil
	}),
	"/api/notebook/lsNotebooks": uiRouteFor(func(q struct{ Flashcard bool }) (uiCall, error) {
		if q.Flashcard {
			return uiCall{}, ErrUIEndpoint
		}
		return uiCall{action: "configuration.notebooks", pageWide: true, project: func(value any) any {
			notebooks := value.([]conf.Notebook)
			rows := make([]map[string]any, 0, len(notebooks))
			for _, notebook := range notebooks {
				rows = append(rows, map[string]any{
					"id": notebook.ID, "name": notebook.Name, "icon": notebook.Icon, "closed": notebook.Closed,
					"sort": notebook.Sort, "sortMode": notebook.SortMode, "newFlashcardCount": notebook.NewFlashcardCount,
					"dueFlashcardCount": notebook.DueFlashcardCount, "flashcardCount": notebook.FlashcardCount,
				})
			}
			return map[string]any{"notebooks": rows}
		}}, nil
	}),
	"/api/filetree/getDocCreateSavePath": uiCreateSavePath(false),
	"/api/filetree/getRefCreateSavePath": uiCreateSavePath(true),
	"/api/filetree/listDocsByPath": uiRouteFor(func(q struct {
		Notebook, Path, App                      string
		Sort                                     *int
		MaxListCount                             *int
		Flashcard, ShowHidden, IgnoreMaxListHint bool
	}) (uiCall, error) {
		if !nativeID.MatchString(q.Notebook) || q.Flashcard {
			return uiCall{}, ErrArgument
		}
		sort := configtypes.SortModeUnassigned
		if q.Sort != nil {
			sort = *q.Sort
		}
		maxCount := 0
		if q.MaxListCount != nil {
			maxCount = *q.MaxListCount
			if maxCount <= 0 {
				maxCount = 102400
			}
		}
		return uiCall{action: "document.tree", options: doctree.Request{Box: q.Notebook, Path: q.Path, Sort: sort, Max: maxCount, ShowHidden: q.ShowHidden},
			pageWide: true, project: func(value any) any {
				result := value.(struct {
					Files any `json:"files"`
					Total int `json:"total"`
				})
				return map[string]any{"box": q.Notebook, "path": q.Path, "files": result.Files, "total": result.Total}
			}}, nil
	}),
	"/api/block/getBlockDOM": uiRouteFor(func(q uiID) (uiCall, error) {
		return uiCall{action: "block.dom", options: readDOM{IDs: []string{q.ID}}, blocks: []string{q.ID}, project: func(value any) any {
			return map[string]any{"id": q.ID, "dom": value.(map[string]string)[q.ID]}
		}}, nil
	}),
	"/api/block/getBlockIndex": uiRouteFor(func(q uiID) (uiCall, error) {
		return uiCall{action: "block.position", options: docquery.PositionRequest{Mode: "index", ID: q.ID}, blocks: []string{q.ID}, project: func(value any) any {
			return value.(docquery.PositionResult).Index
		}}, nil
	}),
	"/api/block/getBlockBreadcrumb": uiRouteFor(func(q struct {
		ID           string
		ExcludeTypes []string
	}) (uiCall, error) {
		return uiCall{action: "block.position", options: docquery.PositionRequest{Mode: "breadcrumb", ID: q.ID, ExcludeTypes: q.ExcludeTypes},
			blocks: []string{q.ID}, project: func(value any) any { return value.(docquery.PositionResult).Breadcrumb }}, nil
	}),
	"/api/asset/getDocImageAssets": uiRouteFor(func(q uiID) (uiCall, error) {
		return uiCall{action: "document.assets", options: struct {
			ID     string
			Images bool
		}{q.ID, true}, blocks: []string{q.ID}}, nil
	}),
	"/api/asset/getMissingAssets": uiAssetReferences(true),
	"/api/asset/getUnusedAssets":  uiAssetReferences(false),
	"/api/search/searchRefBlock":  uiRouteFor(prepareUIReferenceSearch),
	"/api/ref/getBacklink2":       uiRouteFor(prepareUIBacklinks),
	"/api/ref/getBacklink":        uiRouteFor(prepareUILegacyBacklinks),
	"/api/ref/getBacklinkDoc":     uiBacklinkDocument(false),
	"/api/ref/getBackmentionDoc":  uiBacklinkDocument(true),
}

func prepareUILegacyBacklinks(q struct {
	ID, K, MK       string
	BeforeLen       *int
	ContainChildren *bool
}) (uiCall, error) {
	if !nativeID.MatchString(q.ID) {
		return uiCall{}, ErrArgument
	}
	before := 12
	if q.BeforeLen != nil {
		before = *q.BeforeLen
	}
	if before < 0 || before > 4096 {
		return uiCall{}, ErrArgument
	}
	return uiCall{action: "references.backlinks", pageWide: true, options: blockview.BacklinkRequest{
		Mode: "links", ID: q.ID, Keyword: q.K, MentionKeyword: q.MK, BeforeLen: before,
		Children: q.ContainChildren != nil && *q.ContainChildren, DefaultChildren: q.ContainChildren == nil,
	}, project: func(value any) any {
		v := value.(blockview.BacklinkResult)
		return map[string]any{"backlinks": v.Links, "linkRefsCount": v.ReferenceCount, "backmentions": v.Mentions,
			"mentionsCount": v.MentionCount, "k": q.K, "mk": q.MK, "box": v.Box}
	}}, nil
}

func uiBacklinkDocument(mention bool) uiRoute {
	return uiRouteFor(func(q struct {
		DefID, RefTreeID, Keyword  string
		ContainChildren, Highlight *bool
	}) (uiCall, error) {
		if !nativeID.MatchString(q.DefID) || !nativeID.MatchString(q.RefTreeID) || len(q.Keyword) > 4096 {
			return uiCall{}, ErrArgument
		}
		mode, field := "document", "backlinks"
		if mention {
			mode, field = "mention-document", "backmentions"
		}
		return uiCall{action: "references.backlinks", pageWide: true, options: blockview.BacklinkRequest{
			Mode: mode, ID: q.DefID, RefTreeID: q.RefTreeID, Keyword: q.Keyword,
			Children: q.ContainChildren != nil && *q.ContainChildren, DefaultChildren: q.ContainChildren == nil,
			Highlight: q.Highlight == nil || *q.Highlight,
		}, project: func(value any) any {
			v := value.(blockview.BacklinkResult)
			return map[string]any{field: v.Documents, "keywords": v.Keywords}
		}}, nil
	})
}

func uiAssetReferences(missing bool) uiRoute {
	return uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{action: "assets.references", options: struct{ Missing bool }{missing}, pageWide: true}, nil
	})
}

type uiReferenceSearch struct {
	ID, RootID, K                string
	BeforeLen                    int
	IsSquareBrackets, IsDatabase bool
	ReqID                        int64
}

func prepareUIReferenceSearch(q uiReferenceSearch) (uiCall, error) {
	return uiCall{action: "references.search", options: blockview.ReferenceSearchRequest{
		ID: q.ID, RootID: q.RootID, Keyword: q.K, BeforeLen: q.BeforeLen, SquareBrackets: q.IsSquareBrackets, Database: q.IsDatabase},
		pageWide: true, project: func(value any) any {
			v := value.(blockview.ReferenceSearchResult)
			return map[string]any{"blocks": v.Blocks, "newDoc": v.NewDoc, "k": html.EscapeString(q.K), "reqId": q.ReqID}
		}}, nil
}

func prepareUIEntryReferenceSearch(entry wikistore.EntryDocument, q uiReferenceSearch) (uiCall, error) {
	if q.RootID != entry.DocID || !nativeID.MatchString(q.ID) || q.IsDatabase {
		return uiCall{}, ErrUnauthorized
	}
	call, err := prepareUIReferenceSearch(q)
	if err != nil {
		return uiCall{}, err
	}
	options := call.options.(blockview.ReferenceSearchRequest)
	// The editing block may still be queued for save. It is only an exclusion
	// in the entry-only query and is never used to open another document.
	return uiCall{run: func(ctx context.Context, l *workspace.Lease) (any, error) {
		value, e := l.SearchNativeEntryReferences(ctx, entry, options)
		if e != nil {
			return nil, e
		}
		return call.project(value), nil
	}}, nil
}

type uiBacklinks struct {
	ID, K, MK, Sort, MSort string
	ContainChildren        *bool
}

func prepareUIBacklinks(q uiBacklinks) (uiCall, error) {
	sort, mentionSort := configtypes.SortModeUpdatedDESC, configtypes.SortModeUpdatedDESC
	if q.Sort != "" {
		sort, _ = strconv.Atoi(q.Sort)
	}
	if q.MSort != "" {
		mentionSort, _ = strconv.Atoi(q.MSort)
	}
	return uiCall{action: "references.backlinks", options: blockview.BacklinkRequest{
		Mode: "links2", ID: q.ID, Keyword: q.K, MentionKeyword: q.MK, SortMode: sort, MentionSortMode: mentionSort,
		Children: q.ContainChildren != nil && *q.ContainChildren, DefaultChildren: q.ContainChildren == nil,
	}, pageWide: true, project: func(value any) any {
		v := value.(blockview.BacklinkResult)
		return map[string]any{"backlinks": v.Links, "linkRefsCount": v.ReferenceCount, "backmentions": v.Mentions,
			"mentionsCount": v.MentionCount, "k": q.K, "mk": q.MK, "box": v.Box}
	}}, nil
}
