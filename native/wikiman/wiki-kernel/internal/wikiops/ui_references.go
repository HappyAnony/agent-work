// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"slices"

	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/wikistore"
	"wiki-kernel/internal/workspace"
)

var uiReferenceRoutes = map[string]uiRoute{
	"/api/search/getEmbedBlock":    uiEmbeds(false, nil),
	"/api/search/searchEmbedBlock": uiEmbeds(true, nil),
	"/api/search/listInvalidBlockRefs": uiRouteFor(func(q struct{ Page, PageSize int }) (uiCall, error) {
		if q.PageSize <= 0 {
			q.PageSize = 32
		}
		if q.PageSize > 1000 {
			return uiCall{}, ErrArgument
		}
		return uiCall{action: "references.invalid", pageWide: true,
			options: struct{ Page, PageSize int }{max(q.Page, 1), q.PageSize},
			project: func(value any) any {
				result := value.(docquery.InvalidReferenceResult)
				return map[string]any{"blocks": result.Blocks, "matchedBlockCount": result.MatchedBlocks,
					"matchedRootCount": result.MatchedRoots, "pageCount": result.PageCount}
			}}, nil
	}),
	"/api/block/getBlockDefIDsByRefText": uiRouteFor(func(q struct {
		Anchor     string
		ExcludeIDs []string
	}) (uiCall, error) {
		if len(q.Anchor) > 4096 || len(q.ExcludeIDs) > 1000 {
			return uiCall{}, ErrArgument
		}
		return uiRefDefs(workspace.NativeReferenceQuery{Mode: "definitions-by-text", Keyword: q.Anchor}), nil
	}),
	"/api/block/getRefIDsByFileAnnotationID": uiRouteFor(func(q uiID) (uiCall, error) {
		if q.ID == "" || len(q.ID) > 1024 {
			return uiCall{}, ErrArgument
		}
		return uiRefDefs(workspace.NativeReferenceQuery{Mode: "annotation", ID: q.ID}), nil
	}),
}

func uiRefDefs(options workspace.NativeReferenceQuery) uiCall {
	return uiCall{action: "references.query", pageWide: true, options: options, project: func(value any) any {
		ids := value.([]string)
		refs := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			refs = append(refs, map[string]any{"refID": id, "defIDs": []string{}})
		}
		return map[string]any{"refDefs": refs}
	}}
}

func uiEmbeds(query bool, entry *wikistore.EntryDocument) uiRoute {
	return uiRouteFor(func(q struct {
		EmbedBlockID, Stmt     string
		IncludeIDs, ExcludeIDs []string
		HeadingMode            int
		Breadcrumb             bool
	}) (uiCall, error) {
		invalid := func(id string) bool { return !nativeID.MatchString(id) }
		if invalid(q.EmbedBlockID) || q.HeadingMode < 0 || q.HeadingMode > 2 || len(q.Stmt) > 32<<10 ||
			len(q.IncludeIDs)+len(q.ExcludeIDs) > 1024 || slices.ContainsFunc(q.IncludeIDs, invalid) ||
			slices.ContainsFunc(q.ExcludeIDs, invalid) {
			return uiCall{}, ErrArgument
		}
		options := blockview.EmbedRequest{
			SQL: q.Stmt, IncludeIDs: q.IncludeIDs, ExcludeIDs: q.ExcludeIDs,
			Query: query, HeadingMode: q.HeadingMode, Breadcrumb: q.Breadcrumb,
		}
		project := func(value any) any { return map[string]any{"blocks": value} }
		if entry != nil {
			return uiCall{project: project, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
				return l.NativeEntryEmbeds(ctx, *entry, options)
			}}, nil
		}
		return uiCall{action: "references.embeds", pageWide: true, options: options, project: project}, nil
	})
}
