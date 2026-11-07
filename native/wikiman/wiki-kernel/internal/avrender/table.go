// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original SQL AV table assembly, with explicit handler dependencies.
package avrender

import (
	"github.com/88250/lute/ast"
	"slices"
	"sort"
	"strings"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func GenerateItems(attrView *av.AttributeView, view *av.View) (ret map[string][]*av.KeyValues) {
	ret = map[string][]*av.KeyValues{}
	for _, keyValues := range attrView.KeyValues {
		for _, val := range keyValues.Values {
			values := ret[val.BlockID]
			if nil == values {
				values = []*av.KeyValues{{Key: keyValues.Key, Values: []*av.Value{val}}}
			} else {
				values = append(values, &av.KeyValues{Key: keyValues.Key, Values: []*av.Value{val}})
			}
			ret[val.BlockID] = values
		}
	}

	// 如果是分组视图，则需要过滤掉不在分组中的项目
	if nil != view.GroupItemIDs {
		tmp := map[string][]*av.KeyValues{}
		for _, groupItemID := range view.GroupItemIDs {
			if _, ok := ret[groupItemID]; ok {
				tmp[groupItemID] = ret[groupItemID]
			}
		}
		ret = tmp
	}
	return
}

func FillBaseValue(baseValue *av.BaseValue, fieldID, itemID string, fieldNumberFormat av.NumberFormat, fieldTemplate string, fieldDateIsTime bool) {
	switch baseValue.ValueType {
	case av.KeyTypeNumber: // 格式化数字
		if nil != baseValue.Value && nil != baseValue.Value.Number && baseValue.Value.Number.IsNotEmpty {
			baseValue.Value.Number.Format = fieldNumberFormat
			baseValue.Value.Number.FormatNumber()
		}
	case av.KeyTypeTemplate: // 渲染模板字段
		baseValue.Value = &av.Value{ID: baseValue.ID, KeyID: fieldID, BlockID: itemID, Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: fieldTemplate}}
	case av.KeyTypeCreated: // 填充创建时间字段值，后面再渲染
		baseValue.Value = &av.Value{ID: baseValue.ID, KeyID: fieldID, BlockID: itemID, Type: av.KeyTypeCreated}
	case av.KeyTypeUpdated: // 填充更新时间字段值，后面再渲染
		baseValue.Value = &av.Value{ID: baseValue.ID, KeyID: fieldID, BlockID: itemID, Type: av.KeyTypeUpdated}
	}

	if nil == baseValue.Value {
		baseValue.Value = av.GetAttributeViewDefaultValue(baseValue.ID, fieldID, itemID, baseValue.ValueType, fieldDateIsTime)
	} else {
		FillNilValue(baseValue.Value, baseValue.ValueType)
	}
}

func FillKeyValues(attrView *av.AttributeView, collection av.Collection) {
	fieldValues := map[string][]*av.Value{}
	for _, item := range collection.GetItems() {
		for _, val := range item.GetValues() {
			keyID := val.KeyID
			fieldValues[keyID] = append(fieldValues[keyID], val)
		}
	}
	for keyID, values := range fieldValues {
		keyValues, _ := attrView.GetKeyValues(keyID)
		for _, val := range values {
			exist := false
			for _, kv := range keyValues.Values {
				if kv.ID == val.ID {
					exist = true
					break
				}
			}
			if !exist {
				val.IsRenderAutoFill = true
				keyValues.Values = append(keyValues.Values, val)
			}
		}
	}
}

func FillNilValue(value *av.Value, typ av.KeyType) {
	value.Type = typ
	switch typ {
	case av.KeyTypeText:
		if nil == value.Text {
			value.Text = &av.ValueText{}
		}
	case av.KeyTypeNumber:
		if nil == value.Number {
			value.Number = &av.ValueNumber{}
		}
	case av.KeyTypeDate:
		if nil == value.Date {
			value.Date = &av.ValueDate{}
		}
	case av.KeyTypeSelect:
		if 1 > len(value.MSelect) {
			value.MSelect = []*av.ValueSelect{}
		}
	case av.KeyTypeMSelect:
		if 1 > len(value.MSelect) {
			value.MSelect = []*av.ValueSelect{}
		}
	case av.KeyTypeURL:
		if nil == value.URL {
			value.URL = &av.ValueURL{}
		}
	case av.KeyTypeEmail:
		if nil == value.Email {
			value.Email = &av.ValueEmail{}
		}
	case av.KeyTypePhone:
		if nil == value.Phone {
			value.Phone = &av.ValuePhone{}
		}
	case av.KeyTypeMAsset:
		if 1 > len(value.MAsset) {
			value.MAsset = []*av.ValueAsset{}
		}
	case av.KeyTypeTemplate:
		if nil == value.Template {
			value.Template = &av.ValueTemplate{}
		}
	case av.KeyTypeCreated:
		if nil == value.Created {
			value.Created = &av.ValueCreated{}
		}
	case av.KeyTypeUpdated:
		if nil == value.Updated {
			value.Updated = &av.ValueUpdated{}
		}
	case av.KeyTypeCheckbox:
		if nil == value.Checkbox {
			value.Checkbox = &av.ValueCheckbox{}
		}
	case av.KeyTypeRelation:
		if nil == value.Relation {
			value.Relation = &av.ValueRelation{}
		}
	case av.KeyTypeRollup:
		if nil == value.Rollup {
			value.Rollup = &av.ValueRollup{}
		}
	}
}

