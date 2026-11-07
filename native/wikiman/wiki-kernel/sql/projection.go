package sql

import (
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

// treeProjection owns the mutable parser used by the original SQL metadata
// algorithms. It is operation-scoped; neither an import-time singleton nor a
// workspace selector. DB/query/treenode dependencies still need R02 migration.
type treeProjection struct{ parser *lute.Lute }

func newTreeProjection(markdown nativeparser.Markdown) *treeProjection {
	return &treeProjection{parser: nativeparser.NewIndex(markdown)}
}

// Transitional facade only: contextual services must supply their own settings.
func legacyTreeProjection() *treeProjection {
	return newTreeProjection(*util.MarkdownSettings)
}

func refsFromTree(tree *parse.Tree) ([]*Ref, []*FileAnnotationRef) {
	return legacyTreeProjection().refsFromTree(tree)
}
func buildRef(tree *parse.Tree, node *ast.Node) *Ref {
	return legacyTreeProjection().buildRef(tree, node)
}
func fromTree(node *ast.Node, tree *parse.Tree) ([]*Block, []*Span, []*Asset, []*Attribute) {
	return legacyTreeProjection().fromTree(node, tree)
}
