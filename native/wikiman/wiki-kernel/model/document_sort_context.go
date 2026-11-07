// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"path"
	"sort"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func prepareOwnedSortNotice(ctx context.Context, r *indexRepairContext, box string, values map[string]int) (*markdownSortChange, error) {
	if e := r.guard(ctx); e != nil {
		return nil, e
	}
	if len(values) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return values[ids[i]] < values[ids[j]] })
	rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: ids[0]})
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	if row.BoxID != box || row.ID != ids[0] || !path.IsAbs(row.Path) || path.Clean(row.Path) != row.Path {
		return nil, indexrepair.ErrSourcePath
	}
	return &markdownSortChange{ParentPath: path.Dir(row.Path), ChildIDs: ids}, nil
}
func publishOwnedSortNotice(ctx context.Context, r *indexRepairContext, notice *markdownSortChange) error {
	if e := r.guard(ctx); e != nil {
		return e
	}
	if notice == nil {
		return nil
	}
	r.owner.publishSyncUI(ctx, func() {
		util.BroadcastByType("main", "filetreeSortChanged", 0, "", map[string]any{"parentPath": notice.ParentPath, "childIDs": append([]string(nil), notice.ChildIDs...)})
	})
	return nil
}

func applyOwnedDocumentSort(ctx context.Context, r *indexRepairContext, q doctree.SortMutation, notify bool) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	io, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return e
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	q.Metadata = ".siyuan"
	plan, e := doctree.PrepareSortMutation(ctx, io.Source, q)
	if e != nil {
		return e
	}
	if plan.Name == "" {
		return nil
	}
	var notice *markdownSortChange
	if notify {
		notice, e = prepareOwnedSortNotice(ctx, r, q.Box, plan.Notice)
		if e != nil {
			return e
		}
	}
	if e = io.Source.Rewrite(ctx, plan.Name, plan.Before, plan.After); e != nil {
		return e
	}
	if e = closeIO(); e != nil {
		return e
	}
	return publishOwnedSortNotice(ctx, r, notice)
}

func ChangeFileTreeSortContext(ctx context.Context, box string, paths []string) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	paths = append([]string(nil), paths...)
	return base.run(ctx, func(c, life context.Context) error {
		if e := r.guard(c); e != nil {
			return e
		}
		if len(paths) == 0 {
			return nil
		}
		if e := FlushTxQueueContext(c); e != nil {
			return e
		}
		boxes, e := base.owner.business.Notebooks(c)
		if e != nil {
			return e
		}
		found := false
		for _, b := range boxes {
			if b.ID == box && !b.Closed {
				found = true
				break
			}
		}
		if !found {
			return ErrBoxNotFound
		}
		if e = applyOwnedDocumentSort(c, r, doctree.SortMutation{Mode: "reorder", Box: box, Paths: paths}, true); e != nil {
			return e
		}
		return base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
			if v.Sync == nil {
				return conf.ErrBusinessUnbound
			}
			if e := base.guard(c); e != nil {
				return e
			}
			base.owner.syncRuntime.same.Store(0)
			base.owner.planSyncAfter(time.Duration(v.Sync.Interval) * time.Second)
			return nil
		})
	})
}

func mutateDocumentSortContext(ctx context.Context, q doctree.SortMutation, notify bool) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	return base.run(ctx, func(c, life context.Context) error { return applyOwnedDocumentSort(c, r, q, notify) })
}

func InsertDocumentSortContext(ctx context.Context, box, parent, id, previous string, top *bool) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	return base.run(ctx, func(c, life context.Context) error {
		if previous != "" {
			return r.addConflictSort(c, box, previous, id)
		}
		atTop := false
		if top != nil {
			atTop = *top
		} else if e := base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
			if v.FileTree == nil || v.FileTree.CreateDocAtTop == nil {
				return conf.ErrBusinessUnbound
			}
			atTop = *v.FileTree.CreateDocAtTop
			return nil
		}); e != nil {
			return e
		}
		max := 102400
		if atTop {
			max = 1
		}
		docs, _, e := listDocTreeContext(c, box, parent, util.SortModeUnassigned, false, false, max, false)
		if e != nil {
			return e
		}
		rank := doctree.InsertionRank(docs, atTop)
		if e = applyOwnedDocumentSort(c, r, doctree.SortMutation{Mode: "merge", Box: box, Values: map[string]int{id: rank}}, false); e != nil {
			return e
		}
		notice := map[string]int{id: rank}
		for _, doc := range docs {
			notice[doc.ID] = doc.Sort
		}
		prepared, e := prepareOwnedSortNotice(c, r, box, notice)
		if e != nil {
			return e
		}
		return publishOwnedSortNotice(c, r, prepared)
	})
}
