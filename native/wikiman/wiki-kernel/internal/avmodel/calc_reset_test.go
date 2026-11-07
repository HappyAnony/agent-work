package avmodel

import "testing"

func TestEmptyGroupDoesNotInheritParentDurationResult(t *testing.T) {
	for _, fieldCalc := range []bool{false, true} {
		old := &Value{Date: &ValueDate{FormattedContent: "parent duration"}}
		c := &TableColumn{BaseInstanceField: &BaseInstanceField{ID: "date", Type: KeyTypeDate}}
		group := &GroupCalc{Field: "date", FieldCalc: &FieldCalc{Operator: CalcOperatorRange, Result: old}}
		if fieldCalc {
			c.Calc = &FieldCalc{Operator: CalcOperatorRange, Result: old}
		}
		out := &Table{BaseInstance: &BaseInstance{GroupCalc: group}, Columns: []*TableColumn{c}, Rows: []*TableRow{}}
		a := &AttributeView{KeyValues: []*KeyValues{{Key: &Key{ID: "date", Type: KeyTypeDate}}}}
		Calc(out, a)
		if out.GroupCalc.FieldCalc.Result != nil || (c.Calc != nil && c.Calc.Result != nil) {
			t.Fatal("empty group retained parent result", fieldCalc)
		}
	}
}
