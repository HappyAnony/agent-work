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
	"os"
	"path/filepath"
	"time"

	"github.com/88250/go-humanize"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/filesys"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func PushReloadPlugin(upsertCodePluginSet, upsertDataPluginSet, removePluginNameSet *hashset.Set, excludeApp string) {
	logOwnedTaskError("reload plugins", PushReloadPluginContext(context.Background(), upsertCodePluginSet, upsertDataPluginSet, removePluginNameSet, excludeApp))
}

func PushReloadPluginContext(ctx context.Context, code, data, remove *hashset.Set, exclude string) error {
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return err
	}
	ui, err := captureLegacyDelayedUIOwner(ctx)
	if err != nil {
		return err
	}
	codeNames, dataNames, removeNames := pluginReloadLists(code, data, remove)
	return o.run(ctx, func(op, _ context.Context) error {
		if !ui.publish(op, func() { pushReloadPlugin0(codeNames, dataNames, removeNames, exclude) }) {
			return conf.ErrBusinessUnbound
		}
		return nil
	})
}

func pluginReloadLists(upsertCodePluginSet, upsertDataPluginSet, removePluginNameSet *hashset.Set) (upsertCodePlugins, upsertDataPlugins, removePlugins []string) {
	if nil != removePluginNameSet {
		for _, n := range removePluginNameSet.Values() {
			pluginName := n.(string)
			// 如果插件在 removePluginSet 中，从其他集合中移除
			if nil != upsertCodePluginSet {
				upsertCodePluginSet.Remove(pluginName)
			}
			if nil != upsertDataPluginSet {
				upsertDataPluginSet.Remove(pluginName)
			}
		}
	}
	if nil != upsertCodePluginSet {
		for _, n := range upsertCodePluginSet.Values() {
			pluginName := n.(string)
			// 如果插件在 upsertCodePluginSet 中，从 upsertDataPluginSet 中移除
			if nil != upsertDataPluginSet {
				upsertDataPluginSet.Remove(pluginName)
			}
		}
	}

	upsertCodePlugins, upsertDataPlugins, removePlugins = []string{}, []string{}, []string{}
	if nil != upsertCodePluginSet {
		for _, n := range upsertCodePluginSet.Values() {
			upsertCodePlugins = append(upsertCodePlugins, n.(string))
		}
	}
	if nil != upsertDataPluginSet {
		for _, n := range upsertDataPluginSet.Values() {
			upsertDataPlugins = append(upsertDataPlugins, n.(string))
		}
	}
	if nil != removePluginNameSet {
		for _, n := range removePluginNameSet.Values() {
			removePlugins = append(removePlugins, n.(string))
		}
	}

	return
}

func pushReloadPlugin0(upsertCodePlugins, upsertDataPlugins, removePlugins []string, excludeApp string) {
	logging.LogInfof("reload plugins [codeChanges=%v, dataChanges=%v, removes=%v]", upsertCodePlugins, upsertDataPlugins, removePlugins)
	if "" == excludeApp {
		util.BroadcastByType("main", "reloadPlugin", 0, "", map[string]interface{}{
			"upsertCodePlugins": upsertCodePlugins,
			"upsertDataPlugins": upsertDataPlugins,
			"removePlugins":     removePlugins,
		})
		return
	}

	util.BroadcastByTypeAndExcludeApp(excludeApp, "main", "reloadPlugin", 0, "", map[string]interface{}{
		"upsertCodePlugins": upsertCodePlugins,
		"upsertDataPlugins": upsertDataPlugins,
		"removePlugins":     removePlugins,
	})
}

func refreshDocInfo(tree *parse.Tree) {
	if nil == tree {
		return
	}

	refreshDocInfoWithSize(tree, filesys.TreeSize(tree))
}

func refreshDocInfoWithSize(tree *parse.Tree, size uint64) {
	if nil == tree {
		return
	}

	refreshDocInfo0(tree, size)
	refreshParentDocInfo(tree)
}

