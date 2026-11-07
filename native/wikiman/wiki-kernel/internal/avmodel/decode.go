// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"errors"
	"strings"

	"github.com/88250/gulu"
)

// DecodeAttributeView retains the original legacy relation-format adapter without source IO or global runtime bindings.
func DecodeAttributeView(data []byte) (ret *AttributeView, err error) {
	defer func() {
		if recover() != nil {
			ret = nil
			err = errors.New("invalid AV legacy relation source")
		}
	}()
	ret = &AttributeView{RenderedViewables: map[string]Viewable{}}
	if err = gulu.JSON.UnmarshalJSON(data, ret); err != nil {
		if strings.Contains(err.Error(), ".relation.contents of type av.Value") || strings.Contains(err.Error(), ".relation.contents of type avmodel.Value") {
			mapAv := map[string]interface{}{}
			if err = gulu.JSON.UnmarshalJSON(data, &mapAv); err != nil {
				return
			}

			// v3.0.3 兼容之前旧版本，将 relation.contents[""] 转换为 null
			keyValues := mapAv["keyValues"]
			keyValuesMap := keyValues.([]interface{})
			for _, kv := range keyValuesMap {
				kvMap := kv.(map[string]interface{})
				if values := kvMap["values"]; nil != values {
					valuesMap := values.([]interface{})
					for _, v := range valuesMap {
						if vMap := v.(map[string]interface{}); nil != vMap["relation"] {
							vMap["relation"].(map[string]interface{})["contents"] = nil
						}
					}
				}
			}

			views := mapAv["views"]
			viewsMap := views.([]interface{})
			for _, view := range viewsMap {
				if table := view.(map[string]interface{})["table"]; nil != table {
					tableMap := table.(map[string]interface{})
					if filters := tableMap["filters"]; nil != filters {
						filtersMap := filters.([]interface{})
						for _, f := range filtersMap {
							if fMap := f.(map[string]interface{}); nil != fMap["value"] {
								if valueMap := fMap["value"].(map[string]interface{}); nil != valueMap["relation"] {
									valueMap["relation"].(map[string]interface{})["contents"] = nil
								}
							}
						}
					}
				}
			}

			data, err = gulu.JSON.MarshalJSON(mapAv)
			if err != nil {
				return
			}

			if err = gulu.JSON.UnmarshalJSON(data, ret); err != nil {
				return
			}
		} else {
			return
		}
	}
	return
}
