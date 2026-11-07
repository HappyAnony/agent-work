// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"maps"
	"slices"
	"sort"
	"strings"
	"wiki-kernel/internal/indexdb"
)

type BookmarkLabel string
type BookmarkBlocks []*Block
type Bookmark struct {
	Name   BookmarkLabel `json:"name"`
	Blocks []*Block      `json:"blocks"`
	Type   string        `json:"type"`
	Depth  int           `json:"depth"`
	Count  int           `json:"count"`
}
type Bookmarks []*Bookmark

func (s Bookmarks) Len() int           { return len(s) }
func (s Bookmarks) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s Bookmarks) Less(i, j int) bool { return s[i].Name < s[j].Name }

type BookmarkServices struct {
	Legacy            bool
	Query             func(context.Context) ([]indexdb.BlockRecord, error)
	Parser            func(context.Context) (*lute.Lute, error)
	Node              func(context.Context, string) (*ast.Node, error)
	AttributeViewName func(context.Context, string) (string, error)
	Warn              func(string, error)
}

func ValidateBookmarkServices(ctx context.Context, s BookmarkServices) error {
	if ctx == nil || s.Query == nil || s.Parser == nil || s.Node == nil || s.AttributeViewName == nil {
		return errors.New("bookmark_read_dependencies_unbound")
	}
	return ctx.Err()
}

// BookmarksView retains original SQL order within lexically sorted label groups.
// Native errors fail the whole projection; only the explicit legacy adapter
// keeps the original missing-tree/content fallback and AV best-effort policy.
func BookmarksView(ctx context.Context, caseSensitive bool, s BookmarkServices) (ret Bookmarks, err error) {
	if err = ValidateBookmarkServices(ctx, s); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.Query(ctx)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	parser, err := s.Parser(ctx)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if parser == nil {
		return nil, errors.New("bookmark_parser_required")
	}
	groups := map[BookmarkLabel]BookmarkBlocks{}
	for _, row := range rows {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		block := FromRow(&row, "", 0, caseSensitive)
		if block.Name != "" {
			block.Content = block.Name
		} else if block.Type == "NodeAttributeView" {
			// The original generic helper can slice backwards if the opening
			// av-id quote is the first quote in the markup. Read after the prefix.
			id := ""
			if _, tail, ok := strings.Cut(block.Markdown, "av-id=\""); ok {
				if value, _, closed := strings.Cut(tail, "\""); closed {
					id = value
				}
			}
			name, e := s.AttributeViewName(ctx, id)
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			if e != nil && !s.Legacy {
				return nil, e
			}
			block.Content = name
		} else {
			n, e := s.Node(ctx, block.ID)
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			if e != nil {
				if !s.Legacy {
					return nil, e
				}
				if s.Warn != nil {
					s.Warn(block.ID, e)
				}
			} else {
				if !s.Legacy && (n == nil || n.ID != block.ID) {
					return nil, errors.New("bookmark_node_missing")
				}
				if s.Legacy {
					block.Content = Outline(n, parser)
				} else {
					block.Content, err = OutlinePreview(ctx, n, parser)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		block.Depth = 1
		label := BookmarkLabel(block.IAL["bookmark"])
		groups[label] = append(groups[label], block)
	}
	ret = Bookmarks{}
	for label, blocks := range groups {
		ret = append(ret, &Bookmark{Name: label, Blocks: blocks, Type: "bookmark", Count: len(blocks)})
	}
	sort.Sort(ret)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

// Outline's original renderer temporarily edits style/links. Native preview
// borrows a serialized owner tree and restores every node, even on panic.
func OutlinePreview(ctx context.Context, n *ast.Node, parser *lute.Lute) (text string, err error) {
	if ctx == nil || parser == nil {
		return "", errors.New("bookmark_read_dependencies_unbound")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if n == nil {
		return "", nil
	}
	nodes := map[*ast.Node]ast.Node{}
	ast.Walk(n, func(node *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			copy := *node
			copy.Tokens = bytes.Clone(node.Tokens)
			copy.Children = slices.Clone(node.Children)
			copy.Properties = maps.Clone(node.Properties)
			if node.KramdownIAL != nil {
				copy.KramdownIAL = make([][]string, len(node.KramdownIAL))
				for i, v := range node.KramdownIAL {
					copy.KramdownIAL[i] = slices.Clone(v)
				}
			}
			nodes[node] = copy
		}
		return ast.WalkContinue
	})
	defer func() {
		for node, copy := range nodes {
			*node = copy
		}
		if recover() != nil {
			text = ""
			err = errors.New("bookmark_outline_invalid")
		}
	}()
	text = Outline(n, parser)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text, nil
}
