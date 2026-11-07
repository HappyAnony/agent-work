// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model graph assembly, with explicit immutable settings and indexes.
package graphview

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"math"
	"strings"
	"unicode/utf8"
)

type GraphNode struct {
	ID    string  `json:"id"`
	Box   string  `json:"box"`
	Path  string  `json:"path"`
	Size  float64 `json:"size"`
	Title string  `json:"title,omitempty"`
	Label string  `json:"label"`
	Type  string  `json:"type"`
	Refs  int     `json:"refs"`
	Defs  int     `json:"defs"`
}

type GraphLink struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Ref    bool         `json:"ref"`
	Arrows *GraphArrows `json:"arrows"`
}

type GraphArrows struct {
	To *GraphArrowsTo `json:"to"`
}

type GraphArrowsTo struct {
	Enabled bool `json:"enabled"`
}

func (r *run) BuildTreeGraph(id, query string) (boxID string, nodes []*GraphNode, links []*GraphLink) {
	nodes = []*GraphNode{}
	links = []*GraphLink{}

	block := r.block(id)
	if block == nil {
		return
	}
	boxID = block.Box

	stmt := r.query2Stmt(query)
	stmt += r.graphTypeFilter(true)
	stmt += r.graphDailyNoteFilter(true)
	stmt = strings.ReplaceAll(stmt, "content", "ref.content")
	forwardlinks, backlinks := r.fullLinks(stmt)

	var sqlBlocks []*Block
	var rootID string
	if "NodeDocument" == block.Type {
		sqlBlocks = r.allChildren([]string{block.ID}, stmt, r.Graph.MaxBlocks)
		rootID = block.ID
	} else {
		sqlBlocks = r.children(block.ID, stmt, r.Graph.MaxBlocks)
	}
	blocks := sqlBlocks
	if "" != rootID {
		// 局部关系图中添加文档链接关系 https://github.com/siyuan-note/siyuan/issues/4996
		rootBlock := getBlockIn(blocks, rootID)
		if nil != rootBlock {
			// 按引用处理
			sqlRootDefs := r.defRoots(rootID)
			rootDefBlocks := sqlRootDefs
			var rootIDs []string
			for _, rootDef := range rootDefBlocks {
				if r.stop() {
					break
				}
				blocks = append(blocks, rootDef)
				rootIDs = append(rootIDs, rootDef.ID)
			}

			sqlRefBlocks := r.refRoots(rootIDs)
			for defRootID, sqlRefBs := range sqlRefBlocks {
				if r.stop() {
					break
				}
				rootB := getBlockIn(rootDefBlocks, defRootID)
				if nil == rootB {
					continue
				}

				blocks = append(blocks, rootB)
				refBlocks := sqlRefBs
				rootB.Refs = append(rootB.Refs, refBlocks...)
				blocks = append(blocks, refBlocks...)
			}

			// 按定义处理
			blocks = append(blocks, rootBlock)
			sqlRefBlocks = r.refRoots([]string{rootID})

			// 关系图日记过滤失效 https://github.com/siyuan-note/siyuan/issues/7547
			dailyNotesPaths := r.dailyNotePaths(true)
			for _, sqlRefBs := range sqlRefBlocks {
				if r.stop() {
					break
				}
				refBlocks := sqlRefBs

				if 0 < len(dailyNotesPaths) {
					isDailyNote := false
					var tmp []*Block
					for _, refBlock := range refBlocks {
						if r.stop() {
							break
						}
						for _, dailyNotePath := range dailyNotesPaths {
							if r.stop() {
								break
							}
							if strings.HasPrefix(refBlock.HPath, dailyNotePath) {
								isDailyNote = true
								break
							}
						}

						if !isDailyNote {
							tmp = append(tmp, refBlock)
						}
					}
					refBlocks = tmp
				}

				rootBlock.Refs = append(rootBlock.Refs, refBlocks...)
				blocks = append(blocks, refBlocks...)
			}
		}
	}

	blocks = r.filterDailyNote(blocks, true)
	r.genTreeNodes(blocks, &nodes, &links, true)
	r.growTreeGraph(&forwardlinks, &backlinks, &nodes)
	blocks = append(blocks, forwardlinks...)
	blocks = append(blocks, backlinks...)
	r.buildLinks(&blocks, &links, true)
	if r.Graph.Local.Tag {
		p := block.Path
		r.linkTagBlocks(&blocks, &nodes, &links, p)
	}
	r.markLinkedNodes(&nodes, &links, true)
	nodes = r.removeDuplicatedUnescape(nodes)
	return
}

