package model

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

const importDoc = "20261026080000-doc0001"
const importMirror = "20261026080001-mir0001"

type avImportHarness struct {
	*duplicateHarness
	registered map[string]string
	links      [][2]string
	order      []string
}

func newAVImportHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *avImportHarness {
	d := newDuplicateHarness(t, l, root, page, failure)
	h := &avImportHarness{duplicateHarness: d, registered: map[string]string{}}
	b := *d.env.avDuplicate.bindings
	tree := b.tree
	b.tree = func(t *parse.Tree) error { h.order = append(h.order, "tree:"+t.ID); return tree(t) }
	d.env.avImport = &attributeViewImportEnvironment{ctx: d.ctx,
		read: func(id string) (*av.AttributeView, error) {
			h.order = append(h.order, "read:"+id)
			return d.cell.read(id)
		}, bindings: &b,
		registry: func(nodes []*ast.Node) error {
			if err := d.mark("import-registry"); err != nil {
				return err
			}
			h.order = append(h.order, "registry")
			for _, n := range nodes {
				h.registered[n.ID] = n.AttributeViewID
			}
			return nil
		},
		relation: func(src, dest string) error {
			if err := d.mark("import-relation"); err != nil {
				return err
			}
			h.order = append(h.order, "relation:"+src)
			h.links = append(h.links, [2]string{src, dest})
			return nil
		},
	}
	return h
}

func avImportedTrees() []*parse.Tree {
	n := &ast.Node{Type: ast.NodeDocument, ID: importDoc}
	n.SetIALAttr("id", importDoc)
	n.AppendChild(&ast.Node{Type: ast.NodeAttributeView, ID: importMirror, AttributeViewID: cellAV})
	return []*parse.Tree{{ID: importDoc, Root: n}} // No Box: an archive is not mounted yet.
}

func TestAVImportOwnersBatchTreesAndProjection(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				t.Run(string(layout), func(t *testing.T) {
					h := newAVImportHarness(t, l, root, page, "")
					for _, id := range []string{cellAV, cellDest} {
						a, err := h.cell.read(id)
						if err != nil {
							t.Fatal(err)
						}
						a.ViewID = string(layout)
						a.GetBlockKeyValues().Values[0].Block.ID = "not-row-id"
					}
					load := h.env.load
					h.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
						tree, err := load(box, p, engine)
						if err == nil {
							for _, id := range []string{settingsBlock, cellSecond, nameBlock} {
								if n := treenode.GetNodeInTree(tree, id); n != nil {
									n.RemoveIALAttr(av.NodeAttrNameAvs)
									n.RemoveIALAttr(av.NodeAttrViewNames)
								}
							}
						}
						return tree, err
					}
					before := nameDigest(t, root, page)
					a := mustDuplicateJSON(t, h.cache[cellAV])
					b := mustDuplicateJSON(t, h.cache[cellDest])
					h.counts = map[string]int{}
					if err := h.tx.finishImportedAttributeViews([]string{cellDest, cellAV}, avImportedTrees()); err != nil {
						t.Fatal(err, h.counts)
					}
					if h.registered[importMirror] != cellAV || len(h.trees) != 2 || h.counts["duplicate-tree"] != 2 || h.counts["import-registry"] != 1 || h.counts["import-relation"] != 2 {
						t.Fatal("post-import effects", h.counts, h.order)
					}
					if !reflect.DeepEqual(h.links, [][2]string{{cellDest, cellAV}, {cellAV, cellAV}}) {
						t.Fatal("original last-field relation projection changed", h.links)
					}
					if h.order[0] != "registry" || !strings.HasPrefix(h.order[len(h.order)-1], "relation:") {
						t.Fatal("phase order", h.order)
					}
					for _, id := range []string{settingsBlock, cellSecond, nameBlock} {
						var n *ast.Node
						for _, tree := range h.trees {
							if candidate := treenode.GetNodeInTree(tree, id); candidate != nil {
								n = candidate
							}
						}
						if n == nil || n.IALAttr(av.NodeAttrNameAvs) != cellDest+","+cellAV || n.IALAttr(av.NodeAttrViewNames) != "" {
							t.Fatal("shared batch candidate lost membership", id)
						}
						if !strings.Contains(h.attrs[id][av.NodeAttrViewNames], page) || !strings.Contains(h.attrs[id][av.NodeAttrViewNames], h.cache[cellAV].Name) || !strings.Contains(h.attrs[id][av.NodeAttrViewNames], h.cache[cellDest].Name) {
							t.Fatal("owner names/cache", h.attrs[id])
						}
					}
					if !slices.Equal(a, mustDuplicateJSON(t, h.cache[cellAV])) || !slices.Equal(b, mustDuplicateJSON(t, h.cache[cellDest])) || len(h.staged) != 0 || h.counts["duplicate-id"] != 0 || h.counts["finish"] != 0 {
						t.Fatal("import mutated AV/generated IDs/committed", h.counts)
					}
					assertNameSource(t, h.cellHarness, before)
				})
			}
		})
	}
}

func TestAVImportSkipsEmptyDuplicateAndContinuousCandidates(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty", "empty-values", "detached", "vanished", "duplicate-ids", "continuous", "helper-only"} {
		t.Run(scenario, func(t *testing.T) {
			h := newAVImportHarness(t, l, root, page, "")
			a, err := h.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{cellAV}
			trees := avImportedTrees()
			switch scenario {
			case "empty":
				ids = nil
				trees = nil
			case "empty-values":
				a.GetBlockKeyValues().Values = nil
			case "detached":
				for _, v := range a.GetBlockKeyValues().Values {
					v.IsDetached = true
				}
			case "vanished":
				h.env.avImport.bindings.exists = func(string) (bool, error) { return false, nil }
			case "duplicate-ids":
				ids = append(ids, cellAV)
			}
			before := nameDigest(t, root, page)
			if scenario == "helper-only" {
				err = h.tx.updateBoundBlockAvsAttribute(ids)
			} else {
				err = h.tx.finishImportedAttributeViews(ids, trees)
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "empty" && len(h.order) != 0 {
				t.Fatal("empty batch did work", h.order)
			}
			if scenario == "empty-values" || scenario == "detached" || scenario == "vanished" {
				if len(h.trees) != 0 || len(h.notifications) != 0 {
					t.Fatal("skipped bindings produced writes")
				}
			}
			if scenario == "duplicate-ids" && (h.counts["duplicate-tree"] != 2 || h.counts["import-relation"] != 1) {
				t.Fatal("duplicate IDs changed save/projection semantics", h.counts)
			}
			if scenario == "helper-only" && (len(h.registered) != 0 || len(h.links) != 0) {
				t.Fatal("attribute helper registered unrelated effects")
			}
			if scenario == "continuous" {
				a.Name = page + " staged import name"
				if err := h.tx.updateBoundBlockAvsAttribute(ids); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(h.attrs[settingsBlock][av.NodeAttrViewNames], a.Name) {
					t.Fatal("staged name not read")
				}
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}
