// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import (
	bytes "bytes"
	context "context"
	errors "errors"
	fmt "fmt"
	gulu "github.com/88250/gulu"
	lute "github.com/88250/lute"
	ast "github.com/88250/lute/ast"
	editor "github.com/88250/lute/editor"
	lex "github.com/88250/lute/lex"
	parse "github.com/88250/lute/parse"
	slices "slices"
	strconv "strconv"
	strings "strings"
	av "wiki-kernel/internal/avmodel"
	doclife "wiki-kernel/internal/doclife"
	docview "wiki-kernel/internal/docview"
	treenode "wiki-kernel/internal/docview"
	util "wiki-kernel/internal/markup"
)

type BlockLocation struct{ ID, RootID, ParentID, BoxID, Path, HPath, Updated, Type string }
type Failure struct {
	Code    int
	Msg, ID string
}

func (f *Failure) Error() string { return f.Msg }

type DocumentTransaction struct {
	Environment                     *DocumentEnvironment
	Trees                           map[string]*parse.Tree
	Nodes                           map[string]*ast.Node
	LuteEngine                      *lute.Lute
	RelatedAvIDs, ReloadAfterCommit []string
	DoOperations, UndoOperations    []*Operation
}

func (tx *DocumentTransaction) Report(level, format string, args ...any) {
	if tx.Environment.Diagnostic != nil {
		tx.Environment.Diagnostic(level, format, args...)
	}
}

type DocumentEnvironment struct {
	ReadGuard           func() error
	LookupBlock         func(string) (*BlockLocation, error)
	ApplyHeadingEffects func(*parse.Tree) error
	Diagnostic          func(string, string, ...any)
	AvNames             func(context.Context, string) (string, error)
	CacheIAL            func(string, map[string]string)
	Ctx                 context.Context
	DeleteAV            func(context.Context, *ast.Node, *parse.Tree, *DocumentTransaction) error
	DeleteBlockTree     func(context.Context, string) error
	DeleteReferences    func(context.Context, ...string) error
	EditorParser        func() *lute.Lute
	InsertAVBind        func(context.Context, *ast.Node, *parse.Tree, *DocumentTransaction) error
	InsertAVRegistry    func(context.Context, *ast.Node) error
	InsertAssets        func(context.Context, *parse.Tree, *DocumentTransaction) error
	InsertReferences    func(context.Context, ...string) error
	Legacy              bool
	Load                func(string, string, *lute.Lute) (*parse.Tree, error)
	MoveHistory         func(context.Context, *parse.Tree) error
	MoveReferences      func(context.Context, ...string) error
	Now                 func() string
	OutlineHistory      func(context.Context, *parse.Tree) error
	ReferenceCounts     func(context.Context, []string) (map[string]int, error)
	ReloadUI            func()
	Stage               func(*parse.Tree) error
	UpdateAV            func(context.Context, *ast.Node, *parse.Tree, *DocumentTransaction) error
	UpdateCacheRef      func(context.Context, *parse.Tree, *ast.Node) error
	UpdateDynamicText   func(context.Context, string) (string, error)
	UpdateEvent         func(context.Context, []*DocumentTransaction) error
	UpdateReferences    func(context.Context, ...string) error
	UpdateUnfold        func(context.Context, *ast.Node, *ast.Node) error
}

func (tx *DocumentTransaction) DoFoldHeading(operation *Operation) (ret *Failure) {
	headingID := operation.ID
	tree, err := tx.LoadTree(headingID)
	if err != nil {
		return &Failure{Code: TxErrCodeBlockNotFound, ID: headingID}
	}

	childrenIDs := []string{} // 这里不能用 nil，否则折叠下方没内容的标题时会内核中断 https://github.com/siyuan-note/siyuan/issues/3643
	heading := treenode.GetNodeInTree(tree, headingID)
	if nil == heading {
		return &Failure{Code: TxErrCodeBlockNotFound, ID: headingID}
	}

	children := treenode.HeadingChildren(heading)
	for _, child := range children {
		childrenIDs = append(childrenIDs, child.ID)
		ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering || !n.IsBlock() {
				return ast.WalkContinue
			}

			n.SetIALAttr("fold", "1")
			n.SetIALAttr("heading-fold", "1")
			return ast.WalkContinue
		})
	}
	heading.SetIALAttr("fold", "1")
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: headingID}
	}
	tx.Environment.CacheIAL(headingID, parse.IAL2Map(heading.KramdownIAL))
	for _, child := range children {
		tx.Environment.CacheIAL(child.ID, parse.IAL2Map(child.KramdownIAL))
	}
	if err = tx.Environment.ApplyHeadingEffects(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: headingID}
	}
	operation.RetData = childrenIDs
	return
}

func (tx *DocumentTransaction) DoUnfoldHeading(operation *Operation) (ret *Failure) {
	headingID := operation.ID

	tree, err := tx.LoadTree(headingID)
	if err != nil {
		return &Failure{Code: TxErrCodeBlockNotFound, ID: headingID}
	}

	heading := treenode.GetNodeInTree(tree, headingID)
	if nil == heading {
		return &Failure{Code: TxErrCodeBlockNotFound, ID: headingID}
	}

	luteEngine := tx.Environment.EditorParser()
	parentFoldedHeading := treenode.GetParentFoldedHeading(heading)
	if nil != parentFoldedHeading {
		// 如果当前标题在上方某个折叠的标题下方，则展开上方那个折叠标题以保持一致性
		children := treenode.HeadingChildren(parentFoldedHeading)
		for _, child := range children {
			ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !entering || !n.IsBlock() {
					return ast.WalkContinue
				}

				n.RemoveIALAttr("heading-fold")
				n.RemoveIALAttr("fold")
				return ast.WalkContinue
			})
		}
		parentFoldedHeading.RemoveIALAttr("fold")
		parentFoldedHeading.RemoveIALAttr("heading-fold")
		tx.ReloadAfterCommit = append(tx.ReloadAfterCommit, tree.ID)
	}

	children := treenode.HeadingChildren(heading)
	for _, child := range children {
		ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering {
				return ast.WalkContinue
			}

			n.RemoveIALAttr("heading-fold")
			n.RemoveIALAttr("fold")
			return ast.WalkContinue
		})
	}
	heading.RemoveIALAttr("fold")
	heading.RemoveIALAttr("heading-fold")
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: headingID}
	}
	tx.Environment.CacheIAL(headingID, parse.IAL2Map(heading.KramdownIAL))
	for _, child := range children {
		tx.Environment.CacheIAL(child.ID, parse.IAL2Map(child.KramdownIAL))
	}
	if err = tx.Environment.ApplyHeadingEffects(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: headingID}
	}

	// 展开折叠的标题后显示块引用计数 Display reference counts after unfolding headings https://github.com/siyuan-note/siyuan/issues/13618
	if err = FillBlockRefCountWithQuery(children, func(ids []string) (map[string]int, error) {
		return tx.Environment.ReferenceCounts(tx.Environment.Ctx, ids)
	}, tx.Environment.Ctx); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: headingID}
	}

	html, err := RenderBlockDOMWithNames(children, luteEngine, func(ids string) (string, error) { return tx.Environment.AvNames(tx.Environment.Ctx, ids) }, tx.Environment.Ctx)
	if err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: headingID}
	}
	operation.RetData = html
	return
}

