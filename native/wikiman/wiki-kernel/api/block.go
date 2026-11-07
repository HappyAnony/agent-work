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

package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/filesys"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func checkBlockRef(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}
	ids = gulu.Str.RemoveDuplicatedElem(ids)

	value, err := model.CheckBlockRefContext(c.Request.Context(), ids)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = value
}

func getBlockTreeInfos(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var ids []string
	idsArg := arg["ids"].([]interface{})
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}

	value, err := model.GetBlockTreeInfosContext(c.Request.Context(), ids)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = value
}

func getBlockSiblingID(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	parent, previous, next, err := model.GetBlockSiblingIDContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]string{
		"parent":   parent,
		"next":     next,
		"previous": previous,
	}
}

func getBlockRelevantIDs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	parentID, previousID, nextID, err := model.GetBlockRelevantIDsContext(c.Request.Context(), id)
	if nil != err {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}

	ret.Data = map[string]string{
		"parentID":   parentID,
		"previousID": previousID,
		"nextID":     nextID,
	}
}

func transferBlockRef(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	fromID := arg["fromID"].(string)
	if util.InvalidIDPattern(fromID, ret) {
		return
	}
	toID := arg["toID"].(string)
	if util.InvalidIDPattern(toID, ret) {
		return
	}

	reloadUI := true
	if nil != arg["reloadUI"] {
		reloadUI = arg["reloadUI"].(bool)
	}

	var refIDs []string
	if nil != arg["refIDs"] {
		for _, refID := range arg["refIDs"].([]interface{}) {
			refIDs = append(refIDs, refID.(string))
		}
	}

	err := model.TransferBlockRefContext(c.Request.Context(), fromID, toID, refIDs)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}

	if reloadUI {
		util.ReloadUI()
	}
}

func swapBlockRef(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	refID := arg["refID"].(string)
	defID := arg["defID"].(string)
	includeChildren := arg["includeChildren"].(bool)
	err := model.SwapBlockRefContext(c.Request.Context(), refID, defID, includeChildren)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
}

func getHeadingChildrenIDs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	ids, err := model.GetHeadingChildrenIDsContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = ids
}

func appendHeadingChildren(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	childrenDOM := arg["childrenDOM"].(string)
	if err := model.AppendHeadingChildrenContext(c.Request.Context(), id, childrenDOM); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func getHeadingChildrenDOM(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	removeFoldAttr := true
	if nil != arg["removeFoldAttr"] {
		removeFoldAttr = arg["removeFoldAttr"].(bool)
	}
	dom, err := model.GetHeadingChildrenDOMContext(c.Request.Context(), id, removeFoldAttr)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = dom
}

func getHeadingDeleteTransaction(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)

	transaction, err := model.GetHeadingDeleteTransactionContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}

	ret.Data = transaction
}

func getHeadingInsertTransaction(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)

	transaction, err := model.GetHeadingInsertTransactionContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}

	ret.Data = transaction
}

func getHeadingLevelTransaction(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	level := int(arg["level"].(float64))

	transaction, err := model.GetHeadingLevelTransactionContext(c.Request.Context(), id, level)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}

	ret.Data = transaction
}

func setBlockReminder(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id, idOK := arg["id"].(string)
	timed, timeOK := arg["timed"].(string)
	if !idOK || !timeOK {
		ret.Code = -1
		ret.Msg = "invalid reminder arguments"
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
	err := model.SetBlockReminderContext(c.Request.Context(), id, timed)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
}

func getUnfoldedParentID(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	parentID, err := model.GetUnfoldedParentIDContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"parentID": parentID,
	}
}

func checkBlockFold(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	isFolded, isRoot, err := model.IsBlockFoldedContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"isFolded": isFolded,
		"isRoot":   isRoot,
	}
}

func checkBlockExist(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	b, err := model.GetBlockContext(c.Request.Context(), id)
	if errors.Is(err, model.ErrIndexing) {
		ret.Code = 0
		ret.Data = false
		return
	}
	if err != nil && !errors.Is(err, model.ErrBlockNotFound) {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	ret.Data = nil != b
}

func getDocInfo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	info, err := model.GetDocInfoContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = info
}

func getDocsInfo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}
	queryRefCount := arg["refCount"].(bool)
	queryAv := arg["av"].(bool)
	info, err := model.GetDocsInfoContext(c.Request.Context(), ids, queryRefCount, queryAv)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = info
}

func getRecentUpdatedBlocks(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	blocks, err := model.RecentUpdatedBlocksContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = blocks
}

func getContentWordCount(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	content := arg["content"].(string)
	ret.Data = map[string]any{
		"reqId": arg["reqId"],
		"stat":  filesys.ContentStat(content),
	}
}

func getBlocksWordCount(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}
	ret.Data = map[string]any{
		"reqId": arg["reqId"],
		"stat":  filesys.BlocksWordCount(ids),
	}
}

func getTreeStat(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	ret.Data = map[string]any{
		"reqId": arg["reqId"],
		"stat":  filesys.StatTree(id),
	}
}

