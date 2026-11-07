// SPDX-License-Identifier: AGPL-3.0-or-later
package docquery

import (
	"context"
	"errors"
	"wiki-kernel/internal/indexdb"
)

// CheckReferences preserves the original root/all-children and direct block
// count rules. Extending container descendants is a separate upstream feature.
func CheckReferences(ctx context.Context, ids []string, lookup func(context.Context, []string) ([]indexdb.BlockTreeRecord, error), refs indexdb.References) (bool, error) {
	if ctx == nil || lookup == nil || refs.DB == nil {
		return false, errors.New("reference_check_dependencies_unbound")
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	rows, e := lookup(ctx, append([]string(nil), ids...))
	if e != nil {
		return false, e
	}
	blocks := []string{}
	seen := map[string]bool{}
	positive := func(values map[string]int) bool {
		for _, count := range values {
			if count > 0 {
				return true
			}
		}
		return false
	}
	for _, r := range rows {
		if e := ctx.Err(); e != nil {
			return false, e
		}
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if r.Type == "d" {
			counts, e := refs.RootChildrenCounts(ctx, r.ID)
			if e != nil {
				return false, e
			}
			if positive(counts) {
				return true, nil
			}
		} else {
			blocks = append(blocks, r.ID)
		}
	}
	counts, e := refs.DefinitionCounts(ctx, blocks)
	if e != nil {
		return false, e
	}
	return positive(counts), ctx.Err()
}
