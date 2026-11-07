package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
)

type findReplaceHarness struct {
	*blockAttributeHarness
	replace   *findReplaceEnvironment
	preimages map[string][]byte
	queries   []findReplaceRequest
	stages    []string
	renamed   map[string]string
	cleared   []bool
	refCounts []string
	order     []string
}

func newFindReplaceHarness(t *testing.T, l *workspace.Lease, root, page string) *findReplaceHarness {
	h := &findReplaceHarness{blockAttributeHarness: newBlockAttributeHarness(t, l, root, page), preimages: map[string][]byte{}, renamed: map[string]string{}}
	mark := func(name string) error { h.order = append(h.order, name); return h.mark(name) }
	// The AV stress fixture contains deliberately incomplete AV sources. Index
	// the ordinary second document for the real replace-all query regression.
	for _, doc := range []string{nameDoc} {
		if err := l.QueueNativeTree(h.ctx, settingsBox, doc, "index"); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.FlushNativeMainQueue(h.ctx); err != nil {
		t.Fatal(err)
	}
	e := &findReplaceEnvironment{ctx: h.ctx, gate: h.attributes.gate,
		parser: func() (*lute.Lute, error) { return h.env.parser(), mark("replace-parser") }, reject: func(code int) error { return fmt.Errorf("%s rejected %d", page, code) },
		search: func(q findReplaceRequest) (ret []string, err error) {
			if err = mark("replace-search"); err != nil {
				return
			}
			h.queries = append(h.queries, q)
			view, err := l.SearchAllNativeBlockView(h.ctx, indexdb.BlockSearchRequest{Query: q.Keyword, Boxes: q.Boxes, Paths: q.Paths, Types: q.Types, Method: q.Method, OrderBy: q.OrderBy, Page: 1, PageSize: 1})
			if err != nil {
				return nil, err
			}
			for _, b := range view.Blocks {
				ret = append(ret, b.ID)
			}
			return
		},
		lookup: func(id string) (*treenode.BlockTree, error) {
			if err := mark("replace-lookup"); err != nil {
				return nil, err
			}
			return h.source.lookup(id)
		},
		load: func(id string) (*parse.Tree, error) {
			if err := mark("replace-load"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		beginHistory: func() (string, error) { return page + "-history", mark("replace-history-begin") },
		history: func(id string, tree *parse.Tree) error {
			if err := mark("replace-history"); err != nil {
				return err
			}
			if id != page+"-history" {
				return errors.New("foreign history")
			}
			raw, err := os.ReadFile(filepath.Join(root, "pages", page, "data", tree.Box, strings.TrimPrefix(tree.Path, "/")))
			if err != nil {
				return err
			}
			h.preimages[tree.ID] = bytes.Clone(raw)
			return nil
		},
		indexHistory: func(string) error { return mark("replace-history-index") },
		stage: func(tree *parse.Tree) error {
			if err := mark("replace-stage"); err != nil {
				return err
			}
			h.stages = append(h.stages, tree.ID)
			return h.tx.writeTree(tree)
		},
		rename: func(tree *parse.Tree, title string) error {
			if err := mark("replace-rename"); err != nil {
				return err
			}
			h.renamed[tree.ID] = title
			tree.Root.SetIALAttr("title", title)
			return nil
		},
		progress: func(kind string, _, _ int) error { return mark("replace-progress:" + kind) }, clear: func(success bool) error {
			h.cleared = append(h.cleared, success)
			return mark(fmt.Sprintf("replace-clear:%t", success))
		},
		flush: func() error { return mark("replace-flush") }, tags: func() error { return mark("replace-tags") },
		reload: func(id string) error {
			if err := mark("replace-reload"); err != nil {
				return err
			}
			return h.tx.reloadProtyle(id)
		},
		refCount: func(id string) error {
			if err := mark("replace-refcount"); err != nil {
				return err
			}
			h.refCounts = append(h.refCounts, id)
			return h.source.enqueue(task.SetDefRefCount, id, time.Millisecond, func() error { return h.tx.refreshRefCount(id) })
		},
	}
	h.replace = e
	h.env.findReplace = e
	return h
}
func replaceParagraph(t *testing.T, h *findReplaceHarness, id, text string) *ast.Node {
	t.Helper()
	n, _, err := h.cell.node(id)
	if err != nil {
		t.Fatal(err)
	}
	for n.FirstChild != nil {
		n.FirstChild.Unlink()
	}
	n.RemoveIALAttr("name")
	n.RemoveIALAttr(av.NodeAttrViewStaticText + "-" + cellAV)
	n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(text)})
	return n
}

func TestFindReplaceOwnersHistoryBeforeMutationAndAVRefresh(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newFindReplaceHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		n := replaceParagraph(t, h, settingsBlock, page+" Alpha alpha")
		q := findReplaceRequest{Keyword: "alpha", Replacement: "New", ReplaceTypes: map[string]bool{"text": true}, IDs: []string{settingsBlock, settingsBlock}}
		if err = h.tx.findReplace(q); err != nil {
			t.Fatal(err)
		}
		if string(n.FirstChild.Tokens) != page+" New New" || len(h.preimages) != 1 || len(h.stages) != 1 || h.counts["replace-reload"] != 1 || !reflect.DeepEqual(h.cleared, []bool{true}) {
			t.Fatal("original replace/history phases", n, h.stages, h.counts)
		}
		raw, err := os.ReadFile(filepath.Join(root, "pages", page, "data", settingsBox, settingsDoc+".docj"))
		if err != nil || !bytes.Equal(raw, h.preimages[settingsDoc]) {
			t.Fatal("history preimage changed", err)
		}
		if slices.Index(h.order, "replace-history-index") > slices.Index(h.order, "replace-stage") || slices.Index(h.order, "replace-flush") > slices.Index(h.order, "replace-reload") {
			t.Fatal(h.order)
		}
		if a := h.cellHarness.staged[cellAV]; a == nil || !strings.Contains(a.GetBlockKeyValues().Values[0].Block.Content, page+" New New") {
			t.Fatal("AV owner text not refreshed", a)
		}
		if !reflect.DeepEqual(q.IDs, []string{settingsBlock, settingsBlock}) {
			t.Fatal("request slice mutated")
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestFindReplaceAllUsesActualOwnerQueryWithoutPagination(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newFindReplaceHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		q := findReplaceRequest{Keyword: "body", Replacement: "knowledge", Method: 1, ReplaceTypes: map[string]bool{"text": true}}
		if err = h.tx.findReplace(q); err != nil {
			t.Fatal(err)
		}
		n, _, _ := h.cell.node(nameBlock)
		if len(h.queries) != 1 || h.queries[0].Method != 0 || !strings.Contains(string(n.FirstChild.Tokens), page+" second knowledge") || len(h.preimages) == 0 {
			t.Fatal("real owner replace-all failed", h.queries, n)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestFindReplaceOriginalRichTextFlagsRegexAndCase(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, mode := range []int{0, 3} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			h := newFindReplaceHarness(t, l, root, page)
			n := replaceParagraph(t, h, settingsBlock, "Alpha plain")
			code := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "code", TextMarkTextContent: "Alpha &amp; code"}
			link := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "a", TextMarkTextContent: "Alpha link", TextMarkAHref: "https://Alpha.test", TextMarkATitle: "Alpha title"}
			strong := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "strong", TextMarkTextContent: "Alpha strong"}
			ref := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: nameBlock, TextMarkBlockRefSubtype: "d", TextMarkTextContent: "Alpha ref"}
			mathNode := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "inline-math", TextMarkInlineMathContent: "Alpha math"}
			for _, x := range []*ast.Node{code, link, strong, ref, mathNode} {
				n.AppendChild(x)
			}
			q := findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", Method: mode, IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"text": true, "code": true, "aText": true, "aHref": true, "strong": true, "blockRef": true, "inlineMath": true}}
			if err := h.tx.findReplace(q); err != nil {
				t.Fatal(err)
			}
			if string(n.FirstChild.Tokens) != "Beta plain" || code.TextMarkTextContent != "Beta &amp; code" || link.TextMarkTextContent != "Beta link" || link.TextMarkAHref != "https://Beta.test" || link.TextMarkATitle != "Alpha title" || strong.TextMarkTextContent != "Beta strong" || ref.TextMarkTextContent != "Beta ref" || ref.TextMarkBlockRefSubtype != "s" || mathNode.TextMarkInlineMathContent != "Beta math" {
				t.Fatal("rich text flags changed", n)
			}
		})
	}
	h := newFindReplaceHarness(t, l, root, page)
	h.replace.caseSensitive = true
	n := replaceParagraph(t, h, settingsBlock, "Alpha alpha")
	if err = h.tx.findReplace(findReplaceRequest{Keyword: "alpha", Replacement: "beta", IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"text": true}}); err != nil || string(n.FirstChild.Tokens) != "Alpha beta" {
		t.Fatal("owner case sensitivity", err, n)
	}
}