func (tx *DocumentTransaction) DoMoveOutlineHeading(operation *Operation) *Failure {
	e := tx.Environment
	if e == nil || e.Ctx == nil || e.OutlineHistory == nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: "outline_history_unbound", ID: operation.ID}
	}
	tree, err := tx.LoadTree(operation.ID)
	if err != nil {
		return &Failure{Code: TxErrCodeBlockNotFound, Msg: err.Error(), ID: operation.ID}
	}
	operation.RetData = tree.Root.ID
	changed, err := doclife.MoveHeading(e.Ctx, tree, doclife.HeadingMoveRequest{ID: operation.ID, PreviousID: operation.PreviousID, ParentID: operation.ParentID}, e.OutlineHistory)
	if err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: operation.ID}
	}
	if changed {
		if err = tx.WriteTree(tree); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: operation.ID}
		}
	}
	return nil
}

func FillBlockRefCountWithQuery(nodes []*ast.Node, query func([]string) (map[string]int, error), contexts ...context.Context) error {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	return docview.FillReferenceCounts(ctx, nodes, func(c context.Context, ids []string) (map[string]int, error) { return query(ids) })
}

func RenderBlockDOMWithNames(nodes []*ast.Node, parser *lute.Lute, names func(string) (string, error), contexts ...context.Context) (string, error) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	return docview.RenderNodes(ctx, nodes, parser, func(c context.Context, ids string) (string, error) { return names(ids) })
}

const (
	TxErrCodeBlockNotFound   = 0
	TxErrCodeDataIsSyncing   = 1
	TxErrCodeWriteTree       = 2
	TxErrHandleAttributeView = 3
	TxErrCodeAfterCommit     = 4
)

func (tx *DocumentTransaction) ProcessUndoInsertWithFoldedHeading() {
	// 删除折叠标题后撤销，需要调整 insert 顺序和 previousID
	// https://github.com/siyuan-note/siyuan/issues/16120

	if 1 > len(tx.DoOperations) {
		return
	}

	// 所有操作均为 insert 才处理
	for _, op := range tx.DoOperations {
		if "insert" != op.Action {
			return
		}
	}

	// 找到 ignoreProcess=true 的区间 [j, k]
	var j, k int
	for i := 0; i < len(tx.DoOperations); i++ {
		op := tx.DoOperations[i]
		ignoreProcess := false
		if nil != op.Context["ignoreProcess"] {
			var convErr error
			ignoreProcess, convErr = strconv.ParseBool(op.Context["ignoreProcess"].(string))
			if nil != convErr {
				tx.Report("error", "parse ignoreProcess failed: %s", convErr)
				return
			}
		}

		if !ignoreProcess {
			if 0 != j && 0 != k {
				break
			}
			continue
		}

		if 0 == j && 0 == k {
			j = i
		}
		k = i
	}

	// 调整 [j, k] 区间内的操作顺序和 previousID
	for x := j; x <= k; x++ {
		opx := tx.DoOperations[x]
		opx.PreviousID = tx.DoOperations[k].PreviousID
	}
	slices.Reverse(tx.DoOperations[j : k+1])
}

func LargeInsertAnchor(ops []*Operation) (string, bool) {
	if len(ops) <= 128 || ops[len(ops)-1] == nil || ops[len(ops)-1].Action != "delete" {
		return "", false
	}
	previous := ""
	for _, op := range ops[:len(ops)-1] {
		if op == nil || op.Action != "insert" || op.PreviousID == "" {
			return "", false
		}
		if previous == "" {
			previous = op.PreviousID
		} else if previous != op.PreviousID {
			return "", false
		}
	}
	return previous, true
}

func (tx *DocumentTransaction) ProcessLargeInsert() (bool, *Failure) {
	previous, large := LargeInsertAnchor(tx.DoOperations)
	if !large {
		return false, nil
	}
	return true, tx.DoLargeInsert(previous)
}

