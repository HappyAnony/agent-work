// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

import (
	"context"
	"github.com/88250/lute/ast"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

type IDNode struct {
	ID       string    `json:"id"`
	Children []*IDNode `json:"children,omitempty"`
}

func IDs(ctx context.Context, source indexrepair.Source, box, p string) ([]*IDNode, error) {
	p = strings.Trim(strings.TrimSuffix(p, ".docj"), "/")
	if !ast.IsNodeIDPattern(box) || (p != "" && !fs.ValidPath(p)) {
		return nil, indexrepair.ErrSourcePath
	}
	seen := map[string]bool{}
	count := 0
	var walk func(string, int) ([]*IDNode, error)
	walk = func(name string, depth int) ([]*IDNode, error) {
		if depth > 255 {
			return nil, indexrepair.ErrSourcePath
		}
		entries, e := source.ReadDir(ctx, name)
		if e != nil {
			return nil, e
		}
		out := []*IDNode{}
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			n := entry.Name()
			if strings.HasPrefix(n, ".") {
				continue
			}
			if !entry.IsDir() && !strings.HasSuffix(n, ".docj") {
				continue
			}
			id := strings.TrimSuffix(n, ".docj")
			if !ast.IsNodeIDPattern(id) {
				continue
			}
			if e = source.Check(path.Join(name, n), false); e != nil {
				return nil, e
			}
			count++
			if count > 1000000 {
				return nil, indexrepair.ErrSourcePath
			}
			node := &IDNode{ID: id}
			if entry.IsDir() {
				node.Children, e = walk(path.Join(name, n), depth+1)
				if e != nil {
					return nil, e
				}
				out = append(out, node)
				seen[id] = true
			} else if !seen[id] {
				out = append(out, node)
				seen[id] = true
			}
		}
		return out, ctx.Err()
	}
	return walk(path.Join(box, p), 0)
}
