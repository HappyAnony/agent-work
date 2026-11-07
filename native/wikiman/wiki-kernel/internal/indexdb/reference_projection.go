// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql/database.go and treenode/node.go reference projection algorithms.
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/vitess-sqlparser/sqlparser"
	"strings"
	"wiki-kernel/internal/nodetype"
)

type RefRecord struct {
	ID, DefBlockID, DefBlockParentID, DefBlockRootID, DefBlockPath, BlockID, RootID, Box, Path, Content, Markdown, Type string
}
type AnnotationRecord struct{ ID, FilePath, AnnotationID, BlockID, RootID, Box, Path, Content, Type string }
type BlockLocation struct{ ParentID, RootID, Path string }
type ReferenceLookup func(context.Context, string) (BlockLocation, bool, error)

var ErrProjection = errors.New("projection_dependency_unbound")

// ProjectReferences keeps exit-order traversal, same-block reference de-duplication,
// exact embed SQL recognition, original Markdown and annotation path/content rules.
// The caller lends a detached mutable tree and parser; no global index is read.
func ProjectReferences(ctx context.Context, tree *parse.Tree, parser *lute.Lute, lookup ReferenceLookup) (refs []RefRecord, annotations []AnnotationRecord, err error) {
	if tree == nil || tree.Root == nil || parser == nil || lookup == nil {
		return nil, nil, ErrProjection
	}
	appendRef := func(ref RefRecord) {
		for _, r := range refs {
			if r.DefBlockID == ref.DefBlockID && r.BlockID == ref.BlockID {
				return
			}
		}
		refs = append(refs, ref)
	}
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if err = ctx.Err(); err != nil {
			return ast.WalkStop
		}
		if entering {
			return ast.WalkContinue
		}
		switch {
		case (n.Type == ast.NodeTextMark && n.IsTextMarkType("block-ref")) || n.Type == ast.NodeBlockRef:
			r, e := ProjectReference(ctx, tree, n, parser, lookup)
			if e != nil {
				err = e
				return ast.WalkStop
			}
			appendRef(r)
		case n.Type == ast.NodeTextMark && n.IsTextMarkType("file-annotation-ref"):
			at := strings.LastIndex(n.TextMarkFileAnnotationRefID, "/")
			if at < 0 {
				return ast.WalkContinue
			}
			file, annotation := n.TextMarkFileAnnotationRefID[:at], n.TextMarkFileAnnotationRefID[at+1:]
			content := n.TextMarkTextContent
			if content == "" {
				content = file
			}
			parent := parentBlock(n)
			if parent == nil {
				err = ErrProjection
				return ast.WalkStop
			}
			annotations = append(annotations, AnnotationRecord{ID: ast.NewNodeID(), FilePath: file, AnnotationID: annotation, BlockID: parent.ID, RootID: tree.ID, Box: tree.Box, Path: tree.Path, Content: content, Type: nodetype.TypeAbbr(n.Type.String())})
		default:
			id := embedReference(n)
			if id == "" {
				return ast.WalkContinue
			}
			location, _, lookupErr := lookup(ctx, id)
			if lookupErr != nil {
				err = lookupErr
				return ast.WalkStop
			}
			appendRef(RefRecord{ID: ast.NewNodeID(), DefBlockID: id, DefBlockParentID: location.ParentID, DefBlockRootID: location.RootID, DefBlockPath: location.Path, BlockID: n.ID, RootID: tree.ID, Box: tree.Box, Path: tree.Path, Type: nodetype.TypeAbbr(n.Type.String())})
		}
		return ast.WalkContinue
	})
	return
}

func parentBlock(node *ast.Node) *ast.Node {
	for p := node.Parent; p != nil; p = p.Parent {
		if p.ID != "" && p.IsBlock() {
			return p
		}
	}
	return nil
}

