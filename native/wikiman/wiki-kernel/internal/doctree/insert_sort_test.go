// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

import (
	"reflect"
	"testing"
)

func TestInsertSortPreservesOriginalValuesAndUnrelatedKeys(t *testing.T) {
	source := map[string]int{"other": 99, "before": 50}
	full, notice := InsertSort(source, []*File{{ID: "first"}, {ID: "before"}, {ID: "last"}}, "before", "new")
	if !reflect.DeepEqual(full, map[string]int{"other": 99, "first": 0, "before": 1, "new": 2, "last": 3}) || !reflect.DeepEqual(notice, map[string]int{"first": 1, "before": 3, "last": 4}) {
		t.Fatal(full, notice)
	}
	if source["before"] != 50 || len(source) != 2 {
		t.Fatal("input map mutated")
	}
	full, notice = InsertSort(source, nil, "missing", "new")
	if len(notice) != 0 || len(full) != 2 {
		t.Fatal(full, notice)
	}
}