func (tx *DocumentTransaction) DoMove(operation *Operation) (ret *Failure) {
	var err error
	updated := tx.Environment.Now()
	id := operation.ID
	srcTree, err := tx.LoadTree(id)
	if err != nil {
		tx.Report("error", "load tree [%s] failed: %s", id, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	srcNode := treenode.GetNodeInTree(srcTree, id)
	if nil == srcNode {
		tx.Report("error", "get node [%s] in tree [%s] failed", id, srcTree.Root.ID)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	// 生成文档历史 https://github.com/siyuan-note/siyuan/issues/14359
	if err = tx.Environment.MoveHistory(tx.Environment.Ctx, srcTree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}

	var headingChildren []*ast.Node
	if isMovingFoldHeading := ast.NodeHeading == srcNode.Type && "1" == srcNode.IALAttr("fold"); isMovingFoldHeading {
		headingChildren = treenode.HeadingChildren(srcNode)
		// Blocks below other non-folded headings are no longer moved when moving a folded heading https://github.com/siyuan-note/siyuan/issues/8321
		headingChildren = treenode.GetHeadingFold(headingChildren)
	}

	var srcEmptyList *ast.Node
	if ast.NodeListItem == srcNode.Type && srcNode.Parent.FirstChild == srcNode && srcNode.Parent.LastChild == srcNode {
		// 列表中唯一的列表项被移除后，该列表就为空了
		srcEmptyList = srcNode.Parent
	}

	targetPreviousID := operation.PreviousID
	targetParentID := operation.ParentID
	if "" != targetPreviousID {
		if id == targetPreviousID {
			return
		}

		var targetTree *parse.Tree
		targetTree, err = tx.LoadTree(targetPreviousID)
		if err != nil {
			tx.Report("error", "load tree [%s] failed: %s", targetPreviousID, err)
			return &Failure{Code: TxErrCodeBlockNotFound, ID: targetPreviousID}
		}
		isSameTree := srcTree.ID == targetTree.ID
		if isSameTree {
			targetTree = srcTree
		}

		targetNode := treenode.GetNodeInTree(targetTree, targetPreviousID)
		if nil == targetNode {
			tx.Report("error", "get node [%s] in tree [%s] failed", targetPreviousID, targetTree.Root.ID)
			return &Failure{Code: TxErrCodeBlockNotFound, ID: targetPreviousID}
		}

		if ast.NodeHeading == targetNode.Type && "1" == targetNode.IALAttr("fold") {
			targetChildren := treenode.HeadingChildren(targetNode)
			targetChildren = treenode.GetHeadingFold(targetChildren)

			if l := len(targetChildren); 0 < l {
				targetNode = targetChildren[l-1]
			}
		}

		if IsMovingFoldHeadingIntoSelf(targetNode, headingChildren) {
			return
		}

		if IsMovingParentIntoChild(srcNode, targetNode) {
			return
		}

		if 0 < len(headingChildren) {
			// 折叠标题再编辑形成外层列表（前面加上 * ）时，前端给的 tx 序列会形成死循环，在这里解开
			// Nested lists cause hang after collapsing headings https://github.com/siyuan-note/siyuan/issues/15943
			lastChild := headingChildren[len(headingChildren)-1]
			if "1" == lastChild.IALAttr("heading-fold") && ast.NodeList == lastChild.Type &&
				nil != lastChild.FirstChild && nil != lastChild.FirstChild.FirstChild && lastChild.FirstChild.FirstChild.ID == targetPreviousID {
				ast.Walk(lastChild, func(n *ast.Node, entering bool) ast.WalkStatus {
					if !entering || !n.IsBlock() {
						return ast.WalkContinue
					}

					n.RemoveIALAttr("heading-fold")
					n.RemoveIALAttr("fold")
					return ast.WalkContinue
				})
				headingChildren = headingChildren[:len(headingChildren)-1]
			}
		}

		for i := len(headingChildren) - 1; -1 < i; i-- {
			c := headingChildren[i]
			targetNode.InsertAfter(c)
		}
		targetNode.InsertAfter(srcNode)
		if nil != srcEmptyList {
			srcEmptyList.Unlink()
		}

		RefreshUpdatedAt(srcNode, updated)
		tx.Nodes[srcNode.ID] = srcNode
		RefreshUpdatedAt(srcTree.Root, updated)
		if err = tx.WriteTree(srcTree); err != nil {
			if !tx.Environment.Legacy {
				return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
			}
			return
		}
		if !isSameTree {
			if err = tx.WriteTree(targetTree); err != nil {
				if !tx.Environment.Legacy {
					return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
				}
				return
			}
			if err = tx.Environment.MoveReferences(tx.Environment.Ctx, srcTree.ID, srcNode.ID); err != nil {
				return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
			}
		}
		return
	}

	if id == targetParentID {
		return
	}

	targetTree, err := tx.LoadTree(targetParentID)
	if err != nil {
		tx.Report("error", "load tree [%s] failed: %s", targetParentID, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: targetParentID}
	}
	isSameTree := srcTree.ID == targetTree.ID
	if isSameTree {
		targetTree = srcTree
	}

	targetNode := treenode.GetNodeInTree(targetTree, targetParentID)
	if nil == targetNode {
		tx.Report("error", "get node [%s] in tree [%s] failed", targetParentID, targetTree.Root.ID)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: targetParentID}
	}

	if IsMovingFoldHeadingIntoSelf(targetNode, headingChildren) {
		return
	}

	if IsMovingParentIntoChild(srcNode, targetNode) {
		return
	}

	processed := false
	if ast.NodeSuperBlock == targetNode.Type {
		// 在布局节点后插入
		targetNode = targetNode.FirstChild.Next
		for i := len(headingChildren) - 1; -1 < i; i-- {
			c := headingChildren[i]
			targetNode.InsertAfter(c)
		}
		targetNode.InsertAfter(srcNode)
		if nil != srcEmptyList {
			srcEmptyList.Unlink()
		}
		processed = true
	} else if ast.NodeListItem == targetNode.Type {
		if 3 == targetNode.ListData.Typ {
			// 在任务列表标记节点后插入
			targetNode = targetNode.FirstChild
			for i := len(headingChildren) - 1; -1 < i; i-- {
				c := headingChildren[i]
				targetNode.InsertAfter(c)
			}
			targetNode.InsertAfter(srcNode)
			if nil != srcEmptyList {
				srcEmptyList.Unlink()
			}
			processed = true
		}
	}

	if !processed {
		for i := len(headingChildren) - 1; -1 < i; i-- {
			c := headingChildren[i]
			targetNode.PrependChild(c)
		}

		targetNode.PrependChild(srcNode)
		if nil != srcEmptyList {
			srcEmptyList.Unlink()
		}
	}

	RefreshUpdatedAt(srcNode, updated)
	tx.Nodes[srcNode.ID] = srcNode
	RefreshUpdatedAt(srcTree.Root, updated)
	if err = tx.WriteTree(srcTree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}
	if !isSameTree {
		if err = tx.WriteTree(targetTree); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
		if err = tx.Environment.MoveReferences(tx.Environment.Ctx, srcTree.ID, srcNode.ID); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
	}
	return
}

func IsMovingFoldHeadingIntoSelf(targetNode *ast.Node, headingChildren []*ast.Node) bool {
	for _, headingChild := range headingChildren {
		if headingChild.ID == targetNode.ID {
			// 不能将折叠标题移动到自己下方节点的前或后 https://github.com/siyuan-note/siyuan/issues/7163
			return true
		}
	}
	return false
}

func IsMovingParentIntoChild(srcNode, targetNode *ast.Node) bool {
	for parent := targetNode.Parent; nil != parent; parent = parent.Parent {
		if parent.ID == srcNode.ID {
			return true
		}
	}
	return false
}

func (tx *DocumentTransaction) DoPrependInsert(operation *Operation) (ret *Failure) {
	var err error
	block, err := tx.Environment.LookupBlock(operation.ParentID)
	if err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: operation.ParentID}
	}
	if nil == block {
		tx.Report("error", "not found block [%s]", operation.ParentID)
		if tx.Environment.Legacy {
			tx.Environment.ReloadUI()
		} else {
			return &Failure{Code: TxErrCodeBlockNotFound, ID: operation.ParentID}
		} // 比如分屏后编辑器状态不一致，这里强制重新载入界面
		return
	}
	tree, err := tx.LoadTree(block.ID)
	if err != nil {
		msg := fmt.Sprintf("load tree [%s] failed: %s", block.ID, err)
		tx.Report("error", "%s", msg)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: block.ID}
	}

	data := strings.ReplaceAll(operation.Data.(string), editor.FrontEndCaret, "")
	subTree := tx.LuteEngine.BlockDOM2Tree(data)
	insertedNode := subTree.Root.FirstChild
	if nil == insertedNode {
		return &Failure{Code: TxErrCodeBlockNotFound, Msg: "invalid data tree", ID: block.ID}
	}
	if "" == insertedNode.ID {
		insertedNode.ID = ast.NewNodeID()
		insertedNode.SetIALAttr("id", insertedNode.ID)
	}
	var toInserts []*ast.Node
	for toInsert := insertedNode; nil != toInsert; toInsert = toInsert.Next {
		if ast.NodeKramdownBlockIAL != toInsert.Type {
			if "" == toInsert.ID {
				toInsert.ID = ast.NewNodeID()
				toInsert.SetIALAttr("id", toInsert.ID)
			}
			toInserts = append(toInserts, toInsert)
		}
	}

	node := treenode.GetNodeInTree(tree, operation.ParentID)
	if nil == node {
		tx.Report("error", "get node [%s] in tree [%s] failed", operation.ParentID, tree.Root.ID)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: operation.ParentID}
	}
	isContainer := node.IsContainerBlock()
	slices.Reverse(toInserts)

	for _, toInsert := range toInserts {
		if isContainer {
			if ast.NodeList == node.Type {
				// 列表下只能挂列表项，所以这里需要分情况处理
				if ast.NodeList == toInsert.Type {
					var childLis []*ast.Node
					for childLi := toInsert.FirstChild; nil != childLi; childLi = childLi.Next {
						childLis = append(childLis, childLi)
					}
					for i := len(childLis) - 1; -1 < i; i-- {
						node.PrependChild(childLis[i])
					}
				} else {
					newLiID := ast.NewNodeID()
					newLi := &ast.Node{ID: newLiID, Type: ast.NodeListItem, ListData: &ast.ListData{Typ: node.ListData.Typ}}
					newLi.SetIALAttr("id", newLiID)
					node.PrependChild(newLi)
					newLi.AppendChild(toInsert)
				}
			} else if ast.NodeSuperBlock == node.Type {
				layout := node.ChildByType(ast.NodeSuperBlockLayoutMarker)
				if nil != layout {
					layout.InsertAfter(toInsert)
				} else {
					node.FirstChild.InsertAfter(toInsert)
				}
			} else {
				node.PrependChild(toInsert)
			}
		} else {
			node.InsertAfter(toInsert)
		}

		CreatedUpdatedWithCache(toInsert, tx.Environment.CacheIAL)
		tx.Nodes[toInsert.ID] = toInsert
	}

	CreatedUpdatedWithCache(insertedNode, tx.Environment.CacheIAL)
	tx.Nodes[insertedNode.ID] = insertedNode
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: block.ID}
	}

	operation.ID = insertedNode.ID
	operation.ParentID = insertedNode.Parent.ID

	// 将 prependInsert 转换为 insert 推送
	operation.Action = "insert"
	if nil != insertedNode.Previous {
		operation.PreviousID = insertedNode.Previous.ID
	}
	return
}

