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
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/graphview"
)

type GraphNode = graphview.GraphNode
type GraphLink = graphview.GraphLink
type GraphArrows = graphview.GraphArrows
type GraphArrowsTo = graphview.GraphArrowsTo

func BuildTreeGraph(id, query string) (string, []*GraphNode, []*GraphLink) {
	out, err := BuildTreeGraphContext(context.Background(), id, query)
	if err != nil {
		logging.LogErrorf("build tree graph: %s", err)
	}
	return out.Box, out.Nodes, out.Links
}
func BuildGraph(query string) (string, []*GraphNode, []*GraphLink) {
	out, err := BuildGraphContext(context.Background(), query)
	if err != nil {
		logging.LogErrorf("build graph: %s", err)
	}
	return out.Box, out.Nodes, out.Links
}
