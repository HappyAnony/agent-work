package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"wiki-kernel/av"
)

func TestAVFieldContextFailuresCancellationAndAdmission(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range []string{"addAttrViewCol", "updateAttrViewCol", "removeAttrViewCol"} {
				points := []string{"read:" + cellAV, "stage:" + cellAV, "finish"}
				if action != "addAttrViewCol" {
					points = append(points, "sources", "read:"+cellRelated, "groups:"+cellRelated, "stage:"+cellRelated, "reload:"+cellRelated)
				}
				if action == "removeAttrViewCol" {
					points = append(points, "read:"+cellDest, "stage:"+cellDest, "reload:"+cellDest, "relation:"+cellAV+":"+cellDest, "relation:"+cellDest+":"+cellAV)
				}
				failures := []string{"pre-cancel", "nil-source", "nil-kv", "nil-key", "nil-value", "nil-view", "nil-layout", "nil-field", "nil-base"}
				if action == "addAttrViewCol" {
					failures = append(failures, "bad-icon", "no-views")
				}
				if action != "addAttrViewCol" {
					failures = append(failures, "nil-rollup", "nil-rollup-value")
				}
				for _, point := range points {
					failures = append(failures, point, "cancel-"+point)
				}
				for _, failure := range failures {
					t.Run(action+"/"+failure, func(t *testing.T) {
						injected := ""
						for _, p := range points {
							if failure == p || failure == "cancel-"+p {
								injected = failure
							}
						}
						h := fieldHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						op := fieldOp(action, "status", "number")
						if action == "addAttrViewCol" {
							op.ID = "added"
						}
						if action == "removeAttrViewCol" {
							op.ID = "relation"
							op.RemoveDest = true
						}
						a, _ := h.cell.read(cellAV)
						rel, _ := h.cell.read(cellRelated)
						switch failure {
						case "pre-cancel":
							h.cancel()
						case "nil-source":
							h.env.avFields.read = func(string) (*av.AttributeView, error) { return nil, nil }
						case "nil-kv":
							a.KeyValues = append(a.KeyValues, nil)
						case "nil-key":
							a.KeyValues[0].Key = nil
						case "nil-value":
							a.KeyValues[0].Values = append(a.KeyValues[0].Values, nil)
						case "nil-view":
							a.Views = append(a.Views, nil)
						case "nil-layout":
							a.Views[0].Table = nil
						case "nil-field":
							a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, nil)
						case "nil-base":
							a.Views[0].Table.Columns[0].BaseField = nil
						case "bad-icon":
							op.Data = 42
						case "no-views":
							a.Views = nil
						case "nil-rollup":
							kv, _ := rel.GetKeyValues("roll")
							kv.Key.Rollup = nil
						case "nil-rollup-value":
							kv, _ := rel.GetKeyValues("roll")
							kv.Values[0].Rollup = nil
							op.ID = "status"
						}
						// Try to inject compatibility's ignore-errors behavior into a scoped
						// transaction: admission/execution must still fail closed.
						h.env.avFields.legacy = true
						h.failure = injected
						h.tx.DoOperations = []*Operation{op}
						ret := performTx(h.tx)
						if ret == nil || h.committed || h.tx.state.Load() == 2 || strings.Contains(ret.msg, "PANIC RECOVERED") {
							t.Fatal("failure swallowed/panic", ret, h.counts)
						}
						if injected != "" && !strings.HasPrefix(injected, "cancel-") && !strings.Contains(ret.msg, "synthetic "+failure) {
							t.Fatal("wrong fault reached", ret, h.counts)
						}
						if failure != "finish" && failure != "cancel-finish" && h.counts["finish"] != 0 {
							t.Fatal("failed candidate finished")
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
				for _, dep := range []string{"environment", "read", "stage", "groups", "sources", "reload", "removeRelation"} {
					if action == "addAttrViewCol" && (dep == "groups" || dep == "sources" || dep == "reload" || dep == "removeRelation") {
						continue
					}
					if action == "updateAttrViewCol" && dep == "removeRelation" {
						continue
					}
					t.Run(action+"/missing-"+dep, func(t *testing.T) {
						h := fieldHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						e := h.env.avFields
						switch dep {
						case "environment":
							h.env.avFields = nil
						case "read":
							e.read = nil
						case "stage":
							e.stage = nil
						case "groups":
							e.groups = nil
						case "sources":
							e.sources = nil
						case "reload":
							e.reload = nil
						case "removeRelation":
							e.removeRelation = nil
						}
						h.tx.DoOperations = []*Operation{fieldOp(action, "status", "number")}
						ret := performTx(h.tx)
						if ret == nil || h.committed || len(h.counts) != 0 || h.tx.m != nil {
							t.Fatal("missing service entered transaction", ret, h.counts)
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
			}
		})
	}
}

func TestAVFieldContextCancellationAfterSuccessfulCallbacks(t *testing.T) {
	m, store, root := fieldSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	for _, point := range []string{"read", "stage", "groups", "sources", "reload", "relation"} {
		t.Run(point, func(t *testing.T) {
			h := fieldHarness(t, l, root, page, "")
			before := cellDigest(t, root, page)
			e := h.env.avFields
			op := fieldOp("updateAttrViewCol", "status", "number")
			switch point {
			case "read":
				fn := e.read
				e.read = func(id string) (*av.AttributeView, error) { a, err := fn(id); h.cancel(); return a, err }
			case "stage":
				fn := e.stage
				e.stage = func(a *av.AttributeView) error { err := fn(a); h.cancel(); return err }
			case "groups":
				fn := e.groups
				e.groups = func(a *av.AttributeView) error { err := fn(a); h.cancel(); return err }
			case "sources":
				fn := e.sources
				e.sources = func(id string) ([]string, error) { ids, err := fn(id); h.cancel(); return ids, err }
			case "reload":
				fn := e.reload
				e.reload = func(id string) error { err := fn(id); h.cancel(); return err }
			case "relation":
				op = fieldOp("removeAttrViewCol", "relation", "")
				op.RemoveDest = true
				fn := e.removeRelation
				e.removeRelation = func(a, b string) error { err := fn(a, b); h.cancel(); return err }
			}
			h.tx.DoOperations = []*Operation{op}
			ret := performTx(h.tx)
			if ret == nil || !strings.Contains(ret.msg, context.Canceled.Error()) || h.committed || h.counts["finish"] != 0 {
				t.Fatal("post-callback cancellation swallowed", ret, h.counts)
			}
			assertFieldSourceUnchanged(t, h, before)
		})
	}
}

func TestAVFieldContextMixedCandidatesAndReadMemo(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, scenario := range []string{"success", "last-stage", "bad-second", "unaudited"} {
				t.Run(scenario, func(t *testing.T) {
					h := fieldHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					h.env.avCell = h.cell
					h.env.avOptions = &attributeViewOptionEnvironment{read: h.cell.read, stage: h.cell.stage, groups: h.cell.groups}
					ops := []*Operation{fieldOp("addAttrViewCol", "added", "select"), fieldOp("updateAttrViewCol", "added", "select"), optionOp("updateAttrViewColOptions", "added", []*av.SelectOption{{Name: "New", Color: "8"}}), cellOp("added", map[string]interface{}{"mSelect": []interface{}{map[string]interface{}{"content": "New", "color": "8"}}})}
					remove := fieldOp("removeAttrViewCol", "added", "")
					ops = append(ops, remove)
					var middle bool
					oldStage := h.env.avFields.stage
					h.env.avFields.stage = func(a *av.AttributeView) error {
						k, _ := a.GetKey("added")
						if k == nil {
							v := a.GetValue("added", settingsBlock)
							if v != nil {
								return errors.New("removed field still readable")
							}
							if scenario == "last-stage" {
								return errors.New("synthetic last stage")
							}
						} else if k.Name == "Edited" {
							middle = true
						}
						return oldStage(a)
					}
					if scenario == "bad-second" {
						ops[1] = fieldOp("addAttrViewCol", "bad", "text")
						ops[1].Data = 42
					}
					if scenario == "unaudited" {
						ops = append(ops, &Operation{Action: "sortAttrViewCol", AvID: cellAV})
					}
					h.tx.DoOperations = ops
					ret := performTx(h.tx)
					if scenario == "success" {
						if ret != nil || !h.committed || !middle {
							t.Fatal("mixed candidate failed", ret, h.counts)
						}
						a := h.staged[cellAV]
						if k, _ := a.GetKey("added"); k != nil {
							t.Fatal("removed candidate field remains")
						}
					} else {
						if ret == nil || h.committed {
							t.Fatal("failed mixed candidate finished", ret, h.counts)
						}
						if (scenario == "bad-second" || scenario == "unaudited") && len(h.counts) != 0 {
							t.Fatal("bad batch partially admitted")
						}
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
			// read() may return fresh objects: source/self-related refresh must reuse
			// the operation candidate, not replace the just-edited AV with old bytes.
			t.Run("self-fresh-loader", func(t *testing.T) {
				h := fieldHarness(t, l, root, page, "")
				before := cellDigest(t, root, page)
				read := h.env.avFields.read
				calls := 0
				h.env.avFields.read = func(id string) (*av.AttributeView, error) {
					calls++
					a, err := read(id)
					if err != nil {
						return nil, err
					}
					b, _ := jsonCloneAV(a)
					return b, nil
				}
				h.env.avFields.sources = func(string) ([]string, error) { return []string{cellAV}, nil }
				h.tx.DoOperations = []*Operation{fieldOp("updateAttrViewCol", "status", "number")}
				if ret := performTx(h.tx); ret != nil || !h.committed {
					t.Fatal(ret)
				}
				k, _ := h.staged[cellAV].GetKey("status")
				if k.Type != av.KeyTypeNumber || calls != 1 {
					t.Fatal("self-read lost edited candidate", k, calls)
				}
				assertFieldSourceUnchanged(t, h, before)
			})
		})
	}
}

func jsonCloneAV(a *av.AttributeView) (*av.AttributeView, error) {
	// Reuse original deep-copy semantics rather than aliasing the source cache.
	b := &av.AttributeView{}
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, b)
	return b, err
}

func TestAVFieldInputGateAndOriginalWrapperActions(t *testing.T) {
	if err := validateAttributeViewFieldInput(nil); err == nil {
		t.Fatal("nil operation admitted")
	}
	if err := validateAttributeViewFieldInput(&Operation{Action: "unknown"}); err == nil {
		t.Fatal("unknown operation admitted")
	}
	if err := runLegacyAttributeViewFieldAction("updateAttrViewCol", nil); err == nil {
		t.Fatal("nil wrapper operation admitted")
	}
	// Admission should be independent of the compatibility flag on a service.
	if !reflect.DeepEqual([]bool{isAttributeViewFieldAction("addAttrViewCol"), isAttributeViewFieldAction("updateAttrViewCol"), isAttributeViewFieldAction("removeAttrViewCol")}, []bool{true, true, true}) {
		t.Fatal("field action classification")
	}
}

func TestAVFieldContextUpdateTypesAndMemo(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, typ := range []string{"block", "text", "number", "date", "select", "mSelect", "url", "email", "phone", "mAsset", "template", "created", "updated", "checkbox", "relation", "rollup", "lineNumber", "unknown"} {
				t.Run(typ, func(t *testing.T) {
					h := fieldHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					a, _ := h.cell.read(cellAV)
					kv, _ := a.GetKeyValues("text")
					a.RenderedViewables = map[string]av.Viewable{"stale": &av.Table{}}
					for _, v := range a.Views {
						v.Group = &av.ViewGroup{Field: "text"}
					}
					// An unconfigured relation/rollup is allowed by the original metadata
					// change; no eager value conversion or invented defaults are added.
					h.env.avFields.sources = func(string) ([]string, error) { return nil, nil }
					h.tx.DoOperations = []*Operation{fieldOp("updateAttrViewCol", "text", typ)}
					if ret := performTx(h.tx); ret != nil || !h.committed {
						t.Fatal(ret, h.counts)
					}
					want := av.KeyType(typ)
					if typ == "unknown" {
						want = av.KeyTypeText
					}
					if kv.Key.Type != want || len(a.RenderedViewables) != 0 {
						t.Fatal("metadata type/memo inheritance", kv.Key.Type, a.RenderedViewables)
					}
					for _, v := range kv.Values {
						if v.Type != want || v.Text == nil || v.Text.Content != "old" {
							t.Fatal("original payload lost", v)
						}
					}
					for _, v := range a.Views {
						if typ != "text" && typ != "unknown" {
							if v.Group != nil {
								t.Fatal("changed type kept group")
							}
						} else if v.Group == nil {
							t.Fatal("unchanged type cleared group")
						}
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
		})
	}
}