func (tx *DocumentTransaction) DoAppendInsert(operation *Operation) (ret *Failure) {
	var err error
	block, err := tx.Environment.LookupBlock(operation.ParentID)
	if err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: operation.ParentID}
	}
	if nil == block {
		tx.Report("error", "not found block [%s]", operation.ParentID)
		if tx.Environment.Legacy {
			tx.Environment.ReloadUI()
		} else {
			return &Failure{Code: TxErrCodeBlockNotFound, ID: operation.ParentID}
		} // 比如分屏后编辑器状态不一致，这里强制重新载入界面
		return
	}
	tree, err := tx.LoadTree(block.ID)
	if err != nil {
		msg := fmt.Sprintf("load tree [%s] failed: %s", block.ID, err)
		tx.Report("error", "%s", msg)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: block.ID}
	}

	data := strings.ReplaceAll(operation.Data.(string), editor.FrontEndCaret, "")
	subTree := tx.LuteEngine.BlockDOM2Tree(data)
	insertedNode := subTree.Root.FirstChild
	if nil == insertedNode {
		return &Failure{Code: TxErrCodeBlockNotFound, Msg: "invalid data tree", ID: block.ID}
	}
	if "" == insertedNode.ID {
		insertedNode.ID = ast.NewNodeID()
		insertedNode.SetIALAttr("id", insertedNode.ID)
	}
	var toInserts []*ast.Node
	for toInsert := insertedNode; nil != toInsert; toInsert = toInsert.Next {
		if ast.NodeKramdownBlockIAL != toInsert.Type {
			if "" == toInsert.ID {
				toInsert.ID = ast.NewNodeID()
				toInsert.SetIALAttr("id", toInsert.ID)
			}
			toInserts = append(toInserts, toInsert)
		}
	}

	node := treenode.GetNodeInTree(tree, operation.ParentID)
	if nil == node {
		tx.Report("error", "get node [%s] in tree [%s] failed", operation.ParentID, tree.Root.ID)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: operation.ParentID}
	}
	isContainer := node.IsContainerBlock()
	if !isContainer {
		slices.Reverse(toInserts)
	}
	var lastChildBelowHeading *ast.Node
	if ast.NodeHeading == node.Type {
		if children := treenode.HeadingChildren(node); 0 < len(children) {
			lastChildBelowHeading = children[len(children)-1]
		}
	}

	for _, toInsert := range toInserts {
		if isContainer {
			if ast.NodeList == node.Type {
				// 列表下只能挂列表项，所以这里需要分情况处理 https://github.com/siyuan-note/siyuan/issues/9955
				if ast.NodeList == toInsert.Type {
					var childLis []*ast.Node
					for childLi := toInsert.FirstChild; nil != childLi; childLi = childLi.Next {
						childLis = append(childLis, childLi)
					}
					for _, childLi := range childLis {
						node.AppendChild(childLi)
					}
				} else {
					newLiID := ast.NewNodeID()
					newLi := &ast.Node{ID: newLiID, Type: ast.NodeListItem, ListData: &ast.ListData{Typ: node.ListData.Typ}}
					newLi.SetIALAttr("id", newLiID)
					node.AppendChild(newLi)
					newLi.AppendChild(toInsert)
				}
			} else if ast.NodeSuperBlock == node.Type {
				node.LastChild.InsertBefore(toInsert)
			} else {
				node.AppendChild(toInsert)
			}
		} else {
			if ast.NodeHeading == node.Type {
				if nil != lastChildBelowHeading {
					lastChildBelowHeading.InsertAfter(toInsert)
				} else {
					node.InsertAfter(toInsert)
				}
			} else {
				node.InsertAfter(toInsert)
			}
		}

		CreatedUpdatedWithCache(toInsert, tx.Environment.CacheIAL)
		tx.Nodes[toInsert.ID] = toInsert
	}

	CreatedUpdatedWithCache(insertedNode, tx.Environment.CacheIAL)
	tx.Nodes[insertedNode.ID] = insertedNode
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: block.ID}
	}

	operation.ID = insertedNode.ID
	operation.ParentID = insertedNode.Parent.ID

	// 将 appendInsert 转换为 insert 推送
	operation.Action = "insert"
	if nil != insertedNode.Previous {
		operation.PreviousID = insertedNode.Previous.ID
	}
	return
}

