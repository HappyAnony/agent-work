//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

const mergeRoot = "20261101120000-mergro1"
const mergeA = "20261101120001-merga01"
const mergeB = "20261101120002-mergb01"
const mergeGrand = "20261101120003-mergg01"
const mergeHidden = "20261101120004-mergh01"

func mergeDoc(t *testing.T, m *markdownExportContext, box, p, title string, hidden, register bool, children ...interface{}) string {
	t.Helper()
	id := util.GetTreeID(p)
	attrs := map[string]string{"id": id, "title": title}
	if hidden {
		attrs["custom-hidden"] = "true"
	}
	raw, err := json.Marshal(map[string]interface{}{"ID": id, "Type": "NodeDocument", "Properties": attrs, "Opaque": map[string]string{"keep": "native"}, "Children": children})
	if err != nil {
		t.Fatal(err)
	}
	name := archiveWrite(t, m.read.binding.scope.commit.r.data, box+p, raw)
	if register {
		trees, err := m.read.selectedTrees(box, []string{p})
		if err != nil {
			t.Fatal(err)
		}
		if err = m.read.binding.scope.commit.r.registry.Upsert(context.Background(), trees[id]); err != nil {
			t.Fatal(err)
		}
	}
	return name
}
func mergeFixture(t *testing.T) (*markdownExportContext, string, map[string][]byte) {
	t.Helper()
	m, box := markdownFixture(t)
	m.mergeBound = true
	m.mergeSort = util.SortModeNameASC
	m.mergeMax = 100
	files := []string{
		mergeDoc(t, m, box, "/"+mergeRoot+".docj", "Root", false, true, embedTextNode(ast.NewNodeID(), "Root body"), map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph"}),
		mergeDoc(t, m, box, "/"+mergeRoot+"/"+mergeA+".docj", "Alpha", false, false, embedTextNode(ast.NewNodeID(), "Alpha body"), map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph"}),
		mergeDoc(t, m, box, "/"+mergeRoot+"/"+mergeB+".docj", "Beta", false, false, embedTextNode(ast.NewNodeID(), "Beta body")),
		mergeDoc(t, m, box, "/"+mergeRoot+"/"+mergeA+"/"+mergeGrand+".docj", "Grand", false, false, focusHeading(ast.NewNodeID(), 4, "Existing heading"), embedTextNode(ast.NewNodeID(), "Grand body")),
		mergeDoc(t, m, box, "/"+mergeRoot+"/"+mergeHidden+".docj", "Hidden", true, false, embedTextNode(ast.NewNodeID(), "Hidden body")),
	}
	before := map[string][]byte{}
	for _, file := range files {
		before[file], _ = os.ReadFile(file)
	}
	return m, box, before
}
func mergeHeadingSummary(tree *parse.Tree) []string {
	out := []string{}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.Type == ast.NodeHeading {
			out = append(out, strings.Repeat("#", n.HeadingLevel)+string(n.FirstChild.Tokens))
		}
		return ast.WalkContinue
	})
	return out
}
func TestHTMLMergeOriginalOrderHiddenLevelsAndUnindexedDocuments(t *testing.T) {
	m, _, before := mergeFixture(t)
	tree, node, bt, err := m.prepareHTMLDocument(mergeRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if node != tree.Root || bt.RootID != mergeRoot || tree.ID != mergeRoot {
		t.Fatal("root identity changed")
	}
	want := []string{"#Alpha", "##Grand", "####Existing heading", "#Beta"}
	if got := mergeHeadingSummary(tree); !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	p := m.referenceRenderer()
	if _, err = p.title(tree); err != nil {
		t.Fatal("merged source candidate rejected", err)
	}
	for file, raw := range before {
		after, _ := os.ReadFile(file)
		if !bytes.Equal(raw, after) {
			t.Fatal("source changed", file)
		}
	}
	again, _, _, err := m.prepareHTMLDocument(mergeRoot, true)
	if err != nil || !reflect.DeepEqual(mergeHeadingSummary(again), want) {
		t.Fatal(err)
	}
	if again == tree || again.Root == tree.Root {
		t.Fatal("candidate reused")
	}
}
func TestHTMLMergeCheckedTreeEnumerationBeforeProjection(t *testing.T) {
	primary := errors.New("html merge tree enumeration incomplete")
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			m, box, _ := mergeFixture(t)
			op := htmlExportTreeIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name != box {
					return source.ReadDir(ctx, name)
				}
				if kind == "incomplete" {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				entries, readErr := source.ReadDir(ctx, name)
				if readErr != nil {
					return nil, readErr
				}
				if err := source.Data.Rename(name, name+".old"); err != nil {
					return nil, err
				}
				if err := source.Data.Mkdir(name, 0700); err != nil {
					return nil, err
				}
				return entries, nil
			}}
			services, closeServices, err := m.mergeServicesWithIO(box, op)
			if err != nil {
				t.Fatal(err)
			}
			defer closeServices()
			files, err := services.list(box, "/")
			if err == nil || files != nil {
				t.Fatal("failed tree enumeration admitted projection", files, err)
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete tree enumeration error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("replaced tree directory accepted", err)
			}
		})
	}
}
func TestHTMLMergeNotebookSortLimitAndCapturedPreferences(t *testing.T) {
	m, box, _ := mergeFixture(t)
	cfg := conf.NewBoxConf()
	cfg.SortMode = util.SortModeCustom
	raw, _ := json.Marshal(cfg)
	archiveWrite(t, m.read.binding.scope.commit.r.data, box+"/.siyuan/conf.json", raw)
	raw, _ = json.Marshal(map[string]int{mergeA: 1, mergeB: 0})
	archiveWrite(t, m.read.binding.scope.commit.r.data, box+"/.siyuan/sort.json", raw)
	Conf.m.Lock()
	Conf.FileTree = nil
	Conf.m.Unlock()
	tree, _, _, err := m.prepareHTMLDocument(mergeRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := mergeHeadingSummary(tree); !reflect.DeepEqual(got, []string{"#Beta", "#Alpha", "##Grand", "####Existing heading"}) {
		t.Fatal(got)
	}
	m.mergeMax = 1
	tree, _, _, err = m.prepareHTMLDocument(mergeRoot, true)
	if err != nil || !reflect.DeepEqual(mergeHeadingSummary(tree), []string{"#Beta"}) {
		t.Fatal(mergeHeadingSummary(tree), err)
	}
}
func TestHTMLMergeEmptyParentsDeepClampAndOriginalBodyHeadings(t *testing.T) {
	m, box := markdownFixture(t)
	m.mergeBound = true
	m.mergeSort = util.SortModeNameASC
	m.mergeMax = 100
	mergeDoc(t, m, box, "/"+mergeRoot+".docj", "Empty", false, true)
	p := "/" + mergeRoot
	for i := 0; i < 8; i++ {
		id := ast.NewNodeID()
		p += "/" + id
		mergeDoc(t, m, box, p+".docj", "Child", false, false, embedTextNode(ast.NewNodeID(), "body"))
	}
	tree, _, _, err := m.prepareHTMLDocument(mergeRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	summary := mergeHeadingSummary(tree)
	if len(summary) != 8 {
		t.Fatal(summary)
	}
	for i, h := range summary {
		level := i + 1
		if level > 6 {
			level = 6
		}
		if h != strings.Repeat("#", level)+"Child" {
			t.Fatal(summary)
		}
	}
	if tree.Root.FirstChild.Type == ast.NodeParagraph && tree.Root.FirstChild.FirstChild == nil {
		t.Fatal("empty insertion point retained")
	}
}
func TestHTMLMergeSourceSnapshotsAndNoCleanupSideEffects(t *testing.T) {
	m, box, before := mergeFixture(t)
	old := archiveWrite(t, m.read.binding.scope.commit.r.data, box+"/"+mergeRoot+"/old.tmp", []byte("old tmp"))
	stamp := time.Now().Add(-time.Hour)
	os.Chtimes(old, stamp, stamp)
	tree, _, _, err := m.prepareHTMLDocument(mergeRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	expected := mergeHeadingSummary(tree)
	child := filepath.Join(m.read.binding.scope.commit.r.data, box, mergeRoot, mergeA+".docj")
	os.WriteFile(child, []byte("later corrupt source"), 0600)
	again, _, _, err := m.prepareHTMLDocument(mergeRoot, true)
	if err != nil || !reflect.DeepEqual(mergeHeadingSummary(again), expected) {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "old tmp" {
		t.Fatal("read-only merge cleaned source tmp")
	}
	for file, raw := range before {
		if file == child {
			continue
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(raw, after) {
			t.Fatal("source changed", file)
		}
	}
}
func TestHTMLMergeOwnedFailuresDiscardCandidatesAndNeverRepairSource(t *testing.T) {
	for _, kind := range []string{"corrupt-child", "linked-child", "bad-sort", "null-sort", "canceled", "closed", "unbound", "sql"} {
		t.Run(kind, func(t *testing.T) {
			m, box, _ := mergeFixture(t)
			child := filepath.Join(m.read.binding.scope.commit.r.data, box, mergeRoot, mergeA+".docj")
			switch kind {
			case "corrupt-child":
				os.WriteFile(child, []byte("corrupt native source"), 0600)
			case "linked-child":
				raw, _ := os.ReadFile(child)
				target := archiveWrite(t, t.TempDir(), "other.docj", raw)
				os.Remove(child)
				if err := os.Symlink(target, child); err != nil {
					t.Fatal(err)
				}
			case "bad-sort", "null-sort":
				cfg := conf.NewBoxConf()
				cfg.SortMode = util.SortModeCustom
				raw, _ := json.Marshal(cfg)
				archiveWrite(t, m.read.binding.scope.commit.r.data, box+"/.siyuan/conf.json", raw)
				value := "bad"
				if kind == "null-sort" {
					value = "null"
				}
				archiveWrite(t, m.read.binding.scope.commit.r.data, box+"/.siyuan/sort.json", []byte(value))
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			case "closed":
				owner := m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			case "sql":
				m.read.binding.scope.commit.r.main.DB.Close()
			case "unbound":
				m.mergeBound = false
			}
			tree, node, bt, err := m.prepareHTMLDocument(mergeRoot, true)
			if err == nil || tree != nil || node != nil || bt != nil {
				t.Fatal(tree, node, bt, err)
			}
			if kind == "corrupt-child" {
				raw, _ := os.ReadFile(child)
				if string(raw) != "corrupt native source" {
					t.Fatal("corrupt source moved or repaired")
				}
			}
		})
	}
}
func TestHTMLMergeLoadFailurePropagationAndClosedPorts(t *testing.T) {
	m, box, _ := mergeFixture(t)
	failure := errors.New("fixture load failure")
	point := &ast.Node{Type: ast.NodeParagraph}
	block := &Block{Children: []*Block{{Box: box, Path: "/" + mergeA + ".docj"}}}
	ports := &exportMergeServices{guard: func() error { return nil }, list: func(string, string) ([]*File, error) { return nil, nil }, load: func(string, string) (*parse.Tree, error) { return nil, failure }}
	if err := walkBlock(point, block, 0, ports); !errors.Is(err, failure) {
		t.Fatal("load failure swallowed", err)
	}
	ports.load = func(string, string) (*parse.Tree, error) { return nil, nil }
	if _, err := loadTreeNodes(box, "/"+mergeA+".docj", 1, ports); err == nil {
		t.Fatal("nil tree accepted")
	}
	if _, err := mergeSubDocs(nil, ports); err == nil {
		t.Fatal("nil root accepted")
	}
	if _, err := mergeSubDocs(&parse.Tree{Root: &ast.Node{Type: ast.NodeDocument}}, &exportMergeServices{}); err == nil {
		t.Fatal("missing ports fell back globally")
	}
	services, closeServices, err := m.mergeServices(box)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = services.list("wrong", "/"); err == nil {
		t.Fatal("other notebook accepted")
	}
	if err = closeServices(); err != nil {
		t.Fatal(err)
	}
	if _, err = services.list(box, "/"+mergeRoot+".docj"); err == nil {
		t.Fatal("closed source read accepted")
	}
	// The no-adapter legacy recursion also forwards a failed source load now.
	block = &Block{Children: []*Block{{Box: box, Path: "/" + ast.NewNodeID() + ".docj"}}}
	if err = walkBlock(point, block, 0); err == nil {
		t.Fatal("legacy load failure still swallowed")
	}
}

func TestHTMLMergeCapturedDefaultsAndLegacyValidProjection(t *testing.T) {
	_, _, _ = mergeFixture(t)
	Conf.m.Lock()
	Conf.FileTree = conf.NewFileTree()
	Conf.FileTree.Sort = util.SortModeNameASC
	Conf.FileTree.MaxListCount = 100
	Conf.m.Unlock()
	captured, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prepared, _, err := captured.prepareFocusedExport(mergeRoot)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := mergeSubDocs(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := mergeHeadingSummary(legacy)
	Conf.m.Lock()
	Conf.FileTree = nil
	Conf.m.Unlock()
	tree, _, _, err := captured.prepareHTMLDocument(mergeRoot, true)
	if err != nil || !reflect.DeepEqual(mergeHeadingSummary(tree), want) {
		t.Fatal(err)
	}
	plain, _, _, err := captured.prepareHTMLDocument(mergeRoot, false)
	if err != nil || len(mergeHeadingSummary(plain)) != 0 {
		t.Fatal(err)
	}
	if captured.mergeSort != util.SortModeNameASC || captured.mergeMax != 100 || !captured.mergeBound {
		t.Fatal("defaults were not captured")
	}
}
