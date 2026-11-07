package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
)

func fieldSource(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	m, store, root := cellSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		for _, id := range []string{cellAV, cellDest, cellRelated} {
			file := filepath.Join(root, "pages", page, "data", "storage", "av", id+".json")
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			a := &av.AttributeView{}
			if err = json.Unmarshal(raw, a); err != nil {
				t.Fatal(err)
			}
			a.Views = nil
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				v := &av.View{ID: string(layout), LayoutType: layout, PageSize: 16, Group: &av.ViewGroup{Field: "status"}, GroupCreated: 123, Groups: []*av.View{{ID: "stale-group"}}}
				switch layout {
				case av.LayoutTypeTable:
					v.Table = av.NewLayoutTable()
					v.Table.WrapField = true
				case av.LayoutTypeGallery:
					v.Gallery = av.NewLayoutGallery()
					v.Gallery.WrapField = true
				case av.LayoutTypeKanban:
					v.Kanban = av.NewLayoutKanban()
					v.Kanban.WrapField = true
				}
				for _, kv := range a.KeyValues {
					addFieldFixture(v, kv.Key.ID)
				}
				a.Views = append(a.Views, v)
			}
			a.ViewID = string(av.LayoutTypeTable)
			if id != cellAV {
				for _, v := range a.Views {
					v.Group = nil
					v.Groups = nil
				}
			}
			if id == cellRelated {
				a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "roll", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: "relation", KeyID: "status", Calc: &av.RollupCalc{Operator: av.CalcOperatorSum}}}, Values: []*av.Value{{ID: "roll-value", Type: av.KeyTypeRollup, Rollup: &av.ValueRollup{Contents: []*av.Value{{Type: av.KeyTypeText, Text: &av.ValueText{Content: "cached"}}}}}}})
			}
			raw, err = json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		raw, err := msgpack.Marshal(map[string][]string{cellAV: {cellDest, cellRelated}, cellDest: {cellAV}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "pages", page, "data", "storage", "av", "relations.msgpack"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return m, store, root
}

func addFieldFixture(v *av.View, id string) {
	f := &av.BaseField{ID: id}
	switch v.LayoutType {
	case av.LayoutTypeTable:
		v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: f})
	case av.LayoutTypeGallery:
		v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: f})
	case av.LayoutTypeKanban:
		v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: f})
	}
}
func fieldIDs(v *av.View) []string {
	var ret []string
	if v.Table != nil {
		for _, f := range v.Table.Columns {
			ret = append(ret, f.ID)
		}
	}
	if v.Gallery != nil {
		for _, f := range v.Gallery.CardFields {
			ret = append(ret, f.ID)
		}
	}
	if v.Kanban != nil {
		for _, f := range v.Kanban.Fields {
			ret = append(ret, f.ID)
		}
	}
	return ret
}

func fieldHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *cellHarness {
	h := newCellHarness(t, l, root, page, failure)
	raw, err := os.ReadFile(filepath.Join(root, "pages", page, "data", "storage", "av", "relations.msgpack"))
	if err != nil {
		t.Fatal(err)
	}
	relations := map[string][]string{}
	if err = msgpack.Unmarshal(raw, &relations); err != nil {
		t.Fatal(err)
	}
	h.env.avFields = &attributeViewFieldEnvironment{read: h.cell.read, stage: h.cell.stage, groups: h.cell.groups,
		sources: func(id string) ([]string, error) {
			if err := h.mark("sources"); err != nil {
				return nil, err
			}
			return slices.Clone(relations[id]), nil
		},
		removeRelation: func(src, dst string) error {
			if err := h.mark("relation:" + src + ":" + dst); err != nil {
				return err
			}
			relations[dst] = slices.DeleteFunc(relations[dst], func(s string) bool { return s == src })
			h.events = append(h.events, "relation:"+src+":"+dst)
			return nil
		},
		reload: func(id string) error {
			if err := h.mark("reload:" + id); err != nil {
				return err
			}
			h.events = append(h.events, "reload:"+id)
			return nil
		}}
	h.env.avCell = nil // Field operations do not depend on cell binding services.
	return h
}
func fieldOp(action, key, typ string) *Operation {
	return &Operation{Action: action, AvID: cellAV, ID: key, Typ: typ, Name: "  Edited  "}
}
func assertFieldSourceUnchanged(t *testing.T, h *cellHarness, before map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(before, cellDigest(t, h.root, h.page)) {
		t.Fatal("field candidate wrote source")
	}
	if h.tx.m != nil {
		if !h.tx.m.TryLock() {
			t.Fatal("transaction lock leaked")
		}
		h.tx.m.Unlock()
	}
}