func (r *run) BuildGraph(query string) (boxID string, nodes []*GraphNode, links []*GraphLink) {
	nodes = []*GraphNode{}
	links = []*GraphLink{}

	stmt := r.query2Stmt(query)
	stmt = strings.TrimPrefix(stmt, "select * from blocks where")
	stmt += r.graphTypeFilter(false)
	stmt += r.graphDailyNoteFilter(false)
	stmt = strings.ReplaceAll(stmt, "content", "ref.content")
	forwardlinks, backlinks := r.fullLinks(stmt)

	var blocks []*Block
	roots := r.roots()
	if 0 < len(roots) {
		boxID = roots[0].Box
	}
	var rootIDs []string
	for _, root := range roots {
		if r.stop() {
			break
		}
		rootIDs = append(rootIDs, root.ID)
	}
	rootIDs = gulu.Str.RemoveDuplicatedElem(rootIDs)

	sqlBlocks := r.allChildren(rootIDs, stmt, r.Graph.MaxBlocks)
	treeBlocks := sqlBlocks
	treeBlocks = r.filterDailyNote(treeBlocks, false)
	r.genTreeNodes(treeBlocks, &nodes, &links, false)
	blocks = append(blocks, treeBlocks...)

	// 文档块关联
	sqlRootRefBlocks := r.refRoots(rootIDs)
	for defRootID, sqlRefBlocks := range sqlRootRefBlocks {
		if r.stop() {
			break
		}
		rootBlock := getBlockIn(treeBlocks, defRootID)
		if nil == rootBlock {
			continue
		}

		refBlocks := sqlRefBlocks
		rootBlock.Refs = append(rootBlock.Refs, refBlocks...)
	}

	r.growTreeGraph(&forwardlinks, &backlinks, &nodes)
	blocks = append(blocks, forwardlinks...)
	blocks = append(blocks, backlinks...)
	r.buildLinks(&blocks, &links, false)
	if r.Graph.Global.Tag {
		r.linkTagBlocks(&blocks, &nodes, &links, "")
	}
	r.markLinkedNodes(&nodes, &links, false)
	r.pruneUnref(&nodes, &links)
	nodes = r.removeDuplicatedUnescape(nodes)
	return
}

func (r *run) linkTagBlocks(blocks *[]*Block, nodes *[]*GraphNode, links *[]*GraphLink, p string) {
	tagSpans := r.tags(p)
	if 1 > len(tagSpans) {
		return
	}

	isGlobal := "" == p
	nodeSize := r.Graph.Global.NodeSize
	if !isGlobal {
		nodeSize = r.Graph.Local.NodeSize
	}

	// 构造标签节点
	var tagNodes []*GraphNode
	for _, tagSpan := range tagSpans {
		if r.stop() {
			break
		}
		if nil == r.tagNodeIn(tagNodes, tagSpan.Content) {
			node := &GraphNode{
				ID:    tagSpan.Content,
				Label: tagSpan.Content,
				Size:  nodeSize,
				Type:  tagSpan.Type,
			}
			*nodes = append(*nodes, node)
			tagNodes = append(tagNodes, node)
		}
	}

	// 连接标签和块
	for _, block := range *blocks {
		if r.stop() {
			break
		}
		for _, tagSpan := range tagSpans {
			if r.stop() {
				break
			}
			if isGlobal { // 全局关系图将标签链接到文档块上
				if block.RootID == tagSpan.RootID { // 局部关系图将标签链接到子块上
					*links = append(*links, &GraphLink{
						From: tagSpan.Content,
						To:   block.RootID,
					})
				}
			} else {
				if block.ID == tagSpan.BlockID { // 局部关系图将标签链接到子块上
					*links = append(*links, &GraphLink{
						From: tagSpan.Content,
						To:   block.ID,
					})
				}
			}
		}
	}

	// 连接层级标签
	for _, tagNode := range tagNodes {
		if r.stop() {
			break
		}
		ids := strings.Split(tagNode.ID, "/")
		if 2 > len(ids) {
			continue
		}

		for _, targetID := range ids[:len(ids)-1] {
			if r.stop() {
				break
			}
			if targetTag := r.tagNodeIn(tagNodes, targetID); nil != targetTag {

				*links = append(*links, &GraphLink{
					From: tagNode.ID,
					To:   targetID,
				})
			}
		}
	}
}

