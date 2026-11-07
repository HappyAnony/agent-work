package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

type avUpdateHarness struct {
	*boundHarness
	sources, reloaded []string
	snapshots         map[string][]byte
}

func newAVUpdateHarness(t *testing.T, lease *workspace.Lease, root, page, failure string) *avUpdateHarness {
	d := newBoundHarness(t, lease, root, page, failure)
	h := &avUpdateHarness{boundHarness: d, snapshots: map[string][]byte{}}
	stage := d.env.stage
	d.env.stage = func(tree *parse.Tree) error {
		if err := stage(tree); err != nil {
			return err
		}
		h.snapshots[tree.ID] = render.NewJSONRenderer(tree, d.env.parser().RenderOptions).Render()
		return nil
	}
	d.env.avUpdate = &attributeViewUpdateEnvironment{ctx: d.ctx, read: d.cell.read,
		upsert: func(id, block string) (bool, error) {
			if err := d.mark("update-upsert"); err != nil {
				return false, err
			}
			old := len(d.registry[id])
			if !slices.Contains(d.registry[id], block) {
				d.registry[id] = append(d.registry[id], block)
			}
			return old > 0 && old != len(d.registry[id]), nil
		},
		sources: func(id string) ([]string, error) {
			if err := d.mark("update-sources"); err != nil {
				return nil, err
			}
			h.sources = append(h.sources, id)
			raw, err := os.ReadFile(filepath.Join(root, "pages", page, "data", "storage", "av", "relations.msgpack"))
			if err != nil {
				return nil, err
			}
			var links map[string][]string
			if err = msgpack.Unmarshal(raw, &links); err != nil {
				return nil, err
			}
			return links[id], nil
		}, groups: d.cell.groups, stage: d.cell.stage,
		reload: func(id string) error {
			if err := d.mark("update-reload"); err != nil {
				return err
			}
			h.reloaded = append(h.reloaded, id)
			return nil
		}, attrs: d.cell.attrs,
		tree: func(tree *parse.Tree) error {
			if err := d.mark("update-tree"); err != nil {
				return err
			}
			return d.tx.writeTree(tree)
		},
	}
	d.env.updateAV = transactionUpdateAV
	d.env.updateCacheRef = func(context.Context, *parse.Tree, *ast.Node) error { return nil }
	d.env.updateDynamicText = func(context.Context, string) (string, error) { return "", nil }
	d.env.updateReferences = func(context.Context, ...string) error { return nil }
	d.env.updateUnfold = func(context.Context, *ast.Node, *ast.Node) error { return nil }
	d.env.updateEvent = func(context.Context, []*Transaction) error { return nil }
	return h
}

func avUpdateHTML(id, view string) string {
	return `<div data-node-id="` + id + `" data-type="NodeAttributeView" data-av-id="` + cellAV + `" data-av-type="table"` + view + `></div>`
}

func TestAVUpdateOwnersActualTransactionLayoutsAndSnapshots(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			lease, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(lease.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, explicit := range []bool{false, true} {
					h := newAVUpdateHarness(t, lease, root, page, "")
					before := nameDigest(t, root, page)
					a, err := h.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					a.GetView(a.ViewID).LayoutType = layout
					original := mustDuplicateJSON(t, a)
					ial := ""
					if explicit {
						ial = ` custom-sy-av-view="existing-view"`
					}
					h.tx.DoOperations = []*Operation{{Action: "update", ID: rowMirror, Data: avUpdateHTML(rowMirror, ial)}}
					if ret := performTx(h.tx); ret != nil || !h.committed {
						t.Fatal(ret, h.counts)
					}
					n := treenode.GetNodeInTree(h.tx.trees[settingsDoc], rowMirror)
					wantView := a.ViewID
					if explicit {
						wantView = "existing-view"
					}
					if n.AttributeViewType != string(layout) || n.IALAttr(av.NodeAttrView) != wantView {
						t.Fatal("layout/view", n)
					}
					if !strings.Contains(string(h.snapshots[settingsDoc]), `"AttributeViewType":"`+string(layout)+`"`) || !strings.Contains(string(h.snapshots[settingsDoc]), wantView) {
						t.Fatal("stale tree snapshot", string(h.snapshots[settingsDoc]))
					}
					if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) || len(h.registry[cellAV]) != 1 || len(h.reloaded) != 0 || len(h.sources) != 0 {
						t.Fatal("unchanged mirror should not refresh or rebind", h.counts)
					}
					wantAttrs := 1
					if explicit {
						wantAttrs = 0
					}
					if h.counts["attrs"] != wantAttrs || h.counts["update-tree"] != 1 || h.counts["node"] != 0 || h.counts["name"] != 0 {
						t.Fatal("update unexpectedly bound records", h.counts)
					}
					assertNameSource(t, h.cellHarness, before)
				}
			}
		})
	}
}

