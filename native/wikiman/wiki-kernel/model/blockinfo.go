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
	"unicode/utf8"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
)

type BlockInfo = docquery.Info
type AttrView = docquery.InfoAV

func GetDocInfo(id string) *BlockInfo {
	out, e := GetDocInfoContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("doc info: %s", e)
	}
	return out
}

func GetDocsInfo(ids []string, refs, avs bool) []*BlockInfo {
	out, e := GetDocsInfoContext(context.Background(), ids, refs, avs)
	if e != nil {
		logging.LogErrorf("docs info: %s", e)
	}
	return out
}

func GetBlockRefText(id string) string {
	out, e := GetBlockRefTextContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("block ref text: %s", e)
	}
	return out
}

func GetDOMText(dom string) string {
	out, e := GetDOMTextContext(context.Background(), dom)
	if e != nil {
		logging.LogErrorf("DOM text: %s", e)
	}
	return out
}

func getBlockRefText(id string, tree *parse.Tree) (ret string) {
	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		return
	}

	ret = getNodeRefText(node)
	ret = maxContent(ret, Conf.Editor.BlockRefDynamicAnchorTextMaxLen)
	return
}

func getNodeRefText(n *ast.Node) string {
	if n == nil {
		return ""
	}
	maxLen := 0
	if n.IALAttr("name") == "" {
		if Conf == nil || Conf.Editor == nil {
			logging.LogErrorf("anchor editor configuration unbound")
			return ""
		}
		maxLen = Conf.Editor.BlockRefDynamicAnchorTextMaxLen
	}
	ret, err := anchor.Text(context.Background(), n, maxLen, indexdb.ContentServices{AttributeView: func(ctx context.Context, id string, full bool) (string, error) {
		name, _ := av.GetAttributeViewName(id)
		return name, nil
	}})
	if err != nil {
		logging.LogErrorf("anchor text failed: %s", err)
	}
	return ret
}

func getNodeAvBlockText(node *ast.Node, id string) (icon, content string) {
	var err error
	icon, content, err = newLegacyAVBinding(context.Background()).text(node, id)
	if err != nil {
		logging.LogErrorf("AV block text: %s", err)
	}
	return
}

func getNodeRefText0(node *ast.Node, maxLen int, removeLineBreak bool) string {
	switch node.Type {
	case ast.NodeBlockQueryEmbed:
		return "Query Embed Block..."
	case ast.NodeIFrame:
		return "IFrame..."
	case ast.NodeThematicBreak:
		return "Thematic Break..."
	case ast.NodeVideo:
		return "Video..."
	case ast.NodeAudio:
		return "Audio..."
	case ast.NodeAttributeView:
		ret, _ := av.GetAttributeViewName(node.AttributeViewID)
		if "" == ret {
			ret = "Database..."
		}
		return ret
	}

	if ast.NodeDocument != node.Type && node.IsContainerBlock() {
		node = treenode.FirstLeafBlock(node)
	}
	ret := renderBlockText(node, nil, removeLineBreak)
	if maxLen < utf8.RuneCountInString(ret) {
		ret = gulu.Str.SubStr(ret, maxLen) + "..."
	}
	return ret
}

type RefDefs = docquery.RefDefs

func GetBlockRefs(id string) ([]*RefDefs, map[string]string) {
	a, b, e := GetBlockRefsContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("block refs: %s", e)
	}
	return a, b
}

func queryBlockRefDefs(bt *treenode.BlockTree) (refDefs []*RefDefs) {
	refDefs = []*RefDefs{}
	if nil == bt {
		return
	}

	isDoc := bt.ID == bt.RootID
	if isDoc {
		refDefIDs := sql.QueryChildRefDefIDsByRootDefID(bt.RootID)
		for rID, dIDs := range refDefIDs {
			var defIDs []string
			for _, dID := range dIDs {
				defIDs = append(defIDs, dID)
			}
			if 1 > len(defIDs) {
				defIDs = []string{}
			}
			refDefs = append(refDefs, &RefDefs{RefID: rID, DefIDs: defIDs})
		}
	} else {
		refIDs := sql.QueryRefIDsByDefID(bt.ID, false)
		for _, refID := range refIDs {
			refDefs = append(refDefs, &RefDefs{RefID: refID, DefIDs: []string{bt.ID}})
		}
	}
	return
}

func GetBlockRefIDsByFileAnnotationID(id string) []string {
	v, e := GetBlockRefIDsByFileAnnotationIDContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("annotation refs: %s", e)
	}
	return v
}

func GetBlockDefIDsByRefText(text string, exclude []string) []string {
	v, e := GetBlockDefIDsByRefTextContext(context.Background(), text, exclude)
	if e != nil {
		logging.LogErrorf("definition IDs: %s", e)
	}
	return v
}

type BlockPath = docquery.BlockPath

func GetBlockIndex(id string) int {
	out, e := GetBlockIndexContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("block index: %s", e)
	}
	return out
}

func GetBlocksIndexes(ids []string) map[string]int {
	out, e := GetBlocksIndexesContext(context.Background(), ids)
	if e != nil {
		logging.LogErrorf("block indexes: %s", e)
	}
	return out
}

func BuildBlockBreadcrumb(id string, exclude []string) ([]*BlockPath, error) {
	return BuildBlockBreadcrumbContext(context.Background(), id, exclude)
}

func buildBlockBreadcrumb(n *ast.Node, exclude []string, embed bool, headingMode ...int) []*BlockPath {
	return legacyNodeBreadcrumb(n, exclude, embed, headingMode...)
}

func buildBacklinkListItemRefs(defs []*RefDefs) map[string]string {
	ret, err := legacyPromoteRefDefs(defs)
	if err != nil {
		logging.LogErrorf("backlink list promotion failed: %s", err)
		return map[string]string{}
	}
	return ret
}
