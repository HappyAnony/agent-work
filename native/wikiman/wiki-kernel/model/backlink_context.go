// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
)

func BacklinksContext(ctx context.Context, q blockview.BacklinkRequest) (blockview.BacklinkResult, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (blockview.BacklinkResult, error) {
		if q.DefaultChildren {
			if o.settings.Editor == nil {
				return blockview.BacklinkResult{}, errors.New("backlink_editor_settings_missing")
			}
			q.Children = o.settings.Editor.BacklinkContainChildren
		}
		return blockview.ReadBacklinks(c, q, o.blockReadServices())
	})
}
func GetBacklinkDocContext(ctx context.Context, id, refTree, keyword string, children, highlight bool) ([]*Backlink, []string, error) {
	v, e := BacklinksContext(ctx, blockview.BacklinkRequest{Mode: "document", ID: id, RefTreeID: refTree, Keyword: keyword, Children: children, Highlight: highlight})
	return v.Documents, v.Keywords, e
}
func GetBackmentionDocContext(ctx context.Context, id, refTree, keyword string, children, highlight bool) ([]*Backlink, []string, error) {
	v, e := BacklinksContext(ctx, blockview.BacklinkRequest{Mode: "mention-document", ID: id, RefTreeID: refTree, Keyword: keyword, Children: children, Highlight: highlight})
	return v.Documents, v.Keywords, e
}
func GetBacklinkContext(ctx context.Context, id, keyword, mentionKeyword string, beforeLen int, children bool) (string, []*Path, []*Path, int, int, error) {
	v, e := BacklinksContext(ctx, blockview.BacklinkRequest{Mode: "links", ID: id, Keyword: keyword, MentionKeyword: mentionKeyword, BeforeLen: beforeLen, Children: children})
	return v.Box, v.Links, v.Mentions, v.ReferenceCount, v.MentionCount, e
}
func GetBacklink2Context(ctx context.Context, id, keyword, mentionKeyword string, sortMode, mentionSortMode int, children bool) (string, []*Path, []*Path, int, int, error) {
	v, e := BacklinksContext(ctx, blockview.BacklinkRequest{Mode: "links2", ID: id, Keyword: keyword, MentionKeyword: mentionKeyword, SortMode: sortMode, MentionSortMode: mentionSortMode, Children: children})
	return v.Box, v.Links, v.Mentions, v.ReferenceCount, v.MentionCount, e
}

func (o *workspaceReadOwner) blockReadServices() blockview.ReadServices {
	return blockview.BacklinkServices{DB: o.r.main.DB, Search: o.settings.Search, Parser: o.parser, Content: o.content(), Load: o.loadOptional, Trees: o.trees, Location: o.r.referenceLookup, Notebooks: o.base.owner.business.Notebooks, Render: o.renderNodes,
		Breadcrumb: func(c context.Context, n *ast.Node, embed bool, headingMode int) ([]*blockview.BlockPath, error) {
			return docquery.BreadcrumbFromNode(c, n, n.Box, n.Path, docquery.PositionRequest{Embed: embed, HeadingMode: headingMode}, o.positionServices())
		},
	}
}
