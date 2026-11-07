// SPDX-License-Identifier: AGPL-3.0-or-later
// Original backlink grouping, filtering and document rendering with explicit resources.
package blockview

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/emirpasic/gods/sets/hashset"
	"path"
	"sort"
	"strconv"
	"strings"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/textsort"
	"wiki-kernel/search"
)

type Backlink struct {
	DOM        string       `json:"dom"`
	BlockPaths []*BlockPath `json:"blockPaths"`
	Expand     bool         `json:"expand"`

	node *ast.Node // 仅用于按文档内容顺序排序
}

func (scope *blockReadScope) GetBackmentionDoc(defID, refTreeID, keyword string, containChildren, highlight bool) (ret []*Backlink, keywords []string) {
	keyword = strings.TrimSpace(keyword)
	if "" != keyword {
		keywords = strings.Split(keyword, " ")
	}
	ret = []*Backlink{}
	beforeLen := 12
	sqlBlock := scope.block(defID)
	if nil == sqlBlock {
		return
	}
	rootID := sqlBlock.RootID

	refs := scope.refs(defID, containChildren)
	refs = scope.removeDuplicatedRefs(refs)

	linkRefs, _, excludeBacklinkIDs, originalRefBlockIDs := scope.buildLinkRefs(rootID, refs, keywords)
	tmpMentions, mentionKeywords := scope.buildTreeBackmention(sqlBlock, linkRefs, keyword, excludeBacklinkIDs, beforeLen)
	luteEngine := scope.newParser()
	var mentions []*Block
	for _, mention := range tmpMentions {
		if mention.RootID == refTreeID {
			mentions = append(mentions, mention)
		}
	}
	var mentionBlockIDs []string
	for _, mention := range mentions {
		mentionBlockIDs = append(mentionBlockIDs, mention.ID)
	}
	mentionBlockIDs = gulu.Str.RemoveDuplicatedElem(mentionBlockIDs)

	if "" != keyword {
		mentionKeywords = append(mentionKeywords, strings.Split(keyword, " ")...)
	}
	mentionKeywords = gulu.Str.RemoveDuplicatedElem(mentionKeywords)
	keywords = append(keywords, mentionKeywords...)
	keywords = gulu.Str.RemoveDuplicatedElem(keywords)
	if 1 > len(keywords) {
		keywords = []string{}
	}

	var refTree *parse.Tree
	trees := scope.loadTrees(mentionBlockIDs)
	for id, tree := range trees {
		backlink := scope.buildBacklink(id, tree, originalRefBlockIDs, mentionKeywords, highlight, luteEngine)
		if nil != backlink {
			ret = append(ret, backlink)
		}
		if nil != tree && nil == refTree {
			refTree = tree
		}
	}

	if 0 < len(trees) {
		scope.sortBacklinks(ret, refTree)
		scope.filterBlockPaths(ret)
	}
	return
}

func (scope *blockReadScope) GetBacklinkDoc(defID, refTreeID, keyword string, containChildren, highlight bool) (ret []*Backlink, keywords []string) {
	keyword = strings.TrimSpace(keyword)
	if "" != keyword {
		keywords = strings.Split(keyword, " ")
	}
	keywords = gulu.Str.RemoveDuplicatedElem(keywords)
	if 1 > len(keywords) {
		keywords = []string{}
	}

	ret = []*Backlink{}
	sqlBlock := scope.block(defID)
	if nil == sqlBlock {
		return
	}
	rootID := sqlBlock.RootID

	tmpRefs := scope.refs(defID, containChildren)
	var refs []*indexdb.RefRecord
	for _, ref := range tmpRefs {
		if ref.RootID == refTreeID {
			refs = append(refs, ref)
		}
	}
	refs = scope.removeDuplicatedRefs(refs)

	linkRefs, _, _, originalRefBlockIDs := scope.buildLinkRefs(rootID, refs, keywords)
	refTree, err := scope.load(refTreeID)
	if err != nil {
		scope.fail(err)
		return
	}

	luteEngine := scope.newParser()
	for _, linkRef := range linkRefs {
		backlink := scope.buildBacklink(linkRef.ID, refTree, originalRefBlockIDs, keywords, highlight, luteEngine)
		if nil != backlink {
			ret = append(ret, backlink)
		}
	}

	scope.sortBacklinks(ret, refTree)
	scope.filterBlockPaths(ret)
	return
}

func (scope *blockReadScope) filterBlockPaths(blockLinks []*Backlink) {
	for _, b := range blockLinks {
		if 2 == len(b.BlockPaths) {
			// 根下只有一层则不显示
			b.BlockPaths = []*BlockPath{}
		}
	}
	return
}

