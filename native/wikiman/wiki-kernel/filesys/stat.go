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

package filesys

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/util"
)

func legacyStatServices() docquery.StatServices {
	return docquery.StatServices{Trees: func(_ context.Context, ids []string) (map[string]*parse.Tree, error) { return LoadTrees(ids), nil }, Parser: func(context.Context) (*lute.Lute, error) { return util.NewLute(), nil }, AttributeView: func(_ context.Context, id string) (*avmodel.AttributeView, error) {
		a, _ := av.ParseAttributeView(id)
		return a, nil
	}, IgnoreAVErrors: true}
}
func ContentStat(content string) *util.BlockStatResult {
	v, _ := docquery.Statistics(context.Background(), docquery.StatRequest{Mode: "content", Content: content}, legacyStatServices())
	return v
}
func contentStat(content string, p *lute.Lute) *util.BlockStatResult {
	v, _ := docquery.ContentStatistics(context.Background(), content, p)
	return v
}
func StatBlock(id string) *util.BlockStatResult {
	v, _ := docquery.Statistics(context.Background(), docquery.StatRequest{Mode: "block", IDs: []string{id}}, legacyStatServices())
	return v
}
func StatTree(id string) *util.BlockStatResult {
	v, _ := docquery.Statistics(context.Background(), docquery.StatRequest{Mode: "tree", IDs: []string{id}}, legacyStatServices())
	return v
}
func statTree(t *parse.Tree) *util.BlockStatResult {
	v, _ := docquery.TreeStatistics(context.Background(), t, legacyStatServices())
	return v
}
func BlocksWordCount(ids []string) *util.BlockStatResult {
	v, _ := docquery.Statistics(context.Background(), docquery.StatRequest{Mode: "blocks", IDs: ids}, legacyStatServices())
	return v
}