func refreshParentDocInfo(tree *parse.Tree) {
	parentTree := loadParentTree(tree)
	if nil == parentTree {
		return
	}

	luteEngine := lute.New()
	renderer := render.NewJSONRenderer(parentTree, luteEngine.RenderOptions)
	data := renderer.Render()
	refreshDocInfo0(parentTree, uint64(len(data)))
}

func refreshDocInfo0(tree *parse.Tree, size uint64) {
	owner, err := captureLegacyDelayedUIOwner(context.Background())
	if err != nil {
		return
	}
	subFileCount := 0
	subFiles, err := os.ReadDir(filepath.Join(owner.data, tree.Box, util.StripDocExt(tree.Path)))
	if err == nil {
		for _, subFile := range subFiles {
			if "true" == tree.Root.IALAttr("custom-hidden") {
				continue
			}

			if util.HasDocExt(subFile.Name()) {
				subFileCount++
			}
		}
	}

	docInfo := projectReloadDocInfo(tree, size, subFileCount, func(t time.Time) string { return util.HumanizeTime(t, owner.lang) })

	_ = owner.schedule(context.Background(), task.ReloadProtyle, 500*time.Millisecond, func() { util.PushReloadDocInfo(docInfo) }, docInfo)
}

// Shared original UI projection; callers supply owner-bound reads and language.
func projectReloadDocInfo(tree *parse.Tree, size uint64, subFileCount int, relative func(time.Time) string) map[string]interface{} {
	stamp := tree.ID
	if len(stamp) > 14 {
		stamp = stamp[:14]
	}
	cTime, _ := time.ParseInLocation("20060102150405", stamp, time.Local)
	mTime := cTime
	if updated := tree.Root.IALAttr("updated"); "" != updated {
		if updatedTime, err := time.ParseInLocation("20060102150405", updated, time.Local); err == nil {
			mTime = updatedTime
		}
	}

	docInfo := map[string]interface{}{
		"rootID":       tree.ID,
		"name":         tree.Root.IALAttr("title"),
		"alias":        tree.Root.IALAttr("alias"),
		"name1":        tree.Root.IALAttr("name"),
		"memo":         tree.Root.IALAttr("memo"),
		"bookmark":     tree.Root.IALAttr("bookmark"),
		"size":         size,
		"hSize":        humanize.BytesCustomCeil(size, 2),
		"mtime":        mTime.Unix(),
		"ctime":        cTime.Unix(),
		"hMtime":       mTime.Format("2006-01-02 15:04:05") + ", " + relative(mTime),
		"hCtime":       cTime.Format("2006-01-02 15:04:05") + ", " + relative(cTime),
		"subFileCount": subFileCount,
	}

	return docInfo
}

func ReloadFiletree() {
	logOwnedTaskError("reload filetree", ReloadFiletreeContext(context.Background()))
}

func ReloadTag() {
	logOwnedTaskError("reload tags", ReloadTagContext(context.Background()))
}

func ReloadProtyle(rootID string) {
	if err := ReloadProtyleContext(context.Background(), rootID); err != nil {
		logging.LogErrorf("reload protyle failed: %s", err)
	}
}

// refreshRefCount retains the old delayed callback entry, with explicit services.
func refreshRefCount(blockID string) {
	if err := refreshRefCountWithEnvironment(blockID, legacySourceEventEnvironment()); err != nil {
		logging.LogErrorf("refresh refcount failed: %s", err)
	}
}

func updateAttributeViewBlockText(updatedDefNodes map[string]*ast.Node) {
	if err := updateAttributeViewBlockTextWithEnvironment(updatedDefNodes, legacyAttributeViewTextEnvironment()); err != nil {
		logging.LogErrorf("update AV block text failed: %s", err)
	}
}

// ReloadAttrView 用于重新加载属性视图。
func ReloadAttrView(avID string) {
	logOwnedTaskError("reload attribute view", ReloadAttrViewContext(context.Background(), avID))
}

func pushReloadAttrView(avID string) {
	util.BroadcastByType("protyle", "refreshAttributeView", 0, "", map[string]interface{}{"id": avID})
}
