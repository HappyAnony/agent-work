// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model.SearchAttributeView ranking and result construction.
package avquery

import (
	"context"
	"errors"
	"github.com/xrash/smetrics"
	"sort"
	"strings"
)

type View struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Layout string `json:"type"`
}
type Entry struct {
	ID, Name string
	Updated  int64
}
type Location struct{ BlockID, HPath string }
type Result struct {
	AvID       string    `json:"avID"`
	AvName     string    `json:"avName"`
	ViewName   string    `json:"viewName"`
	ViewID     string    `json:"viewID"`
	ViewLayout string    `json:"viewLayout"`
	BlockID    string    `json:"blockID"`
	HPath      string    `json:"hPath"`
	Children   []*Result `json:"children,omitempty"`
}
type Services struct {
	Strict    bool
	Entries   []Entry
	Relations map[string][]string
	Resolve   func(context.Context, []string) (Location, bool, error)
	Views     func(context.Context, string) ([]View, error)
}

func Search(ctx context.Context, keyword string, exclude []string, s Services) (ret []*Result, err error) {
	if s.Resolve == nil || s.Views == nil {
		return nil, errors.New("av_query_dependency_unbound")
	}
	ret = []*Result{}
	keyword = strings.TrimSpace(keyword)
	words := strings.Fields(keyword)
	excluded := map[string]bool{}
	for _, id := range exclude {
		excluded[id] = true
	}
	type scored struct {
		Entry
		score float64
	}
	var matches []scored
	for _, entry := range s.Entries {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if excluded[entry.ID] || s.Relations[entry.ID] == nil {
			continue
		}
		score := 0.0
		hit := true
		for _, word := range words {
			if !strings.Contains(strings.ToLower(entry.Name), strings.ToLower(word)) {
				hit = false
				break
			}
			score += smetrics.JaroWinkler(entry.Name, word, 0.7, 4)
		}
		if keyword == "" || hit {
			matches = append(matches, scored{Entry: entry, score: score})
		}
	}
	if keyword == "" {
		sort.Slice(matches, func(i, j int) bool { return matches[i].Updated > matches[j].Updated })
	} else {
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].score == matches[j].score {
				return matches[i].Updated > matches[j].Updated
			}
			return matches[i].score > matches[j].score
		})
	}
	if len(matches) >= 12 {
		matches = matches[:12]
	}
	for _, match := range matches {
		where, ok, e := s.Resolve(ctx, append([]string(nil), s.Relations[match.ID]...))
		if e != nil {
			if s.Strict {
				return nil, e
			}
			continue
		}
		if !ok {
			continue
		}
		views, e := s.Views(ctx, match.ID)
		if e != nil {
			if s.Strict {
				return nil, e
			}
			continue
		}
		parent := &Result{AvID: match.ID, AvName: match.Name, BlockID: where.BlockID, HPath: where.HPath}
		for _, view := range views {
			parent.Children = append(parent.Children, &Result{AvID: match.ID, AvName: match.Name, ViewName: view.Name, ViewID: view.ID, ViewLayout: view.Layout, BlockID: where.BlockID, HPath: where.HPath})
		}
		ret = append(ret, parent)
	}
	return ret, ctx.Err()
}
