// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"context"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

// Inventory preserves notebook-first/global-last asset lookup precedence.
// Every selected directory/read failure aborts the complete result.
func Inventory(ctx context.Context, source indexrepair.Source, boxes []string) (map[string]string, error) {
	out := map[string]string{}
	count := 0
	skip := func(n string) bool {
		return strings.HasPrefix(n, ".") || n == "node_modules" || n == "dist" || n == "target"
	}
	var walk func(string, bool, int) error
	walk = func(name string, assets bool, depth int) error {
		if depth > 255 {
			return indexrepair.ErrSourcePath
		}
		entries, e := source.ReadDir(ctx, name)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				return e
			}
			if skip(entry.Name()) {
				continue
			}
			count++
			if count > 1000000 {
				return indexrepair.ErrSourcePath
			}
			rel := path.Join(name, entry.Name())
			if e = source.Check(rel, false); e != nil {
				return e
			}
			childAssets := assets || entry.IsDir() && entry.Name() == "assets"
			if assets {
				idx := strings.Index(rel, "assets/")
				if idx < 0 {
					return indexrepair.ErrSourcePath
				}
				key := rel[idx:]
				if entry.IsDir() {
					key += "/"
				}
				out[key] = rel
			}
			if entry.IsDir() {
				if e = walk(rel, childAssets, depth+1); e != nil {
					return e
				}
			}
		}
		return ctx.Err()
	}
	for _, box := range boxes {
		if e := walk(box, false, 0); e != nil {
			return nil, e
		}
	}
	if e := walk("assets", true, 0); e != nil && !indexrepair.IsSourceNotExist(e) {
		return nil, e
	}
	return out, ctx.Err()
}
