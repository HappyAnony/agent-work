// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"context"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

func AssetDirectories(ctx context.Context, source indexrepair.Source, root string) ([]string, error) {
	out := []string{}
	count := 0
	var walk func(string, int) error
	walk = func(name string, depth int) error {
		if depth > 255 {
			return indexrepair.ErrSourcePath
		}
		entries, e := source.ReadDir(ctx, name)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			n := entry.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" || n == "dist" || n == "target" {
				continue
			}
			if !entry.IsDir() {
				continue
			}
			count++
			if count > 1000000 {
				return indexrepair.ErrSourcePath
			}
			child := path.Join(name, n)
			if e = source.Check(child, false); e != nil {
				return e
			}
			if n == "assets" {
				out = append(out, child)
				continue
			}
			if e = walk(child, depth+1); e != nil {
				return e
			}
		}
		return ctx.Err()
	}
	if e := walk(root, 0); e != nil {
		return nil, e
	}
	return out, ctx.Err()
}