func TestFindReplaceDocumentTitleTagsAndReplacementCarry(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newFindReplaceHarness(t, l, root, page)
	tree := h.tx.trees[settingsDoc]
	tree.Root.SetIALAttr("title", "Alpha title")
	tree.Root.SetIALAttr("tags", "Alpha,Alpha/sub")
	n := replaceParagraph(t, h, settingsBlock, "Alpha body")
	if err = h.tx.findReplace(findReplaceRequest{Keyword: "Alpha", Replacement: "#New/Value#", IDs: []string{settingsDoc, settingsBlock}, ReplaceTypes: map[string]bool{"docTitle": true, "text": true}}); err != nil {
		t.Fatal(err)
	}
	if h.renamed[settingsDoc] != "#New／Value# title" || tree.Root.IALAttr("tags") != "New/Value,New/Value/sub" || string(n.FirstChild.Tokens) != "New/Value body" || h.counts["replace-tags"] != 1 || len(h.preimages) != 1 || len(h.stages) != 2 {
		t.Fatal("title/tag carry rule", h.renamed, tree.Root.KramdownIAL, string(n.FirstChild.Tokens), h.stages)
	}
}

func TestFindReplaceTagConversionSchedulesOwnerReferenceCounts(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newFindReplaceHarness(t, l, root, page)
	n := replaceParagraph(t, h, settingsBlock, "")
	n.FirstChild.Unlink()
	n.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "tag", TextMarkTextContent: "AGI"})
	replacement := "((" + nameBlock + " 'Static reference'))"
	if err = h.tx.findReplace(findReplaceRequest{Keyword: "#AGI#", Replacement: replacement, IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"tag": true}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.refCounts, []string{nameBlock}) || h.counts["replace-tags"] != 1 {
		t.Fatal("inline ref queued globally or omitted", h.refCounts, h.counts)
	}
	found := false
	ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.IsTextMarkType("block-ref") && n.TextMarkBlockRefID == nameBlock {
			found = true
		}
		return ast.WalkContinue
	})
	if !found {
		t.Fatal("original tag-to-inline conversion lost")
	}
}