func (tx *DocumentTransaction) DoAppend(operation *Operation) (ret *Failure) {
	var err error
	id := operation.ID
	srcTree, err := tx.LoadTree(id)
	if err != nil {
		tx.Report("error", "load tree [%s] failed: %s", id, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	srcNode := treenode.GetNodeInTree(srcTree, id)
	if nil == srcNode {
		tx.Report("error", "get node [%s] in tree [%s] failed", id, srcTree.Root.ID)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	if ast.NodeDocument == srcNode.Type {
		tx.Report("error", "can't append a root to another root")
		return
	}

	var headingChildren []*ast.Node
	if isMovingFoldHeading := ast.NodeHeading == srcNode.Type && "1" == srcNode.IALAttr("fold"); isMovingFoldHeading {
		headingChildren = treenode.HeadingChildren(srcNode)
	}
	var srcEmptyList, targetNewList *ast.Node
	if ast.NodeListItem == srcNode.Type {
		targetNewListID := ast.NewNodeID()
		targetNewList = &ast.Node{ID: targetNewListID, Type: ast.NodeList, ListData: &ast.ListData{Typ: srcNode.ListData.Typ}}
		targetNewList.SetIALAttr("id", targetNewListID)
		if srcNode.Parent.FirstChild == srcNode && srcNode.Parent.LastChild == srcNode {
			// 列表中唯一的列表项被移除后，该列表就为空了
			srcEmptyList = srcNode.Parent
		}
	}

	targetRootID := operation.ParentID
	if id == targetRootID {
		tx.Report("error", "target root id is nil")
		return
	}

	targetTree, err := tx.LoadTree(targetRootID)
	if err != nil {
		tx.Report("error", "load tree [%s] failed: %s", targetRootID, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: targetRootID}
	}
	isSameTree := srcTree.ID == targetTree.ID
	if isSameTree {
		targetTree = srcTree
	}

	targetRoot := targetTree.Root
	if nil != targetNewList {
		if nil != targetRoot.LastChild {
			if ast.NodeList != targetRoot.LastChild.Type {
				targetNewList.AppendChild(srcNode)
				targetRoot.AppendChild(targetNewList)
			} else {
				targetRoot.LastChild.AppendChild(srcNode)
			}
		} else {
			targetRoot.AppendChild(srcNode)
		}
	} else {
		targetRoot.AppendChild(srcNode)
	}
	for _, c := range headingChildren {
		targetRoot.AppendChild(c)
	}
	if nil != srcEmptyList {
		srcEmptyList.Unlink()
	}

	if err = tx.WriteTree(srcTree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}

	if !isSameTree {
		if err = tx.WriteTree(targetTree); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
	}
	return
}

func (tx *DocumentTransaction) DoDelete(operation *Operation) (ret *Failure) {
	var err error
	id := operation.ID
	tree, err := tx.LoadTree(id)
	if err != nil {
		if errors.Is(err, ErrBlockNotFound) {
			// move 以后这里会空，算作正常情况
			return
		}

		msg := fmt.Sprintf("load tree [%s] failed: %s", id, err)
		tx.Report("error", "%s", msg)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		return nil // move 以后的情况，列表项移动导致的状态异常 https://github.com/siyuan-note/insider/issues/961
	}

	// 收集引用的定义块 ID
	refDefIDs := GetRefDefIDs(node)
	// 推送定义节点引用计数
	if err = tx.Environment.DeleteReferences(tx.Environment.Ctx, refDefIDs...); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}

	parent := node.Parent
	if nil != node.Next && ast.NodeKramdownBlockIAL == node.Next.Type && bytes.Contains(node.Next.Tokens, []byte(node.ID)) {
		// 列表块撤销状态异常 https://github.com/siyuan-note/siyuan/issues/3985
		node.Next.Unlink()
	}

	node.Unlink()

	if nil != parent && ast.NodeListItem == parent.Type && nil == parent.FirstChild {
		needAppendEmptyListItem := true
		for _, op := range tx.DoOperations {
			if "insert" == op.Action && op.ParentID == parent.ID {
				needAppendEmptyListItem = false
				break
			}
		}

		if needAppendEmptyListItem {
			parent.AppendChild(treenode.NewParagraph(ast.NewNodeID()))
		}
	}
	if err = tx.Environment.DeleteBlockTree(tx.Environment.Ctx, node.ID); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}

	delete(tx.Nodes, node.ID)
	if err = tx.WriteTree(tree); err != nil {
		if !tx.Environment.Legacy {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
		return
	}

	// 如果是断开列表时的删除列表项事务，则不需要删除数据库绑定块，因为断开列表事务后面会再次插入相同 ID 的列表项
	// List item disconnection no longer affects database binding blocks https://github.com/siyuan-note/siyuan/issues/12235
	needSyncDel2AvBlock := true
	if ast.NodeListItem == node.Type {
		for _, op := range tx.DoOperations {
			// 不可能出现相同 ID 先插入再删除的情况，只可能出现先删除再插入的情况，所以这里只需要查找插入操作
			if "insert" == op.Action {
				data := strings.ReplaceAll(op.Data.(string), editor.FrontEndCaret, "")
				subTree := tx.LuteEngine.BlockDOM2Tree(data)
				ast.Walk(subTree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
					if !entering || ast.NodeListItem != n.Type {
						return ast.WalkContinue
					}

					if n.ID == operation.ID {
						needSyncDel2AvBlock = false
						return ast.WalkStop
					}
					return ast.WalkContinue
				})

				break
			}
		}
	}

	if needSyncDel2AvBlock {
		if err = tx.Environment.DeleteAV(tx.Environment.Ctx, node, tree, tx); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
	}
	return
}

func (tx *DocumentTransaction) DoLargeInsert(previousID string) (ret *Failure) {
	effect := func(fn func() error) error {
		if err := tx.Environment.Ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		return tx.Environment.Ctx.Err()
	}
	tree, err := tx.LoadTree(previousID)
	if nil != err {
		tx.Report("error", "load tree [%s] failed: %s", previousID, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: previousID}
	}

	for _, operation := range tx.DoOperations {
		if err := tx.Environment.Ctx.Err(); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
		}
		if "insert" != operation.Action {
			break
		}

		data := strings.ReplaceAll(operation.Data.(string), editor.FrontEndCaret, "")
		subTree := tx.LuteEngine.BlockDOM2Tree(data)
		subTree.Box, subTree.Path = tree.Box, tree.Path
		if err = effect(func() error { return tx.Environment.InsertAssets(tx.Environment.Ctx, subTree, tx) }); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
		}

		insertedNode := subTree.Root.FirstChild
		if nil == insertedNode {
			return &Failure{Code: TxErrCodeBlockNotFound, Msg: "invalid data tree", ID: tree.ID}
		}
		var remains []*ast.Node
		for remain := insertedNode.Next; nil != remain; remain = remain.Next {
			if ast.NodeKramdownBlockIAL != remain.Type {
				if "" == remain.ID {
					remain.ID = ast.NewNodeID()
					remain.SetIALAttr("id", remain.ID)
				}
				remains = append(remains, remain)
			}
		}
		if "" == insertedNode.ID {
			insertedNode.ID = ast.NewNodeID()
			insertedNode.SetIALAttr("id", insertedNode.ID)
		}

		node := treenode.GetNodeInTree(tree, previousID)
		if nil == node {
			tx.Report("error", "get node [%s] in tree [%s] failed", previousID, tree.Root.ID)
			return &Failure{Code: TxErrCodeBlockNotFound, ID: previousID}
		}

		if ast.NodeHeading == node.Type && "1" == node.IALAttr("fold") {
			children := treenode.HeadingChildren(node)
			if l := len(children); 0 < l {
				node = children[l-1]
			}
		}
		if ast.NodeList == insertedNode.Type && nil != node.Parent && ast.NodeList == node.Parent.Type {
			insertedNode = insertedNode.FirstChild
		}
		for i := len(remains) - 1; 0 <= i; i-- {
			remain := remains[i]
			node.InsertAfter(remain)
		}
		node.InsertAfter(insertedNode)

		CreatedUpdatedWithCache(insertedNode, tx.Environment.CacheIAL)
		tx.Nodes[insertedNode.ID] = insertedNode
		tx.Trees[tree.ID] = tree

		// 收集引用的定义块 ID
		refDefIDs := GetRefDefIDs(insertedNode)
		// 推送定义节点引用计数
		if err = effect(func() error { return tx.Environment.InsertReferences(tx.Environment.Ctx, refDefIDs...) }); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
		}

		if err = effect(func() error { return tx.Environment.InsertAVRegistry(tx.Environment.Ctx, insertedNode) }); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
		}

		// 复制为副本时移除数据库绑定状态 https://github.com/siyuan-note/siyuan/issues/12294
		insertedNode.RemoveIALAttr(av.NodeAttrNameAvs)
		insertedNode.RemoveIALAttr(av.NodeAttrViewNames)
		insertedNode.RemoveIALAttrsByPrefix(av.NodeAttrViewStaticText)

		if err = effect(func() error { return tx.Environment.InsertAVBind(tx.Environment.Ctx, insertedNode, tree, tx) }); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
		}

		operation.ID = insertedNode.ID
		operation.ParentID = insertedNode.Parent.ID
	}

	if err = effect(func() error { return tx.WriteTree(tree) }); nil != err {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: tree.ID}
	}
	return tx.DoDelete(tx.DoOperations[len(tx.DoOperations)-1])
}

