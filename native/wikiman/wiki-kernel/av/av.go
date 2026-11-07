// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Extracted from the original AV implementation; legacy UI aliases these types.
package av

import (
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	jsoniter "github.com/json-iterator/go"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"os"
	"path/filepath"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/util"
)

type AttributeView = avmodel.AttributeView

type KeyValues = avmodel.KeyValues

var GetKeyBlockValue = avmodel.GetKeyBlockValue

var GetValue = avmodel.GetValue

type KeyType = avmodel.KeyType

const KeyTypeBlock = avmodel.KeyTypeBlock

const KeyTypeText = avmodel.KeyTypeText

const KeyTypeNumber = avmodel.KeyTypeNumber

const KeyTypeDate = avmodel.KeyTypeDate

const KeyTypeSelect = avmodel.KeyTypeSelect

const KeyTypeMSelect = avmodel.KeyTypeMSelect

const KeyTypeURL = avmodel.KeyTypeURL

const KeyTypeEmail = avmodel.KeyTypeEmail

const KeyTypePhone = avmodel.KeyTypePhone

const KeyTypeMAsset = avmodel.KeyTypeMAsset

const KeyTypeTemplate = avmodel.KeyTypeTemplate

const KeyTypeCreated = avmodel.KeyTypeCreated

const KeyTypeUpdated = avmodel.KeyTypeUpdated

const KeyTypeCheckbox = avmodel.KeyTypeCheckbox

const KeyTypeRelation = avmodel.KeyTypeRelation

const KeyTypeRollup = avmodel.KeyTypeRollup

const KeyTypeLineNumber = avmodel.KeyTypeLineNumber

type Key = avmodel.Key

var NewKey = avmodel.NewKey

type Created = avmodel.Created

type Updated = avmodel.Updated

type Date = avmodel.Date

type Rollup = avmodel.Rollup

type RollupCalc = avmodel.RollupCalc

type Relation = avmodel.Relation

type SelectOption = avmodel.SelectOption

type View = avmodel.View

type GroupCalc = avmodel.GroupCalc

type LayoutType = avmodel.LayoutType

const LayoutTypeTable = avmodel.LayoutTypeTable

const LayoutTypeGallery = avmodel.LayoutTypeGallery

const LayoutTypeKanban = avmodel.LayoutTypeKanban

const ViewDefaultPageSize = avmodel.ViewDefaultPageSize

func NewTableView() *View {
	return &View{
		ID:         ast.NewNodeID(),
		Name:       GetAttributeViewI18n("table"),
		Filters:    []*ViewFilter{},
		Sorts:      []*ViewSort{},
		PageSize:   ViewDefaultPageSize,
		LayoutType: LayoutTypeTable,
		Table:      NewLayoutTable(),
	}
}

func NewTableViewWithBlockKey(blockKeyID string) (view *View, blockKey, selectKey *Key) {
	var err error
	view, blockKey, selectKey, err = avmodel.DefaultTable(blockKeyID, defaultViewLabels(), ast.NewNodeID)
	if err != nil {
		panic(err)
	}
	return
}
func defaultViewLabels() map[string]string {
	return map[string]string{"table": GetAttributeViewI18n("table"), "key": GetAttributeViewI18n("key"), "select": GetAttributeViewI18n("select")}
}

func NewGalleryView() (ret *View) {
	return &View{
		ID:         ast.NewNodeID(),
		Name:       GetAttributeViewI18n("gallery"),
		Filters:    []*ViewFilter{},
		Sorts:      []*ViewSort{},
		PageSize:   ViewDefaultPageSize,
		LayoutType: LayoutTypeGallery,
		Gallery:    NewLayoutGallery(),
	}
}

func NewKanbanView() (ret *View) {
	return &View{
		ID:         ast.NewNodeID(),
		Name:       GetAttributeViewI18n("kanban"),
		Filters:    []*ViewFilter{},
		Sorts:      []*ViewSort{},
		PageSize:   ViewDefaultPageSize,
		LayoutType: LayoutTypeKanban,
		Kanban:     NewLayoutKanban(),
	}
}

type Viewable = avmodel.Viewable

func NewAttributeView(id string) (ret *AttributeView) {
	var err error
	ret, err = avmodel.DefaultAttributeView(id, defaultViewLabels(), ast.NewNodeID)
	if err != nil {
		panic(err)
	}
	ret.ReadAttributeView = ParseAttributeView
	ret.DurationFormatter = FormatDuration
	ret.ErrorSink = reportError
	return
}

