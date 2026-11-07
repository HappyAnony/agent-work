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
	"errors"
	"strings"
	"time"
	"wiki-kernel/sql"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/treenode"
)

func GetAttributeViewItemIDs(avID string, blockIDs []string, contexts ...context.Context) map[string]string {
	return legacyAttributeViewIdentities(avID, blockIDs, true, contexts...)
}

func GetAttributeViewBoundBlockIDs(avID string, itemIDs []string, contexts ...context.Context) map[string]string {
	return legacyAttributeViewIdentities(avID, itemIDs, false, contexts...)
}

func GetAttrViewAddingBlockDefaultValues(avID, viewID, groupID, previousBlockID, addingBlockID string, contexts ...context.Context) (ret map[string]*av.Value) {
	values, err := attrViewAddingBlockDefaultValuesWithEnvironment(avID, viewID, groupID, previousBlockID, addingBlockID, legacyAttributeViewPreviewEnvironment(contexts...))
	if err != nil {
		logging.LogErrorf("attribute view defaults [%s] failed: %s", avID, err)
		return map[string]*av.Value{}
	}
	return values
}

func getAttrViewAddingBlockDefaultValues(a *av.AttributeView, v, g *av.View, p, row string, create bool) map[string]*av.Value {
	values, _ := attributeViewDefaultValues(a, v, g, p, row, create, legacyAttributeViewDefaults())
	return values
}

func (tx *Transaction) doSortAttrViewGroup(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("sortAttrViewGroup", operation)
}

func sortAttributeViewGroup(avID, blockID, previousGroupID, groupID string) (err error) {
	return runAttributeViewManagement(&Operation{Action: "sortAttrViewGroup", AvID: avID, BlockID: blockID, PreviousID: previousGroupID, ID: groupID}, legacyAttributeViewManagementEnvironment())
}

func (tx *Transaction) doRemoveAttrViewGroup(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("removeAttrViewGroup", operation)
}

func removeAttributeViewGroup(avID, blockID string) (err error) {
	return runAttributeViewManagement(&Operation{Action: "removeAttrViewGroup", AvID: avID, BlockID: blockID}, legacyAttributeViewManagementEnvironment())
}

func removeAttributeViewGroup0(view *av.View) { editcore.RemoveAttributeViewGroup0(view) }

func (tx *Transaction) doSyncAttrViewTableColWidth(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("syncAttrViewTableColWidth", operation)
}

func syncAttrViewTableColWidth(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("syncAttrViewTableColWidth", operation)
}

func (tx *Transaction) doHideAttrViewGroup(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("hideAttrViewGroup", operation)
}

func hideAttributeViewGroup(avID, blockID, groupID string, hidden int) (err error) {
	return runAttributeViewManagement(&Operation{Action: "hideAttrViewGroup", AvID: avID, BlockID: blockID, ID: groupID, Data: float64(hidden)}, legacyAttributeViewManagementEnvironment())
}

func (tx *Transaction) doHideAttrViewAllGroups(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("hideAttrViewAllGroups", operation)
}

func hideAttributeViewAllGroups(avID, blockID string, hidden bool) (err error) {
	return runAttributeViewManagement(&Operation{Action: "hideAttrViewAllGroups", AvID: avID, BlockID: blockID, Data: hidden}, legacyAttributeViewManagementEnvironment())
}

func (tx *Transaction) doFoldAttrViewGroup(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("foldAttrViewGroup", operation)
}

func foldAttrViewGroup(avID, blockID, groupID string, folded bool) (err error) {
	return runAttributeViewManagement(&Operation{Action: "foldAttrViewGroup", AvID: avID, BlockID: blockID, ID: groupID, Data: folded}, legacyAttributeViewManagementEnvironment())
}

func (tx *Transaction) doSetAttrViewGroup(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewGroup", operation)
}

func SetAttributeViewGroup(avID, blockID string, group *av.ViewGroup) (err error) {
	return runAttributeViewManagement(&Operation{Action: "setAttrViewGroup", AvID: avID, BlockID: blockID, Data: group}, legacyAttributeViewManagementEnvironment())
}

func setAttributeViewGroup(a *av.AttributeView, v *av.View, g *av.ViewGroup) {
	_ = setAttributeViewGroupWithEnvironment(a, v, g, legacyAttributeViewManagementEnvironment())
}

func (tx *Transaction) doSetAttrViewCardAspectRatio(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewCardAspectRatio", operation)
}

