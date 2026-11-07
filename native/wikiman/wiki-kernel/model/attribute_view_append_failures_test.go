package model

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/av"
)

func TestAVAppendFailuresCancellationAndDependencies(t *testing.T) {
	m, store, root := managementSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, point := range []string{"read:" + cellAV, "append-clone", "append-id", "append-clock", "groups:" + cellAV, "stage:" + cellAV, "append-reload"} {
				for _, prefix := range []string{"", "cancel-"} {
					if point == "append-clock" && prefix == "" {
						continue
					}
					t.Run(prefix+point, func(t *testing.T) {
						h := appendHarness(t, l, root, page, "", av.LayoutTypeTable)
						a, _, _, _ := orderGroup(t, h.cellHarness, av.LayoutTypeTable, "status")
						before := cellDigest(t, root, page)
						reg := managementRegistry(t, h)
						rows := appendValues(page)
						input, _ := json.Marshal(rows)
						h.failure = prefix + point
						h.counts = map[string]int{}
						err := h.tx.appendAttributeViewDetachedBlocksWithValues(cellAV, rows)
						if err == nil || h.counts[point] == 0 {
							t.Fatal("append failure escaped", err, h.counts)
						}
						if point != "append-reload" && h.counts["append-reload"] != 0 {
							t.Fatal("failed append reached reload")
						}
						if point == "groups:"+cellAV && h.counts["stage:"+cellAV] != 0 {
							t.Fatal("failed group reached stage")
						}
						raw, _ := json.Marshal(rows)
						if string(raw) != string(input) {
							t.Fatal("failed append mutated input")
						}
						assertManagementSource(t, h, before, reg)
						_ = a
					})
				}
			}
			for _, missing := range []string{"environment", "ctx", "read", "stage", "groups", "reload", "clock", "id", "clone"} {
				t.Run("missing-"+missing, func(t *testing.T) {
					h := appendHarness(t, l, root, page, "", av.LayoutTypeTable)
					managementPrepare(t, h, av.LayoutTypeTable)
					before := cellDigest(t, root, page)
					e := h.env.avAppend
					switch missing {
					case "environment":
						h.env.avAppend = nil
					case "ctx":
						e.ctx = nil
					case "read":
						e.read = nil
					case "stage":
						e.stage = nil
					case "groups":
						e.groups = nil
					case "reload":
						e.reload = nil
					case "clock":
						e.now = nil
					case "id":
						e.newID = nil
					case "clone":
						e.cloneValue = nil
					}
					h.counts = map[string]int{}
					var err error
					if missing == "ctx" {
						err = appendAttributeViewDetachedBlocksWithEnvironment(cellAV, appendValues(page), e)
					} else {
						err = h.tx.appendAttributeViewDetachedBlocksWithValues(cellAV, appendValues(page))
					}
					if err == nil || len(h.counts) != 0 {
						t.Fatal("missing append dependency reached effects", err, h.counts)
					}
					assertFieldSourceUnchanged(t, h.cellHarness, before)
				})
			}
		})
	}
}

