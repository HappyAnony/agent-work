package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
)

func optionSource(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	t.Helper()
	m, store, root := cellSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		file := filepath.Join(root, "pages", page, "data", "storage", "av", cellAV+".json")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		a := &av.AttributeView{}
		if e = json.Unmarshal(raw, a); e != nil {
			t.Fatal(e)
		}
		for _, id := range []string{"status", "multi"} {
			kv, _ := a.GetKeyValues(id)
			kv.Key.Options = []*av.SelectOption{{Name: "Old", Color: "2", Desc: "old desc"}, {Name: "Other", Color: "4", Desc: "other desc"}}
			kv.Values[0].MSelect = []*av.ValueSelect{{Content: "Old", Color: "2"}, {Content: "Other", Color: "4"}}
			kv.Values[1].MSelect = []*av.ValueSelect{{Content: "Old", Color: "2"}}
			for _, filter := range []*av.ViewFilter{
				{Column: id, Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: kv.Key.Type, MSelect: []*av.ValueSelect{{Content: "Old", Color: "2"}}}},
				{Column: id, Operator: av.FilterOperatorIsNotEqual, Value: &av.Value{Type: kv.Key.Type, MSelect: []*av.ValueSelect{{Content: "Old", Color: "2"}, {Content: "Other", Color: "4"}}}},
				{Column: id, Operator: av.FilterOperatorIsEmpty, Value: &av.Value{Type: kv.Key.Type, MSelect: []*av.ValueSelect{{Content: "Old", Color: "2"}}}},
				{Column: id, Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "Old"}}},
			} {
				a.Views[0].Filters = append(a.Views[0].Filters, filter)
			}
		}
		a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: "multi"}})
		raw, e = json.Marshal(a.Views[0])
		if e != nil {
			t.Fatal(e)
		}
		v := &av.View{}
		if e = json.Unmarshal(raw, v); e != nil {
			t.Fatal(e)
		}
		v.ID = "multi-view"
		v.Group.Field = "multi"
		a.Views = append(a.Views, v)
		raw, e = json.Marshal(a)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return m, store, root
}

func optionHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *cellHarness {
	h := newCellHarness(t, l, root, page, failure)
	h.env.avOptions = &attributeViewOptionEnvironment{read: h.cell.read, stage: h.cell.stage, groups: h.cell.groups}
	// Option management must not require the unrelated cell/relationship services.
	h.env.avCell = nil
	return h
}
func optionOp(action, key string, data interface{}) *Operation {
	return &Operation{Action: action, AvID: cellAV, ID: key, Data: data}
}
func renameOption(old, new, color, desc string) map[string]interface{} {
	return map[string]interface{}{"oldName": old, "newName": new, "newColor": color, "newDesc": desc}
}
func optionNames(k *av.Key) []string {
	var s []string
	for _, o := range k.Options {
		s = append(s, o.Name)
	}
	return s
}
func optionValues(v *av.Value) []string {
	var s []string
	for _, o := range v.MSelect {
		s = append(s, o.Content)
	}
	return s
}
func filtersFor(v *av.View, key string) []*av.ViewFilter {
	var ret []*av.ViewFilter
	for _, f := range v.Filters {
		if f.Column == key {
			ret = append(ret, f)
		}
	}
	return ret
}