func (scope *blockReadScope) sortBacklinks(backlinks []*Backlink, tree *parse.Tree) {
	contentSorts := map[string]int{}
	sortVal := 0
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if scope.ctx.Err() != nil {
			scope.fail(scope.ctx.Err())
			return ast.WalkStop
		}
		if !entering || !n.IsBlock() {
			return ast.WalkContinue
		}

		contentSorts[n.ID] = sortVal
		sortVal++
		return ast.WalkContinue
	})

	sort.Slice(backlinks, func(i, j int) bool {
		s1 := contentSorts[backlinks[i].node.ID]
		s2 := contentSorts[backlinks[j].node.ID]
		return s1 < s2
	})
}

func (scope *blockReadScope) buildBacklink(refID string, refTree *parse.Tree, originalRefBlockIDs map[string]string, keywords []string, highlight bool, luteEngine *lute.Lute) (ret *Backlink) {
	node := docview.GetNodeInTree(refTree, refID)
	if nil == node {
		return
	}

	renderNodes, expand := scope.getBacklinkRenderNodes(node, originalRefBlockIDs)

	if highlight && 0 < len(keywords) {
		for _, renderNode := range renderNodes {
			var unlinks []*ast.Node

			ast.Walk(renderNode, func(n *ast.Node, entering bool) ast.WalkStatus {
				if scope.ctx.Err() != nil {
					scope.fail(scope.ctx.Err())
					return ast.WalkStop
				}
				if !entering {
					return ast.WalkContinue
				}

				if n.IsBlock() {
					return ast.WalkContinue
				}

				scope.markSpan(n, &unlinks, keywords, search.MarkDataType, luteEngine)
				return ast.WalkContinue
			})

			for _, unlink := range unlinks {
				unlink.Unlink()
			}
		}
	}

	// 反链面板中显示块引用计数 Display reference counts in the backlink panel https://github.com/siyuan-note/siyuan/issues/13618
	scope.fillCounts(renderNodes)

	dom := scope.render(renderNodes, luteEngine)
	var blockPaths []*BlockPath
	if (nil != node.Parent && ast.NodeDocument != node.Parent.Type) || (ast.NodeHeading != node.Type && 0 < scope.headingLevel(node)) {
		blockPaths = scope.breadcrumb(node, nil, false, 0)
	}
	if 1 > len(blockPaths) {
		blockPaths = []*BlockPath{}
	}
	ret = &Backlink{DOM: dom, BlockPaths: blockPaths, Expand: expand, node: node}
	return
}

func (scope *blockReadScope) getBacklinkRenderNodes(n *ast.Node, refs map[string]string) ([]*ast.Node, bool) {
	return docview.BacklinkNodes(n, refs)
}

func (scope *blockReadScope) GetBacklink2(id, keyword, mentionKeyword string, sortMode, mentionSortMode int, containChildren bool) (boxID string, backlinks, backmentions []*Path, linkRefsCount, mentionsCount int) {
	keyword = strings.TrimSpace(keyword)
	var keywords []string
	if "" != keyword {
		keywords = strings.Split(keyword, " ")
	}
	mentionKeyword = strings.TrimSpace(mentionKeyword)
	backlinks, backmentions = []*Path{}, []*Path{}

	sqlBlock := scope.block(id)
	if nil == sqlBlock {
		return
	}
	rootID := sqlBlock.RootID
	boxID = sqlBlock.Box

	refs := scope.refs(id, containChildren)
	refs = scope.removeDuplicatedRefs(refs)

	linkRefs, linkRefsCount, excludeBacklinkIDs, _ := scope.buildLinkRefs(rootID, refs, keywords)
	tmpBacklinks := scope.toFlatTree(linkRefs, 0, "backlink", nil)
	for _, l := range tmpBacklinks {
		l.Blocks = nil
		backlinks = append(backlinks, l)
	}

	sort.Slice(backlinks, func(i, j int) bool {
		switch sortMode {
		case configtypes.SortModeUpdatedDESC:
			return backlinks[i].Updated > backlinks[j].Updated
		case configtypes.SortModeUpdatedASC:
			return backlinks[i].Updated < backlinks[j].Updated
		case configtypes.SortModeCreatedDESC:
			return backlinks[i].Created > backlinks[j].Created
		case configtypes.SortModeCreatedASC:
			return backlinks[i].Created < backlinks[j].Created
		case configtypes.SortModeNameDESC:
			return textsort.PinYinCompare(backlinks[j].Name, backlinks[i].Name)
		case configtypes.SortModeNameASC:
			return textsort.PinYinCompare(backlinks[i].Name, backlinks[j].Name)
		case configtypes.SortModeAlphanumDESC:
			return textsort.NaturalCompare(backlinks[j].Name, backlinks[i].Name)
		case configtypes.SortModeAlphanumASC:
			return textsort.NaturalCompare(backlinks[i].Name, backlinks[j].Name)
		}
		return backlinks[i].ID > backlinks[j].ID
	})

	mentionRefs, _ := scope.buildTreeBackmention(sqlBlock, linkRefs, mentionKeyword, excludeBacklinkIDs, 12)
	tmpBackmentions := scope.toFlatTree(mentionRefs, 0, "backlink", nil)
	for _, l := range tmpBackmentions {
		l.Blocks = nil
		backmentions = append(backmentions, l)
	}

	sort.Slice(backmentions, func(i, j int) bool {
		switch mentionSortMode {
		case configtypes.SortModeUpdatedDESC:
			return backmentions[i].Updated > backmentions[j].Updated
		case configtypes.SortModeUpdatedASC:
			return backmentions[i].Updated < backmentions[j].Updated
		case configtypes.SortModeCreatedDESC:
			return backmentions[i].Created > backmentions[j].Created
		case configtypes.SortModeCreatedASC:
			return backmentions[i].Created < backmentions[j].Created
		case configtypes.SortModeNameDESC:
			return textsort.PinYinCompare(backmentions[j].Name, backmentions[i].Name)
		case configtypes.SortModeNameASC:
			return textsort.PinYinCompare(backmentions[i].Name, backmentions[j].Name)
		case configtypes.SortModeAlphanumDESC:
			return textsort.NaturalCompare(backmentions[j].Name, backmentions[i].Name)
		case configtypes.SortModeAlphanumASC:
			return textsort.NaturalCompare(backmentions[i].Name, backmentions[j].Name)
		}
		return backmentions[i].ID > backmentions[j].ID
	})

	for _, backmention := range backmentions {
		mentionsCount += backmention.Count
	}

	// 添加笔记本名称
	var boxIDs []string
	for _, l := range backlinks {
		boxIDs = append(boxIDs, l.Box)
	}
	for _, l := range backmentions {
		boxIDs = append(boxIDs, l.Box)
	}
	boxIDs = gulu.Str.RemoveDuplicatedElem(boxIDs)
	boxNames := scope.boxNames(boxIDs)
	for _, l := range backlinks {
		name := boxNames[l.Box]
		l.HPath = name + l.HPath
	}
	for _, l := range backmentions {
		name := boxNames[l.Box]
		l.HPath = name + l.HPath
	}
	return
}

