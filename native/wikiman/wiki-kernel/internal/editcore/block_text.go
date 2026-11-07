// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import (
	ast "github.com/88250/lute/ast"
	strings "strings"
	av "wiki-kernel/internal/avmodel"
	util "wiki-kernel/internal/markup"
)

// Same label precedence as the original block helper, with owner reads supplied.
func AttributeViewBlockTextWithServices(n *ast.Node, avID string, reference func(*ast.Node) (string, error), untitled func() string) (icon, content string, err error) {
	if n == nil {
		return
	}
	icon = n.IALAttr("icon")
	if name := n.IALAttr("name"); name != "" {
		content = util.EscapeHTML(strings.TrimSpace(name))
	} else {
		content, err = reference(n)
		if err != nil {
			return
		}
	}
	content = strings.TrimSpace(content)
	if avID != "" {
		if static := n.IALAttr(av.NodeAttrViewStaticText + "-" + avID); static != "" {
			content = static
		}
	}
	if content == "" {
		content = untitled()
	}
	return
}
