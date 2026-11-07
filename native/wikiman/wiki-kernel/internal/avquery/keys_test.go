package avquery

import (
	"context"
	"reflect"
	"testing"
)

func TestOriginalAVKeyFiltersKeepOrderAndDuplicateIDs(t *testing.T) {
	keys := []KeyMeta{{ID: "t", Name: "AGI text", Type: "text"}, {ID: "r", Name: "agi relation", Type: "relation", Relation: true}, {ID: "broken", Name: "agi", Type: "relation"}, {ID: "roll", Name: "agi", Type: "rollup"}, {ID: "line", Name: "agi", Type: "lineNumber"}}
	for _, test := range []struct {
		Kind string
		IDs  []string
		Want []string
	}{{"ids", []string{"r", "t", "r"}, []string{"t", "r", "r"}}, {"nonrelation", nil, []string{"t"}}, {"rollupdest", nil, []string{"t", "r", "broken"}}, {"relation", nil, []string{"r"}}} {
		got, e := SelectKeys(context.Background(), keys, func(v KeyMeta) KeyMeta { return v }, test.Kind, "AGI", test.IDs)
		if e != nil {
			t.Fatal(e)
		}
		ids := []string{}
		for _, v := range got {
			ids = append(ids, v.ID)
		}
		if !reflect.DeepEqual(ids, test.Want) {
			t.Fatal(test.Kind, ids, test.Want)
		}
	}
}
