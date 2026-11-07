//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"wiki-kernel/sql"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/conf"
	"wiki-kernel/internal/nativeparser"
)

func preparationFixture(ctx context.Context, tags bool) *markdownImportPreparation {
	options := nativeparser.EditorOptions{Markdown: nativeparser.Defaults(), Emojis: map[string]string{"owned": "captured"}}
	options.Markdown.InlineTag = tags
	return newMarkdownImportPreparation(ctx, options, func() error { return nil })
}
func preparationDocument(t *testing.T, m *markdownImportPreparation, id, hpath, markdown string) *parse.Tree {
	t.Helper()
	tree, _, _, _, err := m.parse([]byte(markdown))
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	tree.Root.ID = id
	tree.Root.SetIALAttr("id", id)
	reassignIDUpdated(tree, id, "")
	tree.HPath = hpath
	tree.Box = "20261031170000-box0001"
	m.trees = append(m.trees, tree)
	return tree
}
func preparationRefs(tree *parse.Tree) map[string]string {
	refs := map[string]string{}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.IsTextMarkType("block-ref") {
			refs[n.TextMarkBlockRefID+":"+n.TextMarkBlockRefSubtype] = n.TextMarkTextContent
		}
		if enter && n.Type == ast.NodeBlockRef {
			id, label, kind := "", "", "d"
			for child := n.FirstChild; child != nil; child = child.Next {
				switch child.Type {
				case ast.NodeBlockRefID:
					id = child.TokensStr()
				case ast.NodeBlockRefText:
					label = child.TokensStr()
					kind = "s"
				case ast.NodeBlockRefDynamicText:
					label = child.TokensStr()
				}
			}
			refs[id+":"+kind] = label
		}
		return ast.WalkContinue
	})
	return refs
}
func TestMarkdownImportPreparationLinksHeadingsTagsAndMissingTargets(t *testing.T) {
	m := preparationFixture(context.Background(), true)
	from := preparationDocument(t, m, "20261031170001-from001", "/Shelf/From", "[[Target]] and [[Target#Section|Named section]] and [Markdown alias](Target.md) and [[Missing]] #topic")
	target := preparationDocument(t, m, "20261031170002-dest001", "/Shelf/Target", "# Section\n\nTarget body")
	heading := target.Root.FirstChild.ID
	if err := m.prepareLinks(); err != nil {
		t.Fatal(err)
	}
	refs := preparationRefs(from)
	if refs[target.ID+":d"] != "Target" || refs[target.ID+":s"] != "Markdown alias" || refs[heading+":s"] != "Named section" {
		t.Fatal("original link/anchor/subtype rules changed", refs)
	}
	tags, missing := 0, false
	ast.Walk(from.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			if n.IsTextMarkType("tag") || n.Type == ast.NodeTag {
				tags++
			}
			if strings.Contains(n.TokensStr(), "[[Missing]]") {
				missing = true
			}
		}
		return ast.WalkContinue
	})
	if tags != 1 || !missing {
		t.Fatal("tag or unresolved reference changed", tags, missing)
	}
}
func TestMarkdownImportPreparationParserSnapshotAndHTMLConversion(t *testing.T) {
	options := nativeparser.EditorOptions{Markdown: nativeparser.Defaults(), Emojis: map[string]string{"owned": "captured"}}
	options.Markdown.InlineTag = false
	m := newMarkdownImportPreparation(context.Background(), options, func() error { return nil })
	options.Emojis["owned"] = "later"
	options.Markdown.InlineTag = true
	tree, id, title, updated, err := m.parse([]byte("---\ndate: '2026-10-31 17:00:03'\ntitle: Preserved front matter\nlastmod: '2026-10-31 17:00:04'\n---\n\n    indented code\n\nhttps://example.invalid\n\n<img src=\"assets/picture.png\" width=\"24\">\n\n<a href=\"Target.md\">HTML alias</a>\n\n#topic"))
	if err != nil || tree == nil || !strings.HasPrefix(id, "20261031170003-") || title != "Preserved front matter" || updated != "20261031170004" {
		t.Fatal(id, title, updated, err)
	}
	tree.HPath = "/Source"
	m.trees = []*parse.Tree{tree}
	if err = m.prepareLinks(); err != nil {
		t.Fatal(err)
	}
	code, images, tag, automaticLinks := 0, 0, 0, 0
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			switch n.Type {
			case ast.NodeCodeBlock:
				code++
			case ast.NodeImage:
				images++
				if n.IALAttr("style") != "width: 24px;" {
					t.Error("HTML image size lost", n.KramdownIAL)
				}
			}
			if n.IsTextMarkType("tag") || n.Type == ast.NodeTag {
				tag++
			}
			if n.IsTextMarkType("a") && strings.HasPrefix(n.TextMarkAHref, "https://example.invalid") {
				automaticLinks++
			}
		}
		return ast.WalkContinue
	})
	if code != 2 || images != 1 || tag != 0 || automaticLinks != 0 || m.options.Emojis["owned"] != "captured" {
		t.Fatal(code, images, tag, automaticLinks, m.options.Emojis)
	}
}
func TestMarkdownImportPreparationConcurrentIndependentNamespaces(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := preparationFixture(context.Background(), i%2 == 0)
			rootID := fmt.Sprintf("20261031170%03d-target1", i)
			// The same human path intentionally belongs to independent import contexts.
			target := preparationDocument(t, m, rootID, "/Same/Target", "# Heading")
			from := preparationDocument(t, m, "20261031170001-from001", "/Same/From", "[[Target]] #topic")
			if err := m.prepareLinks(); err != nil {
				t.Error(err)
				return
			}
			refs := preparationRefs(from)
			if len(refs) != 1 || refs[target.ID+":d"] != "Target" {
				t.Errorf("namespace leaked: %d %v", i, refs)
			}
			m.discard()
			if m.trees != nil || m.links != nil {
				t.Error("invocation state retained")
			}
		}(i)
	}
	wg.Wait()
}
func TestMarkdownImportPreparationCanceledAndOwnerFailureDiscardCandidates(t *testing.T) {
	for _, kind := range []string{"canceled", "during-walk", "owner", "nil-tree", "unbound"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m := preparationFixture(ctx, true)
			preparationDocument(t, m, "20261031170002-dest001", "/Root/Target", "# Header\n\nTarget body")
			preparationDocument(t, m, "20261031170001-from001", "/Root/From", strings.Repeat("[[Target]] text\n\n", 40))
			sentinel := errors.New("captured owner changed")
			switch kind {
			case "canceled":
				cancel()
			case "during-walk":
				count := 0
				m.authorize = func() error {
					count++
					if count == 16 {
						cancel()
					}
					return nil
				}
			case "owner":
				m.authorize = func() error { return sentinel }
			case "nil-tree":
				m.trees = append(m.trees, nil)
			case "unbound":
				m.authorize = nil
			}
			err := m.prepareLinks()
			if err == nil || m.trees != nil || m.links != nil {
				t.Fatal("failure retained a success candidate", err)
			}
			if (kind == "canceled" || kind == "during-walk") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind == "owner" && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if again := m.prepareLinks(); again == nil {
				t.Fatal("failed invocation reused")
			}
		})
	}
}
func TestMarkdownImportPreparationParseCancellationNoResultOrResidue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := preparationFixture(ctx, true)
	preparationDocument(t, m, "20261031170001-from001", "/Source", "previous candidate")
	cancel()
	tree, id, title, updated, err := m.parse([]byte("next"))
	if !errors.Is(err, context.Canceled) || tree != nil || id != "" || title != "" || updated != "" || m.trees != nil || m.links != nil {
		t.Fatal("parse returned candidate after cancel", err)
	}
}
func TestMarkdownImportPreparationCapturedOwnerAndEntryCancellation(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	Conf.m.Lock()
	Conf.Export = conf.NewExport()
	Conf.m.Unlock()
	m, err := captureMarkdownImportPreparation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.archive.binding.scope.commit.r.owner != e.binding.scope.commit.r.owner {
		t.Fatal("owner was not captured")
	}
	old := Conf
	Conf = NewAppConf()
	defer func() { Conf = old }()
	if _, _, _, _, err = m.parse([]byte("must reject stale owner")); err == nil {
		t.Fatal("stale owner accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = ImportFromLocalPath("box", "missing.md", "/", ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestMarkdownImportPreparationCapturesConfigurationBeforeChanges(t *testing.T) {
	archiveEntryFixture(t)
	Conf.m.Lock()
	Conf.Export = conf.NewExport()
	Conf.Editor.Markdown.InlineTag = false
	Conf.m.Unlock()
	m, err := captureMarkdownImportPreparation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	Conf.m.Lock()
	Conf.Editor.Markdown.InlineTag = true
	Conf.m.Unlock()
	tree := preparationDocument(t, m, "20261031170001-from001", "/Source", "#topic")
	if err = m.prepareLinks(); err != nil {
		t.Fatal(err)
	}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && (n.IsTextMarkType("tag") || n.Type == ast.NodeTag) {
			t.Error("captured config followed later settings")
		}
		return ast.WalkContinue
	})
}
func TestMarkdownImportPreparationReadFailureDoesNotLeakIntoNextInvocation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses source permissions")
	}
	e, file, before := archiveEntryFixture(t)
	Conf.m.Lock()
	Conf.Export = conf.NewExport()
	Conf.m.Unlock()
	data := e.binding.scope.commit.r.data
	count := func() int {
		n := 0
		err := filepath.WalkDir(data, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".docj") {
				n++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	original := count()
	input := t.TempDir()
	archiveWrite(t, input, "a.md", []byte("Candidate from failed invocation"))
	bad := archiveWrite(t, input, "b.md", []byte("Unreadable source"))
	os.Chmod(bad, 0000)
	defer os.Chmod(bad, 0600)
	Conf.syncRuntime.same.Store(7)
	err := ImportFromLocalPath(filepath.Base(filepath.Dir(file)), input, "/", context.Background())
	if !errors.Is(err, os.ErrPermission) || count() != original || Conf.syncRuntime.same.Load() != 7 {
		t.Fatal("failed preparation published candidates or hid error", err)
	}
	next := archiveWrite(t, t.TempDir(), "Next.md", []byte("New independent invocation"))
	if err = ImportFromLocalPath(filepath.Base(filepath.Dir(file)), next, "/", context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = (sql.LegacyMainQueue{DB: e.binding.scope.commit.r.main.DB}).Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count() != original+1 {
		t.Fatal("earlier candidates leaked into next import", count(), original)
	}
	if raw, _ := os.ReadFile(file); !bytes.Equal(raw, before) {
		t.Fatal("existing document changed")
	}
}
