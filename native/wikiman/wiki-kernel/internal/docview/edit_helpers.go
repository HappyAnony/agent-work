// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	ast "github.com/88250/lute/ast"
	sqlparser "github.com/88250/vitess-sqlparser/sqlparser"
)

func MoveFoldHeading(updateNode, oldNode *ast.Node) {
	foldHeadings := map[string][]*ast.Node{}
	// 找到原有节点中所有折叠标题节点的下方节点
	ast.Walk(oldNode, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeHeading == n.Type && "1" == n.IALAttr("fold") {
			children := HeadingChildren(n)
			foldHeadings[n.ID] = children
		}
		return ast.WalkContinue
	})

	// 将原来所有折叠标题对应的下方节点移动到新节点下
	var updateFoldHeadings []*ast.Node
	ast.Walk(updateNode, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeHeading == n.Type && "1" == n.IALAttr("fold") {
			updateFoldHeadings = append(updateFoldHeadings, n)
		}
		return ast.WalkContinue
	})
	for _, h := range updateFoldHeadings {
		children := foldHeadings[h.ID]
		for i := len(children) - 1; 0 <= i; i-- {
			h.Next.InsertAfter(children[i]) // Next 是 Block IAL
		}
	}
	return
}

func GetHeadingFold(nodes []*ast.Node) (ret []*ast.Node) {
	for _, n := range nodes {
		if "1" == n.IALAttr("heading-fold") {
			ret = append(ret, n)
		}
	}
	return
}

func GetParentFoldedHeading(node *ast.Node) (parentFoldedHeading *ast.Node) {
	if nil == node {
		return
	}

	currentLevel := 7
	if ast.NodeHeading == node.Type {
		currentLevel = node.HeadingLevel
	}
	for n := node.Previous; nil != n; n = n.Previous {
		if ast.NodeHeading != n.Type {
			continue
		}

		if n.HeadingLevel >= currentLevel {
			continue
		}
		currentLevel = n.HeadingLevel

		if "1" == n.IALAttr("fold") {
			if ast.NodeHeading != node.Type {
				parentFoldedHeading = n
			}
			if n.HeadingLevel < node.HeadingLevel {
				parentFoldedHeading = n
			}
		}
	}
	return
}

func GetEmbedBlockRef(embedNode *ast.Node) (blockRefID string) {
	if nil == embedNode || ast.NodeBlockQueryEmbed != embedNode.Type {
		return
	}

	scriptNode := embedNode.ChildByType(ast.NodeBlockQueryEmbedScript)
	if nil == scriptNode {
		return
	}

	stmt := scriptNode.TokensStr()
	parsedStmt, err := sqlparser.Parse(stmt)
	if err != nil {
		return
	}

	switch parsedStmt.(type) {
	case *sqlparser.Select:
		slct := parsedStmt.(*sqlparser.Select)
		if nil == slct.Where || nil == slct.Where.Expr {
			return
		}

		switch slct.Where.Expr.(type) {
		case *sqlparser.ComparisonExpr: // WHERE id = '20060102150405-1a2b3c4'
			comp := slct.Where.Expr.(*sqlparser.ComparisonExpr)
			switch comp.Left.(type) {
			case *sqlparser.ColName:
				col := comp.Left.(*sqlparser.ColName)
				if nil == col || "id" != col.Name.Lowered() {
					return
				}
			}
			switch comp.Right.(type) {
			case *sqlparser.SQLVal:
				val := comp.Right.(*sqlparser.SQLVal)
				if nil == val || sqlparser.StrVal != val.Type {
					return
				}

				idVal := string(val.Val)
				if !ast.IsNodeIDPattern(idVal) {
					return
				}
				blockRefID = idVal
			}
		}
	}
	return
}

func IsEmbedBlockRef(n *ast.Node) bool {
	return "" != GetEmbedBlockRef(n)
}

func NewParagraph(id string) (ret *ast.Node) {
	newID := id
	if "" == newID {
		newID = ast.NewNodeID()
	}
	ret = &ast.Node{ID: newID, Type: ast.NodeParagraph}
	ret.SetIALAttr("id", newID)
	ret.SetIALAttr("updated", newID[:14])
	return
}
