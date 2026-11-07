package sql

import (
	"testing"

	"github.com/88250/lute/parse"
	"wiki-kernel/internal/nativeparser"
)

func TestOriginalSQLProjectionOwnsParser(t *testing.T) {
	settings := nativeparser.Defaults()
	a, b := newTreeProjection(settings), newTreeProjection(settings)
	if a.parser == b.parser || a.parser.ParseOptions == b.parser.ParseOptions {
		t.Fatal("shared parser")
	}
	if a.parser.RenderOptions.KramdownBlockIAL {
		t.Fatal("SQL Markdown rule lost")
	}
	a.parser.SetTag(false)
	if !b.parser.ParseOptions.Tag {
		t.Fatal("options leaked")
	}
	tree := parse.Parse("synthetic", []byte("# Heading\n\nhello **world**\n"), b.parser.ParseOptions)
	tree.Box, tree.Path, tree.HPath = "20261021000000-aaaaaaa", "/20261021000000-bbbbbbb.docj", "/synthetic"
	blocks, _, _, _ := b.fromTree(tree.Root, tree)
	if len(blocks) < 2 {
		t.Fatal("original node projector lost blocks", len(blocks))
	}
	for _, block := range blocks {
		if block.Box != tree.Box || block.Path != tree.Path {
			t.Fatal("projection identity lost", block)
		}
	}
}
