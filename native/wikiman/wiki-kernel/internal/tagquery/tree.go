// SPDX-License-Identifier: AGPL-3.0-or-later
package tagquery

import (
	"context"
	"errors"
	"sort"
	"strings"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/markup"
	"wiki-kernel/internal/textsort"
)

type Tag struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Children Tags   `json:"children"`
	Type     string `json:"type"` // "tag"
	Depth    int    `json:"depth"`
	Count    int    `json:"count"`

	tags Tags
}

type Tags []*Tag

func Build(ctx context.Context, labels map[string]int, mode, maxCount int, ignoreLimit bool) (ret Tags, exceeded bool, err error) {
	tags := Tags{}
	if len(labels) > 100000 {
		return nil, false, errors.New("tag_tree_budget")
	}
	for label := range labels {
		if len(label) > 1<<20 || strings.Count(label, "/") > 255 {
			return nil, false, errors.New("tag_tree_budget")
		}
		if err = ctx.Err(); err != nil {
			return nil, false, err
		}
		tags = buildTags(tags, strings.Split(label, "/"), 0)
	}
	appendTagChildren(&tags, labels, mode)
	sortTags(tags, mode)
	total := 0
	ret = Tags{}
	for _, tag := range tags {
		ret = append(ret, tag)
		countTag(tag, &total)
		if maxCount < total && !ignoreLimit {
			exceeded = true
			break
		}
	}
	return ret, exceeded, ctx.Err()
}
func countTag(tag *Tag, total *int) {
	*total += 1
	for _, child := range tag.tags {
		countTag(child, total)
	}
}

func sortTags(tags Tags, mode int) {
	switch mode {
	case configtypes.SortModeNameASC:
		sort.Slice(tags, func(i, j int) bool {
			return textsort.PinYinCompare(tags[i].Name, tags[j].Name)
		})
	case configtypes.SortModeNameDESC:
		sort.Slice(tags, func(j, i int) bool {
			return textsort.PinYinCompare(tags[i].Name, tags[j].Name)
		})
	case configtypes.SortModeAlphanumASC:
		sort.Slice(tags, func(i, j int) bool {
			return textsort.NaturalCompare((tags)[i].Name, (tags)[j].Name)
		})
	case configtypes.SortModeAlphanumDESC:
		sort.Slice(tags, func(i, j int) bool {
			return textsort.NaturalCompare((tags)[j].Name, (tags)[i].Name)
		})
	case configtypes.SortModeRefCountASC:
		sort.Slice(tags, func(i, j int) bool { return (tags)[i].Count < (tags)[j].Count })
	case configtypes.SortModeRefCountDESC:
		sort.Slice(tags, func(i, j int) bool { return (tags)[i].Count > (tags)[j].Count })
	default:
		sort.Slice(tags, func(i, j int) bool {
			return textsort.NaturalCompare((tags)[i].Name, (tags)[j].Name)
		})
	}
}

func appendTagChildren(tags *Tags, labels map[string]int, mode int) {
	for _, tag := range *tags {
		tag.Label = tag.Name
		if _, ok := labels[tag.Label]; ok {
			tag.Count = labels[tag.Label]
		}
		appendChildren0(tag, labels, mode)
		sortTags(tag.Children, mode)
	}
}

func appendChildren0(tag *Tag, labels map[string]int, mode int) {
	sortTags(tag.tags, mode)
	for _, t := range tag.tags {
		t.Label = tag.Label + "/" + t.Name
		if _, ok := labels[t.Label]; ok {
			t.Count = labels[t.Label]
		}
		tag.Children = append(tag.Children, t)
	}
	for _, child := range tag.tags {
		appendChildren0(child, labels, mode)
	}
}

func buildTags(root Tags, labels []string, depth int) Tags {
	if 1 > len(labels) {
		return root
	}

	i := 0
	for ; i < len(root); i++ {
		if (root)[i].Name == labels[0] {
			break
		}
	}
	if i == len(root) {
		root = append(root, &Tag{Name: markup.EscapeHTML(labels[0]), Type: "tag", Depth: depth})
	}
	depth++
	root[i].tags = buildTags(root[i].tags, labels[1:], depth)
	return root
}
