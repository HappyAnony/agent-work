// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

var ErrNotFound = errors.New("asset_not_found")

func Resolve(ctx context.Context, source indexrepair.Source, boxes []string, relative string) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	relative = strings.TrimSpace(relative)
	if i := strings.Index(relative, "?"); i >= 0 {
		relative = relative[:i]
	}
	if !fs.ValidPath(relative) || relative == "." {
		return "", indexrepair.ErrSourcePath
	}
	if e := source.Check(relative, false); e == nil {
		return relative, ctx.Err()
	} else if !indexrepair.IsSourceNotExist(e) {
		return "", e
	}
	var walk func(string, int) (string, error)
	count := 0
	walk = func(name string, depth int) (string, error) {
		if depth > 255 {
			return "", indexrepair.ErrSourcePath
		}
		entries, e := source.ReadDir(ctx, name)
		if e != nil {
			return "", e
		}
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				return "", e
			}
			n := entry.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" || n == "dist" || n == "target" {
				continue
			}
			count++
			if count > 1000000 {
				return "", indexrepair.ErrSourcePath
			}
			child := path.Join(name, n)
			if e = source.Check(child, false); e != nil {
				return "", e
			}
			if strings.HasSuffix(child, relative) {
				return child, nil
			}
			if entry.IsDir() {
				out, e := walk(child, depth+1)
				if e != nil {
					return "", e
				}
				if out != "" {
					return out, nil
				}
			}
		}
		return "", ctx.Err()
	}
	for _, box := range boxes {
		if !fs.ValidPath(box) || strings.Contains(box, "/") {
			return "", indexrepair.ErrSourcePath
		}
		out, e := walk(box, 0)
		if e != nil {
			return "", e
		}
		if out != "" {
			return out, ctx.Err()
		}
	}
	return "", ErrNotFound
}