func GetAttributeViewName(avID string) (ret string, err error) {
	avJSONPath := GetAttributeViewDataPath(avID)
	if !filelock.IsExist(avJSONPath) {
		return
	}

	return GetAttributeViewNameByPath(avJSONPath)
}

func GetAttributeViewNameByPath(avJSONPath string) (ret string, err error) {
	data, err := filelock.ReadFile(avJSONPath)
	if err != nil {
		logging.LogErrorf("read attribute view [%s] failed: %s", avJSONPath, err)
		return
	}

	val := jsoniter.Get(data, "name")
	if nil == val || val.ValueType() == jsoniter.InvalidValue {
		return
	}
	ret = val.ToString()
	return
}

func IsAttributeViewExist(avID string) bool {
	avJSONPath := GetAttributeViewDataPath(avID)
	return filelock.IsExist(avJSONPath)
}

func ParseAttributeView(avID string) (ret *AttributeView, err error) {
	defer func() {
		if ret != nil {
			ret.ReadAttributeView = ParseAttributeView
			ret.ErrorSink = reportError
			ret.DurationFormatter = FormatDuration
		}
	}()
	avJSONPath := GetAttributeViewDataPath(avID)
	if !filelock.IsExist(avJSONPath) {
		err = ErrViewNotFound
		return
	}

	data, readErr := filelock.ReadFile(avJSONPath)
	if nil != readErr {
		logging.LogErrorf("read attribute view [%s] failed: %s", avID, readErr)
		err = readErr
		return
	}

	return DecodeAttributeView(data)
}

// DecodeAttributeView retains the original legacy relation-format adapter.
func DecodeAttributeView(data []byte) (*AttributeView, error) {
	ret, err := avmodel.DecodeAttributeView(data)
	if err != nil {
		logging.LogErrorf("unmarshal attribute view [%s] failed: %s", "source", err)
	}
	return ret, err
}

func SaveAttributeView(av *AttributeView) (err error) {
	if "" == av.ID {
		err = errors.New("av id is empty")
		logging.LogErrorf("save attribute view failed: %s", err)
		return
	}

	// 做一些数据兼容和订正处理
	UpgradeSpec(av)

	data, err := EncodeAttributeView(av, util.UseSingleLineSave)
	if err != nil {
		return err
	}

	avJSONPath := GetAttributeViewDataPath(av.ID)
	if err = filelock.WriteFile(avJSONPath, data); err != nil {
		logging.LogErrorf("save attribute view [%s] failed: %s", av.ID, err)
		return
	}

	if util.ExceedLargeFileWarningSize(len(data)) {
		msg := fmt.Sprintf(util.Langs[util.Lang][268], av.Name+" "+filepath.Base(avJSONPath), util.LargeFileWarningSize)
		util.PushErrMsg(msg, 7000)
	}
	return
}

func GetAttributeViewDataPath(avID string) (ret string) {
	av := filepath.Join(util.DataDir, "storage", "av")
	ret = filepath.Join(av, avID+".json")
	if !gulu.File.IsDir(av) {
		if err := os.MkdirAll(av, 0755); err != nil {
			logging.LogErrorf("create attribute view dir failed: %s", err)
			return
		}
	}
	return
}

func GetAttributeViewI18n(key string) string {
	return util.AttrViewLangs[util.Lang][key].(string)
}

var ErrViewNotFound = avmodel.ErrViewNotFound

var ErrKeyNotFound = avmodel.ErrKeyNotFound

var ErrWrongLayoutType = avmodel.ErrWrongLayoutType

const NodeAttrNameAvs = avmodel.NodeAttrNameAvs

const NodeAttrView = avmodel.NodeAttrView

const NodeAttrViewStaticText = avmodel.NodeAttrViewStaticText

const NodeAttrViewNames = avmodel.NodeAttrViewNames

// EncodeAttributeView normalizes an already-upgraded detached candidate with the original deduplication/format rules.
func EncodeAttributeView(av *AttributeView, singleLine bool) (data []byte, err error) {
	data, err = avmodel.EncodeAttributeView(av, singleLine)
	if err != nil {
		logging.LogErrorf("marshal attribute view [%s] failed: %s", av.ID, err)
	}
	return
}