func TestOriginalAVFieldAddTypesLayoutsAndOwners(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, typ := range []string{"text", "number", "date", "select", "mSelect", "url", "email", "phone", "mAsset", "template", "created", "updated", "checkbox", "relation", "rollup", "lineNumber", "block", "unknown"} {
					for _, prev := range []string{"", "status", "missing"} {
						t.Run(string(layout)+"/"+typ+"/"+prev, func(t *testing.T) {
							h := fieldHarness(t, l, root, page, "")
							before := cellDigest(t, root, page)
							a, err := h.cell.read(cellAV)
							if err != nil {
								t.Fatal(err)
							}
							a.ViewID = string(layout)
							old := map[string][]string{}
							for _, v := range a.Views {
								old[v.ID] = fieldIDs(v)
							}
							op := fieldOp("addAttrViewCol", "added", typ)
							op.PreviousID = prev
							op.Data = "icon"
							// Addition needs neither groups, relations, reload nor source lookup.
							h.env.avFields.groups = nil
							h.env.avFields.sources = nil
							h.env.avFields.reload = nil
							h.env.avFields.removeRelation = nil
							h.tx.DoOperations = []*Operation{op}
							if ret := performTx(h.tx); ret != nil || !h.committed {
								t.Fatal(ret, h.counts)
							}
							key, _ := a.GetKey("added")
							supported := typ != "block" && typ != "unknown"
							if (key != nil) != supported {
								t.Fatal("type admission changed", typ, key)
							}
							if supported {
								if key.Name != "  Edited  " || key.Icon != "icon" || string(key.Type) != typ {
									t.Fatal("original NewKey metadata changed", key)
								}
								if typ == "rollup" && (key.Rollup == nil || key.Rollup.Calc.Operator != av.CalcOperatorNone) {
									t.Fatal("rollup default")
								}
							}
							for _, v := range a.Views {
								want := slices.Clone(old[v.ID])
								if supported {
									at := len(want)
									if prev == "status" {
										at = slices.Index(want, prev) + 1
									} else if prev == "" && v.LayoutType == av.LayoutTypeTable && layout == av.LayoutTypeTable {
										at = 0
									}
									want = slices.Insert(want, at, "added")
								}
								if !reflect.DeepEqual(want, fieldIDs(v)) {
									t.Fatal("layout insertion changed", v.ID, want, fieldIDs(v))
								}
								if supported {
									switch v.LayoutType {
									case av.LayoutTypeTable:
										for _, f := range v.Table.Columns {
											if f.ID == "added" && !f.Wrap {
												t.Fatal("table wrap")
											}
										}
									case av.LayoutTypeGallery:
										for _, f := range v.Gallery.CardFields {
											if f.ID == "added" && !f.Wrap {
												t.Fatal("gallery wrap")
											}
										}
									case av.LayoutTypeKanban:
										for _, f := range v.Kanban.Fields {
											if f.ID == "added" && !f.Wrap {
												t.Fatal("kanban wrap")
											}
										}
									}
								}
							}
							if !strings.HasPrefix(a.Name, page) || h.counts["sources"] != 0 || h.counts["stage:"+cellAV] != 1 {
								t.Fatal("owner/effect mismatch", h.counts)
							}
							assertFieldSourceUnchanged(t, h, before)
						})
					}
				}
			}
		})
	}
}

