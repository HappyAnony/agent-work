// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

// A detached metadata candidate. Native source publication remains gated by R04.
func (l *Lease) PlanNativeDocumentSort(ctx context.Context, q doctree.SortMutation) (out doctree.SortPlan, err error) {
	q.Paths = append([]string(nil), q.Paths...)
	q.Remove = append([]string(nil), q.Remove...)
	values := map[string]int{}
	for id, rank := range q.Values {
		values[id] = rank
	}
	q.Values = values
	q.Metadata = ".wiki"
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		out, e = doctree.PrepareSortMutation(ctx, indexrepair.Source{Data: l.workspace.data}, q)
		return e
	})
	if err != nil {
		return doctree.SortPlan{}, err
	}
	return
}
