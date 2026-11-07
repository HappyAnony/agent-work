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

	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
)

type Block = blockview.Block
type RiffCard = blockview.RiffCard

type Path = blockview.Path

func CheckBlockRef(ids []string) bool {
	v, e := CheckBlockRefContext(context.Background(), ids)
	if e != nil {
		logging.LogErrorf("check block refs: %s", e)
	}
	return v
}

type BlockTreeInfo = docquery.BlockTreeInfo

func GetBlockTreeInfos(ids []string) map[string]*BlockTreeInfo {
	out, e := GetBlockTreeInfosContext(context.Background(), ids)
	if e != nil {
		logging.LogErrorf("block tree infos: %s", e)
	}
	return out
}

func GetBlockSiblingID(id string) (string, string, string) {
	a, b, c, e := GetBlockSiblingIDContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("block siblings: %s", e)
	}
	return a, b, c
}

func GetBlockRelevantIDs(id string) (string, string, string, error) {
	return GetBlockRelevantIDsContext(context.Background(), id)
}

func GetUnfoldedParentID(id string) string {
	out, e := GetUnfoldedParentIDContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("unfolded parent: %s", e)
	}
	return out
}

func IsBlockFolded(id string) (bool, bool) {
	a, b, e := IsBlockFoldedContext(context.Background(), id)
	if e != nil {
		logging.LogErrorf("block fold: %s", e)
	}
	return a, b
}

func RecentUpdatedBlocks() []*Block {
	out, e := RecentUpdatedBlocksContext(context.Background())
	if e != nil {
		logging.LogErrorf("recent blocks: %s", e)
	}
	return out
}

func TransferBlockRef(fromID, toID string, refIDs []string) error {
	return TransferBlockRefContext(context.Background(), fromID, toID, refIDs)
}
func SwapBlockRef(refID, defID string, includeChildren bool) error {
	return SwapBlockRefContext(context.Background(), refID, defID, includeChildren)
}

func GetHeadingDeleteTransaction(id string) (*Transaction, error) {
	return GetHeadingDeleteTransactionContext(context.Background(), id)
}

func GetHeadingInsertTransaction(id string) (*Transaction, error) {
	return GetHeadingInsertTransactionContext(context.Background(), id)
}

func GetHeadingChildrenIDs(id string) (ret []string) {
	ret, err := GetHeadingChildrenIDsContext(context.Background(), id)
	if err != nil {
		logging.LogErrorf("heading IDs: %s", err)
	}
	return ret
}

func AppendHeadingChildren(id, dom string) {
	if e := AppendHeadingChildrenContext(context.Background(), id, dom); e != nil {
		logging.LogErrorf("append heading children: %s", e)
	}
}

func GetHeadingChildrenDOM(id string, removeFoldAttr bool) (ret string) {
	ret, err := GetHeadingChildrenDOMContext(context.Background(), id, removeFoldAttr)
	if err != nil {
		logging.LogErrorf("heading DOM: %s", err)
	}
	return ret
}

func GetHeadingLevelTransaction(id string, level int) (*Transaction, error) {
	return GetHeadingLevelTransactionContext(context.Background(), id, level)
}

func GetBlockDOM(id string) (ret string) {
	ret, e := GetBlockDOMContext(context.Background(), id, false)
	if e != nil {
		logging.LogErrorf("block DOM: %s", e)
	}
	return ret
}

func GetBlockDOMs(ids []string) (ret map[string]string) {
	ret, e := GetBlockDOMsContext(context.Background(), ids, false)
	if e != nil {
		logging.LogErrorf("block DOMs: %s", e)
	}
	return ret
}

func GetBlockDOMWithEmbed(id string) (ret string) {
	ret, e := GetBlockDOMContext(context.Background(), id, true)
	if e != nil {
		logging.LogErrorf("embedded DOM: %s", e)
	}
	return ret
}

func GetBlockDOMsWithEmbed(ids []string) (ret map[string]string) {
	ret, e := GetBlockDOMsContext(context.Background(), ids, true)
	if e != nil {
		logging.LogErrorf("embedded DOMs: %s", e)
	}
	return ret
}

func GetBlockKramdown(id, mode string) (ret string) {
	ret, err := GetBlockKramdownContext(context.Background(), id, mode)
	if err != nil {
		logging.LogErrorf("block markdown: %s", err)
	}
	return ret
}

type ChildBlock = docquery.ChildBlock

func GetChildBlocks(id string) (ret []*ChildBlock) {
	ret, err := GetChildBlocksContext(context.Background(), id)
	if err != nil {
		logging.LogErrorf("child blocks: %s", err)
	}
	return ret
}

func GetTailChildBlocks(id string, n int) (ret []*ChildBlock) {
	ret, err := GetTailChildBlocksContext(context.Background(), id, n)
	if err != nil {
		logging.LogErrorf("tail blocks: %s", err)
	}
	return ret
}

func GetBlock(id string, tree *parse.Tree) (ret *Block, err error) {
	ret, err = getBlock(id, tree)
	return
}

func getBlock(id string, tree *parse.Tree) (*Block, error) {
	return GetBlockContext(context.Background(), id, tree)
}