func (tx *DocumentTransaction) DoInsert(operation *Operation) (ret *Failure) {
	var bt *BlockLocation
	for _, id := range []string{operation.ParentID, operation.PreviousID, operation.NextID} {
		if id == "" {
			continue
		}
		b, lookupErr := tx.Environment.LookupBlock(id)
		if lookupErr != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: lookupErr.Error(), ID: id}
		}
		if b != nil && b.ID != "" {
			bt = b
			break
		}
	}
	if nil == bt {
		tx.Report("error", "not found block tree [%s, %s, %s]", operation.ParentID, operation.PreviousID, operation.NextID)
		if tx.Environment.Legacy {
			tx.Environment.ReloadUI()
			return
		}
		return &Failure{Code: TxErrCodeBlockNotFound, Msg: "insert target not found"}
	}

	var err error
	tree, err := tx.LoadTreeByBlockTree(bt)
	if err != nil {
		msg := fmt.Sprintf("load tree [%s] failed: %s", bt.ID, err)
		tx.Report("error", "%s", msg)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: bt.ID}
	}

	data := strings.ReplaceAll(operation.Data.(string), editor.FrontEndCaret, "")
	subTree := tx.LuteEngine.BlockDOM2Tree(data)
	subTree.Box, subTree.Path = tree.Box, tree.Path
	if err = tx.Environment.InsertAssets(tx.Environment.Ctx, subTree, tx); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: bt.ID}
	}

	insertedNode := subTree.Root.FirstChild
	if nil == insertedNode {
		return &Failure{Code: TxErrCodeBlockNotFound, Msg: "invalid data tree", ID: bt.ID}
	}
	var remains []*ast.Node
	for remain := insertedNode.Next; nil != remain; remain = remain.Next {
		if ast.NodeKramdownBlockIAL != remain.Type {
			if "" == remain.ID {
				remain.ID = ast.NewNodeID()
				remain.SetIALAttr("id", remain.ID)
			}
			remains = append(remains, remain)
		}
	}
	if "" == insertedNode.ID {
		insertedNode.ID = ast.NewNodeID()
		insertedNode.SetIALAttr("id", insertedNode.ID)
	}

	var node *ast.Node
	nextID := operation.NextID
	previousID := operation.PreviousID
	if "" != nextID {
		node = treenode.GetNodeInTree(tree, nextID)
		if nil == node {
			tx.Report("error", "get node [%s] in tree [%s] failed", nextID, tree.Root.ID)
			return &Failure{Code: TxErrCodeBlockNotFound, ID: nextID}
		}

		if ast.NodeList == insertedNode.Type && nil != node.Parent && ast.NodeList == node.Parent.Type {
			insertedNode = insertedNode.FirstChild
		}
		node.InsertBefore(insertedNode)
	} else if "" != previousID {
		node = treenode.GetNodeInTree(tree, previousID)
		if nil == node {
			tx.Report("error", "get node [%s] in tree [%s] failed", previousID, tree.Root.ID)
			return &Failure{Code: TxErrCodeBlockNotFound, ID: previousID}
		}

		if ast.NodeHeading == node.Type && "1" == node.IALAttr("fold") {
			children := treenode.HeadingChildren(node)
			if l := len(children); 0 < l {
				node = children[l-1]
			}
		}
		if ast.NodeList == insertedNode.Type && nil != node.Parent && ast.NodeList == node.Parent.Type {
			insertedNode = insertedNode.FirstChild
		}
		for i := len(remains) - 1; 0 <= i; i-- {
			remain := remains[i]
			node.InsertAfter(remain)
		}
		node.InsertAfter(insertedNode)
	} else {
		node = treenode.GetNodeInTree(tree, operation.ParentID)
		if nil == node {
			tx.Report("error", "get node [%s] in tree [%s] failed", operation.ParentID, tree.Root.ID)
			return &Failure{Code: TxErrCodeBlockNotFound, ID: operation.ParentID}
		}
		if ast.NodeSuperBlock == node.Type {
			// 在布局节点后插入
			node.FirstChild.Next.InsertAfter(insertedNode)
		} else {
			if ast.NodeList == insertedNode.Type && nil != insertedNode.FirstChild && operation.ID == insertedNode.FirstChild.ID && operation.ID != insertedNode.ID {
				// 将一个列表项移动到另一个列表的第一项时 https://github.com/siyuan-note/siyuan/issues/2341
				insertedNode = insertedNode.FirstChild
			}

			if ast.NodeListItem == node.Type && 3 == node.ListData.Typ {
				// 在任务列表标记节点后插入
				node.FirstChild.InsertAfter(insertedNode)
				for _, remain := range remains {
					node.FirstChild.InsertAfter(remain)
				}
			} else {
				if !node.IsContainerBlock() {
					for i := len(remains) - 1; 0 <= i; i-- {
						remain := remains[i]
						node.InsertAfter(remain)
					}
					node.InsertAfter(insertedNode)
				} else {
					for i := len(remains) - 1; 0 <= i; i-- {
						remain := remains[i]
						node.PrependChild(remain)
					}
					node.PrependChild(insertedNode)
				}
			}
		}
	}

	CreatedUpdatedWithCache(insertedNode, tx.Environment.CacheIAL)
	tx.Nodes[insertedNode.ID] = insertedNode
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: bt.ID}
	}

	// 收集引用的定义块 ID
	refDefIDs := GetRefDefIDs(insertedNode)
	if err = tx.Environment.InsertReferences(tx.Environment.Ctx, refDefIDs...); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
	}
	if err = tx.Environment.InsertAVRegistry(tx.Environment.Ctx, insertedNode); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
	}

	// 复制为副本时移除数据库绑定状态 https://github.com/siyuan-note/siyuan/issues/12294
	insertedNode.RemoveIALAttr(av.NodeAttrNameAvs)
	insertedNode.RemoveIALAttr(av.NodeAttrViewNames)
	insertedNode.RemoveIALAttrsByPrefix(av.NodeAttrViewStaticText)

	if err = tx.Environment.InsertAVBind(tx.Environment.Ctx, insertedNode, tree, tx); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error()}
	}

	operation.ID = insertedNode.ID
	operation.ParentID = insertedNode.Parent.ID
	return
}

