// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/88250/lute/ast"
)

type nativeTagRename struct {
	oldLabel, newLabel string
}

// PlanNativeTagRename updates tags across this knowledge base's notebooks.
// Use the source manifest rather than search results so index exclusions cannot
// leave old labels behind. Publication revalidates the whole captured snapshot.
func (l *Lease) PlanNativeTagRename(ctx context.Context, oldLabel, newLabel string) (NativeEditPlan, error) {
	return l.planNativeEdits(ctx, nil, nil, &nativeTagRename{oldLabel, newLabel})
}

func (p *nativeEdit) renameTag(rename nativeTagRename, files map[string]string) error {
	if rename.oldLabel == rename.newLabel {
		return nil
	}
	var documents []string
	for name := range files {
		parts := strings.Split(name, "/")
		if len(parts) < 2 || !strings.HasSuffix(name, ".docj") ||
			!documentID.MatchString(strings.TrimSuffix(parts[len(parts)-1], ".docj")) {
			continue
		}
		valid := true
		for _, part := range parts[:len(parts)-1] {
			if !documentID.MatchString(part) {
				valid = false
				break
			}
		}
		if valid {
			documents = append(documents, name)
		}
	}
	slices.Sort(documents)
	next := func(label string) string {
		if label == rename.oldLabel || strings.HasPrefix(label, rename.oldLabel+"/") {
			return rename.newLabel + strings.TrimPrefix(label, rename.oldLabel)
		}
		return label
	}
	for _, name := range documents {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		box, relative, _ := strings.Cut(name, "/")
		tree, err := p.load(box, "/"+relative, p.parser)
		if err != nil {
			return err
		}
		changed := false
		tags := strings.Split(tree.Root.IALAttr("tags"), ",")
		for i, label := range tags {
			if tags[i] = next(label); tags[i] != label {
				changed = true
			}
		}
		if changed {
			tree.Root.SetIALAttr("tags", strings.Join(tags, ","))
		}
		ast.Walk(tree.Root, func(node *ast.Node, entering bool) ast.WalkStatus {
			if p.ctx.Err() != nil {
				return ast.WalkStop
			}
			if entering && node.Type == ast.NodeTextMark && node.IsTextMarkType("tag") {
				label := next(node.TextMarkTextContent)
				if label != node.TextMarkTextContent {
					node.TextMarkTextContent = label
					changed = true
					for parent := node.Parent; parent != nil; parent = parent.Parent {
						if parent.IsBlock() && parent.ID != "" {
							p.tx.Nodes[parent.ID] = parent
							break
						}
					}
				}
			}
			return ast.WalkContinue
		})
		if err := p.ctx.Err(); err != nil {
			return err
		}
		if !changed {
			delete(p.tx.Trees, tree.ID)
			delete(p.before, tree.ID)
			delete(p.normalized, tree.ID)
			continue
		}
		if len(p.direct) >= 1000 {
			return ErrBudget
		}
		if key, err := treeRelative(box, tree.Path); err != nil || path.Clean(key) != name {
			return ErrDocument
		}
		p.direct[name] = files[name]
		if err := p.stage(tree); err != nil {
			return err
		}
	}
	return nil
}
