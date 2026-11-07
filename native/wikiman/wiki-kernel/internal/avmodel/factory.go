// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import "errors"

// The original table/primary/select defaults with explicit language and IDs.
func DefaultTable(blockID string, labels map[string]string, newID func() string) (*View, *Key, *Key, error) {
	if newID == nil {
		return nil, nil, nil, errors.New("AV factory IDs unbound")
	}
	for _, key := range []string{"table", "key", "select"} {
		if _, ok := labels[key]; !ok {
			return nil, nil, nil, errors.New("AV factory language unbound")
		}
	}
	v := &View{ID: newID(), Name: labels["table"], Filters: []*ViewFilter{}, Sorts: []*ViewSort{}, LayoutType: LayoutTypeTable, Table: NewLayoutTable(), PageSize: ViewDefaultPageSize}
	block := NewKey(blockID, labels["key"], "", KeyTypeBlock)
	selectKey := NewKey(newID(), labels["select"], "", KeyTypeSelect)
	v.Table.Columns = []*ViewTableColumn{{BaseField: &BaseField{ID: blockID}}, {BaseField: &BaseField{ID: selectKey.ID}}}
	return v, block, selectKey, nil
}
func DefaultAttributeView(id string, labels map[string]string, newID func() string) (*AttributeView, error) {
	if newID == nil {
		return nil, errors.New("AV factory IDs unbound")
	}
	v, block, selectKey, e := DefaultTable(newID(), labels, newID)
	if e != nil {
		return nil, e
	}
	return &AttributeView{Spec: CurrentSpec, ID: id, KeyValues: []*KeyValues{{Key: block}, {Key: selectKey}}, ViewID: v.ID, Views: []*View{v}}, nil
}