func TestOriginalAVOptionContextRulesValuesFiltersGroupsAndOwners(t *testing.T) {
	m, store, root := optionSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, e := m.Acquire(context.Background(), store, page)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(l.Release)
			for _, key := range []string{"status", "multi"} {
				for _, scenario := range []string{"reorder", "update", "add", "duplicate-new", "empty", "null", "blank", "remove", "remove-missing", "rename", "recolor", "merge", "rename-missing", "description", "description-missing"} {
					t.Run(key+"/"+scenario, func(t *testing.T) {
						before := cellDigest(t, root, page)
						h := optionHarness(t, l, root, page, "")
						a, e := h.cell.read(cellAV)
						if e != nil {
							t.Fatal(e)
						}
						kv, _ := a.GetKeyValues(key)
						other := "multi"
						if key == "multi" {
							other = "status"
						}
						otherKV, _ := a.GetKeyValues(other)
						otherRaw, _ := json.Marshal(otherKV)
						otherFilters := map[string]string{}
						for _, v := range a.Views {
							b, _ := json.Marshal(filtersFor(v, other))
							otherFilters[v.ID] = string(b)
						}
						action := "updateAttrViewColOptions"
						var data interface{}
						switch scenario {
						case "reorder":
							data = []*av.SelectOption{{Name: "Other", Color: "4", Desc: "other desc"}, {Name: "Old", Color: "2", Desc: "old desc"}}
						case "update":
							data = []interface{}{map[string]interface{}{"name": "Old", "color": "8", "desc": "updated"}, map[string]interface{}{"name": "Other", "color": "4", "desc": "other desc"}}
						case "add":
							data = []*av.SelectOption{{Name: "New", Color: "9", Desc: "new"}, {Name: "Old", Color: "2", Desc: "old desc"}, {Name: "", Color: "1"}}
						case "duplicate-new":
							data = []*av.SelectOption{{Name: "New", Color: "5"}, {Name: "New", Color: "6"}}
						case "empty":
							data = []interface{}{}
						case "null":
							data = nil
						case "blank":
							data = []interface{}{map[string]interface{}{"name": ""}}
						case "remove", "remove-missing":
							action = "removeAttrViewColOption"
							data = "Old"
							if scenario == "remove-missing" {
								data = "missing"
							}
						case "rename":
							action = "updateAttrViewColOption"
							data = renameOption(" Old ", " New ", "8", " new desc ")
						case "recolor":
							action = "updateAttrViewColOption"
							data = renameOption("Old", "Old", "8", " revised ")
						case "merge":
							action = "updateAttrViewColOption"
							data = renameOption("Old", "Other", "99", "ignored")
						case "rename-missing":
							action = "updateAttrViewColOption"
							data = renameOption("missing", "New", "8", "desc")
						case "description", "description-missing":
							action = "setAttrViewColOptionDesc"
							name := "Old"
							if scenario == "description-missing" {
								name = "missing"
							}
							data = map[string]interface{}{"name": name, "desc": " untrimmed description "}
							h.env.avOptions.groups = nil
						}
						h.tx.DoOperations = []*Operation{optionOp(action, key, data)}
						ret := performTx(h.tx)
						if ret != nil || !h.committed || a.Name != page+" "+cellAV {
							t.Fatal(ret, h.counts)
						}
						early := scenario == "empty" || scenario == "null" || scenario == "blank"
						if early {
							if len(h.staged) != 0 || h.counts["groups:"+cellAV] != 0 {
								t.Fatal("original empty-input early return changed", h.counts)
							}
						} else if h.staged[cellAV] != a {
							t.Fatal("no staged candidate")
						}
						if action == "setAttrViewColOptionDesc" {
							if h.counts["groups:"+cellAV] != 0 {
								t.Fatal("description unexpectedly regenerated groups")
							}
						} else if !early {
							if h.counts["groups:"+cellAV] != 1 || len(a.Views[0].Groups) == 0 || len(a.Views[1].Groups) == 0 {
								t.Fatal("groups not regenerated", h.counts)
							}
						}
						switch scenario {
						case "reorder":
							if !reflect.DeepEqual(optionNames(kv.Key), []string{"Other", "Old"}) {
								t.Fatal(optionNames(kv.Key))
							}
						case "update":
							if kv.Key.GetOption("Old").Color != "8" || kv.Key.GetOption("Old").Desc != "updated" || kv.Values[0].MSelect[0].Color != "2" {
								t.Fatal("original metadata-only update changed", kv)
							}
						case "add":
							if !reflect.DeepEqual(optionNames(kv.Key), []string{"Old", "Other", "New"}) {
								t.Fatal("new option must not reorder old options", kv.Key.Options)
							}
						case "duplicate-new":
							if !reflect.DeepEqual(optionNames(kv.Key), []string{"Old", "Other", "New", "New"}) {
								t.Fatal("original duplicate-input behavior changed")
							}
						case "remove":
							if kv.Key.GetOption("Old") != nil || !reflect.DeepEqual(optionValues(kv.Values[0]), []string{"Other"}) || len(kv.Values[1].MSelect) != 0 {
								t.Fatal("delete did not update cells", kv)
							}
							for _, v := range a.Views {
								f := filtersFor(v, key)
								if len(f) != 3 || len(f[0].Value.MSelect) != 1 || f[0].Value.MSelect[0].Content != "Other" || f[1].Operator != av.FilterOperatorIsEmpty || f[1].Value.MSelect[0].Content != "Old" || f[2].Value.Text.Content != "Old" {
									t.Fatal("delete filter/empty-operator rules", f)
								}
							}
						case "rename", "recolor":
							name, desc := "New", "new desc"
							if scenario == "recolor" {
								name = "Old"
								desc = "revised"
							}
							if kv.Key.GetOption(name) == nil || kv.Key.GetOption(name).Desc != desc || kv.Values[0].MSelect[0].Content != name || kv.Values[0].MSelect[0].Color != "8" {
								t.Fatal("rename/recolor", kv)
							}
							for _, v := range a.Views {
								f := filtersFor(v, key)
								if len(f) != 4 || f[0].Value.MSelect[0].Content != name || f[0].Value.MSelect[0].Color != "8" || f[2].Value.MSelect[0].Content != name || f[3].Value.Text.Content != "Old" {
									t.Fatal("rename filter propagation", f)
								}
							}
						case "merge":
							if !reflect.DeepEqual(optionValues(kv.Values[0]), []string{"Other"}) || !reflect.DeepEqual(optionValues(kv.Values[1]), []string{"Other"}) || kv.Values[1].MSelect[0].Color != "4" || kv.Key.GetOption("Old") == nil || kv.Key.GetOption("Other").Desc != "other desc" {
								t.Fatal("original merge/existing-target rules", kv)
							}
							for _, v := range a.Views {
								f := filtersFor(v, key)
								if f[0].Value.MSelect[0].Color != "4" || f[1].Value.MSelect[0].Content != "Other" || f[1].Value.MSelect[1].Content != "Other" {
									t.Fatal("original filter duplicate retention changed", f)
								}
							}
						case "description":
							if kv.Key.GetOption("Old").Desc != " untrimmed description " || kv.Values[0].MSelect[0].Color != "2" {
								t.Fatal("description trimming/value rule changed")
							}
						}
						otherAfter, _ := json.Marshal(otherKV)
						if string(otherAfter) != string(otherRaw) {
							t.Fatal("unrelated field changed")
						}
						for _, v := range a.Views {
							b, _ := json.Marshal(filtersFor(v, other))
							if string(b) != otherFilters[v.ID] {
								t.Fatal("unrelated filter changed")
							}
						}
						if h.counts["related"] != 0 || h.counts["reload"] != 0 || h.counts["tree-stage"] != 0 {
							t.Fatal("unexpected new side effects", h.counts)
						}
						if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
							t.Fatal("source changed")
						}
					})
				}
			}
		})
	}
}
