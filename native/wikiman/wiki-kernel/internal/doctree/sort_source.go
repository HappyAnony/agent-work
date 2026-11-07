// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

type SortMutation struct {
	Mode, Box, Metadata string
	Paths, Remove       []string
	Values              map[string]int
	SkipMissing         bool
}
type SortPlan struct {
	Name          string
	Before, After []byte
	Notice        map[string]int
}

// PrepareSortMutation preserves unrelated ranks and original sibling ordering.
// The complete selected folder and old metadata are read before publication.
func PrepareSortMutation(ctx context.Context, source indexrepair.Source, q SortMutation) (out SortPlan, err error) {
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if !ast.IsNodeIDPattern(q.Box) || (q.Metadata != ".siyuan" && q.Metadata != ".wiki") {
		return out, indexrepair.ErrSourcePath
	}
	if q.Mode == "reorder" && len(q.Paths) == 0 {
		return out, nil
	}
	if q.Mode != "reorder" && q.Mode != "merge" && q.Mode != "remove" {
		return out, errors.New("invalid_sort_mutation")
	}
	name := path.Join(q.Box, q.Metadata, "sort.json")
	before, e := source.Read(ctx, name)
	if indexrepair.IsSourceNotExist(e) {
		if q.SkipMissing || q.Mode == "remove" {
			return out, nil
		}
		before = nil
	} else if e != nil {
		return out, e
	}
	values := map[string]int{}
	if before != nil {
		if e = json.Unmarshal(before, &values); e != nil {
			return out, e
		}
		if values == nil {
			return out, errors.New("invalid_notebook_sort")
		}
	}
	notice := map[string]int{}
	if q.Mode == "reorder" {
		selected := map[string]int{}
		maximum := len(q.Paths) + 1
		for i, p := range q.Paths {
			if !strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.Contains(p, "\\") {
				return out, indexrepair.ErrSourcePath
			}
			docs, e := indexrepair.DocumentPaths([]string{q.Box + p})
			if e != nil || len(docs) != 1 {
				if e == nil {
					e = indexrepair.ErrSourcePath
				}
				return out, e
			}
			selected[strings.TrimSuffix(path.Base(p), ".docj")] = i + 1
		}
		folder := path.Join(q.Box, path.Dir(q.Paths[0]))
		entries, e := source.ReadDir(ctx, folder)
		if e != nil {
			return out, e
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".docj") {
				continue
			}
			member := path.Join(folder, entry.Name())
			if e = source.Check(member, false); e != nil {
				return out, e
			}
			info, e := source.Data.Lstat(member)
			if e != nil {
				return out, e
			}
			if !info.Mode().IsRegular() {
				return out, indexrepair.ErrSourcePath
			}
			id := strings.TrimSuffix(entry.Name(), ".docj")
			if !ast.IsNodeIDPattern(id) {
				return out, indexrepair.ErrSourcePath
			}
			rank := selected[id]
			if rank == 0 {
				rank = maximum
				maximum++
			}
			notice[id] = rank
			values[id] = rank
		}
	} else if q.Mode == "merge" {
		for id, rank := range q.Values {
			if !ast.IsNodeIDPattern(id) {
				return out, indexrepair.ErrSourcePath
			}
			values[id] = rank
			notice[id] = rank
		}
	} else {
		for _, id := range q.Remove {
			if !ast.IsNodeIDPattern(id) {
				return out, indexrepair.ErrSourcePath
			}
			delete(values, id)
		}
	}
	after, e := json.Marshal(values)
	if e != nil {
		return out, e
	}
	return SortPlan{Name: name, Before: before, After: after, Notice: notice}, ctx.Err()
}

// InsertionRank implements original new-document placement in the resolved
// sibling order.
func InsertionRank(docs []*File, top bool) int {
	if len(docs) == 0 {
		return 0
	}
	if top {
		return docs[0].Sort - 1
	}
	return docs[len(docs)-1].Sort + 1
}