func TestAVAppendInvalidInputsAndCandidateShapes(t *testing.T) {
	m, store, root := managementSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty-av-id", "nil-value", "missing-key-last", "nil-block", "nil-option", "nan-payload", "nil-clone", "aliased-clone", "wrong-clone-key", "clone-late-cancel", "stage-late-cancel", "empty-id", "reused-id", "existing-id", "nil-source", "wrong-source", "nil-kv", "nil-key", "nil-source-value", "nil-source-option", "no-primary", "nil-source-block", "nil-view", "nil-layout", "nil-base", "nil-group", "unknown-layout", "scope-cancel", "unregistered-action"} {
		t.Run(scenario, func(t *testing.T) {
			h := appendHarness(t, l, root, page, "", av.LayoutTypeTable)
			a := managementPrepare(t, h, av.LayoutTypeTable)
			before := cellDigest(t, root, page)
			rows := appendValues(page)
			e := h.env.avAppend
			id := cellAV
			switch scenario {
			case "empty-av-id":
				id = ""
			case "nil-value":
				rows[1] = append(rows[1], nil)
			case "missing-key-last":
				rows[1] = append(rows[1], &av.Value{KeyID: "not-present"})
			case "nil-block":
				rows[0][0].Block = nil
			case "nil-option":
				rows[1][1].MSelect = append(rows[1][1].MSelect, nil)
			case "nan-payload":
				rows[0][2].Number.Content = math.NaN()
			case "nil-clone":
				e.cloneValue = func(*av.Value) (*av.Value, error) { return nil, nil }
			case "aliased-clone":
				e.cloneValue = func(v *av.Value) (*av.Value, error) { return v, nil }
			case "wrong-clone-key":
				e.cloneValue = func(v *av.Value) (*av.Value, error) { copy := v.Clone(); copy.KeyID = "wrong"; return copy, nil }
			case "clone-late-cancel":
				clone := e.cloneValue
				e.cloneValue = func(v *av.Value) (*av.Value, error) { copy, err := clone(v); h.cancel(); return copy, err }
			case "stage-late-cancel":
				stage := e.stage
				e.stage = func(a *av.AttributeView) error {
					if err := stage(a); err != nil {
						return err
					}
					h.cancel()
					return nil
				}
			case "empty-id":
				e.newID = func() string { return "" }
			case "reused-id":
				e.newID = func() string { return "same-id" }
			case "existing-id":
				e.newID = func() string { return settingsBlock }
			case "nil-source":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "wrong-source":
				a.ID = cellDest
			case "nil-kv":
				a.KeyValues = append(a.KeyValues, nil)
			case "nil-key":
				a.KeyValues[0].Key = nil
			case "nil-source-value":
				a.KeyValues[0].Values = append(a.KeyValues[0].Values, nil)
			case "nil-source-option":
				a.KeyValues[4].Key.Options = append(a.KeyValues[4].Key.Options, nil)
			case "no-primary":
				a.KeyValues[0].Key.Type = av.KeyTypeText
			case "nil-source-block":
				a.KeyValues[0].Values[0].Block = nil
			case "nil-view":
				a.Views = append(a.Views, nil)
			case "nil-layout":
				a.Views[0].Table = nil
			case "nil-base":
				a.Views[0].Table.BaseLayout = nil
			case "nil-group":
				a.Views[0].Groups = append(a.Views[0].Groups, nil)
			case "unknown-layout":
				a.Views[0].LayoutType = "unknown"
			case "scope-cancel":
				h.env.ctx = cancelledAppendContext()
			case "unregistered-action":
				h.tx.DoOperations = []*Operation{{Action: "appendAttributeViewDetachedBlocksWithValues", AvID: cellAV}}
			}
			input, _ := json.Marshal(rows)
			candidate, _ := json.Marshal(a)
			h.counts = map[string]int{}
			if scenario == "unregistered-action" {
				if ret := performTx(h.tx); ret == nil || !strings.Contains(ret.msg, "not_contextualized") {
					t.Fatal("standalone append became a transaction action", ret)
				}
			} else if err := h.tx.appendAttributeViewDetachedBlocksWithValues(id, rows); err == nil {
				t.Fatal("invalid append succeeded", h.counts)
			}
			raw, _ := json.Marshal(rows)
			if !reflect.DeepEqual(raw, input) {
				t.Fatal("bad input changed")
			}
			if scenario == "missing-key-last" || scenario == "nil-block" || scenario == "nil-option" || scenario == "nil-value" {
				after, _ := json.Marshal(a)
				if string(after) != string(candidate) || h.counts["append-clone"] != 0 || h.counts["append-id"] != 0 {
					t.Fatal("bad last row partially applied", h.counts)
				}
			}
			if scenario != "stage-late-cancel" && h.counts["stage:"+cellAV] != 0 {
				t.Fatal("bad input staged", h.counts)
			}
			if h.counts["append-reload"] != 0 {
				t.Fatal("bad input reloaded")
			}
			assertFieldSourceUnchanged(t, h.cellHarness, before)
		})
	}
}
func cancelledAppendContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestAVAppendStandaloneLegacyFactory(t *testing.T) {
	e := (*Transaction)(nil).attributeViewAppendEnvironment()
	if err := e.validate(); err != nil {
		t.Fatal(err)
	}
	// The original exported entry shares the same core; no global source is touched.
	if err := AppendAttributeViewDetachedBlocksWithValues("", nil); err == nil {
		t.Fatal("empty identity accepted")
	}
}
