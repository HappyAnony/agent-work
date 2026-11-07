package docview

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestVirtualKeywordFilteringAndOriginalMatching(t *testing.T) {
	root := &ast.Node{Type: ast.NodeDocument}
	root.SetIALAttr("title", "Current")
	root.SetIALAttr("name", "RootName")
	root.SetIALAttr("alias", "Alias,Another")
	source := []string{"Current", "RootName", "Alias", "Another", "Keep", "DropMe", "*", ""}
	before := append([]string(nil), source...)
	keys := FilterVirtualKeywords(root, source, VirtualOptions{Enabled: true, Include: "Extra,escaped\\,comma\nKeep", Exclude: "/Drop.*/"})
	for _, bad := range []string{"Current", "RootName", "Alias", "Another", "DropMe", "*", ""} {
		for _, k := range keys {
			if k == bad {
				t.Fatal(keys)
			}
		}
	}
	for _, good := range []string{"Keep", "Extra", "escaped,comma"} {
		found := false
		for _, k := range keys {
			found = found || k == good
		}
		if !found {
			t.Fatal(keys)
		}
	}
	if !reflect.DeepEqual(before, source) {
		t.Fatal("dictionary mutated")
	}
	if got := MatchVirtualKeywords("KEEP", []string{"Keep"}, false); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Fatal(got)
	}
	if got := MatchVirtualKeywords("KEEP", []string{"Keep"}, true); len(got) != 0 {
		t.Fatal(got)
	}
	if got := MarkSplit("Keep KeepSuffix", []string{"Keep"}, "<v>", "</v>", true); got != "<v>Keep</v> KeepSuffix" {
		t.Fatal(got)
	}
}

func TestVirtualRenderingSelfExclusionAnchorsAndCancellation(t *testing.T) {
	for _, kind := range []string{"ordinary", "name", "alias", "referenced", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			parent := &ast.Node{Type: ast.NodeParagraph, ID: "20261027180000-par0001"}
			n := &ast.Node{Type: ast.NodeText, Tokens: []byte("Keep")}
			parent.AppendChild(n)
			if kind == "name" || kind == "alias" {
				parent.SetIALAttr(kind, "Keep")
			}
			refs := map[string]int{}
			if kind == "referenced" || kind == "cancel" {
				refs[parent.ID] = 1
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			text := func(ctx context.Context, _ *ast.Node) (string, error) {
				if e := ctx.Err(); e != nil {
					return "", e
				}
				return "Keep", nil
			}
			var unlinks []*ast.Node
			_, err := ProcessVirtual(ctx, n, &unlinks, []string{"Keep"}, refs, nativeparser.NewEditor(nativeparser.EditorOptions{}), VirtualOptions{Enabled: true, CaseSensitive: true}, text)
			if kind == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (len(unlinks) > 0) != (kind == "ordinary") {
				t.Fatal(kind, unlinks)
			}
			if kind == "ordinary" && !strings.Contains(string(n.Tokens), "virtual-block-ref") {
				t.Fatal(string(n.Tokens))
			}
		})
	}
}