func getDOMText(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	dom := arg["dom"].(string)
	text, err := model.GetDOMTextContext(c.Request.Context(), dom)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = text
}

func getRefText(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	refText, err := model.GetBlockRefTextContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	if "" == refText {
		// 空块返回 id https://github.com/siyuan-note/siyuan/issues/10259
		refText = id
		ret.Data = refText
		return
	}

	if strings.Count(refText, "\\") == len(refText) {
		// 全部都是 \ 的话使用实体 https://github.com/siyuan-note/siyuan/issues/11473
		refText = strings.ReplaceAll(refText, "\\", "&#92;")
		ret.Data = refText
		return
	}

	ret.Data = refText
}

func getRefIDs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	if nil == arg["id"] {
		arg["id"] = ""
	}

	id := arg["id"].(string)
	refDefs, originalRefBlockIDs, err := model.GetBlockRefsContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]any{
		"refDefs":             refDefs,
		"originalRefBlockIDs": originalRefBlockIDs,
	}
}

func getRefIDsByFileAnnotationID(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	refIDs, err := model.GetBlockRefIDsByFileAnnotationIDContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	var retRefDefs []model.RefDefs
	for _, blockID := range refIDs {
		retRefDefs = append(retRefDefs, model.RefDefs{RefID: blockID, DefIDs: []string{}})
	}
	if 1 > len(retRefDefs) {
		retRefDefs = []model.RefDefs{}
	}

	ret.Data = map[string]any{
		"refDefs": retRefDefs,
	}
}

func getBlockDefIDsByRefText(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	anchor := arg["anchor"].(string)
	excludeIDsArg := arg["excludeIDs"].([]interface{})
	var excludeIDs []string
	for _, excludeID := range excludeIDsArg {
		excludeIDs = append(excludeIDs, excludeID.(string))
	}
	excludeIDs = nil // 不限制虚拟引用搜索自己 https://ld246.com/article/1633243424177
	ids, err := model.GetBlockDefIDsByRefTextContext(c.Request.Context(), anchor, excludeIDs)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	var retRefDefs []model.RefDefs
	for _, id := range ids {
		retRefDefs = append(retRefDefs, model.RefDefs{RefID: id, DefIDs: []string{}})
	}
	if 1 > len(retRefDefs) {
		retRefDefs = []model.RefDefs{}
	}

	ret.Data = map[string]any{
		"refDefs": retRefDefs,
	}
}

func getBlockBreadcrumb(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	excludeTypesArg := arg["excludeTypes"]
	var excludeTypes []string
	if nil != excludeTypesArg {
		for _, excludeType := range excludeTypesArg.([]interface{}) {
			excludeTypes = append(excludeTypes, excludeType.(string))
		}
	}

	blockPath, err := model.BuildBlockBreadcrumbContext(c.Request.Context(), id, excludeTypes)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = blockPath
}

func getBlockIndex(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	index, err := model.GetBlockIndexContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = index
}

func getBlocksIndexes(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}
	index, err := model.GetBlocksIndexesContext(c.Request.Context(), ids)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = index
}

func getBlockInfo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)

	out, err := model.GetBlockInfoContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		if errors.Is(err, model.ErrIndexing) {
			ret.Code = 3
		}
		ret.Msg = err.Error()
		return
	}
	ret.Data = out
}

func getBlockDOM(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	dom, err := model.GetBlockDOMContext(c.Request.Context(), id, false)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]string{
		"id":  id,
		"dom": dom,
	}
}

func getBlockDOMs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}

	doms, err := model.GetBlockDOMsContext(c.Request.Context(), ids, false)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = doms
}

func getBlockDOMWithEmbed(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	dom, err := model.GetBlockDOMContext(c.Request.Context(), id, true)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]string{
		"id":  id,
		"dom": dom,
	}
}

func getBlockDOMsWithEmbed(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}

	doms, err := model.GetBlockDOMsContext(c.Request.Context(), ids, true)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = doms
}

func getBlockKramdown(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	// md：Markdown 标记符模式，使用标记符导出
	// textmark：文本标记模式，使用 span 标签导出
	// https://github.com/siyuan-note/siyuan/issues/13183
	mode := "md"
	if modeArg := arg["mode"]; nil != modeArg {
		mode = modeArg.(string)
		if "md" != mode && "textmark" != mode {
			ret.Code = -1
			ret.Msg = "Invalid mode"
			return
		}
	}

	kramdown, err := model.GetBlockKramdownContext(c.Request.Context(), id, mode)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]string{
		"id":       id,
		"kramdown": kramdown,
	}
}

func getChildBlocks(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	value, err := model.GetChildBlocksContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = value
}

func getTailChildBlocks(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	var n int
	nArg := arg["n"]
	if nil != nArg {
		n = int(nArg.(float64))
	}
	if 1 > n {
		n = 7
	}

	value, err := model.GetTailChildBlocksContext(c.Request.Context(), id, n)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = value
}
