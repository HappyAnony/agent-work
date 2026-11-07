package avmodel

import (
	"reflect"
	"testing"
	"time"
)

func TestDefaultFilterClockIDsAndRelativeRegions(t *testing.T) {
	for _, offset := range []int{0, 8 * 3600, -7 * 3600} {
		zone := time.FixedZone("owner", offset)
		now := time.Date(2026, 10, 24, 14, 15, 16, 0, zone)
		clock := func() time.Time { return now }
		for _, unit := range []RelativeDateUnit{RelativeDateUnitDay, RelativeDateUnitWeek, RelativeDateUnitMonth, RelativeDateUnitYear} {
			for _, direction := range []RelativeDateDirection{RelativeDateDirectionBefore, RelativeDateDirectionThis, RelativeDateDirectionAfter} {
				f := &ViewFilter{Column: "date", Operator: FilterOperatorIsEqual, RelativeDate: &RelativeDate{Count: 2, Unit: unit, Direction: direction}, Value: &Value{Type: KeyTypeDate, Date: &ValueDate{}}}
				v := f.GetAffectValueWithServices(&Key{ID: "date", Type: KeyTypeDate}, "row", clock, func() string { return "owner-id" })
				start, _ := calcRelativeTimeRegionAt(2, unit, direction, now)
				if v == nil || v.ID != "owner-id" || v.CreatedAt != now.UnixMilli() || v.UpdatedAt != now.UnixMilli()+1000 || v.Date.Content != start.UnixMilli() {
					t.Fatal("owner date/region lost", offset, unit, direction, v)
				}
			}
		}
		f := &ViewFilter{Operator: FilterOperatorIsBetween, Value: &Value{Type: KeyTypeDate, Date: &ValueDate{Content: now.Add(-time.Hour).UnixMilli(), Content2: now.Add(time.Hour).UnixMilli(), IsNotEmpty: true}}}
		if v := f.GetAffectValueWithServices(&Key{ID: "date", Type: KeyTypeDate}, "row", clock, func() string { return "id" }); v.Date.Content != now.UnixMilli() {
			t.Fatal("between default ignored clock")
		}
		for _, id := range []string{"20261024123456-test001", "bad", ""} {
			v := GetAttributeViewDefaultValueWithServices(id, "date", "row", KeyTypeDate, true, clock, func() string { return "20261024123456-test001" })
			want := now
			if id != "bad" {
				want = time.Date(2026, 10, 24, 12, 34, 56, 0, zone)
			}
			if v.CreatedAt != want.UnixMilli() || v.UpdatedAt != want.UnixMilli() || v.Date.IsNotTime {
				t.Fatal("ID parse timezone/fallback", id, v)
			}
		}
	}
}

func TestDefaultFilterOriginalRulesDifferential(t *testing.T) {
	keys := []*Key{{ID: "text", Type: KeyTypeText}, {ID: "number", Type: KeyTypeNumber}, {ID: "select", Type: KeyTypeSelect, Options: []*SelectOption{{Name: "Old", Color: "2"}}}, {ID: "check", Type: KeyTypeCheckbox}}
	values := []*Value{{Type: KeyTypeText, Text: &ValueText{Content: "needle"}}, {Type: KeyTypeNumber, Number: &ValueNumber{Content: 3, IsNotEmpty: true}}, {Type: KeyTypeSelect, MSelect: []*ValueSelect{{Content: "Old", Color: "2"}}}, {Type: KeyTypeCheckbox, Checkbox: &ValueCheckbox{Checked: true}}}
	for i, key := range keys {
		for _, op := range []FilterOperator{FilterOperatorIsEqual, FilterOperatorContains, FilterOperatorIsEmpty, FilterOperatorIsNotEmpty, FilterOperatorIsGreater, FilterOperatorIsTrue} {
			f := &ViewFilter{Operator: op, Value: values[i]}
			a := f.GetAffectValue(key, "row")
			b := f.GetAffectValueWithServices(key, "row", time.Now, func() string { return "fixed" })
			if a != nil {
				a.ID = ""
				a.CreatedAt = 0
				a.UpdatedAt = 0
			}
			if b != nil {
				b.ID = ""
				b.CreatedAt = 0
				b.UpdatedAt = 0
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatal("legacy default differs", key.Type, op, a, b)
			}
		}
	}
}
