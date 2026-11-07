package model

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
)

type tagMutationHarness struct {
	*boundHarness
	order    []string
	cleared  []bool
	reloaded []string
}

func newTagMutationHarness(t *testing.T, l *workspace.Lease, root, page string) *tagMutationHarness {
	d := newBoundHarness(t, l, root, page, "")
	h := &tagMutationHarness{boundHarness: d}
	first, err := h.tx.loadTree(settingsBlock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.tx.loadTree(nameBlock)
	if err != nil {
		t.Fatal(err)
	}
	first.Root.SetIALAttr("tags", "AGI,AGI/sub,AGIX,Other")
	for id, labels := range map[string][]string{settingsBlock: {"AGI", "AGI/sub", "Other"}, cellSecond: {"AGIX"}, nameBlock: {"AGI/sub"}} {
		n, _, err := h.cell.node(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, label := range labels {
			n.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "tag", TextMarkTextContent: label})
		}
	}
	if err = l.WithNativeIndexes(h.ctx, func(b *indexdb.Bundle) error {
		db, err := b.Database(indexdb.Main)
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(h.ctx, "DELETE FROM spans"); err != nil {
			return err
		}
		for i, row := range [][3]string{{settingsDoc, settingsDoc, "AGI,AGI/sub,AGIX,Other"}, {settingsDoc, settingsBlock, "AGI"}, {settingsDoc, settingsBlock, "AGI/sub"}, {settingsDoc, cellSecond, "AGIX"}, {nameDoc, nameBlock, "AGI/sub"}} {
			if _, err = db.ExecContext(h.ctx, "INSERT INTO spans(id,block_id,root_id,box,path,content,markdown,type,ial) VALUES(?,?,?,?,?,?,?,?,?)", fmt.Sprintf("tag-span-%d", i), row[1], row[0], settingsBox, "/"+row[0]+".docj", row[2], "#"+row[2]+"#", "tag", ""); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = second
	mark := func(name string) error { h.order = append(h.order, name); return h.mark(name) }
	read := h.cell.read
	h.cell.read = func(id string) (*av.AttributeView, error) {
		if a := h.staged[id]; a != nil {
			return a, nil
		}
		return read(id)
	}
	h.env.avText = &attributeViewTextEnvironment{ctx: h.ctx, read: h.cell.read, clone: cloneAttributeViewPreview,
		text: func(n *ast.Node, id string) (string, string, error) {
			if err := mark("tag-av-text"); err != nil {
				return "", "", err
			}
			return attributeViewBlockTextWithServices(n, id, h.cell.blockText, func() string { return page + " owner untitled" })
		},
		stage: func(a *av.AttributeView) error {
			if err := mark("tag-av-stage"); err != nil {
				return err
			}
			return h.cell.stage(a)
		},
		reload: func(string) error { return mark("tag-av-reload") },
		related: func(id string) error {
			if err := mark("tag-av-related"); err != nil {
				return err
			}
			before := len(h.related)
			if err := h.cell.related(id); err != nil {
				return err
			}
			return flushRelatedAttributeViews(h.ctx, uniqueUpdatedAttributeViews(h.related[before:]), func(id string) (*av.AttributeView, error) {
				a, err := h.cell.read(id)
				if err != nil {
					return nil, err
				}
				return cloneAttributeViewPreview(a)
			}, h.cell.groups, h.cell.stage, func(string) error { return mark("tag-related-reload") }, false)
		},
	}
	h.env.tagMutation = &tagMutationEnvironment{ctx: h.ctx,
		query: func(label string) ([]indexdb.SpanRecord, error) {
			if err := mark("tag-query"); err != nil {
				return nil, err
			}
			return l.NativeTagSpans(h.ctx, "label", label, 0)
		},
		load: func(id string) (*parse.Tree, error) {
			if err := mark("tag-load"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		stage: func(tree *parse.Tree) error {
			if err := mark("tag-stage"); err != nil {
				return err
			}
			return h.tx.writeTree(tree)
		},
		flush: func() error { return mark("tag-flush") }, reload: func(id string) error {
			if err := mark("tag-reload"); err != nil {
				return err
			}
			h.reloaded = append(h.reloaded, id)
			return nil
		},
		progress: func(kind, _ string) error { return mark("tag-progress:" + kind) }, clear: func(success bool) error {
			h.cleared = append(h.cleared, success)
			return mark(fmt.Sprintf("tag-clear:%t", success))
		}, delay: func(int, int) error { return mark("tag-delay") },
		invalid: func(marker string) error { return fmt.Errorf("%s invalid %s", page, marker) }, empty: func() error { return errors.New(page + " empty label") },
	}
	return h
}

func tagNodeLabels(n *ast.Node) []string {
	var ret []string
	for _, mark := range n.ChildrenByType(ast.NodeTextMark) {
		if mark.IsTextMarkType("tag") {
			ret = append(ret, mark.TextMarkTextContent)
		}
	}
	return ret
}

func TestTagMutationOwnersRealTagQueryOriginalRulesAndAVRefresh(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, rename := range []bool{false, true} {
				h := newTagMutationHarness(t, l, root, page)
				before := nameDigest(t, root, page)
				n, tree, err := h.cell.node(settingsBlock)
				if err != nil {
					t.Fatal(err)
				}
				n.SetIALAttr("icon", "1f680")
				original := map[string][]byte{}
				for _, id := range []string{cellAV, cellDest, cellRelated} {
					a, err := h.cell.read(id)
					if err != nil {
						t.Fatal(err)
					}
					original[id] = mustDuplicateJSON(t, a)
				}
				if rename {
					err = h.tx.renameTag("AGI", " / Target / ")
				} else {
					err = h.tx.removeTag("AGI")
				}
				if err != nil {
					t.Fatal(err, h.order)
				}
				wantDoc, wantBlock, wantOther := "AGI/sub,AGIX,Other", []string{"AGI/sub", "Other"}, []string{"AGI/sub"}
				if rename {
					wantDoc = "Target,Target/sub,AGIX,Other"
					wantBlock = []string{"Target", "Target/sub", "Other"}
					wantOther = []string{"Target/sub"}
				}
				other, _, err := h.cell.node(nameBlock)
				if err != nil {
					t.Fatal(err)
				}
				code, _, _ := h.cell.node(cellSecond)
				if tree.Root.IALAttr("tags") != wantDoc || !slices.Equal(tagNodeLabels(n), wantBlock) || !slices.Equal(tagNodeLabels(other), wantOther) || !slices.Equal(tagNodeLabels(code), []string{"AGIX"}) {
					t.Fatal("tag matching changed", tree.Root.KramdownIAL, tagNodeLabels(n), tagNodeLabels(other))
				}
				if h.counts["tag-query"] != 1 || h.counts["tag-stage"] != 2 || h.counts["tag-flush"] != 2 || len(h.reloaded) != 2 || !slices.Equal(h.cleared, []bool{true}) || h.counts["finish"] != 0 {
					t.Fatal("phase counts", h.counts)
				}
				if slices.Index(h.order, "tag-flush") > slices.Index(h.order, "tag-reload") || slices.Index(h.order, "tag-av-stage") < slices.Index(h.order, "tag-reload") {
					t.Fatal("original flush/reload/AV ordering", h.order)
				}
				for _, id := range []string{cellAV, cellDest} {
					a := h.staged[id]
					if a == nil || a.GetBlockKeyValues().Values[0].Block.Icon != "1f680" || len(a.GetBlockKeyValues().Values) != 3 {
						t.Fatal("AV text/identity lost", id)
					}
					if !reflect.DeepEqual(original[id], mustDuplicateJSON(t, h.cache[id])) {
						t.Fatal("source AV cache modified", id)
					}
				}
				if rename && !strings.Contains(h.staged[cellDest].GetBlockKeyValues().Values[0].Block.Content, "Target") {
					t.Fatal("AV did not reflect changed tag text")
				}
				assertNameSource(t, h.cellHarness, before)
			}
		})
	}
}

func TestTagMutationValidationNoOpsMissingDependenciesAndBadTrees(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"same", "empty-remove", "invalid", "empty-name", "missing-env", "missing-av", "query", "load", "stage", "flush", "reload", "progress", "clear", "delay", "bad-tree", "bad-span", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			h := newTagMutationHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			e := h.env.tagMutation
			noop := false
			switch scenario {
			case "same":
				noop = true
				err = h.tx.renameTag("Target", " / Target / ")
			case "empty-remove":
				noop = true
				err = h.tx.removeTag("")
			case "invalid":
				err = h.tx.renameTag("AGI", "#bad")
			case "empty-name":
				err = h.tx.renameTag("AGI", " / ")
			case "missing-env":
				h.env.tagMutation = nil
			case "missing-av":
				h.env.avText = nil
			case "query":
				e.query = nil
			case "load":
				e.load = nil
			case "stage":
				e.stage = nil
			case "flush":
				e.flush = nil
			case "reload":
				e.reload = nil
			case "progress":
				e.progress = nil
			case "clear":
				e.clear = nil
			case "delay":
				e.delay = nil
			case "bad-tree":
				e.load = func(string) (*parse.Tree, error) { return &parse.Tree{}, nil }
			case "bad-span":
				e.query = func(string) ([]indexdb.SpanRecord, error) { return []indexdb.SpanRecord{{RootID: settingsDoc}}, nil }
			case "cancel":
				h.cancel()
			}
			if scenario != "same" && scenario != "empty-remove" && scenario != "invalid" && scenario != "empty-name" {
				err = h.tx.renameTag("AGI", "Target")
			}
			if noop {
				if err != nil || len(h.order) != 0 {
					t.Fatal("no-op produced effects", err, h.order)
				}
			} else if err == nil {
				t.Fatal("invalid source/dependency accepted", scenario)
			}
			if h.counts["finish"] != 0 {
				t.Fatal("candidate committed")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestTagMutationFailuresCancellationAndOwnerProgressCleanup(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"tag-progress:rename", "tag-delay", "tag-query", "tag-progress:load", "tag-load", "tag-progress:save", "tag-stage", "tree-stage", "tag-flush", "tag-reload", "tag-av-text", "tag-av-stage", "tag-av-reload", "tag-av-related", "tag-clear:true"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newTagMutationHarness(t, l, root, page)
				before := nameDigest(t, root, page)
				h.failure = prefix + point
				err := h.tx.renameTag("AGI", "Target")
				if err == nil || h.counts[point] == 0 || h.counts["finish"] != 0 {
					t.Fatal("failure lost", err, h.order)
				}
				if len(h.cleared) == 0 || h.cleared[len(h.cleared)-1] {
					t.Fatal("owner progress not cleared on failure", h.cleared)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
}

func TestAVTextRefreshParentsFirstMatchAndChangedOnlyEffects(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newTagMutationHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	n, tree, err := h.cell.node(settingsBlock)
	if err != nil {
		t.Fatal(err)
	}
	n.SetIALAttr("name", "Name & value")
	n.SetIALAttr(av.NodeAttrViewStaticText+"-"+cellAV, "Static &amp; value")
	parent := &ast.Node{Type: ast.NodeBlockquote, ID: "20261026190000-quo0001"}
	parent.SetIALAttr("name", "Parent")
	parent.SetIALAttr(av.NodeAttrNameAvs, cellAV)
	n.InsertBefore(parent)
	n.Unlink()
	parent.AppendChild(n)
	a, err := h.cell.read(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	primary := a.GetBlockKeyValues()
	primary.Values = append(primary.Values, &av.Value{BlockID: "parent-row", Block: &av.ValueBlock{ID: parent.ID, Content: "old parent"}}, &av.Value{BlockID: "duplicate-row", Block: &av.ValueBlock{ID: n.ID, Content: "duplicate stays"}})
	h.env.avText.related = func(string) error { return nil }
	nodes := map[string]*ast.Node{n.ID: n}
	if err = h.tx.updateAttributeViewBlockText(nodes); err != nil {
		t.Fatal(err)
	}
	staged := h.staged[cellAV].GetBlockKeyValues()
	if nodes[parent.ID] != parent || nodes[tree.Root.ID] != nil || staged.Values[0].Block.Content != "Static & value" || staged.Values[len(staged.Values)-2].Block.Content != "Parent" || staged.Values[len(staged.Values)-1].Block.Content != "duplicate stays" {
		t.Fatal("original parent/static/first-match rules lost")
	}
	for _, id := range []string{cellAV, cellDest} {
		if a := h.staged[id]; a != nil {
			for _, v := range a.GetBlockKeyValues().Values {
				if v.Block.ID == n.ID && id == cellDest && v.Block.Content != "Name & value" {
					t.Fatal("name/unescape", v.Block.Content)
				}
			}
		}
	}
	// Plain equal content avoids the original escaped-vs-unescaped comparison.
	n.SetIALAttr("name", "Same")
	n.SetIALAttr(av.NodeAttrViewStaticText+"-"+cellAV, "Same")
	parent.SetIALAttr("name", "Parent")
	for _, id := range []string{cellAV, cellDest} {
		if a := h.staged[id]; a != nil {
			for _, v := range a.GetBlockKeyValues().Values {
				if v.Block.ID == n.ID {
					v.Block.Content = "Same"
				}
			}
		}
	}
	stages := h.counts["tag-av-stage"]
	if err = h.tx.updateAttributeViewBlockText(map[string]*ast.Node{n.ID: n}); err != nil || h.counts["tag-av-stage"] != stages {
		t.Fatal("unchanged rows emitted effects", err, h.order)
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestAVTextRefreshRetainsRelatedCandidatesBetweenBlocks(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newTagMutationHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	nodes := map[string]*ast.Node{}
	for i, id := range []string{settingsBlock, nameBlock} {
		n, _, err := h.cell.node(id)
		if err != nil {
			t.Fatal(err)
		}
		n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
		n.SetIALAttr("name", fmt.Sprintf("Updated %d", i))
		n.SetIALAttr(av.NodeAttrViewStaticText+"-"+cellAV, "")
		nodes[id] = n
	}
	// A derived refresh stages a newer AV after the first block update. The
	// second block must start from that candidate rather than an earlier clone.
	calls := 0
	h.env.avText.related = func(id string) error {
		calls++
		if calls != 1 {
			return nil
		}
		a, err := h.cell.read(id)
		if err != nil {
			return err
		}
		copy, err := h.env.avText.clone(a)
		if err != nil {
			return err
		}
		copy.Views[0].GroupCreated = 777
		return h.cell.stage(copy)
	}
	if err = h.tx.updateAttributeViewBlockText(nodes); err != nil {
		t.Fatal(err)
	}
	a := h.staged[cellAV]
	if calls != 2 || a == nil || a.Views[0].GroupCreated != 777 {
		t.Fatal("related candidate overwritten", calls, a)
	}
	for i, id := range []string{settingsBlock, nameBlock} {
		found := false
		for _, v := range a.GetBlockKeyValues().Values {
			if v.Block.ID == id && v.Block.Content == fmt.Sprintf("Updated %d", i) {
				found = true
			}
		}
		if !found {
			t.Fatal("block update lost", id)
		}
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestTagMutationAndAVTextLateCancellationNoGlobalFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"query", "stage", "av-stage", "av-read", "av-clone", "av-text", "av-related", "av-slot"} {
		h := newTagMutationHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		e := h.env.tagMutation
		h.env.avText.legacy = true // Admission must override this flag.
		switch point {
		case "query":
			f := e.query
			e.query = func(s string) ([]indexdb.SpanRecord, error) { v, err := f(s); h.cancel(); return v, err }
		case "stage":
			f := e.stage
			e.stage = func(t *parse.Tree) error { err := f(t); h.cancel(); return err }
		case "av-stage":
			f := h.env.avText.stage
			h.env.avText.stage = func(a *av.AttributeView) error { err := f(a); h.cancel(); return err }
		case "av-read":
			h.env.avText.read = nil
		case "av-clone":
			h.env.avText.clone = nil
		case "av-text":
			h.env.avText.text = nil
		case "av-related":
			h.env.avText.related = nil
		case "av-slot":
			h.env.avText = nil
		}
		err := h.tx.renameTag("AGI", "Target")
		if err == nil || h.counts["finish"] != 0 {
			t.Fatal("error became success", point, err)
		}
		if point == "query" || point == "stage" || point == "av-stage" {
			if !errors.Is(err, context.Canceled) {
				t.Fatal("late cancellation lost", err)
			}
		} else if h.counts["tag-stage"] != 0 {
			t.Fatal("unbound AV admitted mutation", h.order)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVTextRefreshMalformedSourcesLateCancelAndLegacyPolicy(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"nil-node", "empty-node-id", "read-error", "nil-source", "wrong-source", "no-primary", "nil-value", "nil-block", "clone-alias", "late-read", "late-clone", "late-text", "late-reload"} {
		t.Run(scenario, func(t *testing.T) {
			h := newTagMutationHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			n, _, err := h.cell.node(settingsBlock)
			if err != nil {
				t.Fatal(err)
			}
			n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
			a, err := h.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			e := h.env.avText
			nodes := map[string]*ast.Node{n.ID: n}
			switch scenario {
			case "nil-node":
				nodes[n.ID] = nil
			case "empty-node-id":
				n.ID = ""
			case "read-error":
				e.read = func(string) (*av.AttributeView, error) { return nil, errors.New("owner AV missing") }
			case "nil-source":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "wrong-source":
				a.ID = cellDest
			case "no-primary":
				a.GetBlockKeyValues().Key.Type = av.KeyTypeText
			case "nil-value":
				a.GetBlockKeyValues().Values[0] = nil
			case "nil-block":
				a.GetBlockKeyValues().Values[0].Block = nil
			case "clone-alias":
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { return a, nil }
			case "late-read":
				f := e.read
				e.read = func(id string) (*av.AttributeView, error) { a, err := f(id); h.cancel(); return a, err }
			case "late-clone":
				f := e.clone
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { a, err := f(a); h.cancel(); return a, err }
			case "late-text":
				f := e.text
				e.text = func(n *ast.Node, id string) (string, string, error) {
					i, s, err := f(n, id)
					h.cancel()
					return i, s, err
				}
			case "late-reload":
				f := e.reload
				e.reload = func(id string) error { err := f(id); h.cancel(); return err }
			}
			if err = h.tx.updateAttributeViewBlockText(nodes); err == nil {
				t.Fatal("bad source/cancellation became success", scenario)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
	// Keep legacy best-effort save and missing-source behavior explicit, with no globals.
	n := &ast.Node{Type: ast.NodeParagraph, ID: settingsBlock}
	n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
	a := &av.AttributeView{ID: cellAV, KeyValues: []*av.KeyValues{{Key: &av.Key{Type: av.KeyTypeBlock}, Values: []*av.Value{{Block: &av.ValueBlock{ID: settingsBlock, Content: "old"}}}}}}
	reloads, related := 0, 0
	e := &attributeViewTextEnvironment{ctx: context.Background(), legacy: true, read: func(string) (*av.AttributeView, error) { return a, nil }, clone: cloneAttributeViewPreview, text: func(*ast.Node, string) (string, string, error) { return "", "new", nil }, stage: func(*av.AttributeView) error { return errors.New("original logged IO failure") }, reload: func(string) error { reloads++; return nil }, related: func(string) error { related++; return nil }}
	if err = updateAttributeViewBlockTextWithEnvironment(map[string]*ast.Node{n.ID: n}, e); err != nil || reloads != 1 || related != 1 {
		t.Fatal("legacy save policy changed", err)
	}
	e.read = func(string) (*av.AttributeView, error) { return nil, errors.New("missing source") }
	if err = updateAttributeViewBlockTextWithEnvironment(map[string]*ast.Node{n.ID: n}, e); err != nil || reloads != 1 {
		t.Fatal("legacy missing source no longer skipped", err)
	}
}
