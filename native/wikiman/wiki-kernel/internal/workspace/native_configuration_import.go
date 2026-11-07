// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/notebooks"
)

// Native import produces the same twelve-group candidate and classified delta;
// it does not save preferences, publish effects or rewrite source metadata.
func (l *Lease) PlanNativeConfigurationImport(ctx context.Context, raw []byte) (out NativeSettingPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		before := l.workspace.resources.configuration
		plan, e := before.PrepareImport(ctx, raw)
		if e != nil {
			return e
		}
		patches, e := plan.PreferencePatches(before)
		if e != nil {
			return e
		}
		out = NativeSettingPlan{Setting: plan, Patches: patches}
		return nil
	})
	if err != nil {
		return NativeSettingPlan{}, err
	}
	return
}

// The candidate retains exact corrupt preimages and missing markers. Only an
// authoritative later source transaction may publish these repairs and effects.
func (l *Lease) PlanNativeNotebookRepairs(ctx context.Context) (out notebooks.Plan, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		order := 0
		if r.configuration.FileTree != nil {
			order = r.configuration.FileTree.Sort
		}
		var e error
		out, e = notebooks.Scan(ctx, w.data, "auto", order)
		return e
	})
	if err != nil {
		return notebooks.Plan{}, err
	}
	return
}