func TestOriginalAVFieldUpdateDeleteGroupsRollupsAndOwners(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, scenario := range []string{"rename", "change-type", "delete", "missing-update", "missing-delete", "unknown-type"} {
				t.Run(scenario, func(t *testing.T) {
					h := fieldHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					a, _ := h.cell.read(cellAV)
					related, _ := h.cell.read(cellRelated)
					kv, _ := a.GetKeyValues("status")
					oldSelect, _ := json.Marshal(kv.Values[0].MSelect)
					op := fieldOp("updateAttrViewCol", "status", "select")
					if scenario == "change-type" {
						op.Typ = "number"
					}
					if scenario == "delete" || scenario == "missing-delete" {
						op.Action = "removeAttrViewCol"
					}
					if strings.HasPrefix(scenario, "missing") {
						op.ID = "missing"
					}
					if scenario == "unknown-type" {
						op.Typ = "unknown"
					}
					h.tx.DoOperations = []*Operation{op}
					if ret := performTx(h.tx); ret != nil || !h.committed {
						t.Fatal(ret, h.counts)
					}
					key, _ := a.GetKey("status")
					changed := scenario == "change-type"
					deleted := scenario == "delete"
					if deleted {
						if key != nil {
							t.Fatal("field not deleted")
						}
					} else if scenario == "rename" || changed {
						if key.Name != "Edited" {
							t.Fatal("name trim changed")
						}
						want := av.KeyTypeSelect
						if changed {
							want = av.KeyTypeNumber
						}
						for _, v := range kv.Values {
							if v.Type != want {
								t.Fatal("value type not updated")
							}
						}
						nowSelect, _ := json.Marshal(kv.Values[0].MSelect)
						if string(oldSelect) != string(nowSelect) {
							t.Fatal("old payload converted/destructively cleared")
						}
					}
					for _, v := range a.Views {
						if changed || deleted {
							if v.Group != nil || v.Groups != nil || v.GroupCreated != 0 {
								t.Fatal("group not cleared", v)
							}
						} else if v.Group == nil {
							t.Fatal("unchanged group removed")
						}
						if slices.Contains(fieldIDs(v), "status") == deleted {
							t.Fatal("layout field removal mismatch", v.ID)
						}
					}
					r, _ := related.GetKeyValues("roll")
					refresh := changed || op.Action == "removeAttrViewCol"
					if changed || deleted {
						if len(r.Values[0].Rollup.Contents) != 0 {
							t.Fatal("rollup not invalidated")
						}
					}
					wantCalc := av.CalcOperatorSum
					if changed {
						wantCalc = av.CalcOperatorNone
					}
					if r.Key.Rollup.Calc.Operator != wantCalc {
						t.Fatal("original change/delete calc distinction lost")
					}
					if refresh {
						if h.counts["sources"] != 1 || h.counts["groups:"+cellRelated] != 1 || h.counts["reload:"+cellRelated] != 1 {
							t.Fatal("related refresh missing", h.counts)
						}
					} else if h.counts["sources"] != 0 {
						t.Fatal("rename/no-op refreshed related views")
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
		})
	}
}

func TestOriginalAVFieldRelationDeleteReverseSelfAndRemainingEdges(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, extra := range []bool{false, true} {
				t.Run(map[bool]string{false: "one-way-sole", true: "one-way-extra"}[extra], func(t *testing.T) {
					h := fieldHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					a, _ := h.cell.read(cellAV)
					k, _ := a.GetKey("relation")
					k.Relation.IsTwoWay = false
					k.Relation.BackKeyID = ""
					if extra {
						a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "extra-forward", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: cellDest}}})
					}
					h.tx.DoOperations = []*Operation{fieldOp("removeAttrViewCol", "relation", "")}
					if ret := performTx(h.tx); ret != nil || !h.committed {
						t.Fatal(ret)
					}
					if (h.counts["relation:"+cellAV+":"+cellDest] > 0) == extra {
						t.Fatal("one-way registry edge mismatch", h.counts)
					}
					if h.counts["read:"+cellDest] != 1 || h.counts["reload:"+cellDest] != 1 {
						t.Fatal("one-way target should only participate as a related source", h.counts)
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
			for _, self := range []bool{false, true} {
				for _, remove := range []bool{false, true} {
					for _, extra := range []bool{false, true} {
						name := strings.Join([]string{map[bool]string{false: "other", true: "self"}[self], map[bool]string{false: "retain", true: "remove"}[remove], map[bool]string{false: "sole", true: "extra"}[extra]}, "/")
						t.Run(name, func(t *testing.T) {
							h := fieldHarness(t, l, root, page, "")
							before := cellDigest(t, root, page)
							a, _ := h.cell.read(cellAV)
							dest, _ := h.cell.read(cellDest)
							if self {
								dest = a
							}
							src, _ := a.GetKey("relation")
							src.Relation.AvID = dest.ID
							back, _ := dest.GetKey("back")
							back.Relation.AvID = a.ID
							back.Relation.IsTwoWay = true
							back.Relation.BackKeyID = "relation"
							for _, v := range dest.Views {
								v.Group = &av.ViewGroup{Field: "back"}
								v.GroupCreated = 123
								v.Groups = []*av.View{{ID: "stale"}}
							}
							if extra {
								// Put another relation immediately after back: deleting while ranging
								// must not skip it or drop its still-live registry edge.
								i := 0
								for j, kv := range dest.KeyValues {
									if kv.Key.ID == "back" {
										i = j + 1
										break
									}
								}
								dest.KeyValues = slices.Insert(dest.KeyValues, i, &av.KeyValues{Key: &av.Key{ID: "extra", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: a.ID}}})
							}
							op := fieldOp("removeAttrViewCol", "relation", "")
							op.RemoveDest = remove
							h.tx.DoOperations = []*Operation{op}
							if ret := performTx(h.tx); ret != nil || !h.committed {
								t.Fatal(ret, h.counts)
							}
							if k, _ := a.GetKey("relation"); k != nil {
								t.Fatal("source relation remains")
							}
							bk, _ := dest.GetKey("back")
							if remove {
								if bk != nil {
									t.Fatal("back field remains")
								}
							} else if bk == nil || bk.Relation.IsTwoWay || bk.Relation.BackKeyID != "" {
								t.Fatal("back field did not become one-way")
							}
							for _, v := range dest.Views {
								if slices.Contains(fieldIDs(v), "back") == remove {
									t.Fatal("back layout mismatch")
								}
								if remove && v.Group != nil {
									t.Fatal("back group retained")
								}
							}
							removed := h.counts["relation:"+dest.ID+":"+a.ID] > 0
							if removed != (remove && !extra) {
								t.Fatal("live back/adjacent edge lost", h.counts)
							}
							if !self && (h.counts["stage:"+dest.ID] == 0 || h.counts["reload:"+dest.ID] == 0) {
								t.Fatal("target not staged/reloaded")
							}
							assertFieldSourceUnchanged(t, h, before)
						})
					}
				}
			}
		})
	}
}
