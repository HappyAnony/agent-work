// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/88250/gulu"
	"github.com/siyuan-note/filelock"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// Same payload as the original filetreeSortChanged event, frozen before its
// deferred publication so dispatch never selects another blocktree registry.
type markdownSortChange struct {
	ParentPath string   `json:"parentPath"`
	ChildIDs   []string `json:"childIDs"`
}

func (a *markdownImportAssets) sortNotice(box string, values map[string]int) (*commitNotice, error) {
	if err := a.guard(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	// Preserve the original rank comparison (equal ranks remain unspecified).
	sort.Slice(ids, func(i, j int) bool { return values[ids[i]] < values[ids[j]] })
	rows, err := a.authority.binding.scope.commit.r.registry.Query(a.authority.ctx, indexdb.BlockTreeFilter{Kind: "id", Value: ids[0]})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, a.guard()
	}
	row := rows[0]
	if row.BoxID != box || row.ID != ids[0] || !path.IsAbs(row.Path) || path.Clean(row.Path) != row.Path || strings.Contains(row.Path, "\\") {
		return nil, indexrepair.ErrSourcePath
	}
	raw, err := json.Marshal(markdownSortChange{ParentPath: path.Dir(row.Path), ChildIDs: ids})
	if err != nil {
		return nil, err
	}
	return &commitNotice{kind: "filetree-sort", payload: raw}, a.guard()
}

// Missing sort.json still means skip. Existing entries and caller rank values
// are retained; malformed/read/write failures are error-valued rather than logs.
func (a *markdownImportAssets) persistSort(box string, values map[string]int) ([]commitNotice, error) {
	if err := validateArchiveDestination(box, "/"); err != nil {
		return nil, err
	}
	if err := a.guard(); err != nil {
		return nil, err
	}
	name := path.Join(box, ".siyuan", "sort.json")
	before, err := a.target.Read(a.authority.ctx, name)
	if indexrepair.IsSourceNotExist(err) {
		return nil, a.guard()
	}
	if err != nil {
		return nil, err
	}
	merged := map[string]int{}
	if err = gulu.JSON.UnmarshalJSON(before, &merged); err != nil {
		return nil, err
	}
	if merged == nil {
		return nil, errors.New("invalid_notebook_sort")
	}
	for id, rank := range values {
		merged[id] = rank
	}
	notice, err := a.sortNotice(box, values)
	if err != nil {
		return nil, err
	}
	after, err := gulu.JSON.MarshalJSON(merged)
	if err != nil {
		return nil, err
	}
	target := a.target
	boxInfo, err := target.Data.Lstat(box)
	if err != nil {
		return nil, err
	}
	target.Authorize = func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.guard(); err != nil {
			return err
		}
		if err := target.Check(box, false); err != nil {
			return err
		}
		current, err := target.Data.Lstat(box)
		if err != nil {
			return err
		}
		if !current.IsDir() || !os.SameFile(boxInfo, current) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	target.Lock = func(name string) func() {
		p := filepath.Join(a.authority.binding.scope.commit.r.data, filepath.FromSlash(name))
		filelock.Lock(p)
		return func() { filelock.Unlock(p) }
	}
	if err = target.Rewrite(a.authority.ctx, name, before, after); err != nil {
		return nil, err
	}
	if err = a.guard(); err != nil {
		return nil, err
	}
	if notice == nil {
		return nil, nil
	}
	return []commitNotice{*notice}, nil
}
func publishMarkdownSortChange(raw []byte) {
	var change markdownSortChange
	if json.Unmarshal(raw, &change) != nil || len(change.ChildIDs) == 0 {
		return
	}
	util.BroadcastByType("main", "filetreeSortChanged", 0, "", map[string]any{"parentPath": change.ParentPath, "childIDs": change.ChildIDs})
}
