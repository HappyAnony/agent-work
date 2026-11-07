// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

// InsertSort preserves the original addSort loop, including the distinct
// persisted values and notification values. Unrelated existing keys survive.
func InsertSort(existing map[string]int, docs []*File, previous, id string) (full, notice map[string]int) {
	full = make(map[string]int, len(existing)+len(docs)+1)
	for k, v := range existing {
		full[k] = v
	}
	notice = map[string]int{}
	value := 0
	for _, doc := range docs {
		full[doc.ID] = value
		if doc.ID == previous {
			value++
			full[id] = value
		}
		value++
		notice[doc.ID] = value
	}
	return
}
