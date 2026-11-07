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
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func UpsertIndexes(paths []string) {
	if e := UpsertIndexesContext(context.Background(), paths); e != nil {
		logging.LogErrorf("upsert indexes failed: %s", e)
	}
}
func RemoveIndexes(paths []string) {
	if e := RemoveIndexesContext(context.Background(), paths); e != nil {
		logging.LogErrorf("remove indexes failed: %s", e)
	}
}

func listSyFiles(dir string) (ret []string) {
	dirPath := filepath.Join(util.DataDir, dir)
	err := filelock.Walk(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logging.LogWarnf("walk dir [%s] failed: %s", dirPath, err)
			return err
		}

		if d.IsDir() {
			return nil
		}

		if util.HasDocExt(path) {
			p := filepath.ToSlash(strings.TrimPrefix(path, util.DataDir))
			ret = append(ret, p)
		}
		return nil
	})
	if err != nil {
		logging.LogWarnf("walk dir [%s] failed: %s", dirPath, err)
	}
	return
}

func (box *Box) Unindex() {
	if e := NotebookIndexContext(context.Background(), "unindex", box.ID); e != nil {
		logging.LogErrorf("unindex notebook failed: %s", e)
	}
}
func (box *Box) Index() {
	if e := NotebookIndexContext(context.Background(), "index", box.ID); e != nil {
		logging.LogErrorf("index notebook failed: %s", e)
	}
}
func indexBox(id string) {
	r, e := captureIndexRepair(context.Background())
	if e == nil {
		e = r.indexNotebook(context.Background(), id)
	}
	if e != nil {
		logging.LogErrorf("index notebook failed: %s", e)
	}
}
func IndexRefs() {
	if e := NotebookIndexContext(context.Background(), "refs", ""); e != nil {
		logging.LogErrorf("index references failed: %s", e)
	}
}

var indexEmbedBlockLock = sync.Mutex{}

// IndexEmbedBlockJob 嵌入块支持搜索 https://github.com/siyuan-note/siyuan/issues/7112
func IndexEmbedBlockJob() {
	if err := IndexEmbedBlockJobContext(context.Background()); err != nil {
		logging.LogErrorf("index embed block content not admitted: %s", err)
	}
}

var (
	pushSQLInsertBlocksFTSMsg atomic.Bool
	pushSQLDeleteBlocksMsg    atomic.Bool
)

func subscribeSQLEvents() {
	// 使用下面的 EvtSQLInsertBlocksFTS 就可以了
	//eventbus.Subscribe(eventbus.EvtSQLInsertBlocks, func(context map[string]interface{}, current, total, blockCount int, hash string) {
	//
	//	msg := fmt.Sprintf(Conf.Language(89), current, total, blockCount, hash)
	//	util.SetBootDetails(msg)
	//	util.ContextPushMsg(context, msg)
	//})
	subscribeLegacyEvent(eventbus.EvtSQLInsertBlocksFTS, func(context map[string]interface{}, blockCount int, hash string) {
		legacySQLProgress(context, conf.SQLProgress{Kind: "blocks", BlockCount: blockCount, Detail: hash})
	})
	subscribeLegacyEvent(eventbus.EvtSQLDeleteBlocks, func(context map[string]interface{}, rootID string) {
		legacySQLProgress(context, conf.SQLProgress{Kind: "delete", Detail: rootID})
	})
	subscribeLegacyEvent(eventbus.EvtSQLUpdateBlocksHPaths, func(context map[string]interface{}, blockCount int, hash string) {
		legacySQLProgress(context, conf.SQLProgress{Kind: "rename", BlockCount: blockCount, Detail: hash})
	})
	subscribeLegacyEvent(eventbus.EvtSQLInsertHistory, func(context map[string]interface{}) { legacySQLProgress(context, conf.SQLProgress{Kind: "history"}) })
	subscribeLegacyEvent(eventbus.EvtSQLInsertAssetContent, func(context map[string]interface{}) { legacySQLProgress(context, conf.SQLProgress{Kind: "asset"}) })

	subscribeLegacyEvent(eventbus.EvtSQLIndexChanged, func(payload map[string]interface{}) {
		withLegacySQLEvent(payload, func(_ context.Context, session *legacySQLEventSession) {
			session.owner.m.Lock()
			session.owner.AppData.SetIndexPending(true)
			session.owner.m.Unlock()
			session.owner.Save()
		})
	})

	subscribeLegacyEvent(eventbus.EvtSQLIndexFlushed, func(payload map[string]interface{}) {
		withLegacySQLEvent(payload, func(_ context.Context, session *legacySQLEventSession) {
			session.owner.m.Lock()
			session.owner.AppData.SetIndexPending(false)
			session.owner.m.Unlock()
			session.owner.Save()
		})
	})
}

func legacySQLProgress(payload map[string]interface{}, p conf.SQLProgress) {
	current, ok := payload["current"].(int)
	if !ok {
		return
	}
	total, ok := payload["total"].(int)
	if !ok {
		return
	}
	p.Current, p.Total = current, total
	withLegacySQLEvent(payload, func(ctx context.Context, session *legacySQLEventSession) {
		message, enabled := conf.SQLProgressMessage(p, session.policy, session.language)
		if enabled {
			session.owner.publishSyncUI(ctx, func() {
				util.SetBootDetails(message)
				util.ContextPushMsg(payload, message)
			})
		}
	})
}

func unindex(id string) {
	r, e := captureIndexRepair(context.Background())
	if e == nil {
		e = r.unindexNotebook(context.Background(), id)
	}
	if e != nil {
		logging.LogErrorf("unindex notebook failed: %s", e)
	}
}
