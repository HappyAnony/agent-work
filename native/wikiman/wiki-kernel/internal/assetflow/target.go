// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"context"
	"path"
	"wiki-kernel/internal/indexrepair"
)

func TargetDirectory(ctx context.Context, source indexrepair.Source, box, document string) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if box != "" {
		for _, candidate := range []string{path.Join(box, path.Dir(document), "assets"), path.Join(box, "assets")} {
			e := source.Check(candidate, false)
			if indexrepair.IsSourceNotExist(e) {
				continue
			}
			if e != nil {
				return "", e
			}
			info, e := source.Data.Lstat(candidate)
			if e != nil {
				return "", e
			}
			if !info.IsDir() {
				return "", indexrepair.ErrSourcePath
			}
			return candidate, nil
		}
	}
	return "assets", nil
}