func setAttrViewCardAspectRatio(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewCardAspectRatio", operation)
}

func (tx *Transaction) doSetAttrViewBlockView(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewBlockView", operation)
}

func (tx *Transaction) doChangeAttrViewLayout(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("changeAttrViewLayout", operation)
}

func ChangeAttrViewLayout(blockID, avID string, newLayout av.LayoutType) (err error) {
	return runAttributeViewManagement(&Operation{Action: "changeAttrViewLayout", AvID: avID, BlockID: blockID, Layout: newLayout}, legacyAttributeViewManagementEnvironment())
}

func (tx *Transaction) doSetAttrViewWrapField(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewWrapField", operation)
}

func setAttrViewWrapField(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewWrapField", operation)
}

func (tx *Transaction) doSetAttrViewShowIcon(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewShowIcon", operation)
}

func setAttrViewShowIcon(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewShowIcon", operation)
}

func (tx *Transaction) doSetAttrViewFitImage(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewFitImage", operation)
}

func setAttrViewFitImage(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewFitImage", operation)
}

func (tx *Transaction) doSetAttrViewDisplayFieldName(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewDisplayFieldName", operation)
}

func (tx *Transaction) doSetAttrViewFillColBackgroundColor(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewFillColBackgroundColor", operation)
}

func setAttrViewDisplayFieldName(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewDisplayFieldName", operation)
}

func setAttrViewFillColBackgroundColor(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewFillColBackgroundColor", operation)
}

func (tx *Transaction) doSetAttrViewCardSize(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewCardSize", operation)
}

func setAttrViewCardSize(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewCardSize", operation)
}

func (tx *Transaction) doSetAttrViewCoverFromAssetKeyID(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewCoverFromAssetKeyID", operation)
}

func setAttrViewCoverFromAssetKeyID(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewCoverFromAssetKeyID", operation)
}

func (tx *Transaction) doSetAttrViewCoverFrom(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewCoverFrom", operation)
}

func setAttrViewCoverFrom(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("setAttrViewCoverFrom", operation)
}

func AppendAttributeViewDetachedBlocksWithValues(id string, rows [][]*av.Value) error {
	return (*Transaction)(nil).appendAttributeViewDetachedBlocksWithValues(id, rows)
}

func DuplicateDatabaseBlock(avID string, contexts ...context.Context) (newAvID, newBlockID string, err error) {
	return duplicateDatabaseBlockWithEnvironment(avID, legacyAttributeViewDuplicateEnvironment(contexts...))
}

func GetAttributeViewKeysByID(avID string, keyIDs ...string) []*av.Key {
	return originalAttributeKeys(avID, "ids", "", keyIDs)
}
func SetDatabaseBlockView(blockID, avID, viewID string) (err error) {
	return runAttributeViewManagement(&Operation{Action: "setAttrViewBlockView", AvID: avID, BlockID: blockID, ID: viewID}, legacyAttributeViewManagementEnvironment())
}

func GetAttributeViewPrimaryKeyValues(avID, keyword string, page, pageSize int) (string, []string, *av.KeyValues, error) {
	r, err := attributeViewPrimaryWithEnvironment(avID, keyword, page, pageSize, legacyAttributeViewReadEnvironment())
	if err != nil {
		logging.LogErrorf("get AV [%s] primary values failed: %s", avID, err)
	}
	return r.name, r.blocks, r.values, err
}

func GetAttributeViewFilterSort(avID, blockID string) ([]*av.ViewFilter, []*av.ViewSort) {
	filters, sorts, err := attributeViewRulesWithEnvironment(avID, blockID, legacyAttributeViewReadEnvironment())
	if err != nil {
		logging.LogErrorf("get AV [%s] rules failed: %s", avID, err)
	}
	return filters, sorts
}

func SearchAttributeViewNonRelationKey(avID, keyword string) []*av.Key {
	waitForSyncingStorages()
	return originalAttributeKeys(avID, "nonrelation", keyword, nil)
}
func SearchAttributeViewRollupDestKeys(avID, keyword string) []*av.Key {
	waitForSyncingStorages()
	return originalAttributeKeys(avID, "rollupdest", keyword, nil)
}
func SearchAttributeViewRelationKey(avID, keyword string) []*av.Key {
	waitForSyncingStorages()
	return originalAttributeKeys(avID, "relation", keyword, nil)
}
func GetAttributeView(avID string) *av.AttributeView {
	ret, err := readAttributeViewWithEnvironment(avID, "source", legacyAttributeViewReadEnvironment())
	if err != nil {
		logging.LogErrorf("get AV [%s] failed: %s", avID, err)
	}
	return ret
}