func (tx *DocumentTransaction) DoUpdate(operation *Operation) (ret *Failure) {
	id := operation.ID
	tree, err := tx.LoadTree(id)
	if err != nil {
		tx.Report("error", "load tree [%s] failed: %s", id, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	data := strings.ReplaceAll(operation.Data.(string), editor.FrontEndCaret, "")
	if "" == data {
		tx.Report("error", "update data is nil")
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	subTree := tx.LuteEngine.BlockDOM2Tree(data)
	subTree.ID, subTree.Box, subTree.Path = tree.ID, tree.Box, tree.Path
	oldNode := treenode.GetNodeInTree(tree, id)
	if nil == oldNode {
		tx.Report("error", "get node [%s] in tree [%s] failed", id, tree.Root.ID)
		return &Failure{Msg: ErrBlockNotFound.Error(), ID: id}
	}

	// 收集引用的定义块 ID
	oldDefIDs := GetRefDefIDs(oldNode)
	var newDefIDs []string

	var walkErr error
	var unlinks []*ast.Node
	ast.Walk(subTree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeTextMark == n.Type {
			if n.IsTextMarkType("inline-math") {
				if "" == strings.TrimSpace(n.TextMarkInlineMathContent) {
					// 剔除空白的行级公式
					unlinks = append(unlinks, n)
				}
			} else if n.IsTextMarkType("block-ref") {
				if walkErr = tx.Environment.UpdateCacheRef(tx.Environment.Ctx, subTree, n); walkErr != nil {
					return ast.WalkStop
				}

				if "d" == n.TextMarkBlockRefSubtype {
					// 偶发编辑文档标题后引用处的动态锚文本不更新 https://github.com/siyuan-note/siyuan/issues/5891
					// 使用缓存的动态锚文本强制覆盖当前块中的引用节点动态锚文本
					dRefText, e := tx.Environment.UpdateDynamicText(tx.Environment.Ctx, n.TextMarkBlockRefID)
					if e != nil {
						walkErr = e
						return ast.WalkStop
					}
					if dRefText != "" {
						n.TextMarkTextContent = dRefText
					}
				}

				newDefIDs = append(newDefIDs, n.TextMarkBlockRefID)
			}
		}
		return ast.WalkContinue
	})
	if walkErr != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: walkErr.Error(), ID: id}
	}
	for _, n := range unlinks {
		n.Unlink()
	}

	oldDefIDs = gulu.Str.RemoveDuplicatedElem(oldDefIDs)
	newDefIDs = gulu.Str.RemoveDuplicatedElem(newDefIDs)
	refDefIDs := oldDefIDs

	if !slices.Equal(oldDefIDs, newDefIDs) { // 如果引用发生了变化，则推送定义节点引用计数
		refDefIDs = append(refDefIDs, newDefIDs...)
		refDefIDs = gulu.Str.RemoveDuplicatedElem(refDefIDs)
		if err = tx.Environment.UpdateReferences(tx.Environment.Ctx, refDefIDs...); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
	}

	updatedNode := subTree.Root.FirstChild
	if nil == updatedNode {
		tx.Report("error", "get fist node in sub tree [%s] failed", subTree.Root.ID)
		return &Failure{Msg: ErrBlockNotFound.Error(), ID: id}
	}
	if ast.NodeList == updatedNode.Type && ast.NodeList == oldNode.Parent.Type {
		updatedNode = updatedNode.FirstChild
	}

	if oldNode.IsContainerBlock() {
		// 更新容器块的话需要考虑其子块中可能存在的折叠标题，需要把这些折叠标题的下方块移动到新节点下面
		treenode.MoveFoldHeading(updatedNode, oldNode)
	}

	tx.Environment.CacheIAL(updatedNode.ID, parse.IAL2Map(updatedNode.KramdownIAL))

	if ast.NodeHTMLBlock == updatedNode.Type {
		content := string(updatedNode.Tokens)
		// 剔除连续的空行（包括空行内包含空格的情况） https://github.com/siyuan-note/siyuan/issues/15377
		var newLines []string
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				newLines = append(newLines, line)
			}
		}
		updatedNode.Tokens = []byte(strings.Join(newLines, "\n"))
	}

	removedNodes := GetRemovedNodes(oldNode, updatedNode)
	for _, n := range removedNodes {
		if err = tx.Environment.DeleteAV(tx.Environment.Ctx, n, tree, tx); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
	}

	// 将不属于折叠标题的块移动到折叠标题下方，需要展开折叠标题
	needUnfoldParentHeading := 0 < oldNode.HeadingLevel && (0 == updatedNode.HeadingLevel || oldNode.HeadingLevel < updatedNode.HeadingLevel)

	oldParentFoldedHeading := treenode.GetParentFoldedHeading(oldNode)
	// 将原先折叠标题下的块提升为与折叠标题同级或更高一级的标题时，需要在折叠标题后插入该提升后的标题块（只需要推送界面插入）
	needInsertAfterParentHeading := nil != oldParentFoldedHeading && 0 != updatedNode.HeadingLevel && updatedNode.HeadingLevel <= oldParentFoldedHeading.HeadingLevel

	oldNode.InsertAfter(updatedNode)
	oldNode.Unlink()

	if needUnfoldParentHeading {
		newParentFoldedHeading := treenode.GetParentFoldedHeading(updatedNode)
		if nil == oldParentFoldedHeading || (nil != newParentFoldedHeading && oldParentFoldedHeading.ID != newParentFoldedHeading.ID) {
			if err = UnfoldHeadingWithSink(tx.Environment.Ctx, newParentFoldedHeading, updatedNode, func(h, n *ast.Node) error { return tx.Environment.UpdateUnfold(tx.Environment.Ctx, h, n) }); err != nil {
				return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
			}
		}
	}

	if needInsertAfterParentHeading {
		insertDom := data
		if 2 == len(tx.DoOperations) && "foldHeading" == tx.DoOperations[1].Action {
			children := treenode.HeadingChildren(updatedNode)
			for _, child := range children {
				ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
					if !entering || !n.IsBlock() {
						return ast.WalkContinue
					}

					n.SetIALAttr("fold", "1")
					n.SetIALAttr("heading-fold", "1")
					return ast.WalkContinue
				})
			}
			updatedNode.SetIALAttr("fold", "1")
			insertDom = tx.LuteEngine.RenderNodeBlockDOM(updatedNode)
		}

		events := []*DocumentTransaction{{
			DoOperations:   []*Operation{{Action: "insert", ID: updatedNode.ID, PreviousID: oldParentFoldedHeading.ID, Data: insertDom}},
			UndoOperations: []*Operation{{Action: "delete", ID: updatedNode.ID}},
		}}
		if err = tx.Environment.UpdateEvent(tx.Environment.Ctx, events); err != nil {
			return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
		}
	}

	CreatedUpdatedWithCache(updatedNode, tx.Environment.CacheIAL)
	tx.Nodes[updatedNode.ID] = updatedNode
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}

	if err = tx.Environment.UpdateAV(tx.Environment.Ctx, updatedNode, tree, tx); err != nil {
		if tx.Environment.Legacy {
			return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
		}
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}

	return
}