func (scope *blockReadScope) GetBacklink(id, keyword, mentionKeyword string, beforeLen int, containChildren bool) (boxID string, linkPaths, mentionPaths []*Path, linkRefsCount, mentionsCount int) {
	linkPaths = []*Path{}
	mentionPaths = []*Path{}

	sqlBlock := scope.block(id)
	if nil == sqlBlock {
		return
	}
	rootID := sqlBlock.RootID
	boxID = sqlBlock.Box

	var links []*Block
	refs := scope.refs(id, containChildren)
	refs = scope.removeDuplicatedRefs(refs)

	// 为了减少查询，组装好 IDs 后一次查出
	defSQLBlockIDs, refSQLBlockIDs := map[string]bool{}, map[string]bool{}
	var queryBlockIDs []string
	for _, ref := range refs {
		defSQLBlockIDs[ref.DefBlockID] = true
		refSQLBlockIDs[ref.BlockID] = true
		queryBlockIDs = append(queryBlockIDs, ref.DefBlockID)
		queryBlockIDs = append(queryBlockIDs, ref.BlockID)
	}
	querySQLBlocks := scope.blocks(queryBlockIDs)
	defSQLBlocksCache := map[string]*indexdb.BlockRecord{}
	for _, defSQLBlock := range querySQLBlocks {
		if nil != defSQLBlock && defSQLBlockIDs[defSQLBlock.ID] {
			defSQLBlocksCache[defSQLBlock.ID] = defSQLBlock
		}
	}
	refSQLBlocksCache := map[string]*indexdb.BlockRecord{}
	for _, refSQLBlock := range querySQLBlocks {
		if nil != refSQLBlock && refSQLBlockIDs[refSQLBlock.ID] {
			refSQLBlocksCache[refSQLBlock.ID] = refSQLBlock
		}
	}

	excludeBacklinkIDs := hashset.New()
	for _, ref := range refs {
		defSQLBlock := defSQLBlocksCache[(ref.DefBlockID)]
		if nil == defSQLBlock {
			continue
		}

		refSQLBlock := refSQLBlocksCache[ref.BlockID]
		if nil == refSQLBlock {
			continue
		}
		refBlock := scope.fromRow(refSQLBlock, "", beforeLen)
		if rootID == refBlock.RootID { // 排除当前文档内引用提及
			excludeBacklinkIDs.Add(refBlock.RootID, refBlock.ID)
		}
		defBlock := scope.fromRow(defSQLBlock, "", beforeLen)
		if defBlock.RootID == rootID { // 当前文档的定义块
			links = append(links, defBlock)
			if ref.DefBlockID == defBlock.ID {
				defBlock.Refs = append(defBlock.Refs, refBlock)
			}
		}
	}

	for _, link := range links {
		for _, ref := range link.Refs {
			excludeBacklinkIDs.Add(ref.RootID, ref.ID)
		}
		linkRefsCount += len(link.Refs)
	}

	var linkRefs []*Block
	processedParagraphs := hashset.New()
	var paragraphParentIDs []string
	for _, link := range links {
		for _, ref := range link.Refs {
			if "NodeParagraph" == ref.Type {
				paragraphParentIDs = append(paragraphParentIDs, ref.ParentID)
			}
		}
	}
	paragraphParents := scope.blocks(paragraphParentIDs)
	for _, p := range paragraphParents {
		if nil == p {
			continue
		}

		if "i" == p.Type || "h" == p.Type {
			linkRefs = append(linkRefs, scope.fromRow(p, keyword, beforeLen))
			processedParagraphs.Add(p.ID)
		}
	}
	for _, link := range links {
		for _, ref := range link.Refs {
			if "NodeParagraph" == ref.Type {
				if processedParagraphs.Contains(ref.ParentID) {
					continue
				}
			}

			ref.DefID = link.ID
			ref.DefPath = link.Path

			content := ref.Content
			if "" != keyword {
				_, content = search.MarkText(content, keyword, beforeLen, scope.s.Search.CaseSensitive)
				ref.Content = content
			}
			linkRefs = append(linkRefs, ref)
		}
	}
	linkPaths = scope.toSubTree(linkRefs, keyword)

	mentions, _ := scope.buildTreeBackmention(sqlBlock, linkRefs, mentionKeyword, excludeBacklinkIDs, beforeLen)
	mentionsCount = len(mentions)
	mentionPaths = scope.toFlatTree(mentions, 0, "backlink", nil)
	return
}