type AvSearchResult struct {
	AvID       string            `json:"avID"`
	AvName     string            `json:"avName"`
	ViewName   string            `json:"viewName"`
	ViewID     string            `json:"viewID"`
	ViewLayout av.LayoutType     `json:"viewLayout"`
	BlockID    string            `json:"blockID"`
	HPath      string            `json:"hPath"`
	Children   []*AvSearchResult `json:"children,omitempty"`
}

type AvSearchTempResult struct {
	AvID      string
	AvName    string
	AvUpdated int64
	Score     float64
}

func SearchAttributeView(keyword string, exclude []string) []*AvSearchResult {
	out, e := SearchAttributeViewContext(context.Background(), keyword, exclude)
	if e != nil {
		logging.LogErrorf("search AV: %s", e)
		return []*AvSearchResult{}
	}
	return out
}

type BlockAttributeViewKeys struct {
	AvID      string          `json:"avID"`
	AvName    string          `json:"avName"`
	BlockIDs  []string        `json:"blockIDs"`
	KeyValues []*av.KeyValues `json:"keyValues"`
}

func GetBlockAttributeViewKeys(nodeID string, contexts ...context.Context) []*BlockAttributeViewKeys {
	ret, err := blockAttributeViewKeysWithEnvironment(nodeID, legacyAttributeViewBlockKeysEnvironment(contexts...))
	if err != nil {
		logging.LogErrorf("get block [%s] AV keys failed: %s", nodeID, err)
		return []*BlockAttributeViewKeys{}
	}
	return ret
}

// Historical/snapshot rendering keeps its original compatibility facade until
// its detached source and repair permissions are migrated separately.
func genAttrViewGroups(view *av.View, a *av.AttributeView) {
	if err := avrender.GenerateGroups(view, a, avrender.GroupEnvironment{Now: time.Now, Read: av.ParseAttributeView, NewView: legacyGroupView, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
		return sql.RenderView(a, v, q), nil
	}}); err != nil {
		logging.LogErrorf("generate groups: %s", err)
	}
}

// GroupState 用于临时记录每个分组视图的状态，以便后面重新生成分组后可以恢复这些状态。
type GroupState = avrender.GroupState

func getAttrViewGroupStates(v *av.View) map[string]*GroupState {
	return editcore.GetAttrViewGroupStates(v)
}

func setAttrViewGroupStates(v *av.View, s map[string]*GroupState) {
	editcore.SetAttrViewGroupStates(v, s)
}

func GetCurrentAttributeViewImages(avID, viewID, query string, contexts ...context.Context) ([]string, error) {
	return currentAttributeViewImagesWithEnvironment(avID, viewID, query, legacyAttributeViewImagesEnvironment(contexts...))
}

func (tx *Transaction) doSetAttrViewColDateFillCreated(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColDateFillCreated", op)
}

func setAttributeViewColDateFillCreated(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColDateFillCreated", op)
}

func (tx *Transaction) doSetAttrViewColDateFillSpecificTime(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColDateFillSpecificTime", op)
}

func setAttrViewColDateFillSpecificTime(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColDateFillSpecificTime", op)
}

func (tx *Transaction) doSetAttrViewCreatedIncludeTime(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewCreatedIncludeTime", op)
}

func setAttrViewCreatedIncludeTime(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewCreatedIncludeTime", op)
}

func (tx *Transaction) doSetAttrViewUpdatedIncludeTime(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewUpdatedIncludeTime", op)
}

func setAttrViewUpdatedIncludeTime(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewUpdatedIncludeTime", op)
}

func (tx *Transaction) doHideAttrViewName(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("hideAttrViewName", operation)
}

func hideAttrViewName(operation *Operation) (err error) {
	return runLegacyAttributeViewManagement("hideAttrViewName", operation)
}