func FilterByQuery(query string, collection av.Collection) {
	query = strings.TrimSpace(query)
	if "" != query {
		query = strings.Join(strings.Fields(query), " ") // 将连续空格转换为一个空格
		keywords := strings.Split(query, " ")            // 按空格分割关键字

		// 使用 AND 逻辑 https://github.com/siyuan-note/siyuan/issues/11535
		var hitItems []av.Item
		for _, item := range collection.GetItems() {
			hit := false
			for _, cell := range item.GetValues() {
				allKeywordsHit := true
				for _, keyword := range keywords {
					if !strings.Contains(strings.ToLower(cell.String(true)), strings.ToLower(keyword)) {
						allKeywordsHit = false
						break
					}
				}
				if allKeywordsHit {
					hit = true
					break
				}
			}
			if hit {
				hitItems = append(hitItems, item)
			}
		}
		collection.SetItems(hitItems)
		if 1 > len(collection.GetItems()) {
			collection.SetItems([]av.Item{})
		}
	}
}

func ManualSort(view *av.View, collection av.Collection) {
	itemIDs := view.ItemIDs
	// 如果是分组视图，则需要根据分组项的顺序进行排序
	if 0 < len(view.GroupItemIDs) {
		itemIDs = view.GroupItemIDs
	}

	sortItemIDs := map[string]int{}
	for i, itemID := range itemIDs {
		sortItemIDs[itemID] = i
	}

	items := collection.GetItems()
	sort.Slice(items, func(i, j int) bool {
		iv := sortItemIDs[items[i].GetID()]
		jv := sortItemIDs[items[j].GetID()]
		if iv == jv {
			return items[i].GetID() < items[j].GetID()
		}
		return iv < jv
	})
	collection.SetItems(items)
}

func FillBasicAutoValues(attrView *av.AttributeView, collection av.Collection, ials map[string]map[string]string, now func() time.Time) {
	// 渲染主键、创建时间、更新时间

	for _, item := range collection.GetItems() {
		for _, value := range item.GetValues() {
			itemID := item.GetID()

			switch value.Type {
			case av.KeyTypeBlock: // 对于主键可能需要填充静态锚文本 Database-bound block primary key supports setting static anchor text https://github.com/siyuan-note/siyuan/issues/10049
				if nil != value.Block {
					for k, v := range ials[value.Block.ID] {
						if k == av.NodeAttrViewStaticText+"-"+attrView.ID {
							value.Block.Content = v
							break
						}
					}
				}
			case av.KeyTypeCreated: // 渲染创建时间
				key, _ := attrView.GetKey(value.KeyID)
				isNotTime := false
				if nil != key && nil != key.Created {
					isNotTime = !key.Created.IncludeTime
				}

				ial := map[string]string{}
				block := item.GetBlockValue()
				if nil != block {
					ial = ials[block.Block.ID]
				}
				if nil == ial {
					ial = map[string]string{}
				}
				id := itemID
				if "" != ial["id"] {
					id = ial["id"]
				}
				createdStr := id[:len("20060102150405")]
				created, parseErr := time.ParseInLocation("20060102150405", createdStr, time.Local)
				if nil == parseErr {
					value.Created = av.NewFormattedValueCreated(created.UnixMilli(), 0, av.CreatedFormatNone, isNotTime)
					value.Created.IsNotEmpty = true
				} else {
					value.Created = av.NewFormattedValueCreated(now().UnixMilli(), 0, av.CreatedFormatNone, isNotTime)
				}
			case av.KeyTypeUpdated: // 渲染更新时间
				key, _ := attrView.GetKey(value.KeyID)
				isNotTime := false
				if nil != key && nil != key.Updated {
					isNotTime = !key.Updated.IncludeTime
				}

				ial := map[string]string{}
				block := item.GetBlockValue()
				if nil != block {
					ial = ials[block.Block.ID]
				}
				if nil == ial {
					ial = map[string]string{}
				}
				updatedStr := ial["updated"]
				if "" == updatedStr && nil != block {
					value.Updated = av.NewFormattedValueUpdated(block.Block.Updated, 0, av.UpdatedFormatNone, isNotTime)
					value.Updated.IsNotEmpty = true
				} else {
					updated, parseErr := time.ParseInLocation("20060102150405", updatedStr, time.Local)
					if nil == parseErr {
						value.Updated = av.NewFormattedValueUpdated(updated.UnixMilli(), 0, av.UpdatedFormatNone, isNotTime)
						value.Updated.IsNotEmpty = true
					} else {
						value.Updated = av.NewFormattedValueUpdated(now().UnixMilli(), 0, av.UpdatedFormatNone, isNotTime)
					}
				}
			}
		}
	}

}