func (scope *blockReadScope) buildLinkRefs(defRootID string, refs []*indexdb.RefRecord, keywords []string) (ret []*Block, refsCount int, excludeBacklinkIDs *hashset.Set, originalRefBlockIDs map[string]string) {
	// 为了减少查询，组装好 IDs 后一次查出
	defSQLBlockIDs, refSQLBlockIDs := map[string]bool{}, map[string]bool{}
	var queryBlockIDs []string
	for _, ref := range refs {
		defSQLBlockIDs[ref.DefBlockID] = true
		refSQLBlockIDs[ref.BlockID] = true
		queryBlockIDs = append(queryBlockIDs, ref.DefBlockID)
		queryBlockIDs = append(queryBlockIDs, ref.BlockID)
	}
	queryBlockIDs = gulu.Str.RemoveDuplicatedElem(queryBlockIDs)
	querySQLBlocks := scope.blocks(queryBlockIDs)
	defSQLBlocksCache := map[string]*indexdb.BlockRecord{}
	for _, defSQLBlock := range querySQLBlocks {
		if nil != defSQLBlock && defSQLBlockIDs[defSQLBlock.ID] {
			defSQLBlocksCache[defSQLBlock.ID] = defSQLBlock
		}
	}
	refSQLBlocksCache := map[string]*indexdb.BlockRecord{}
	for _, refSQLBlock := range querySQLBlocks {
		if nil != refSQLBlock && refSQLBlockIDs[refSQLBlock.ID] {
			refSQLBlocksCache[refSQLBlock.ID] = refSQLBlock
		}
	}

	var links []*Block
	excludeBacklinkIDs = hashset.New()
	for _, ref := range refs {
		defSQLBlock := defSQLBlocksCache[(ref.DefBlockID)]
		if nil == defSQLBlock {
			continue
		}

		refSQLBlock := refSQLBlocksCache[ref.BlockID]
		if nil == refSQLBlock {
			continue
		}
		refBlock := scope.fromRow(refSQLBlock, "", 12)
		if defRootID == refBlock.RootID { // 排除当前文档内引用提及
			excludeBacklinkIDs.Add(refBlock.RootID, refBlock.ID)
		}
		defBlock := scope.fromRow(defSQLBlock, "", 12)
		if defBlock.RootID == defRootID { // 当前文档的定义块
			links = append(links, defBlock)
			if ref.DefBlockID == defBlock.ID {
				defBlock.Refs = append(defBlock.Refs, refBlock)
			}
		}
	}

	for _, link := range links {
		for _, ref := range link.Refs {
			excludeBacklinkIDs.Add(ref.RootID, ref.ID)
		}
		refsCount += len(link.Refs)
	}

	parentRefParagraphs := map[string]*Block{}
	var paragraphParentIDs []string
	for _, link := range links {
		for _, ref := range link.Refs {
			if "NodeParagraph" == ref.Type {
				parentRefParagraphs[ref.ParentID] = ref
				paragraphParentIDs = append(paragraphParentIDs, ref.ParentID)
			}
		}
	}
	refsCountDelta := len(paragraphParentIDs)
	paragraphParentIDs = gulu.Str.RemoveDuplicatedElem(paragraphParentIDs)
	refsCountDelta -= len(paragraphParentIDs)
	refsCount -= refsCountDelta
	sqlParagraphParents := scope.blocks(paragraphParentIDs)
	paragraphParents := scope.fromRows(&sqlParagraphParents, "", 12)

	luteEngine := scope.newParser()
	originalRefBlockIDs = map[string]string{}
	processedParagraphs := hashset.New()
	for _, parent := range paragraphParents {
		if nil == parent {
			continue
		}

		if "NodeListItem" == parent.Type || "NodeBlockquote" == parent.Type || "NodeSuperBlock" == parent.Type {
			refBlock := parentRefParagraphs[parent.ID]
			if nil == refBlock {
				continue
			}

			paragraphUseParentLi := true
			if "NodeListItem" == parent.Type && parent.FContent != refBlock.Content {
				if inlineTree := parse.Inline("", []byte(refBlock.Markdown), luteEngine.ParseOptions); nil != inlineTree {
					for c := inlineTree.Root.FirstChild.FirstChild; c != nil; c = c.Next {
						if docview.IsBlockRef(c) {
							continue
						}

						if "" != strings.TrimSpace(c.Text()) {
							paragraphUseParentLi = false
							break
						}
					}
				}
			}

			if paragraphUseParentLi {
				processedParagraphs.Add(parent.ID)
			}

			originalRefBlockIDs[parent.ID] = refBlock.ID
			if !scope.matchBacklinkKeyword(parent, keywords) {
				refsCount--
				continue
			}

			if paragraphUseParentLi {
				ret = append(ret, parent)
			}
		}
	}
	for _, link := range links {
		for _, ref := range link.Refs {
			if "NodeParagraph" == ref.Type {
				if processedParagraphs.Contains(ref.ParentID) {
					continue
				}
			}

			if !scope.matchBacklinkKeyword(ref, keywords) {
				refsCount--
				continue
			}

			ref.DefID = link.ID
			ref.DefPath = link.Path
			ret = append(ret, ref)
		}
	}

	if 0 < len(keywords) {
		// 过滤场景处理标题下方块 Improve backlink filtering below the heading https://github.com/siyuan-note/siyuan/issues/14929
		headingRefChildren := map[string]*Block{}
		var headingIDs []string
		for _, link := range links {
			for _, ref := range link.Refs {
				if "NodeHeading" == ref.Type {
					headingRefChildren[ref.ID] = ref
					headingIDs = append(headingIDs, ref.ID)
				}
			}
		}
		var headingChildren []*Block
		for _, headingID := range headingIDs {
			sqlChildren := scope.children(headingID, "", -1)
			children := scope.fromRows(&sqlChildren, "", 12)
			headingChildren = append(headingChildren, children...)
		}
		for _, child := range headingChildren {
			if nil == child {
				continue
			}

			if scope.matchBacklinkKeyword(child, keywords) {
				heading := headingRefChildren[child.ParentID]
				if nil != heading && !scope.existBlock(heading, ret) {
					ret = append(ret, heading)
				}
			}
		}
	}
	return
}

