// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"fmt"
	"sort"

	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/tagquery"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func RemoveTag(label string) error {
	return mutateTagWithEnvironment(label, "", false, legacyTagMutationEnvironment())
}
func RenameTag(oldLabel, newLabel string) error {
	return mutateTagWithEnvironment(oldLabel, newLabel, true, legacyTagMutationEnvironment())
}

type TagBlocks []*Block

func (s TagBlocks) Len() int           { return len(s) }
func (s TagBlocks) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s TagBlocks) Less(i, j int) bool { return s[i].ID < s[j].ID }

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

func BuildTags(ignoreMaxListHintArg bool, appID string) *Tags {
	FlushTxQueue()
	sql.FlushQueue()
	labels := map[string]int{}
	for label, tags := range labelTags() {
		labels[label] = len(tags) + 1
	}
	tree, exceeded, err := tagquery.Build(context.Background(), labels, Conf.Tag.Sort, Conf.FileTree.MaxListCount, ignoreMaxListHintArg)
	if err != nil {
		logging.LogErrorf("build tags failed: %s", err)
	}
	if exceeded {
		util.PushMsgWithApp(appID, fmt.Sprintf(Conf.Language(243), Conf.FileTree.MaxListCount), 7000)
	}
	var convert func(tagquery.Tags) Tags
	convert = func(src tagquery.Tags) Tags {
		if src == nil {
			return nil
		}
		ret := Tags{}
		for _, n := range src {
			ret = append(ret, &Tag{Name: n.Name, Label: n.Label, Type: n.Type, Depth: n.Depth, Count: n.Count, Children: convert(n.Children)})
		}
		return ret
	}
	ret := convert(tree)
	return &ret
}

func countTag(tag *Tag, total *int) {
	*total += 1
	for _, child := range tag.tags {
		countTag(child, total)
	}
}

func sortTags(tags Tags) {
	switch Conf.Tag.Sort {
	case util.SortModeNameASC:
		sort.Slice(tags, func(i, j int) bool {
			return util.PinYinCompare(tags[i].Name, tags[j].Name)
		})
	case util.SortModeNameDESC:
		sort.Slice(tags, func(j, i int) bool {
			return util.PinYinCompare(tags[i].Name, tags[j].Name)
		})
	case util.SortModeAlphanumASC:
		sort.Slice(tags, func(i, j int) bool {
			return util.NaturalCompare((tags)[i].Name, (tags)[j].Name)
		})
	case util.SortModeAlphanumDESC:
		sort.Slice(tags, func(i, j int) bool {
			return util.NaturalCompare((tags)[j].Name, (tags)[i].Name)
		})
	case util.SortModeRefCountASC:
		sort.Slice(tags, func(i, j int) bool { return (tags)[i].Count < (tags)[j].Count })
	case util.SortModeRefCountDESC:
		sort.Slice(tags, func(i, j int) bool { return (tags)[i].Count > (tags)[j].Count })
	default:
		sort.Slice(tags, func(i, j int) bool {
			return util.NaturalCompare((tags)[i].Name, (tags)[j].Name)
		})
	}
}

func SearchTags(keyword string) (ret []string) {
	sql.FlushQueue()
	ret, err := sql.SearchTagLabels(context.Background(), keyword, Conf.Search.Limit, Conf.Search.CaseSensitive)
	if err != nil {
		logging.LogErrorf("search tags failed: %s", err)
	}
	return ret
}

func labelBlocksByKeyword(keyword string) map[string]TagBlocks {
	ret, err := tagBlocksWithEnvironment(keyword, legacyTagNavigationEnvironment())
	if err != nil {
		logging.LogErrorf("tag navigation failed: %s", err)
	}
	return ret
}

func labelTags() (ret map[string]Tags) {
	ret = map[string]Tags{}

	tagSpans := sql.QueryTagSpans("")
	for _, tagSpan := range tagSpans {
		label := util.UnescapeHTML(tagSpan.Content)
		if _, ok := ret[label]; ok {
			ret[label] = append(ret[label], &Tag{})
		} else {
			ret[label] = Tags{}
		}
	}
	return
}

func appendTagChildren(tags *Tags, labels map[string]Tags) {
	for _, tag := range *tags {
		tag.Label = tag.Name
		if _, ok := labels[tag.Label]; ok {
			tag.Count = len(labels[tag.Label]) + 1
		}
		appendChildren0(tag, labels)
		sortTags(tag.Children)
	}
}

func appendChildren0(tag *Tag, labels map[string]Tags) {
	sortTags(tag.tags)
	for _, t := range tag.tags {
		t.Label = tag.Label + "/" + t.Name
		if _, ok := labels[t.Label]; ok {
			t.Count = len(labels[t.Label]) + 1
		}
		tag.Children = append(tag.Children, t)
	}
	for _, child := range tag.tags {
		appendChildren0(child, labels)
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
		root = append(root, &Tag{Name: util.EscapeHTML(labels[0]), Type: "tag", Depth: depth})
	}
	depth++
	root[i].tags = buildTags(root[i].tags, labels[1:], depth)
	return root
}
