// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	gulu "github.com/88250/gulu"
)

func EncodeAttributeView(av *AttributeView, singleLine bool) (data []byte, err error) {
	// 值去重
	blockValues := av.GetBlockKeyValues()
	blockIDs := map[string]bool{}
	var duplicatedValueIDs []string
	for _, blockValue := range blockValues.Values {
		if !blockIDs[blockValue.BlockID] {
			blockIDs[blockValue.BlockID] = true
		} else {
			duplicatedValueIDs = append(duplicatedValueIDs, blockValue.ID)
		}
	}
	var tmp []*Value
	for _, blockValue := range blockValues.Values {
		if !gulu.Str.Contains(blockValue.ID, duplicatedValueIDs) {
			tmp = append(tmp, blockValue)
		}
	}
	blockValues.Values = tmp

	// 视图值去重
	for _, view := range av.Views {
		// 项目自定义排序去重
		view.ItemIDs = gulu.Str.RemoveDuplicatedElem(view.ItemIDs)

		// 分页大小
		if 1 > view.PageSize {
			view.PageSize = ViewDefaultPageSize
		}
	}

	// 清理渲染回填值
	for _, kv := range av.KeyValues {
		for i := len(kv.Values) - 1; i >= 0; i-- {
			if kv.Values[i].IsRenderAutoFill {
				kv.Values = append(kv.Values[:i], kv.Values[i+1:]...)
			}
		}
	}

	if singleLine {
		data, err = gulu.JSON.MarshalJSON(av)
	} else {
		data, err = gulu.JSON.MarshalIndentJSON(av, "", "\t")
	}
	if err != nil {
		return
	}

	return
}