func (scope *blockReadScope) existBlock(block *Block, blocks []*Block) bool {
	for _, b := range blocks {
		if block.ID == b.ID {
			return true
		}
	}
	return false
}

func (scope *blockReadScope) matchBacklinkKeyword(block *Block, keywords []string) bool {
	if 1 > len(keywords) {
		return true
	}

	for _, k := range keywords {
		k = strings.ToLower(k)
		if strings.Contains(strings.ToLower(block.Content), k) ||
			strings.Contains(strings.ToLower(path.Base(block.HPath)), k) ||
			strings.Contains(strings.ToLower(block.Name), k) ||
			strings.Contains(strings.ToLower(block.Alias), k) ||
			strings.Contains(strings.ToLower(block.Memo), k) ||
			strings.Contains(strings.ToLower(block.Tag), k) {
			return true
		}
	}
	return false
}

func (scope *blockReadScope) removeDuplicatedRefs(refs []*indexdb.RefRecord) (ret []*indexdb.RefRecord) {
	// 同一个块中引用多个块后反链去重
	// De-duplication of backlinks after referencing multiple blocks in the same block https://github.com/siyuan-note/siyuan/issues/12147

	for _, ref := range refs {
		contain := false
		for _, r := range ret {
			if ref.BlockID == r.BlockID {
				contain = true
				break
			}
		}
		if !contain {
			ret = append(ret, ref)
		}
	}
	return
}

func (scope *blockReadScope) buildTreeBackmention(defSQLBlock *indexdb.BlockRecord, refBlocks []*Block, keyword string, excludeBacklinkIDs *hashset.Set, beforeLen int) (ret []*Block, mentionKeywords []string) {
	ret = []*Block{}

	var names, aliases []string
	var fName, rootID string
	if "d" == defSQLBlock.Type {
		if scope.s.Search.BacklinkMentionName {
			names = scope.names(defSQLBlock.ID)
		}
		if scope.s.Search.BacklinkMentionAlias {
			aliases = scope.aliases(defSQLBlock.ID)
		}
		if scope.s.Search.BacklinkMentionDoc {
			fName = path.Base(defSQLBlock.HPath)
		}
		rootID = defSQLBlock.ID
	} else {
		if scope.s.Search.BacklinkMentionName {
			if "" != defSQLBlock.Name {
				names = append(names, defSQLBlock.Name)
			}
		}
		if scope.s.Search.BacklinkMentionAlias {
			if "" != defSQLBlock.Alias {
				aliases = strings.Split(defSQLBlock.Alias, ",")
			}
		}
		root := scope.location(defSQLBlock.RootID)
		rootID = root.ID
	}

	set := hashset.New()
	for _, name := range names {
		set.Add(name)
	}
	for _, alias := range aliases {
		set.Add(alias)
	}
	if "" != fName {
		set.Add(fName)
	}

	if scope.s.Search.BacklinkMentionAnchor {
		for _, refBlock := range refBlocks {
			refs := scope.pair(refBlock.DefID, refBlock.ID)
			for _, ref := range refs {
				set.Add(ref.Content)
			}
		}
	}

	for _, v := range set.Values() {
		mentionKeywords = append(mentionKeywords, v.(string))
	}
	mentionKeywords = docview.PrepareKeywords(mentionKeywords)
	mentionKeywords, ret = scope.searchBackmention(mentionKeywords, keyword, excludeBacklinkIDs, rootID, beforeLen)
	return
}