func UnfoldHeadingWithSink(ctx context.Context, heading, currentNode *ast.Node, sink func(*ast.Node, *ast.Node) error) error {
	if sink == nil {
		return fmt.Errorf("unfold sink unbound")
	}
	if err := doclife.UnfoldHeading(ctx, heading); err != nil {
		return err
	}
	if heading == nil {
		return nil
	}
	return sink(heading, currentNode)
}

func GetRefDefIDs(node *ast.Node) (refDefIDs []string) {
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if treenode.IsBlockRef(n) {
			refDefIDs = append(refDefIDs, n.TextMarkBlockRefID)
		} else if treenode.IsEmbedBlockRef(n) {
			defID := treenode.GetEmbedBlockRef(n)
			refDefIDs = append(refDefIDs, defID)
		}
		return ast.WalkContinue
	})
	refDefIDs = gulu.Str.RemoveDuplicatedElem(refDefIDs)
	return
}

func GetRemovedNodes(oldNode, newNode *ast.Node) (ret []*ast.Node) {
	oldNodes := map[string]*ast.Node{}
	ast.Walk(oldNode, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || !n.IsBlock() {
			return ast.WalkContinue
		}
		oldNodes[n.ID] = n
		return ast.WalkContinue
	})
	ast.Walk(newNode, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || !n.IsBlock() {
			return ast.WalkContinue
		}
		if _, ok := oldNodes[n.ID]; ok {
			delete(oldNodes, n.ID)
		}
		return ast.WalkContinue
	})
	for _, n := range oldNodes {
		ret = append(ret, n)
	}
	return
}

func (tx *DocumentTransaction) DoUpdateUpdated(operation *Operation) (ret *Failure) {
	id := operation.ID
	tree, err := tx.LoadTree(id)
	if err != nil {
		if errors.Is(err, ErrBlockNotFound) {
			tx.Report("error", "not found block [%s]", id)
			return
		}
		tx.Report("error", "load tree [%s] failed: %s", id, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		tx.Report("error", "get node [%s] in tree [%s] failed", id, tree.Root.ID)
		return &Failure{Msg: ErrBlockNotFound.Error(), ID: id}
	}

	node.SetIALAttr("updated", operation.Data.(string))
	CreatedUpdatedWithCache(node, tx.Environment.CacheIAL)
	tx.Nodes[node.ID] = node
	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}
	return
}

func (tx *DocumentTransaction) DoCreate(operation *Operation) (ret *Failure) {
	tree := operation.Data.(*parse.Tree)
	if err := tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: tree.ID}
	}
	return
}

func (tx *DocumentTransaction) DoSetAttrs(operation *Operation) (ret *Failure) {
	id := operation.ID
	tree, err := tx.LoadTree(id)
	if err != nil {
		tx.Report("error", "load tree [%s] failed: %s", id, err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		tx.Report("error", "get node [%s] in tree [%s] failed", id, tree.Root.ID)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	attrs := map[string]string{}
	if err = gulu.JSON.UnmarshalJSON([]byte(operation.Data.(string)), &attrs); err != nil {
		tx.Report("error", "unmarshal attrs failed: %s", err)
		return &Failure{Code: TxErrCodeBlockNotFound, ID: id}
	}

	var invalidNames []string
	for name := range attrs {
		for i := 0; i < len(name); i++ {
			if !lex.IsASCIILetterNumHyphen(name[i]) {
				tx.Report("error", "invalid attr name [%s]", name)
				invalidNames = append(invalidNames, name)
			}
		}
	}
	for _, name := range invalidNames {
		delete(attrs, name)
	}

	for name, value := range attrs {
		if "" == value {
			node.RemoveIALAttr(name)
		} else {
			node.SetIALAttr(name, value)
		}
	}

	if err = tx.WriteTree(tree); err != nil {
		return &Failure{Code: TxErrCodeWriteTree, Msg: err.Error(), ID: id}
	}
	tx.Environment.CacheIAL(id, parse.IAL2Map(node.KramdownIAL))
	return
}

func RefreshUpdatedAt(node *ast.Node, updated string) {
	doclife.RefreshUpdatedAt(node, updated)
}

func CreatedUpdatedWithCache(node *ast.Node, putIAL func(string, map[string]string)) {
	// 补全子节点的更新时间 Improve block update time filling https://github.com/siyuan-note/siyuan/issues/12182
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || !n.IsBlock() || ast.NodeKramdownBlockIAL == n.Type {
			return ast.WalkContinue
		}

		updated := n.IALAttr("updated")
		if "" == updated && ast.IsNodeIDPattern(n.ID) {
			created := util.TimeFromID(n.ID)
			n.SetIALAttr("updated", created)
		}
		return ast.WalkContinue
	})

	created := util.TimeFromID(node.ID)
	updated := node.IALAttr("updated")
	if "" == updated {
		updated = created
	}
	if updated < created {
		updated = created
	}
	parents := treenode.ParentNodesWithHeadings(node)
	for _, parent := range parents { // 更新所有父节点的更新时间字段
		parent.SetIALAttr("updated", updated)
		putIAL(parent.ID, parse.IAL2Map(parent.KramdownIAL))
	}
}

func (tx *DocumentTransaction) LoadTreeByBlockTree(bt *BlockLocation) (ret *parse.Tree, err error) {
	if err = tx.Environment.ReadGuard(); err != nil {
		return nil, err
	}
	if nil == bt {
		return nil, ErrBlockNotFound
	}

	ret = tx.Trees[bt.RootID]
	if nil != ret {
		return
	}

	ret, err = tx.Environment.Load(bt.BoxID, bt.Path, tx.LuteEngine)
	if err != nil {
		return
	}
	if err = tx.Environment.ReadGuard(); err != nil {
		return nil, err
	}
	tx.Trees[bt.RootID] = ret
	return
}

func (tx *DocumentTransaction) LoadTree(id string) (ret *parse.Tree, err error) {
	var rootID, box, p string
	bt, err := tx.Environment.LookupBlock(id)
	if err != nil {
		return nil, err
	}
	if nil == bt {
		return nil, ErrBlockNotFound
	}
	rootID = bt.RootID
	box = bt.BoxID
	p = bt.Path

	ret = tx.Trees[rootID]
	if nil != ret {
		return
	}

	ret, err = tx.Environment.Load(box, p, tx.LuteEngine)
	if err != nil {
		return
	}
	if err = tx.Environment.ReadGuard(); err != nil {
		return nil, err
	}
	tx.Trees[rootID] = ret
	return
}

func (tx *DocumentTransaction) WriteTree(tree *parse.Tree) (err error) {
	tx.Trees[tree.ID] = tree
	err = tx.Environment.Stage(tree)
	return
}
