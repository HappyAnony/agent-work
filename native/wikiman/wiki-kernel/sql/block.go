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

package sql

import (
	"context"
	"database/sql"
	"strings"
	"wiki-kernel/internal/indexdb"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/filesys"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type Block struct {
	ID       string
	ParentID string
	RootID   string
	Hash     string
	Box      string
	Path     string
	HPath    string
	Name     string
	Alias    string
	Memo     string
	Tag      string
	Content  string
	FContent string
	Markdown string
	Length   int
	Type     string
	SubType  string
	IAL      string
	Sort     int
	Created  string
	Updated  string
}

func updateRootContent(tx *sql.Tx, content, updated, id string) (err error) {
	if err = indexdb.UpdateRootContent(context.Background(), tx, content, updated, id, caseSensitive); err != nil {
		return err
	}
	removeBlockCache(id)
	cache.RemoveBlockIAL(id)
	return nil

}

func updateBlockContent(tx *sql.Tx, block *Block) (err error) {
	if err = indexdb.UpdateBlockContent(context.Background(), tx, block.ID, block.Content, caseSensitive); err != nil {
		return err
	}
	putBlockCache(block)
	return nil
}

func indexNode(tx *sql.Tx, id string) (err error) {
	bt := treenode.GetBlockTree(id)
	if nil == bt {
		return
	}

	tree, _ := filesys.LoadTree(bt.BoxID, bt.Path, legacyTreeProjection().parser)
	if nil == tree {
		return
	}

	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		return
	}

	content := NodeStaticContent(node, nil, true, indexAssetPath, true)
	content = strings.ReplaceAll(content, editor.Zwsp, "")
	return indexdb.UpdateBlockContent(context.Background(), tx, id, content, caseSensitive)
}

func legacyContentServices() indexdb.ContentServices {
	return indexdb.ContentServices{AssetText: func(_ context.Context, p string) (string, error) { return util.GetAssetText(p), nil }, AttributeView: func(_ context.Context, id string, full bool) (string, error) {
		if full {
			return getAttributeViewContent(id), nil
		}
		name, _ := av.GetAttributeViewName(id)
		return name, nil
	}, Warning: func(msg string) { logging.LogWarnf("%s", msg) }}
}
func NodeStaticContent(node *ast.Node, excludeTypes []string, includeTextMarkATitleURL, includeAssetPath, fullAttrView bool) string {
	ret, err := NodeStaticContentContext(context.Background(), node, excludeTypes, includeTextMarkATitleURL, includeAssetPath, fullAttrView)
	if err != nil {
		logging.LogErrorf("project node content failed: %s", err)
	}
	return ret
}
func NodeStaticContentContext(ctx context.Context, node *ast.Node, excludeTypes []string, includeTextMarkATitleURL, includeAssetPath, fullAttrView bool) (string, error) {
	return indexdb.StaticContent(ctx, node, excludeTypes, includeTextMarkATitleURL, includeAssetPath, fullAttrView, legacyContentServices())
}
func nodeStaticContent(node *ast.Node, excludeTypes []string, includeTextMarkATitleURL, includeAssetPath bool) string {
	if node != nil && node.Type == ast.NodeAttributeView {
		return ""
	}
	return NodeStaticContent(node, excludeTypes, includeTextMarkATitleURL, includeAssetPath, false)
}

func BatchGetBlockAttrsWitTrees(ids []string, trees map[string]*parse.Tree) (ret map[string]map[string]string) {
	ret = map[string]map[string]string{}

	hitCache := true
	for _, id := range ids {
		ial := cache.GetBlockIAL(id)
		if nil != ial {
			ret[id] = ial
			continue
		}
		hitCache = false
		break
	}
	if hitCache {
		return
	}

	for _, id := range ids {
		tree := trees[id]
		if nil == tree {
			continue
		}

		ret[id] = getBlockAttrsFromTree(id, tree)
	}
	return
}

func BatchGetBlockAttrs(ids []string) (ret map[string]map[string]string) {
	ret = map[string]map[string]string{}

	hitCache := true
	for _, id := range ids {
		ial := cache.GetBlockIAL(id)
		if nil != ial {
			ret[id] = ial
			continue
		}
		hitCache = false
		break
	}
	if hitCache {
		return
	}

	trees := filesys.LoadTrees(ids)
	for _, id := range ids {
		tree := trees[id]
		if nil == tree {
			continue
		}

		ret[id] = getBlockAttrsFromTree(id, tree)
	}
	return
}

func GetBlockAttrs(id string) (ret map[string]string) {
	ret = map[string]string{}
	if cached := cache.GetBlockIAL(id); nil != cached {
		ret = cached
		return
	}

	tree := loadTreeByBlockID(id)
	if nil == tree {
		return
	}

	ret = getBlockAttrsFromTree(id, tree)
	return
}

func getBlockAttrsFromTree(id string, tree *parse.Tree) (ret map[string]string) {
	ret = map[string]string{}

	ial := cache.GetBlockIAL(id)
	if nil != ial {
		for k, v := range ial {
			ret[k] = v
		}
		return
	}

	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		logging.LogWarnf("block [%s] not found", id)
		return
	}

	for _, kv := range node.KramdownIAL {
		ret[kv[0]] = html.UnescapeAttrVal(kv[1])
	}
	cache.PutBlockIAL(id, ret)
	return
}

func loadTreeByBlockID(id string) (ret *parse.Tree) {
	bt := treenode.GetBlockTree(id)
	if nil == bt {
		return
	}

	ret, err := filesys.LoadTree(bt.BoxID, bt.Path, legacyTreeProjection().parser)
	if nil != err {
		return
	}
	return
}