func (scope *blockReadScope) searchBackmention(mentionKeywords []string, keyword string, excludeBacklinkIDs *hashset.Set, rootID string, beforeLen int) (retMentionKeywords []string, ret []*Block) {
	ret = []*Block{}
	if 1 > len(mentionKeywords) {
		return
	}

	table := "blocks_fts" // 大小写敏感
	if !scope.s.Search.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}

	buf := bytes.Buffer{}
	buf.WriteString("SELECT * FROM " + table + " WHERE " + table + " MATCH '" + scope.columnFilter() + ":(")
	for i, mentionKeyword := range mentionKeywords {
		if scope.s.Search.BacklinkMentionKeywordsLimit < i {
			scope.warningLimit(len(mentionKeywords))
			mentionKeyword = strings.ReplaceAll(strings.ReplaceAll(mentionKeyword, "'", "''"), "\"", "\"\"")
			buf.WriteString("\"" + mentionKeyword + "\"")
			break
		}

		mentionKeyword = strings.ReplaceAll(strings.ReplaceAll(mentionKeyword, "'", "''"), "\"", "\"\"")
		buf.WriteString("\"" + mentionKeyword + "\"")
		if i < len(mentionKeywords)-1 {
			buf.WriteString(" OR ")
		}
	}
	buf.WriteString(")")
	if "" != keyword {
		keyword = strings.ReplaceAll(strings.ReplaceAll(keyword, "'", "''"), "\"", "\"\"")
		buf.WriteString(" AND (\"" + keyword + "\")")
	}
	buf.WriteString("'")
	buf.WriteString(" AND root_id != '" + rootID + "'") // 不在定义块所在文档中搜索
	buf.WriteString(" AND type IN ('d', 'h', 'p', 't')")
	buf.WriteString(" ORDER BY id DESC LIMIT " + strconv.Itoa(scope.s.Search.Limit))
	query := buf.String()

	sqlBlocks := scope.selectRows(query, 1, scope.s.Search.Limit)
	terms := mentionKeywords
	if "" != keyword {
		terms = append(terms, keyword)
	}
	blocks := scope.fromRows(&sqlBlocks, strings.Join(terms, search.TermSep), beforeLen)

	luteEngine := scope.newParser()
	var tmp []*Block
	for _, b := range blocks {
		tree := parse.Parse("", gulu.Str.ToBytes(b.Markdown), luteEngine.ParseOptions)
		if nil == tree {
			continue
		}

		textBuf := &bytes.Buffer{}
		ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if scope.ctx.Err() != nil {
				scope.fail(scope.ctx.Err())
				return ast.WalkStop
			}
			if !entering || n.IsBlock() {
				return ast.WalkContinue
			}
			if ast.NodeText == n.Type /* NodeText 包含了标签命中的情况 */ || ast.NodeLinkText == n.Type {
				textBuf.Write(n.Tokens)
			}
			return ast.WalkContinue
		})

		text := textBuf.String()
		text = strings.TrimSpace(text)
		if "" == text {
			continue
		}

		newText := scope.markSplit(text, mentionKeywords, search.GetMarkSpanStart(search.MarkDataType), search.GetMarkSpanEnd())
		if text != newText {
			tmp = append(tmp, b)

			k := gulu.Str.SubstringsBetween(newText, search.GetMarkSpanStart(search.MarkDataType), search.GetMarkSpanEnd())
			retMentionKeywords = append(retMentionKeywords, k...)
		} else {
			// columnFilter 中的命名、别名和备注命中的情况
			// 反链提及搜索范围增加命名、别名和备注 https://github.com/siyuan-note/siyuan/issues/7639
			if gulu.Str.Contains(scope.trimMarkTags(b.Name), mentionKeywords) ||
				gulu.Str.Contains(scope.trimMarkTags(b.Alias), mentionKeywords) ||
				gulu.Str.Contains(scope.trimMarkTags(b.Memo), mentionKeywords) {
				tmp = append(tmp, b)
			}
		}
	}
	blocks = tmp
	retMentionKeywords = gulu.Str.RemoveDuplicatedElem(retMentionKeywords)
	mentionKeywords = retMentionKeywords

	mentionBlockMap := map[string]*Block{}
	for _, block := range blocks {
		mentionBlockMap[block.ID] = block

		refText := scope.getContainStr(block.Content, mentionKeywords)
		block.RefText = refText
	}

	for _, mentionBlock := range mentionBlockMap {
		if !excludeBacklinkIDs.Contains(mentionBlock.ID) {
			ret = append(ret, mentionBlock)
		}
	}

	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].ID > ret[j].ID
	})
	return
}

