// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import "github.com/88250/lute/ast"

// RefreshUpdatedAt preserves the original parent and enclosing-heading rules.
func RefreshUpdatedAt(node *ast.Node, updated string) {
	if node == nil {
		return
	}
	node.SetIALAttr("updated", updated)
	for n, depth := node, 0; n != nil && depth <= 255; n, depth = n.Parent, depth+1 {
		parent := n.Parent
		if parent == nil {
			return
		}
		level := 7
		if n.Type == ast.NodeHeading {
			level = n.HeadingLevel
		}
		for prev := n.Previous; prev != nil; prev = prev.Previous {
			if prev.Type == ast.NodeHeading {
				if prev.HeadingLevel >= level {
					break
				}
				prev.SetIALAttr("updated", updated)
				level = prev.HeadingLevel
			}
		}
		parent.SetIALAttr("updated", updated)
		if parent.Type == ast.NodeDocument {
			return
		}
	}
}
