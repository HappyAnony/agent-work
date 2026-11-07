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
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docview"
)

type Backlink = blockview.Backlink

func RefreshBacklink(id string) {
	if e := RefreshBacklinkContext(context.Background(), id); e != nil {
		logging.LogErrorf("refresh backlink: %s", e)
	}
}
func getBacklinkRenderNodes(n *ast.Node, refs map[string]string) ([]*ast.Node, bool) {
	return docview.BacklinkNodes(n, refs)
}
func GetBackmentionDoc(id, tree, keyword string, children, highlight bool) ([]*Backlink, []string) {
	a, b, e := GetBackmentionDocContext(context.Background(), id, tree, keyword, children, highlight)
	if e != nil {
		logging.LogErrorf("backmention doc: %s", e)
	}
	return a, b
}
func GetBacklinkDoc(id, tree, keyword string, children, highlight bool) ([]*Backlink, []string) {
	a, b, e := GetBacklinkDocContext(context.Background(), id, tree, keyword, children, highlight)
	if e != nil {
		logging.LogErrorf("backlink doc: %s", e)
	}
	return a, b
}
func GetBacklink(id, keyword, mentionKeyword string, beforeLen int, children bool) (string, []*Path, []*Path, int, int) {
	a, b, c, d, f, e := GetBacklinkContext(context.Background(), id, keyword, mentionKeyword, beforeLen, children)
	if e != nil {
		logging.LogErrorf("backlink: %s", e)
	}
	return a, b, c, d, f
}
func GetBacklink2(id, keyword, mentionKeyword string, sortMode, mentionSortMode int, children bool) (string, []*Path, []*Path, int, int) {
	a, b, c, d, f, e := GetBacklink2Context(context.Background(), id, keyword, mentionKeyword, sortMode, mentionSortMode, children)
	if e != nil {
		logging.LogErrorf("backlink2: %s", e)
	}
	return a, b, c, d, f
}