func (scope *blockReadScope) trimMarkTags(str string) string {
	return strings.TrimSuffix(strings.TrimPrefix(str, "<mark>"), "</mark>")
}

func (scope *blockReadScope) getContainStr(str string, strs []string) string {
	str = strings.ToLower(str)
	for _, s := range strs {
		if strings.Contains(str, strings.ToLower(s)) {
			return s
		}
	}
	return ""
}

func (scope *blockReadScope) toFlatTree(blocks []*Block, baseDepth int, typ string, tree *parse.Tree) (ret []*Path) {
	var blockRoots []*Block
	for _, block := range blocks {
		root := scope.getBlockIn(blockRoots, block.RootID)
		if nil == root {
			root, _ = scope.projectBlock(block.RootID, tree)
			blockRoots = append(blockRoots, root)
		}
		if nil == root {
			return
		}
		block.Depth = baseDepth + 1
		block.Count = len(block.Children)
		root.Children = append(root.Children, block)
	}

	folded := false
	if "outline" == typ {
		folded = true
	}

	for _, root := range blockRoots {
		treeNode := &Path{
			ID:       root.ID,
			Box:      root.Box,
			Name:     path.Base(root.HPath),
			NodeType: root.Type,
			Type:     typ,
			SubType:  root.SubType,
			Depth:    baseDepth,
			Count:    len(root.Children),
			Folded:   folded,

			Updated: root.IAL["updated"],
			Created: root.ID[:14],
		}
		for _, c := range root.Children {
			treeNode.Blocks = append(treeNode.Blocks, c)
		}
		ret = append(ret, treeNode)

		if "backlink" == typ {
			treeNode.HPath = root.HPath
		}
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].ID > ret[j].ID
	})
	return
}

