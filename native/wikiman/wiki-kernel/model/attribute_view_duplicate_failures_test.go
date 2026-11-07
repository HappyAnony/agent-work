package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

func TestAVDuplicateInvalidSourcesIDsAndShapes(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty-id", "path", "dot", "nil-source", "wrong-source", "nil-key", "nil-value", "bad-source-block", "no-primary", "bad-json", "null-json", "wrong-raw-id", "nil-raw-key", "bad-raw-block", "empty-generated", "same-generated", "original-generated", "path-generated", "existing-av", "existing-block", "reserved-mirror", "nil-bindings", "nil-ctx", "cancelled-owner"} {
		t.Run(scenario, func(t *testing.T) {
			d := newDuplicateHarness(t, l, root, page, "")
			e := d.env.avDuplicate
			a, err := d.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			id := cellAV
			switch scenario {
			case "empty-id":
				id = ""
			case "path":
				id = "../escape"
			case "dot":
				id = ".."
			case "nil-source":
				e.source = func(string) (*av.AttributeView, []byte, error) { return nil, []byte("{}"), nil }
			case "wrong-source":
				a.ID = cellDest
			case "nil-key":
				a.KeyValues[0].Key = nil
			case "nil-value":
				a.KeyValues[0].Values[0] = nil
			case "bad-source-block":
				a.KeyValues[0].Values[0].Block = nil
			case "no-primary":
				a.KeyValues[0].Key.Type = av.KeyTypeText
			case "bad-json", "null-json", "wrong-raw-id", "nil-raw-key", "bad-raw-block":
				raw := mustDuplicateJSON(t, a)
				switch scenario {
				case "bad-json":
					raw = []byte("{")
				case "null-json":
					raw = []byte("null")
				case "wrong-raw-id":
					raw = []byte(strings.Replace(string(raw), cellAV, cellDest, 1))
				case "nil-raw-key":
					var c av.AttributeView
					json.Unmarshal(raw, &c)
					c.KeyValues[0].Key = nil
					raw = mustDuplicateJSON(t, &c)
				case "bad-raw-block":
					var c av.AttributeView
					json.Unmarshal(raw, &c)
					c.KeyValues[0].Values[0].Block = nil
					raw = mustDuplicateJSON(t, &c)
				}
				e.source = func(string) (*av.AttributeView, []byte, error) { return a, raw, nil }
			case "empty-generated":
				e.newID = func() string { return "" }
			case "same-generated":
				e.newID = func() string { return "same" }
			case "original-generated":
				e.newID = func() string { return cellAV }
			case "path-generated":
				e.newID = func() string { return "../escape" }
			case "existing-av":
				d.cell.read(cellDest)
				e.newID = func() string { return cellDest }
			case "existing-block":
				e.newID = func() string { return settingsBlock }
			case "reserved-mirror":
				d.reg["reserved"] = "mirror"
				e.newID = func() string { return "mirror" }
			case "nil-bindings":
				e.bindings = nil
			case "nil-ctx":
				d.env.ctx = nil
			case "cancelled-owner":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.env.ctx = ctx
			}
			d.counts = map[string]int{}
			before := nameDigest(t, root, page)
			_, _, err = d.tx.duplicateDatabaseBlock(id)
			if err == nil || d.counts["duplicate-registry"] != 0 || d.counts["duplicate-stage"] != 0 || len(d.trees) != 0 || len(d.notifications) != 0 {
				t.Fatal("invalid input produced effects", scenario, err, d.counts)
			}
			assertNameSource(t, d.cellHarness, before)
		})
	}
}

func TestAVDuplicateBindingIdentityAndLateCancellation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"nil-tree", "nil-root", "empty-tree-id", "bad-node-id", "conflicting-tree", "vanished-node", "id-cancel", "source-late-cancel", "stage-late-cancel", "tree-late-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			d := newDuplicateHarness(t, l, root, page, "")
			e := d.env.avDuplicate
			node := e.bindings.node
			switch scenario {
			case "nil-tree", "nil-root", "empty-tree-id", "bad-node-id", "conflicting-tree", "vanished-node":
				calls := 0
				e.bindings.node = func(id string) (*ast.Node, *parse.Tree, error) {
					n, tree, err := node(id)
					if err != nil {
						return n, tree, err
					}
					calls++
					switch scenario {
					case "nil-tree":
						tree = nil
					case "nil-root":
						tree.Root = nil
					case "empty-tree-id":
						tree.ID = ""
					case "bad-node-id":
						n = &ast.Node{ID: "wrong"}
					case "conflicting-tree":
						if calls == 2 {
							tree.Path = "/other.docj"
						}
					case "vanished-node":
						n = nil
					}
					return n, tree, nil
				}
			case "id-cancel":
				e.newID = func() string { d.cancel(); return "cancelled-id" }
			case "source-late-cancel":
				source := e.source
				e.source = func(id string) (*av.AttributeView, []byte, error) {
					a, raw, err := source(id)
					d.cancel()
					return a, raw, err
				}
			case "stage-late-cancel":
				stage := e.stage
				e.stage = func(id string, raw []byte) error { err := stage(id, raw); d.cancel(); return err }
			case "tree-late-cancel":
				stage := e.bindings.tree
				e.bindings.tree = func(tree *parse.Tree) error { err := stage(tree); d.cancel(); return err }
			}
			before := nameDigest(t, root, page)
			_, _, err := d.tx.duplicateDatabaseBlock(cellAV)
			if scenario == "vanished-node" {
				if err != nil || len(d.trees) != 0 || len(d.notifications) != 0 {
					t.Fatal("stale node not skipped", err)
				}
			} else if err == nil {
				t.Fatal("invalid binding/cancellation accepted")
			}
			if strings.Contains(scenario, "cancel") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if scenario == "id-cancel" && d.counts["duplicate-used"] != 0 {
				t.Fatal("cancelled generation still checked identity")
			}
			if strings.Contains(scenario, "cancel") && d.committed {
				t.Fatal("cancelled finish")
			}
			assertNameSource(t, d.cellHarness, before)
		})
	}
	// Ensure actual staged trees retain both generations of binding memberships.
	d := newDuplicateHarness(t, l, root, page, "")
	id, _, err := d.tx.duplicateDatabaseBlock(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := d.tx.duplicateDatabaseBlock(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, tree := range d.trees {
		for _, bid := range []string{settingsBlock, cellSecond, nameBlock} {
			if n := treenode.GetNodeInTree(tree, bid); n != nil {
				members := "," + n.IALAttr(av.NodeAttrNameAvs) + ","
				if !strings.Contains(members, ","+id+",") || !strings.Contains(members, ","+second+",") {
					t.Fatal("sequential candidate lost membership", members)
				}
			}
		}
	}
}
