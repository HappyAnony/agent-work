// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

// PlanNativeAssetRename returns source revisions and detached byte candidates.
// It supplies no authoritative source publication capability before R04.
func (l *Lease) PlanNativeAssetRename(ctx context.Context, old, name string) (out assetflow.RenamePlan, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		boxes, e := r.configurationBusiness.Notebooks(ctx)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, box := range boxes {
			ids = append(ids, box.ID)
		}
		out, e = assetflow.PrepareRename(ctx, indexrepair.Source{Data: w.data}, ids, old, name, ast.NewNodeID)
		return e
	})
	if err != nil {
		return assetflow.RenamePlan{}, err
	}
	return out, nil
}