func (scope *blockReadScope) toSubTree(blocks []*Block, keyword string) (ret []*Path) {
	keyword = strings.TrimSpace(keyword)
	var blockRoots []*Block
	for _, block := range blocks {
		root := scope.getBlockIn(blockRoots, block.RootID)
		if nil == root {
			root, _ = scope.projectBlock(block.RootID, nil)
			blockRoots = append(blockRoots, root)
		}
		block.Depth = 1
		block.Count = len(block.Children)
		root.Children = append(root.Children, block)
	}

	for _, root := range blockRoots {
		treeNode := &Path{
			ID:       root.ID,
			Box:      root.Box,
			Name:     path.Base(root.HPath),
			Type:     "backlink",
			NodeType: "NodeDocument",
			SubType:  root.SubType,
			Depth:    0,
			Count:    len(root.Children),
		}
		for _, c := range root.Children {
			if "NodeListItem" == c.Type {
				tree, _ := scope.load(c.RootID)
				li := docview.GetNodeInTree(tree, c.ID)
				if nil == li || nil == li.FirstChild {
					// 反链面板拖拽到文档以后可能会出现这种情况 https://github.com/siyuan-note/siyuan/issues/5363
					continue
				}

				var first *indexdb.BlockRecord
				if 3 != li.ListData.Typ {
					first = scope.block(li.FirstChild.ID)
				} else {
					first = scope.block(li.FirstChild.Next.ID)
				}
				name := first.Content
				parentPos := 0
				if "" != keyword {
					parentPos, name = search.MarkText(name, keyword, 12, scope.s.Search.CaseSensitive)
				}
				subRoot := &Path{
					ID:       li.ID,
					Box:      li.Box,
					Name:     name,
					Type:     "backlink",
					NodeType: li.Type.String(),
					SubType:  c.SubType,
					Depth:    1,
					Count:    1,
				}

				unfold := true
				for liFirstBlockSpan := li.FirstChild.FirstChild; nil != liFirstBlockSpan; liFirstBlockSpan = liFirstBlockSpan.Next {
					if docview.IsBlockRef(liFirstBlockSpan) {
						continue
					}
					if "" != strings.TrimSpace(liFirstBlockSpan.Text()) {
						unfold = false
						break
					}
				}
				for next := li.FirstChild.Next; nil != next; next = next.Next {
					subBlock, _ := scope.projectBlock(next.ID, tree)
					if unfold {
						if ast.NodeList == next.Type {
							for subLi := next.FirstChild; nil != subLi; subLi = subLi.Next {
								subLiBlock, _ := scope.projectBlock(subLi.ID, tree)
								var subFirst *indexdb.BlockRecord
								if 3 != subLi.ListData.Typ {
									subFirst = scope.block(subLi.FirstChild.ID)
								} else {
									subFirst = scope.block(subLi.FirstChild.Next.ID)
								}
								subPos := 0
								content := subFirst.Content
								if "" != keyword {
									subPos, content = search.MarkText(subFirst.Content, keyword, 12, scope.s.Search.CaseSensitive)
								}
								if -1 < subPos {
									parentPos = 0 // 需要显示父级
								}
								subLiBlock.Content = content
								subLiBlock.Depth = 2
								subRoot.Blocks = append(subRoot.Blocks, subLiBlock)
							}
						} else if ast.NodeHeading == next.Type {
							subBlock.Depth = 2
							subRoot.Blocks = append(subRoot.Blocks, subBlock)
							headingChildren := docview.HeadingChildren(next)
							var breakSub bool
							for _, n := range headingChildren {
								block, _ := scope.projectBlock(n.ID, tree)
								subPos := 0
								content := block.Content
								if "" != keyword {
									subPos, content = search.MarkText(block.Content, keyword, 12, scope.s.Search.CaseSensitive)
								}
								if -1 < subPos {
									parentPos = 0
								}
								block.Content = content
								block.Depth = 3
								subRoot.Blocks = append(subRoot.Blocks, block)
								if ast.NodeHeading == n.Type {
									// 跳过子标题下面的块
									breakSub = true
									break
								}
							}
							if breakSub {
								break
							}
						} else {
							if nil == docview.HeadingParent(next) {
								subBlock.Depth = 2
								subRoot.Blocks = append(subRoot.Blocks, subBlock)
							}
						}
					}
				}
				if -1 < parentPos {
					treeNode.Children = append(treeNode.Children, subRoot)
				}
			} else if "NodeHeading" == c.Type {
				tree, _ := scope.load(c.RootID)
				h := docview.GetNodeInTree(tree, c.ID)
				if nil == h {
					continue
				}

				name := scope.block(h.ID).Content
				parentPos := 0
				if "" != keyword {
					parentPos, name = search.MarkText(name, keyword, 12, scope.s.Search.CaseSensitive)
				}
				subRoot := &Path{
					ID:       h.ID,
					Box:      h.Box,
					Name:     name,
					Type:     "backlink",
					NodeType: h.Type.String(),
					SubType:  c.SubType,
					Depth:    1,
					Count:    1,
				}

				unfold := true
				for headingFirstSpan := h.FirstChild; nil != headingFirstSpan; headingFirstSpan = headingFirstSpan.Next {
					if docview.IsBlockRef(headingFirstSpan) {
						continue
					}
					if "" != strings.TrimSpace(headingFirstSpan.Text()) {
						unfold = false
						break
					}
				}

				if unfold {
					headingChildren := docview.HeadingChildren(h)
					for _, headingChild := range headingChildren {
						if ast.NodeList == headingChild.Type {
							for subLi := headingChild.FirstChild; nil != subLi; subLi = subLi.Next {
								subLiBlock, _ := scope.projectBlock(subLi.ID, tree)
								var subFirst *indexdb.BlockRecord
								if 3 != subLi.ListData.Typ {
									subFirst = scope.block(subLi.FirstChild.ID)
								} else {
									subFirst = scope.block(subLi.FirstChild.Next.ID)
								}
								subPos := 0
								content := subFirst.Content
								if "" != keyword {
									subPos, content = search.MarkText(content, keyword, 12, scope.s.Search.CaseSensitive)
								}
								if -1 < subPos {
									parentPos = 0
								}
								subLiBlock.Content = subFirst.Content
								subLiBlock.Depth = 2
								subRoot.Blocks = append(subRoot.Blocks, subLiBlock)
							}
						} else {
							subBlock, _ := scope.projectBlock(headingChild.ID, tree)
							subBlock.Depth = 2
							subRoot.Blocks = append(subRoot.Blocks, subBlock)
						}
					}
				}

				if -1 < parentPos {
					treeNode.Children = append(treeNode.Children, subRoot)
				}
			} else {
				pos := 0
				content := c.Content
				if "" != keyword {
					pos, content = search.MarkText(content, keyword, 12, scope.s.Search.CaseSensitive)
				}
				if -1 < pos {
					treeNode.Blocks = append(treeNode.Blocks, c)
				}
			}
		}

		rootPos := -1
		var rootContent string
		if "" != keyword {
			rootPos, rootContent = search.MarkText(treeNode.Name, keyword, 12, scope.s.Search.CaseSensitive)
			treeNode.Name = rootContent
		}
		if 0 < len(treeNode.Children) || 0 < len(treeNode.Blocks) || (-1 < rootPos && "" != keyword) {
			ret = append(ret, treeNode)
		}
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].ID > ret[j].ID
	})
	return
}

func (scope *blockReadScope) getBlockIn(blocks []*Block, id string) *Block {
	if "" == id {
		return nil
	}
	for _, block := range blocks {
		if block.ID == id {
			return block
		}
	}
	return nil
}
