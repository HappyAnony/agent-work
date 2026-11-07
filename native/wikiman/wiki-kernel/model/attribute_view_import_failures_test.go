package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
)

func TestAVImportFailuresCancellationAndNoLaterPhases(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"import-registry", "read:" + cellAV, "read:" + cellDest, "exists", "duplicate-node", "load", "name", "duplicate-attrs", "name-cache", "name-notify", "duplicate-tree", "tree-stage", "import-relation"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newAVImportHarness(t, l, root, page, prefix+point)
				before := nameDigest(t, root, page)
				err := h.tx.finishImportedAttributeViews([]string{cellAV, cellDest}, avImportedTrees())
				if err == nil || h.counts[point] == 0 || h.counts["finish"] != 0 || h.committed {
					t.Fatal("failure ignored", err, h.counts)
				}
				if prefix != "" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if point != "import-relation" && len(h.links) != 0 {
					t.Fatal("failed binding started relation phase")
				}
				if point == "import-registry" && h.counts["read:"+cellAV] != 0 {
					t.Fatal("failed registry continued")
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
	for _, stage := range []string{"registry-late", "tree-late", "relation-late"} {
		t.Run(stage, func(t *testing.T) {
			h := newAVImportHarness(t, l, root, page, "")
			e := h.env.avImport
			switch stage {
			case "registry-late":
				fn := e.registry
				e.registry = func(n []*ast.Node) error { err := fn(n); h.cancel(); return err }
			case "tree-late":
				fn := e.bindings.tree
				e.bindings.tree = func(t *parse.Tree) error { err := fn(t); h.cancel(); return err }
			case "relation-late":
				fn := e.relation
				e.relation = func(a, b string) error { err := fn(a, b); h.cancel(); return err }
			}
			if err := h.tx.finishImportedAttributeViews([]string{cellAV}, avImportedTrees()); !errors.Is(err, context.Canceled) {
				t.Fatal("late cancellation lost", err)
			}
		})
	}
}

func TestAVImportSourceValidationAndLegacyContract(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty-id", "nil-source", "wrong-source", "nil-key", "nil-primary-block", "no-primary", "nil-tree", "nil-root", "empty-tree-id", "conflicting-binding-tree", "missing-node", "second-read-error", "cancelled-owner", "legacy-flag-no-fallback"} {
		t.Run(scenario, func(t *testing.T) {
			h := newAVImportHarness(t, l, root, page, "")
			a, err := h.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			e := h.env.avImport
			ids := []string{cellAV}
			trees := avImportedTrees()
			switch scenario {
			case "empty-id":
				ids = []string{""}
			case "nil-source":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "wrong-source":
				a.ID = cellDest
			case "nil-key":
				a.KeyValues[0].Key = nil
			case "nil-primary-block":
				a.KeyValues[0].Values[0].Block = nil
			case "no-primary":
				a.KeyValues[0].Key.Type = av.KeyTypeText
			case "nil-tree":
				trees = []*parse.Tree{nil}
			case "nil-root":
				trees[0].Root = nil
			case "empty-tree-id":
				trees[0].ID = ""
			case "conflicting-binding-tree":
				fn := e.bindings.node
				count := 0
				e.bindings.node = func(id string) (*ast.Node, *parse.Tree, error) {
					n, t, err := fn(id)
					count++
					if err == nil && count == 2 {
						t.Path = "/different.docj"
					}
					return n, t, err
				}
			case "missing-node":
				e.bindings.node = func(id string) (*ast.Node, *parse.Tree, error) {
					tree, err := h.env.load(settingsBox, "/"+settingsDoc+".docj", h.env.parser())
					return nil, tree, err
				}
			case "second-read-error":
				fn := e.read
				calls := 0
				e.read = func(id string) (*av.AttributeView, error) {
					calls++
					if calls == 2 {
						return nil, errors.New("second phase unavailable")
					}
					return fn(id)
				}
			case "cancelled-owner":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				h.env.ctx = ctx
			case "legacy-flag-no-fallback":
				e.legacy = true
				e.read = func(string) (*av.AttributeView, error) { return nil, errors.New("owner unavailable") }
			}
			before := nameDigest(t, root, page)
			h.counts = map[string]int{}
			err = h.tx.finishImportedAttributeViews(ids, trees)
			if scenario == "missing-node" {
				if err != nil || len(h.trees) != 0 {
					t.Fatal("vanished node not skipped", err)
				}
			} else if err == nil {
				t.Fatal("invalid source accepted", scenario)
			}
			if err != nil && len(h.links) != 0 {
				t.Fatal("invalid input published relation")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
	// Standalone compatibility retains missing-AV and missing-name skips.
	h := newAVImportHarness(t, l, root, page, "")
	e := *h.env.avImport
	e.legacy = true
	e.read = func(string) (*av.AttributeView, error) { return nil, errors.New("legacy missing") }
	if err := finishImportedAttributeViewsWithEnvironment([]string{cellAV}, nil, &e); err != nil {
		t.Fatal("legacy read skip", err)
	}
	if err := legacyAttributeViewImportEnvironment().validate(true); err != nil {
		t.Fatal("legacy adapter", err)
	}
	if err := updateBoundBlockAvsAttributeWithEnvironment(nil, &e); err != nil {
		t.Fatal("empty legacy helper", err)
	}
}

func TestAVImportDependencyAdmissionAndNoGlobalFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, dep := range []string{"environment", "ctx", "read", "bindings", "exists", "node", "name", "untitled", "attrs", "cache", "notify", "tree", "registry", "relation"} {
		t.Run(dep, func(t *testing.T) {
			h := newAVImportHarness(t, l, root, page, "")
			e := h.env.avImport
			switch dep {
			case "environment":
				h.env.avImport = nil
			case "ctx":
				h.env.ctx = nil
			case "read":
				e.read = nil
			case "bindings":
				e.bindings = nil
			case "exists":
				e.bindings.exists = nil
			case "node":
				e.bindings.node = nil
			case "name":
				e.bindings.name = nil
			case "untitled":
				e.bindings.untitled = nil
			case "attrs":
				e.bindings.attrs = nil
			case "cache":
				e.bindings.cache = nil
			case "notify":
				e.bindings.notify = nil
			case "tree":
				e.bindings.tree = nil
			case "registry":
				e.registry = nil
			case "relation":
				e.relation = nil
			}
			if err := h.tx.finishImportedAttributeViews([]string{cellAV}, avImportedTrees()); err == nil || len(h.counts) != 0 || len(h.order) != 0 {
				t.Fatal("missing service ran effects", dep, err, h.counts)
			}
		})
	}
	h := newAVImportHarness(t, l, root, page, "")
	h.env.avImport = nil
	if err := h.tx.updateBoundBlockAvsAttribute([]string{cellAV}); err == nil || len(h.counts) != 0 {
		t.Fatal("missing helper adapter fell back")
	}
	h = newAVImportHarness(t, l, root, page, "")
	h.env.avImport.registry = nil
	h.env.avImport.relation = nil
	if err := h.tx.updateBoundBlockAvsAttribute([]string{cellAV}); err != nil || len(h.registered) != 0 || len(h.links) != 0 {
		t.Fatal("helper required or invoked unrelated services", err)
	}
	h = newAVImportHarness(t, l, root, page, "")
	h.tx.DoOperations = []*Operation{{Action: "importAttributeViews", AvID: cellAV}}
	if ret := performTx(h.tx); ret == nil || !strings.Contains(ret.msg, "not_contextualized") || len(h.counts) != 0 {
		t.Fatal("import phase admitted as transaction", ret)
	}
}