// The original restricted SELECT/WHERE id = 'node-id' recognition, not arbitrary
// SQL execution and not a new generalized embed query grammar.
func embedReference(n *ast.Node) string {
	if n == nil || n.Type != ast.NodeBlockQueryEmbed {
		return ""
	}
	script := n.ChildByType(ast.NodeBlockQueryEmbedScript)
	if script == nil {
		return ""
	}
	stmt, err := sqlparser.Parse(script.TokensStr())
	if err != nil {
		return ""
	}
	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok || selectStmt.Where == nil {
		return ""
	}
	compare, ok := selectStmt.Where.Expr.(*sqlparser.ComparisonExpr)
	if !ok {
		return ""
	}
	if column, ok := compare.Left.(*sqlparser.ColName); ok && (column == nil || column.Name.Lowered() != "id") {
		return ""
	}
	value, ok := compare.Right.(*sqlparser.SQLVal)
	if !ok || value == nil || value.Type != sqlparser.StrVal || !ast.IsNodeIDPattern(string(value.Val)) {
		return ""
	}
	return string(value.Val)
}

func EmbeddedReference(n *ast.Node) string { return embedReference(n) }

func InsertBlockReferences(ctx context.Context, tx *sql.Tx, refs []RefRecord) error {
	for start := 0; start < len(refs); start += 512 {
		end := min(start+512, len(refs))
		args := make([]interface{}, 0, (end-start)*12)
		for _, r := range refs[start:end] {
			args = append(args, r.ID, r.DefBlockID, r.DefBlockParentID, r.DefBlockRootID, r.DefBlockPath, r.BlockID, r.RootID, r.Box, r.Path, r.Content, r.Markdown, r.Type)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO refs (id,def_block_id,def_block_parent_id,def_block_root_id,def_block_path,block_id,root_id,box,path,content,markdown,type) VALUES "+strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?,?),", end-start), ","), args...); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func InsertAnnotationReferences(ctx context.Context, tx *sql.Tx, refs []AnnotationRecord) error {
	for start := 0; start < len(refs); start += 512 {
		end := min(start+512, len(refs))
		args := make([]interface{}, 0, (end-start)*9)
		for _, r := range refs[start:end] {
			args = append(args, r.ID, r.FilePath, r.AnnotationID, r.BlockID, r.RootID, r.Box, r.Path, r.Content, r.Type)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO file_annotation_refs (id,file_path,annotation_id,block_id,root_id,box,path,content,type) VALUES "+strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?,?,?,?),", end-start), ","), args...); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func ReplaceReferences(ctx context.Context, tx *sql.Tx, box, path string, refs []RefRecord, annotations []AnnotationRecord) error {
	if err := ApplyMainMutation(ctx, tx, MainMutation{Action: "delete_refs", Box: box, Path: path}, false); err != nil {
		return err
	}
	if err := InsertBlockReferences(ctx, tx, refs); err != nil {
		return err
	}
	return InsertAnnotationReferences(ctx, tx, annotations)
}

// ProjectReference is also the original immediate CacheRef business, not a
// whole-tree rewalk that would collapse distinct references in the same block.
func ProjectReference(ctx context.Context, tree *parse.Tree, n *ast.Node, parser *lute.Lute, lookup ReferenceLookup) (RefRecord, error) {
	if tree == nil || n == nil || parser == nil || lookup == nil {
		return RefRecord{}, ErrProjection
	}
	if err := ctx.Err(); err != nil {
		return RefRecord{}, err
	}
	// Temporary type normalization is always restored, including export errors.
	original := n.TextMarkType
	n.TextMarkType = "block-ref"
	markdown, exportErr := lute.ProtyleExportMdNodeSync(n, parser.ParseOptions, parser.RenderOptions)
	n.TextMarkType = original
	if exportErr != nil {
		return RefRecord{}, exportErr
	}
	location, _, lookupErr := lookup(ctx, n.TextMarkBlockRefID)
	if lookupErr != nil {
		return RefRecord{}, lookupErr
	}
	parent := parentBlock(n)
	if parent == nil {
		return RefRecord{}, ErrProjection
	}
	return RefRecord{ID: ast.NewNodeID(), DefBlockID: n.TextMarkBlockRefID, DefBlockParentID: location.ParentID, DefBlockRootID: location.RootID, DefBlockPath: location.Path, BlockID: parent.ID, RootID: tree.ID, Box: tree.Box, Path: tree.Path, Content: n.TextMarkTextContent, Markdown: markdown, Type: nodetype.TypeAbbr(n.Type.String())}, nil
}