func TestAVUpdateAncestorMembershipOrderAndOneHop(t *testing.T) {
	m, store, root := nameSource(t)
	lease, err := m.Acquire(context.Background(), store, "1760000000000000002")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	h := newAVUpdateHarness(t, lease, root, "1760000000000000002", "")
	before := nameDigest(t, root, h.page)
	tree, err := h.tx.loadTree(settingsBlock)
	if err != nil {
		t.Fatal(err)
	}
	node := treenode.GetNodeInTree(tree, settingsBlock)
	quote := &ast.Node{Type: ast.NodeBlockquote, ID: "20261026100000-quo0001"}
	node.InsertBefore(quote)
	node.Unlink()
	quote.AppendChild(node)
	sibling := &ast.Node{Type: ast.NodeParagraph, ID: "20261026100001-par0001"}
	sibling.SetIALAttr(av.NodeAttrNameAvs, cellRelated+","+cellAV)
	quote.AppendChild(sibling)
	tree.Root.SetIALAttr(av.NodeAttrNameAvs, "document-excluded")
	// Newly introduced mirror inside the subtree, preserving first-seen ordering.
	mirror := &ast.Node{Type: ast.NodeAttributeView, ID: boundInserted, AttributeViewID: cellAV}
	node.AppendChild(mirror)
	links := map[string][]string{cellAV: {cellDest, cellAV}, cellDest: {cellRelated}, cellRelated: {cellAV}}
	h.env.avUpdate.sources = func(id string) ([]string, error) { h.sources = append(h.sources, id); return links[id], nil }
	if err := transactionUpdateAV(h.ctx, node, tree, h.tx); err != nil {
		t.Fatal(err)
	}
	want := []string{cellAV, cellDest, cellRelated}
	if !slices.Equal(h.sources, want) || !slices.Equal(h.reloaded, want) || len(h.registry[cellAV]) != 2 || h.counts["attrs"] != 0 {
		t.Fatal(h.sources, h.reloaded, h.counts)
	}
	// Sources found during expansion are not themselves traversed recursively.
	h.sources, h.reloaded = nil, nil
	h.env.avUpdate.sources = func(id string) ([]string, error) { h.sources = append(h.sources, id); return links[id], nil }
	if err := refreshUpdatedAttributeViews([]string{cellAV}, h.tx.attributeViewUpdateEnvironment()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.sources, []string{cellAV}) || !slices.Equal(h.reloaded, []string{cellAV, cellDest}) {
		t.Fatal("recursive expansion", h.sources, h.reloaded)
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestAVUpdateBoundParagraphAndFirstMirrorCandidates(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		lease, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(lease.Release)
		h := newAVUpdateHarness(t, lease, root, page, "")
		before := nameDigest(t, root, page)
		h.tx.DoOperations = []*Operation{{Action: "update", ID: settingsBlock, Data: `<div data-node-id="` + settingsBlock + `" data-type="NodeParagraph" custom-avs="` + cellAV + `,` + cellDest + `"><div contenteditable="true">` + page + ` updated bound body</div></div>`}}
		if ret := performTx(h.tx); ret != nil || !h.committed {
			t.Fatal(ret, h.counts)
		}
		if !slices.Equal(h.sources, []string{cellAV, cellDest}) || !slices.Contains(h.reloaded, cellAV) || !slices.Contains(h.reloaded, cellDest) || h.counts["update-upsert"] != 0 || h.counts["attrs"] != 0 {
			t.Fatal(h.sources, h.reloaded, h.counts)
		}
		if !strings.Contains(string(h.snapshots[settingsDoc]), page+" updated bound body") {
			t.Fatal("wrong workspace tree")
		}
		assertNameSource(t, h.cellHarness, before)
		// First mirror registration must not be interpreted as changed=true.
		h = newAVUpdateHarness(t, lease, root, page, "")
		h.registry = map[string][]string{}
		tree, err := h.tx.loadTree(rowMirror)
		if err != nil {
			t.Fatal(err)
		}
		n := treenode.GetNodeInTree(tree, rowMirror)
		if err := transactionUpdateAV(h.ctx, n, tree, h.tx); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(h.registry[cellAV], []string{rowMirror}) || len(h.reloaded) != 0 || len(h.sources) != 0 {
			t.Fatal("first mirror refresh semantics", h.registry, h.reloaded)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVUpdateFailuresCancellationStopBeforeFinish(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	lease, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	for _, failure := range []string{"update-upsert", "read:" + cellAV, "attrs", "notify", "update-tree", "tree-stage", "update-sources", "groups:" + cellAV, "stage:" + cellAV, "update-reload"} {
		for _, cancel := range []bool{false, true} {
			t.Run(failure+map[bool]string{true: "/cancel"}[cancel], func(t *testing.T) {
				injected := failure
				if cancel {
					injected = "cancel-" + failure
				}
				h := newAVUpdateHarness(t, lease, root, page, injected)
				before := nameDigest(t, root, page)
				// Changing a paragraph to an additional AV triggers refresh as well as layout.
				h.tx.DoOperations = []*Operation{{Action: "update", ID: settingsBlock, Data: avUpdateHTML(settingsBlock, "")}}
				ret := performTx(h.tx)
				if ret == nil || h.committed || h.counts["finish"] != 0 {
					t.Fatal("failure committed", ret, h.counts)
				}
				if failure != "update-tree" && failure != "attrs" && failure != "notify" && h.counts["update-tree"] != 0 {
					t.Fatal("later final tree staged", h.counts)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
}

func TestAVUpdateValidationExactViewAndNoFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	lease, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	for _, scenario := range []string{"missing-env", "missing-port", "cancelled", "bad-tree", "foreign-node", "missing-av", "wrong-av", "nil-view", "nil-field", "missing-view"} {
		t.Run(scenario, func(t *testing.T) {
			h := newAVUpdateHarness(t, lease, root, page, "")
			before := nameDigest(t, root, page)
			tree, err := h.tx.loadTree(rowMirror)
			if err != nil {
				t.Fatal(err)
			}
			n := treenode.GetNodeInTree(tree, rowMirror)
			switch scenario {
			case "missing-env":
				h.env.avUpdate = nil
			case "missing-port":
				h.env.avUpdate.sources = nil
			case "cancelled":
				h.cancel()
			case "bad-tree":
				tree.Box = ""
			case "foreign-node":
				copy := *n
				n = &copy
			case "missing-av":
				h.env.avUpdate.read = func(string) (*av.AttributeView, error) { return nil, errors.New("missing owner AV") }
			default:
				a, err := h.cell.read(cellAV)
				if err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "wrong-av":
					a.ID = cellDest
				case "nil-view":
					a.Views = append(a.Views, nil)
				case "nil-field":
					a.KeyValues = append(a.KeyValues, nil)
				case "missing-view":
					a.ViewID = "absent-current-view"
				}
			}
			err = transactionUpdateAV(context.Background(), n, tree, h.tx)
			if scenario == "missing-view" {
				if err != nil || h.counts["attrs"] != 0 || h.counts["update-tree"] != 0 {
					t.Fatal("fallback view chosen", err, h.counts)
				}
			} else if err == nil {
				t.Fatal("invalid dependency/source admitted", scenario)
			}
			if h.committed || h.counts["finish"] != 0 {
				t.Fatal("helper committed")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestAVUpdateLegacyRefreshMissingReadAndSaveRules(t *testing.T) {
	var stages, reloads []string
	e := &attributeViewUpdateEnvironment{ctx: context.Background(), legacy: true,
		sources: func(id string) ([]string, error) { return []string{"missing"}, nil },
		read: func(id string) (*av.AttributeView, error) {
			if id == "missing" {
				return nil, errors.New("missing")
			}
			return &av.AttributeView{ID: id}, nil
		}, groups: func(*av.AttributeView) error { return nil },
		stage: func(a *av.AttributeView) error {
			stages = append(stages, a.ID)
			return errors.New("original best effort save")
		},
		reload: func(id string) error { reloads = append(reloads, id); return nil },
	}
	if err := refreshUpdatedAttributeViews([]string{"source"}, e); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stages, []string{"source"}) || !slices.Equal(reloads, []string{"source", "missing"}) {
		t.Fatal(stages, reloads)
	}
}