func (tx *Transaction) doUpdateAttrViewColRollup(operation *Operation) (ret *TxErr) {
	ctx := context.Background()
	if tx.environment != nil {
		ctx = tx.environment.ctx
	}
	err := runAttributeViewConfiguration(ctx, operation, tx.attributeViewEnvironment(), updateAttributeViewColRollupWithEnvironment)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func updateAttributeViewColRollup(operation *Operation) error {
	return updateAttributeViewColRollupWithEnvironment(operation, legacyAttributeViewEnvironment())
}

func (tx *Transaction) doUpdateAttrViewColRelation(operation *Operation) (ret *TxErr) {
	ctx := context.Background()
	if tx.environment != nil {
		ctx = tx.environment.ctx
	}
	err := runAttributeViewConfiguration(ctx, operation, tx.attributeViewEnvironment(), updateAttributeViewColRelationWithEnvironment)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func updateAttributeViewColRelation(operation *Operation) error {
	return updateAttributeViewColRelationWithEnvironment(operation, legacyAttributeViewEnvironment())
}

func (tx *Transaction) doSortAttrViewView(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("sortAttrViewView", operation)
}

func (tx *Transaction) doRemoveAttrViewView(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("removeAttrViewView", operation)
}

func getMirrorBlocksNodes(avID string) (trees []*parse.Tree, nodes []*ast.Node) {
	mirrorBlockIDs := treenode.GetMirrorAttrViewBlockIDs(avID)
	mirrorBlockTrees := filesys.LoadTrees(mirrorBlockIDs)
	for id, tree := range mirrorBlockTrees {
		node := treenode.GetNodeInTree(tree, id)
		if nil == node {
			logging.LogErrorf("get node in tree by block ID [%s] failed", id)
			continue
		}
		nodes = append(nodes, node)
	}

	for _, tree := range mirrorBlockTrees {
		trees = append(trees, tree)
	}
	return
}

func (tx *Transaction) doDuplicateAttrViewView(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("duplicateAttrViewView", operation)
}

func (tx *Transaction) doAddAttrViewView(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("addAttrViewView", operation)
}

func addAttrViewView(avID, viewID, blockID string, layout av.LayoutType) (err error) {
	return runAttributeViewManagement(&Operation{Action: "addAttrViewView", AvID: avID, BlockID: blockID, ID: viewID, Layout: layout}, legacyAttributeViewManagementEnvironment())
}

func getKanbanPreferredGroupKey(attrView *av.AttributeView) (ret *av.Key) {
	return editcore.GetKanbanPreferredGroupKey(attrView)
}

func (tx *Transaction) doSetAttrViewViewName(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewViewName", operation)
}

func (tx *Transaction) doSetAttrViewViewIcon(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewViewIcon", operation)
}

func (tx *Transaction) doSetAttrViewViewDesc(operation *Operation) *TxErr {
	return tx.doAttributeViewManagement("setAttrViewViewDesc", operation)
}

func (tx *Transaction) doSetAttrViewName(operation *Operation) (ret *TxErr) {
	err := tx.setAttributeViewName(operation)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

const attrAvNameTpl = `<span data-av-id="${avID}" data-popover-url="/api/av/getMirrorDatabaseBlocks" class="popover__block">${avName}</span>`

func (tx *Transaction) setAttributeViewName(operation *Operation) error {
	local := *operation
	local.Action = "setAttrViewName"
	return runAttributeViewName(&local, tx.attributeViewNameEnvironment())
}

func getAvNames(avIDs string) string {
	s, _ := attributeViewNamesWithServices(avIDs, av.GetAttributeViewName, func() string { return Conf.language(105) }, true)
	return s
}
func attributeViewNamesWithServices(avIDs string, lookup func(string) (string, error), fallback func() string, ignoreErrors bool) (ret string, err error) {
	return editcore.AttributeViewNamesWithServices(avIDs, lookup, fallback, ignoreErrors)
}

func (tx *Transaction) getAttrViewBoundNodes(attrView *av.AttributeView) (map[string]*parse.Tree, []*ast.Node, error) {
	return collectAttributeViewBoundNodes(attrView, tx.attributeViewBoundEnvironment())
}

func (tx *Transaction) doSetAttrViewFilters(operation *Operation) *TxErr {
	return tx.doAttributeViewSettings(operation)
}
func setAttributeViewFilters(operation *Operation) error {
	return setAttributeViewSettings(context.Background(), operation, legacyAttributeViewSettingsEnvironment())
}

func (tx *Transaction) doSetAttrViewSorts(operation *Operation) *TxErr {
	return tx.doAttributeViewSettings(operation)
}
func setAttributeViewSorts(operation *Operation) error {
	return setAttributeViewSettings(context.Background(), operation, legacyAttributeViewSettingsEnvironment())
}

func (tx *Transaction) doSetAttrViewPageSize(operation *Operation) *TxErr {
	return tx.doAttributeViewSettings(operation)
}
func setAttributeViewPageSize(operation *Operation) error {
	return setAttributeViewSettings(context.Background(), operation, legacyAttributeViewSettingsEnvironment())
}

func (tx *Transaction) doSetAttrViewColCalc(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColCalc", op)
}

func setAttributeViewColumnCalc(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColCalc", op)
}

func (tx *Transaction) doInsertAttrViewBlock(operation *Operation) (ret *TxErr) {
	if nil == operation.Context {
		operation.Context = map[string]interface{}{}
	}

	err := AddAttributeViewBlock(tx, operation.Srcs, operation.AvID, operation.BlockID, operation.ViewID, operation.GroupID, operation.PreviousID, operation.IgnoreDefaultFill, operation.Context)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func AddAttributeViewBlock(tx *Transaction, srcs []map[string]interface{}, avID, dbBlockID, viewID, groupID, previousItemID string, ignoreDefaultFill bool, context map[string]interface{}) error {
	return runAttributeViewRow(&Operation{Action: "insertAttrViewBlock", Srcs: srcs, AvID: avID, BlockID: dbBlockID, ViewID: viewID, GroupID: groupID, PreviousID: previousItemID, IgnoreDefaultFill: ignoreDefaultFill, Context: context}, tx.attributeViewRowEnvironment())
}

func addAttributeViewBlock(now int64, avID, dbBlockID, viewID, groupID, previousItemID, addingItemID, addingBoundBlockID, addingBlockContent string, isDetached, ignoreDefaultFill bool, tree *parse.Tree, tx *Transaction, context map[string]interface{}) (err error) {
	return addAttributeViewBlockWithEnvironment(now, avID, dbBlockID, viewID, groupID, previousItemID, addingItemID, addingBoundBlockID, addingBlockContent, isDetached, ignoreDefaultFill, tree, tx.attributeViewRowEnvironment())
}

func fillDefaultValue(a *av.AttributeView, v, g *av.View, p, row string, create bool) {
	_ = fillAttributeViewDefaultValues(a, v, g, p, row, create, legacyAttributeViewDefaults())
}

func getNewValueByNearItem(item av.Item, k *av.Key, row string) *av.Value {
	return legacyAttributeViewDefaults().nearValue(item, k, row)
}

func getNearItem(a *av.AttributeView, v, g *av.View, previous string) (ret av.Item) {
	var err error
	ret, err = newLegacyAVBinding(context.Background()).near(a, v, g, previous)
	if err != nil {
		logging.LogErrorf("AV near item: %s", err)
	}
	return
}

func (tx *Transaction) doRemoveAttrViewBlock(operation *Operation) (ret *TxErr) {
	err := removeAttributeViewBlock(operation.SrcIDs, operation.AvID, tx)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func RemoveAttributeViewBlock(srcIDs []string, avID string) (err error) {
	err = removeAttributeViewBlock(srcIDs, avID, nil)
	return
}

func removeAttributeViewBlock(srcIDs []string, avID string, tx *Transaction) error {
	return runAttributeViewRow(&Operation{Action: "removeAttrViewBlock", AvID: avID, SrcIDs: srcIDs}, tx.attributeViewRowEnvironment())
}

func removeNodeAvID(n *ast.Node, id string, tx *Transaction, t *parse.Tree) error {
	return removeNodeAvIDWithEnvironment(n, id, t, tx.attributeViewRowEnvironment())
}

func (tx *Transaction) doDuplicateAttrViewKey(op *Operation) *TxErr {
	return tx.doAttributeViewOrder("duplicateAttrViewKey", op)
}

func duplicateAttributeViewKey(op *Operation) error {
	return runLegacyAttributeViewOrder("duplicateAttrViewKey", op)
}

func (tx *Transaction) doSetAttrViewColumnWidth(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColWidth", op)
}

func setAttributeViewColWidth(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColWidth", op)
}

func (tx *Transaction) doSetAttrViewColumnWrap(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColWrap", op)
}

func setAttributeViewColWrap(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColWrap", op)
}

func (tx *Transaction) doSetAttrViewColumnHidden(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColHidden", op)
}

func setAttributeViewColHidden(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColHidden", op)
}

func (tx *Transaction) doSetAttrViewColumnPin(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColPin", op)
}

func setAttributeViewColPin(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColPin", op)
}

func (tx *Transaction) doSetAttrViewColumnIcon(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColIcon", op)
}

func setAttributeViewColIcon(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColIcon", op)
}

func (tx *Transaction) doSetAttrViewColumnDesc(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("setAttrViewColDesc", op)
}

func setAttributeViewColDesc(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("setAttrViewColDesc", op)
}

func (tx *Transaction) doSortAttrViewRow(op *Operation) *TxErr {
	return tx.doAttributeViewOrder("sortAttrViewRow", op)
}

func sortAttributeViewRow(op *Operation) error {
	return runLegacyAttributeViewOrder("sortAttrViewRow", op)
}

func (tx *Transaction) doSortAttrViewColumn(op *Operation) *TxErr {
	return tx.doAttributeViewOrder("sortAttrViewCol", op)
}

func SortAttributeViewViewKey(avID, blockID, keyID, previousKeyID string) error {
	return runLegacyAttributeViewOrder("sortAttrViewCol", &Operation{AvID: avID, BlockID: blockID, ID: keyID, PreviousID: previousKeyID})
}

func (tx *Transaction) doSortAttrViewKey(op *Operation) *TxErr {
	return tx.doAttributeViewOrder("sortAttrViewKey", op)
}

func SortAttributeViewKey(avID, keyID, previousKeyID string) error {
	return runLegacyAttributeViewOrder("sortAttrViewKey", &Operation{AvID: avID, ID: keyID, PreviousID: previousKeyID})
}

func refreshAttrViewKeyIDs(a *av.AttributeView, needSave bool) {
	refreshAttributeViewKeyIDs(a)
	if needSave {
		av.SaveAttributeView(a)
	}
}

func (tx *Transaction) doAddAttrViewColumn(op *Operation) *TxErr {
	return tx.doAttributeViewField("addAttrViewCol", op)
}

func AddAttributeViewKey(avID, keyID, keyName, keyType, keyIcon, previousKeyID string) error {
	return runLegacyAttributeViewField(&Operation{Action: "addAttrViewCol", AvID: avID, ID: keyID, Name: keyName, Typ: keyType, Data: keyIcon, PreviousID: previousKeyID})
}

func (tx *Transaction) doUpdateAttrViewColTemplate(operation *Operation) (ret *TxErr) {
	ctx := context.Background()
	if tx.environment != nil {
		ctx = tx.environment.ctx
	}
	err := runAttributeViewConfiguration(ctx, operation, tx.attributeViewEnvironment(), updateAttributeViewColTemplateWithEnvironment)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func updateAttributeViewColTemplate(operation *Operation) error {
	return updateAttributeViewColTemplateWithEnvironment(operation, legacyAttributeViewEnvironment())
}

func (tx *Transaction) doUpdateAttrViewColNumberFormat(op *Operation) *TxErr {
	return tx.doAttributeViewFieldSetting("updateAttrViewColNumberFormat", op)
}

func updateAttributeViewColNumberFormat(op *Operation) error {
	return runLegacyAttributeViewFieldSetting("updateAttrViewColNumberFormat", op)
}

func (tx *Transaction) doUpdateAttrViewColumn(op *Operation) *TxErr {
	return tx.doAttributeViewField("updateAttrViewCol", op)
}

func updateAttributeViewColumn(operation *Operation) error {
	return runLegacyAttributeViewFieldAction("updateAttrViewCol", operation)
}

func (tx *Transaction) doRemoveAttrViewColumn(op *Operation) *TxErr {
	return tx.doAttributeViewField("removeAttrViewCol", op)
}

func RemoveAttributeViewKey(avID, keyID string, removeRelationDest bool) error {
	return runLegacyAttributeViewField(&Operation{Action: "removeAttrViewCol", AvID: avID, ID: keyID, RemoveDest: removeRelationDest})
}

func (tx *Transaction) doReplaceAttrViewBlock(operation *Operation) (ret *TxErr) {
	err := replaceAttributeViewBlock(operation.AvID, operation.PreviousID, operation.NextID, operation.IsDetached, tx)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func replaceAttributeViewBlock(avID, old, next string, detached bool, tx *Transaction) error {
	return runAttributeViewRow(&Operation{Action: "replaceAttrViewBlock", AvID: avID, PreviousID: old, NextID: next, IsDetached: detached}, tx.attributeViewRowEnvironment())
}

func replaceAttributeViewBlock0(a *av.AttributeView, old, next string, detached bool, tx *Transaction) error {
	return replaceAttributeViewBlockWithEnvironment(a, old, next, detached, tx.attributeViewRowEnvironment())
}

func BatchReplaceAttributeViewBlocks(avID string, isDetached bool, oldNew []map[string]string) error {
	return batchReplaceAttributeViewBlocksWithEnvironment(avID, isDetached, oldNew, (*Transaction)(nil).attributeViewRowEnvironment())
}

func (tx *Transaction) doUpdateAttrViewCell(operation *Operation) (ret *TxErr) {
	_, err := UpdateAttributeViewCell(tx, operation.AvID, operation.KeyID, operation.RowID, operation.Data)
	if err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: operation.AvID, msg: err.Error()}
	}
	return
}

func BatchUpdateAttributeViewCells(tx *Transaction, avID string, values []interface{}) (err error) {
	e := tx.attributeViewCellEnvironment()
	return batchUpdateAttributeViewCellsWithEnvironment(avID, values, e)
}
func batchUpdateAttributeViewCellsWithEnvironment(avID string, values []interface{}, e *attributeViewCellEnvironment) (err error) {
	if err = e.validate(); err != nil {
		return
	}
	for _, v := range values {
		m, ok := v.(map[string]interface{})
		if !ok {
			return errors.New("invalid_av_cell_batch")
		}
		if _, ok = m["keyID"].(string); !ok {
			return errors.New("invalid_av_cell_batch")
		}
		if _, exists := m["itemID"]; exists {
			if _, ok = m["itemID"].(string); !ok {
				return errors.New("invalid_av_cell_batch")
			}
		} else if _, ok = m["rowID"].(string); !ok {
			return errors.New("invalid_av_cell_batch")
		}
	}
	attrView, err := e.read(avID)
	if err != nil {
		return
	}

	for _, value := range values {
		v := value.(map[string]interface{})
		keyID := v["keyID"].(string)
		var itemID string
		if _, ok := v["itemID"]; ok {
			itemID = v["itemID"].(string)
		} else if _, ok := v["rowID"]; ok {
			// TODO 划于 2026 年 6 月 30 日后删除 https://github.com/siyuan-note/siyuan/issues/15708#issuecomment-3239694546
			itemID = v["rowID"].(string)
		}
		valueData := v["value"]
		_, err = updateAttributeViewValueWithEnvironment(e, attrView, keyID, itemID, valueData)
		if err != nil {
			return
		}
	}
	if e.complete != nil {
		err = e.complete()
	}
	return
}

func UpdateAttributeViewCell(tx *Transaction, avID, keyID, itemID string, valueData interface{}) (val *av.Value, err error) {
	e := tx.attributeViewCellEnvironment()
	return updateAttributeViewCellWithEnvironment(avID, keyID, itemID, valueData, e)
}
func updateAttributeViewCellWithEnvironment(avID, keyID, itemID string, valueData interface{}, e *attributeViewCellEnvironment) (val *av.Value, err error) {
	return editcore.UpdateAttributeViewCellWithEnvironment(avID, keyID, itemID, valueData, e.r04Core())
}

func updateAttributeViewValue(tx *Transaction, attrView *av.AttributeView, keyID, itemID string, data interface{}) (*av.Value, error) {
	e := tx.attributeViewCellEnvironment()
	val, err := updateAttributeViewValueWithEnvironment(e, attrView, keyID, itemID, data)
	if err == nil && e.complete != nil {
		err = e.complete()
	}
	return val, err
}

func refreshRelatedSrcAvs(destAvID string, tx *Transaction) {
	if err := refreshRelatedSrcAvsContext(context.Background(), destAvID, tx); err != nil {
		logging.LogErrorf("refresh related AVs: %s", err)
	}
}
func refreshRelatedSrcAvsContext(ctx context.Context, id string, tx *Transaction) error {
	b := newLegacyAVBinding(ctx)
	if tx != nil {
		b = legacyAVBindingForTransaction(tx)
	}
	if err := b.related(id, tx); err != nil {
		return err
	}
	if tx == nil {
		return b.complete()
	}
	return nil
}

// relationChangeMode
// 0：关联字段值不变（仅排序），不影响目标值
// 1：关联字段值增加，增加目标值
// 2：关联字段值减少，减少目标值
func updateTwoWayRelationDestAttrView(attrView *av.AttributeView, relKey *av.Key, val *av.Value, relationChangeMode int, oldRelationBlockIDs []string) {
	_ = updateTwoWayRelationDestWithEnvironment((*Transaction)(nil).attributeViewCellEnvironment(), attrView, relKey, val, relationChangeMode, oldRelationBlockIDs)
}

// regenAttrViewGroups regenerates grouped views.

func regenAttrViewGroups(a *av.AttributeView) {
	if err := newLegacyAVBinding(context.Background()).groups(a); err != nil {
		logging.LogErrorf("regenerate groups: %s", err)
	}
}

func unbindBlockAv(tx *Transaction, avID, nodeID string) error {
	return bindBlockAvWithEnvironment(avID, nodeID, false, cellAttributeViewBindingEnvironment(tx.attributeViewCellEnvironment()))
}

func bindBlockAv(tx *Transaction, avID, blockID string) error {
	return bindBlockAvWithEnvironment(avID, blockID, true, cellAttributeViewBindingEnvironment(tx.attributeViewCellEnvironment()))
}

func bindBlockAv0(tx *Transaction, avID string, node *ast.Node, tree *parse.Tree) error {
	return bindBlockAv0WithEnvironment(avID, node, tree, true, cellAttributeViewBindingEnvironment(tx.attributeViewCellEnvironment()))
}

func updateBlockValueStaticText(tx *Transaction, node *ast.Node, tree *parse.Tree, avID, text string) {
	// 设置静态锚文本 Database-bound block primary key supports setting static anchor text https://github.com/siyuan-note/siyuan/issues/10049

	if nil == node {
		return
	}

	attrs := parse.IAL2Map(node.KramdownIAL)
	attrs[av.NodeAttrViewStaticText+"-"+avID] = text
	var err error
	if nil != tx {
		err = setNodeAttrsWithTx(tx, node, tree, attrs)
	} else {
		err = setNodeAttrs(node, tree, attrs)
	}
	if err != nil {
		logging.LogWarnf("set node [%s] attrs failed: %s", node.ID, err)
		return
	}
}

func getNodeByBlockID(tx *Transaction, blockID string) (node *ast.Node, tree *parse.Tree, err error) {
	if nil != tx {
		tree, err = tx.loadTree(blockID)
	} else {
		tree, err = LoadTreeByBlockID(blockID)
	}
	if err != nil {
		return
	}
	node = treenode.GetNodeInTree(tree, blockID)
	if nil == node {
		logging.LogWarnf("node [%s] not found in tree [%s]", blockID, tree.ID)
		return
	}
	return
}

func (tx *Transaction) doUpdateAttrViewColOptions(operation *Operation) *TxErr {
	return tx.doAttributeViewOption(operation)
}
func updateAttributeViewColumnOptions(operation *Operation) error {
	return runLegacyAttributeViewOption("updateAttrViewColOptions", operation)
}

func (tx *Transaction) doRemoveAttrViewColOption(operation *Operation) *TxErr {
	return tx.doAttributeViewOption(operation)
}
func removeAttributeViewColumnOption(operation *Operation) error {
	return runLegacyAttributeViewOption("removeAttrViewColOption", operation)
}

func (tx *Transaction) doUpdateAttrViewColOption(operation *Operation) *TxErr {
	return tx.doAttributeViewOption(operation)
}
func updateAttributeViewColumnOption(operation *Operation) error {
	return runLegacyAttributeViewOption("updateAttrViewColOption", operation)
}

func (tx *Transaction) doSetAttrViewColOptionDesc(operation *Operation) *TxErr {
	return tx.doAttributeViewOption(operation)
}
func setAttributeViewColumnOptionDesc(operation *Operation) error {
	return runLegacyAttributeViewOption("setAttrViewColOptionDesc", operation)
}

func getAttrViewViewByBlockID(attrView *av.AttributeView, blockID string) (*av.View, error) {
	return selectAttributeViewReadView(attrView, blockID, legacyAttributeViewReadEnvironment())
}

func getAttrViewName(attrView *av.AttributeView) string {
	ret := strings.TrimSpace(attrView.Name)
	if "" == ret {
		ret = Conf.language(105)
	}
	return ret
}

func updateBoundBlockAvsAttribute(avIDs []string) error {
	return updateBoundBlockAvsAttributeWithEnvironment(avIDs, legacyAttributeViewImportEnvironment())
}