func TestFindReplaceFailuresAndLateCancellationCleanOwnerProgress(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"replace-lookup", "replace-load", "replace-history-begin", "replace-history", "replace-history-index", "replace-parser", "replace-stage", "replace-progress:replace", "replace-flush", "replace-reload", "tag-av-text", "tag-av-stage", "tag-av-reload", "tag-av-related", "replace-clear:true"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newFindReplaceHarness(t, l, root, page)
				before := nameDigest(t, root, page)
				replaceParagraph(t, h, settingsBlock, "Alpha body")
				h.failure = prefix + point
				err := h.tx.findReplace(findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"text": true}})
				if err == nil || h.counts[point] == 0 || len(h.cleared) == 0 || h.cleared[len(h.cleared)-1] || h.counts["finish"] != 0 {
					t.Fatal("failure/cleanup lost", err, h.counts, h.cleared)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
	for _, scenario := range []string{"late-load", "late-parser", "late-stage", "late-rename", "late-av-read", "refcount-failure", "search-failure"} {
		t.Run(scenario, func(t *testing.T) {
			h := newFindReplaceHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			n := replaceParagraph(t, h, settingsBlock, "Alpha body")
			q := findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"text": true}}
			switch scenario {
			case "late-load":
				h.replace.load = func(string) (*parse.Tree, error) { h.cancel(); return h.tx.trees[settingsDoc], nil }
			case "late-parser":
				h.replace.parser = func() (*lute.Lute, error) { h.cancel(); return h.env.parser(), nil }
			case "late-stage":
				h.replace.stage = func(*parse.Tree) error { h.cancel(); return nil }
			case "late-rename":
				q.IDs = []string{settingsDoc}
				q.ReplaceTypes = map[string]bool{"docTitle": true}
				h.tx.trees[settingsDoc].Root.SetIALAttr("title", "Alpha title")
				h.replace.rename = func(*parse.Tree, string) error { h.cancel(); return nil }
			case "late-av-read":
				h.env.avText.read = func(string) (*av.AttributeView, error) { h.cancel(); return nil, nil }
			case "refcount-failure":
				n.FirstChild.Unlink()
				n.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "tag", TextMarkTextContent: "Alpha"})
				q.ReplaceTypes = map[string]bool{"tag": true}
				q.Replacement = "((" + nameBlock + " 'ref'))"
				h.failure = "replace-refcount"
			case "search-failure":
				q.IDs = nil
				h.failure = "replace-search"
			}
			if err := h.tx.findReplace(q); err == nil {
				t.Fatal("late callback/failure accepted")
			}
			if len(h.cleared) == 0 || h.cleared[len(h.cleared)-1] {
				t.Fatal(h.cleared)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestFindReplaceValidationNoOpsMissingOwnersAndLegacyErrorPolicy(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"noop", "sql", "group", "missing-env", "missing-av", "missing-gate", "bad-tree", "bad-lookup", "empty-history", "nil-parser", "cancelled", "regex-invalid"} {
		t.Run(scenario, func(t *testing.T) {
			h := newFindReplaceHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			n := replaceParagraph(t, h, settingsBlock, "Alpha body")
			q := findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"text": true}}
			switch scenario {
			case "noop":
				q.Replacement = q.Keyword
			case "sql":
				q.Method = 2
			case "group":
				q.GroupBy = 1
			case "missing-env":
				h.env.findReplace = nil
			case "missing-av":
				h.env.avText = nil
				h.replace.av = legacyAttributeViewTextEnvironment()
			case "missing-gate":
				h.replace.gate = nil
			case "bad-tree":
				h.replace.load = func(string) (*parse.Tree, error) {
					return &parse.Tree{ID: "foreign", Root: &ast.Node{ID: "foreign", Type: ast.NodeDocument}}, nil
				}
			case "bad-lookup":
				h.replace.lookup = func(string) (*treenode.BlockTree, error) {
					return &treenode.BlockTree{ID: "foreign", RootID: settingsDoc}, nil
				}
			case "empty-history":
				h.replace.beginHistory = func() (string, error) { return "", nil }
			case "nil-parser":
				h.replace.parser = func() (*lute.Lute, error) { return nil, nil }
			case "cancelled":
				h.cancel()
			case "regex-invalid":
				q.Method = 3
				q.Keyword = "["
			}
			err := h.tx.findReplace(q)
			if scenario == "noop" {
				if err != nil || len(h.order) != 0 {
					t.Fatal(err, h.order)
				}
			} else if scenario == "regex-invalid" {
				if err != nil || string(n.FirstChild.Tokens) != "Alpha body" {
					t.Fatal("original invalid regex targeted-ID policy", err)
				}
			} else if err == nil {
				t.Fatal("invalid owner/query accepted")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
	// The legacy void AV helper logged and continued; native service errors do not.
	h := newFindReplaceHarness(t, l, root, page)
	replaceParagraph(t, h, settingsBlock, "Alpha")
	e := *h.replace
	e.legacy = true
	e.av = h.env.avText
	e.av.clone = func(*av.AttributeView) (*av.AttributeView, error) { return nil, errors.New("AV failure") }
	e.reload = func(string) error { return nil }
	if err = findReplaceWithEnvironment(findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", IDs: []string{settingsBlock}, ReplaceTypes: map[string]bool{"text": true}}, &e); err != nil || !slices.Equal(h.cleared, []bool{true}) {
		t.Fatal("legacy AV policy changed", err, h.cleared)
	}
}

func TestFindReplaceNativeGateCancellationBeforeHistoryAndInputIsolation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newFindReplaceHarness(t, l, root, page)
	h.replace.legacy = true // The native entry cannot opt out of admission.
	if err = h.replace.gate.with(h.ctx, func() error {
		h.cancel()
		return h.tx.findReplace(findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", IDs: []string{settingsBlock}})
	}); !errors.Is(err, context.Canceled) || len(h.order) != 0 {
		t.Fatal(err, h.order)
	}
	h = newFindReplaceHarness(t, l, root, page)
	q := findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", Boxes: []string{"original"}, Types: map[string]bool{"p": true}, ReplaceTypes: map[string]bool{"text": true}}
	replaceParagraph(t, h, settingsBlock, "Alpha")
	h.replace.search = func(request findReplaceRequest) ([]string, error) {
		request.Boxes[0] = "changed"
		request.Types["p"] = false
		request.ReplaceTypes["text"] = false
		return []string{settingsBlock}, nil
	}
	if err = h.tx.findReplace(q); err != nil || q.Boxes[0] != "original" || !q.Types["p"] || !q.ReplaceTypes["text"] {
		t.Fatal("request aliases leaked", err, q)
	}
	n, _, _ := h.cell.node(settingsBlock)
	if string(n.FirstChild.Tokens) != "Beta" {
		t.Fatal("search service mutated the replacement plan")
	}
}

func TestFindReplaceAVProjectionReadsNewCandidateAfterReload(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newFindReplaceHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	replaceParagraph(t, h, settingsBlock, "Alpha definition")
	old := replaceParagraph(t, h, nameBlock, "Old reference")
	tree := h.tx.trees[nameDoc]
	fresh := &parse.Tree{ID: tree.ID, Box: tree.Box, Path: tree.Path, Root: &ast.Node{ID: tree.ID, Type: ast.NodeDocument}}
	n := &ast.Node{ID: nameBlock, Type: ast.NodeParagraph}
	n.SetIALAttr("name", "Fresh Beta reference")
	n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
	fresh.Root.AppendChild(n)
	h.replace.reload = func(root string) error {
		if root == settingsDoc {
			h.tx.trees[nameDoc] = fresh
		}
		return nil
	}
	if err = h.tx.findReplace(findReplaceRequest{Keyword: "Alpha", Replacement: "Beta", IDs: []string{settingsBlock, nameBlock}, ReplaceTypes: map[string]bool{"text": true}}); err != nil {
		t.Fatal(err)
	}
	if string(old.FirstChild.Tokens) != "Old reference" {
		t.Fatal("old candidate unexpectedly mutated")
	}
	found := false
	for _, v := range h.cellHarness.staged[cellAV].GetBlockKeyValues().Values {
		if v.Block.ID == nameBlock && v.Block.Content == "Fresh Beta reference" {
			found = true
		}
	}
	if !found {
		t.Fatal("stale replacement AST overwrote the reloaded AV text")
	}
	assertNameSource(t, h.cellHarness, before)
}
