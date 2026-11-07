// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"bytes"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"html"
	"slices"
	"strings"
)

type CoverServices struct {
	Parser  *lute.Lute
	IsImage func(string) (bool, error)
	Warning func(error)
}

func IsImageExtension(ext string) bool {
	switch ext {
	case ".apng", ".ico", ".cur", ".jpg", ".jpe", ".jpeg", ".jfif", ".pjp", ".pjpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif":
		return true
	}
	return false
}

func nodeInTree(t *parse.Tree, id string) (ret *ast.Node) {
	ast.Walk(t.Root, func(n *ast.Node, e bool) ast.WalkStatus {
		if e && n.ID == id {
			ret = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return
}

// Original title-image CSS and src token rules, without device paths/loggers.
func docTitleImage(root *ast.Node) string {
	const background = "background-image: url("
	s := strings.TrimSpace(root.IALAttr("title-img"))
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "background-image:url(", background)
	if !strings.Contains(s, background) {
		return ""
	}
	a, b := strings.Index(s, background)+len(background), strings.LastIndex(s, ")")
	if b < a {
		return ""
	}
	s = s[a:b]
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimPrefix(s, "'")
	s = strings.TrimSuffix(s, "\"")
	return strings.TrimSuffix(s, "'")
}

func nodeSrc(n *ast.Node) string {
	if i := bytes.Index(n.Tokens, []byte("src=\"")); i > 0 {
		s := n.Tokens[i+5:]
		if j := bytes.IndexByte(s, '"'); j > 0 {
			return strings.TrimSpace(string(s[:j]))
		}
	}
	return ""
}

// Cover rendering temporarily changes IDs/IAL/autoplay. Restore every AST node,
// including tokens, even if a renderer panics; source trees are never edited.
func snapshotCoverNodes(root *ast.Node) func() {
	nodes := map[*ast.Node]ast.Node{}
	ast.Walk(root, func(n *ast.Node, e bool) ast.WalkStatus {
		if e {
			copy := *n
			copy.Tokens = bytes.Clone(n.Tokens)
			if n.KramdownIAL != nil {
				copy.KramdownIAL = make([][]string, len(n.KramdownIAL))
			}
			for i, v := range n.KramdownIAL {
				copy.KramdownIAL[i] = slices.Clone(v)
			}
			nodes[n] = copy
		}
		return ast.WalkContinue
	})
	return func() {
		for n, copy := range nodes {
			*n = copy
		}
	}
}