// TableColumns preserves the original ordering, hidden-column inclusion and
// field metadata. Template previews consume the same assembled header without
// evaluating or publishing rows that are discarded by the preview.
func TableColumns(attrView *av.AttributeView, view *av.View, missing func(*av.AttributeView, *av.View, string) error) (columns []*av.TableColumn, err error) {
	if attrView == nil || view == nil || view.Table == nil || missing == nil {
		return nil, ErrShape
	}
	columns = []*av.TableColumn{}
	// 组装列
	// Repair callbacks can remove fields from the live slice.
	for _, col := range slices.Clone(view.Table.Columns) {
		key, getErr := attrView.GetKey(col.ID)
		if nil != getErr {
			// 找不到字段则在视图中删除
			if err = missing(attrView, view, col.ID); err != nil {
				return nil, err
			}
			continue
		}

		columns = append(columns, &av.TableColumn{
			BaseInstanceField: &av.BaseInstanceField{
				ID:           key.ID,
				Name:         key.Name,
				Type:         key.Type,
				Icon:         key.Icon,
				Wrap:         col.Wrap,
				Hidden:       col.Hidden,
				Desc:         key.Desc,
				Calc:         col.Calc,
				Options:      key.Options,
				NumberFormat: key.NumberFormat,
				Template:     key.Template,
				Relation:     key.Relation,
				Rollup:       key.Rollup,
				Date:         key.Date,
				Created:      key.Created,
				Updated:      key.Updated,
			},
			Width: col.Width,
			Pin:   col.Pin,
		})
	}

	return columns, nil
}
func TableColumnNames(attrView *av.AttributeView, view *av.View) ([]string, error) {
	columns, err := TableColumns(attrView, view, func(*av.AttributeView, *av.View, string) error { return ErrShape })
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	return names, nil
}

func Table(attrView *av.AttributeView, view *av.View, query string, env Environment) (ret *av.Table, err error) {
	if err = env.validate(); err != nil {
		return nil, err
	}
	if attrView == nil || view == nil || view.Table == nil {
		return nil, ErrShape
	}
	viewable := attrView.RenderedViewables[view.ID]
	if nil != viewable {
		ret = viewable.(*av.Table)
		return
	}

	ret = &av.Table{
		BaseInstance: av.NewViewBaseInstance(view),
		Columns:      []*av.TableColumn{},
		Rows:         []*av.TableRow{},
	}

	ret.Columns, err = TableColumns(attrView, view, env.RemoveMissingField)
	if err != nil {
		return nil, err
	}

	rowsValues := GenerateItems(attrView, view) // 生成行
	if err = env.FilterItems(rowsValues); err != nil {
		return nil, err
	} // 过滤掉不存在的行

	// 生成行单元格
	for rowID, rowValues := range rowsValues {
		var tableRow av.TableRow
		for _, col := range ret.Columns {
			var tableCell *av.TableCell
			for _, keyValues := range rowValues {
				if keyValues.Key.ID == col.ID {
					tableCell = &av.TableCell{
						BaseValue: &av.BaseValue{
							ID:        keyValues.Values[0].ID,
							Value:     keyValues.Values[0],
							ValueType: col.Type,
						},
					}
					break
				}
			}
			if nil == tableCell {
				tableCell = &av.TableCell{
					BaseValue: &av.BaseValue{
						ID:        rowID[:14] + ast.NewNodeID()[14:],
						ValueType: col.Type,
					},
				}
			}
			tableRow.ID = rowID

			filedDateIsTime := false
			if nil != col.Date {
				filedDateIsTime = col.Date.FillSpecificTime
			}
			FillBaseValue(tableCell.BaseValue, col.ID, rowID, col.NumberFormat, col.Template, filedDateIsTime)
			tableRow.Cells = append(tableRow.Cells, tableCell)
		}
		ret.Rows = append(ret.Rows, &tableRow)
	}

	// 回填补全数据
	FillKeyValues(attrView, ret)

	// 批量获取块属性以提升性能
	var ialIDs []string
	for _, row := range ret.Rows {
		blockVal := row.GetBlockValue()
		if nil != blockVal && !blockVal.IsDetached {
			ialIDs = append(ialIDs, blockVal.Block.ID)
		}
	}
	ials, err := env.BlockAttrs(ialIDs)
	if err != nil {
		return nil, err
	}

	// 渲染自动生成的字段值，比如关联、汇总、创建时间和更新时间
	if err = env.AutoValues(attrView, ret, ials); err != nil {
		return nil, err
	}

	// 最后渲染模板字段，这样模板就可以使用汇总、关联、创建时间和更新时间的值了
	if err = env.Templates(attrView, view, ret, ials); err != nil {
		return nil, err
	}

	FilterByQuery(query, ret)
	ManualSort(view, ret)
	return
}