func (r *run) tagNodeIn(tagNodes []*GraphNode, content string) *GraphNode {
	for _, tagNode := range tagNodes {
		if r.stop() {
			break
		}
		if tagNode.Label == content {
			return tagNode
		}
	}
	return nil
}

func (r *run) growTreeGraph(forwardlinks, backlinks *[]*Block, nodes *[]*GraphNode) {
	forwardDepth, backDepth := 0, 0
	r.growLinkedNodes(forwardlinks, backlinks, nodes, nodes, &forwardDepth, &backDepth)
}

func (r *run) growLinkedNodes(forwardlinks, backlinks *[]*Block, nodes, all *[]*GraphNode, forwardDepth, backDepth *int) {
	if 1 > len(*nodes) {
		return
	}

	forwardGeneration := &[]*GraphNode{}
	if 16 > *forwardDepth {
		for _, ref := range *forwardlinks {
			if r.stop() {
				break
			}
			for _, node := range *nodes {
				if r.stop() {
					break
				}
				if node.ID == ref.ID {
					var defs []*Block
					for _, refDef := range ref.Defs {
						if r.stop() {
							break
						}
						if r.existNodes(all, refDef.ID) || r.existNodes(forwardGeneration, refDef.ID) || r.existNodes(nodes, refDef.ID) {
							continue
						}
						defs = append(defs, refDef)
					}

					for _, refDef := range defs {
						if r.stop() {
							break
						}
						defNode := &GraphNode{
							ID:   refDef.ID,
							Box:  refDef.Box,
							Path: refDef.Path,
							Size: r.Graph.Local.NodeSize,
							Type: refDef.Type,
						}
						r.nodeTitleLabel(defNode, r.nodeContentByBlock(refDef))
						*forwardGeneration = append(*forwardGeneration, defNode)
					}
				}
			}
		}
	}

	backGeneration := &[]*GraphNode{}
	if 16 > *backDepth {
		for _, def := range *backlinks {
			if r.stop() {
				break
			}
			for _, node := range *nodes {
				if r.stop() {
					break
				}
				if node.ID == def.ID {
					for _, ref := range def.Refs {
						if r.stop() {
							break
						}
						if r.existNodes(all, ref.ID) || r.existNodes(backGeneration, ref.ID) || r.existNodes(nodes, ref.ID) {
							continue
						}

						refNode := &GraphNode{
							ID:   ref.ID,
							Box:  ref.Box,
							Path: ref.Path,
							Size: r.Graph.Local.NodeSize,
							Type: ref.Type,
						}
						r.nodeTitleLabel(refNode, r.nodeContentByBlock(ref))
						*backGeneration = append(*backGeneration, refNode)
					}
				}
			}
		}
	}

	generation := &[]*GraphNode{}
	*generation = append(*generation, *forwardGeneration...)
	*generation = append(*generation, *backGeneration...)
	*forwardDepth++
	*backDepth++
	r.growLinkedNodes(forwardlinks, backlinks, generation, nodes, forwardDepth, backDepth)
	*nodes = append(*nodes, *generation...)
}

func (r *run) existNodes(nodes *[]*GraphNode, id string) bool {
	for _, node := range *nodes {
		if r.stop() {
			break
		}
		if node.ID == id {
			return true
		}
	}
	return false
}

func (r *run) buildLinks(defs *[]*Block, links *[]*GraphLink, local bool) {
	for _, def := range *defs {
		if r.stop() {
			break
		}
		for _, ref := range def.Refs {
			if r.stop() {
				break
			}
			link := &GraphLink{
				From: ref.ID,
				To:   def.ID,
				Ref:  true,
			}
			if local {
				if r.Graph.Local.Arrow {
					link.Arrows = &GraphArrows{To: &GraphArrowsTo{Enabled: true}}
				}
			} else {
				if r.Graph.Global.Arrow {
					link.Arrows = &GraphArrows{To: &GraphArrowsTo{Enabled: true}}
				}
			}
			*links = append(*links, link)
		}
	}
}

func (r *run) genTreeNodes(blocks []*Block, nodes *[]*GraphNode, links *[]*GraphLink, local bool) {
	nodeSize := r.Graph.Local.NodeSize
	if !local {
		nodeSize = r.Graph.Global.NodeSize
	}

	for _, block := range blocks {
		if r.stop() {
			break
		}
		node := &GraphNode{
			ID:   block.ID,
			Box:  block.Box,
			Path: block.Path,
			Type: block.Type,
			Size: nodeSize,
		}
		r.nodeTitleLabel(node, r.nodeContentByBlock(block))
		*nodes = append(*nodes, node)

		*links = append(*links, &GraphLink{
			From: block.ParentID,
			To:   block.ID,
			Ref:  false,
		})
	}
}

