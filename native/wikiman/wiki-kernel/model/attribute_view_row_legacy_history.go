// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"path/filepath"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func legacyAttributeViewRowHistory(attrView *av.AttributeView) (err error) {
	avID := attrView.ID
	trees := map[string]*parse.Tree{}
	historyDir, err := GetHistoryDir(HistoryOpUpdate)
	if err != nil {
		logging.LogErrorf("get history dir failed: %s", err)
		return
	}
	blockIDs := treenode.GetMirrorAttrViewBlockIDs(avID)
	for _, blockID := range blockIDs {
		tree := trees[blockID]
		if nil == tree {
			tree, _ = LoadTreeByBlockID(blockID)
		}
		if nil == tree {
			continue
		}

		historyPath := filepath.Join(historyDir, tree.Box, tree.Path)
		absPath := filepath.Join(util.DataDir, tree.Box, tree.Path)
		if err = filelock.Copy(absPath, historyPath); err != nil {
			logging.LogErrorf("backup [path=%s] to history [%s] failed: %s", absPath, historyPath, err)
			return
		}
	}

	srcAvPath := filepath.Join(util.DataDir, "storage", "av", avID+".json")
	destAvPath := filepath.Join(historyDir, "storage", "av", avID+".json")
	if copyErr := filelock.Copy(srcAvPath, destAvPath); nil != copyErr {
		logging.LogErrorf("copy av [%s] failed: %s", srcAvPath, copyErr)
	}

	indexHistoryDir(filepath.Base(historyDir), util.NewLute())
	return
}