func (r *run) markLinkedNodes(nodes *[]*GraphNode, links *[]*GraphLink, local bool) {
	nodeSize := r.Graph.Local.NodeSize
	if !local {
		nodeSize = r.Graph.Global.NodeSize
	}

	tmpLinks := (*links)[:0]
	for _, link := range *links {
		if r.stop() {
			break
		}
		var sourceFound, targetFound bool
		for _, node := range *nodes {
			if r.stop() {
				break
			}
			if link.To == node.ID {
				if link.Ref {
					size := nodeSize
					node.Defs++
					size = math.Log2(float64(node.Defs))*nodeSize + nodeSize
					node.Size = size
				}
				targetFound = true
			} else if link.From == node.ID {
				node.Refs++
				sourceFound = true
			}
			if targetFound && sourceFound {
				break
			}
		}
		if sourceFound && targetFound {
			tmpLinks = append(tmpLinks, link)
		}
	}
	*links = tmpLinks
}

func (r *run) removeDuplicatedUnescape(nodes []*GraphNode) (ret []*GraphNode) {
	m := map[string]*GraphNode{}
	for _, n := range nodes {
		if r.stop() {
			break
		}
		if nil == m[n.ID] {
			n.Title = html.UnescapeString(n.Title)
			n.Label = html.UnescapeString(n.Label)
			ret = append(ret, n)
			m[n.ID] = n
		}
	}
	return ret
}

func (r *run) pruneUnref(nodes *[]*GraphNode, links *[]*GraphLink) {
	maxBlocks := r.Graph.MaxBlocks
	tmpNodes := (*nodes)[:0]
	for _, node := range *nodes {
		if r.stop() {
			break
		}
		if 0 == r.Graph.Global.MinRefs {
			tmpNodes = append(tmpNodes, node)
		} else {
			if r.Graph.Global.MinRefs <= node.Refs {
				tmpNodes = append(tmpNodes, node)
				continue
			}

			if r.Graph.Global.MinRefs <= node.Defs {
				tmpNodes = append(tmpNodes, node)
				continue
			}
		}

		if maxBlocks < len(tmpNodes) {
			r.warn("exceeded the maximum number of render nodes [%d]", maxBlocks)
			break
		}
	}
	*nodes = tmpNodes

	tmpLinks := (*links)[:0]
	for _, link := range *links {
		if r.stop() {
			break
		}
		var sourceFound, targetFound bool
		for _, node := range *nodes {
			if r.stop() {
				break
			}
			if link.To == node.ID {
				targetFound = true
			} else if link.From == node.ID {
				sourceFound = true
			}
		}
		if sourceFound && targetFound {
			tmpLinks = append(tmpLinks, link)
		}
	}
	*links = tmpLinks
}

func (r *run) nodeContentByBlock(block *Block) (ret string) {
	if ret = block.Name; "" != ret {
		return
	}
	ret = block.Content
	if maxLen := 48; maxLen < utf8.RuneCountInString(ret) {
		ret = gulu.Str.SubStr(ret, maxLen) + "..."
	}
	return
}

func (r *run) graphTypeFilter(local bool) string {
	var inList []string

	paragraph := r.Graph.Local.Paragraph
	if !local {
		paragraph = r.Graph.Global.Paragraph
	}
	if paragraph {
		inList = append(inList, "'p'")
	}

	heading := r.Graph.Local.Heading
	if !local {
		heading = r.Graph.Global.Heading
	}
	if heading {
		inList = append(inList, "'h'")
	}

	math := r.Graph.Local.Math
	if !local {
		math = r.Graph.Global.Math
	}
	if math {
		inList = append(inList, "'m'")
	}

	code := r.Graph.Local.Code
	if !local {
		code = r.Graph.Global.Code
	}
	if code {
		inList = append(inList, "'c'")
	}

	table := r.Graph.Local.Table
	if !local {
		table = r.Graph.Global.Table
	}
	if table {
		inList = append(inList, "'t'")
	}

	list := r.Graph.Local.List
	if !local {
		list = r.Graph.Global.List
	}
	if list {
		inList = append(inList, "'l'")
	}

	listItem := r.Graph.Local.ListItem
	if !local {
		listItem = r.Graph.Global.ListItem
	}
	if listItem {
		inList = append(inList, "'i'")
	}

	blockquote := r.Graph.Local.Blockquote
	if !local {
		blockquote = r.Graph.Global.Blockquote
	}
	if blockquote {
		inList = append(inList, "'b'")
	}

	super := r.Graph.Local.Super
	if !local {
		super = r.Graph.Global.Super
	}
	if super {
		inList = append(inList, "'s'")
	}

	inList = append(inList, "'d'")
	return " AND ref.type IN (" + strings.Join(inList, ",") + ")"
}

func (r *run) filterDailyNote(blocks []*Block, local bool) (ret []*Block) {
	// Graph dailynote filtering not working https://github.com/siyuan-note/siyuan/issues/16463

	dailyNote := r.Graph.Local.DailyNote
	if !local {
		dailyNote = r.Graph.Global.DailyNote
	}

	if dailyNote {
		ret = blocks
		return
	}

	for _, block := range blocks {
		if r.stop() {
			break
		}
		isDailyNote := false
		for k, _ := range block.IAL {
			if r.stop() {
				break
			}
			isDailyNote = strings.HasPrefix(k, "custom-dailynote-")
			if isDailyNote {
				break
			}
		}
		if !isDailyNote {
			ret = append(ret, block)
		}
	}
	return
}

func (r *run) graphDailyNoteFilter(local bool) string {
	dailyNotesPaths := r.dailyNotePaths(local)
	if 1 > len(dailyNotesPaths) {
		return ""
	}

	buf := bytes.Buffer{}
	for _, p := range dailyNotesPaths {
		if r.stop() {
			break
		}
		buf.WriteString(" AND ref.hpath NOT LIKE '" + strings.ReplaceAll(p, "'", "''") + "%'")
	}
	return buf.String()
}

func (r *run) dailyNotePaths(local bool) (ret []string) {
	dailyNote := r.Graph.Local.DailyNote
	if !local {
		dailyNote = r.Graph.Global.DailyNote
	}

	if dailyNote {
		return
	}

	ret = append(ret, r.DailyPaths...)

	return
}

func (r *run) nodeTitleLabel(node *GraphNode, blockContent string) {
	if "NodeDocument" != node.Type && "NodeHeading" != node.Type {
		node.Title = blockContent
	} else {
		node.Label = blockContent
	}
}

func (r *run) query2Stmt(queryStr string) (ret string) {
	buf := bytes.Buffer{}
	if ast.IsNodeIDPattern(queryStr) {
		buf.WriteString("id = '" + queryStr + "'")
	} else {
		var tags []string
		luteEngine := r.Parser
		t := parse.Inline("", []byte(queryStr), luteEngine.ParseOptions)
		ast.Walk(t.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering {
				return ast.WalkContinue
			}
			if n.IsTextMarkType("tag") {
				tags = append(tags, n.Text())
			}
			return ast.WalkContinue
		})

		for _, tag := range tags {
			if r.stop() {
				break
			}
			queryStr = strings.ReplaceAll(queryStr, "#"+tag+"#", "")
		}
		parts := strings.Fields(queryStr)

		for i, part := range parts {
			if r.stop() {
				break
			}
			if "" == part {
				continue
			}
			part = strings.ReplaceAll(part, "'", "''")
			buf.WriteString("(content LIKE '%" + part + "%'")
			buf.WriteString(r.Search.NAMFilter(part))
			buf.WriteString(")")
			if i < len(parts)-1 {
				buf.WriteString(" AND ")
			}
		}

		if 0 < len(tags) {
			if 0 < buf.Len() {
				buf.WriteString(" OR ")
			}
			for i, tag := range tags {
				tag = strings.ReplaceAll(tag, "'", "''")
				if r.stop() {
					break
				}
				buf.WriteString("(content LIKE '%#" + tag + "#%')")
				if i < len(tags)-1 {
					buf.WriteString(" AND ")
				}
			}
			buf.WriteString(" OR ")
			for i, tag := range tags {
				tag = strings.ReplaceAll(tag, "'", "''")
				if r.stop() {
					break
				}
				buf.WriteString("ial LIKE '%tags=\"%" + tag + "%\"%'")
				if i < len(tags)-1 {
					buf.WriteString(" AND ")
				}
			}
		}
	}
	if 1 > buf.Len() {
		buf.WriteString("1=1")
	}
	ret = buf.String()
	return
}
