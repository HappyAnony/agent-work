// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/88250/vitess-sqlparser/sqlparser"
	"github.com/jinzhu/copier"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/search"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func ListInvalidBlockRefs(page, size int) ([]*Block, int, int, int) {
	a, b, c, d, e := ListInvalidBlockRefsContext(context.Background(), page, size)
	if e != nil {
		logging.LogErrorf("invalid references: %s", e)
	}
	return a, b, c, d
}

type EmbedBlock = blockview.EmbedBlock

func UpdateEmbedBlock(id, content string) (err error) {
	return UpdateEmbedBlockContext(context.Background(), id, content)
}

func GetEmbedBlock(id string, include []string, headingMode int, breadcrumb bool) (ret []*EmbedBlock) {
	ret, e := GetEmbedBlockContext(context.Background(), id, include, headingMode, breadcrumb)
	if e != nil {
		logging.LogErrorf("embed blocks: %s", e)
	}
	return ret
}

func SearchEmbedBlock(id, stmt string, exclude []string, headingMode int, breadcrumb bool) (ret []*EmbedBlock) {
	ret, e := SearchEmbedBlockContext(context.Background(), id, stmt, exclude, headingMode, breadcrumb)
	if e != nil {
		logging.LogErrorf("embed search: %s", e)
	}
	return ret
}

func SearchRefBlock(id, rootID, keyword string, beforeLen int, square, database bool) ([]*Block, bool) {
	v, n, e := SearchRefBlockContext(context.Background(), id, rootID, keyword, beforeLen, square, database)
	if e != nil {
		logging.LogErrorf("reference search: %s", e)
	}
	return v, n
}

func filterSelfHPath(blocks []*Block) {
	// 简化搜索结果列表中的文档块路径 Simplify document block paths in search results https://github.com/siyuan-note/siyuan/issues/13364
	// 文档块不显示自己的路径（最后一层）

	for _, b := range blocks {
		if b.IsDoc() {
			b.HPath = strings.TrimSuffix(b.HPath, path.Base(b.HPath))
		}
	}
}

func prependNotebookNameInHPath(blocks []*Block) {
	// 在 hPath 中加入笔记本名 Show notebooks in hpath of block ref search list results https://github.com/siyuan-note/siyuan/issues/9378

	var boxIDs []string
	for _, b := range blocks {
		boxIDs = append(boxIDs, b.Box)
	}
	boxIDs = gulu.Str.RemoveDuplicatedElem(boxIDs)
	boxNames := Conf.BoxNames(boxIDs)
	for _, b := range blocks {
		name := boxNames[b.Box]
		b.HPath = util.EscapeHTML(name) + b.HPath
	}
}

func FindReplace(keyword, replacement string, replaceTypes map[string]bool, ids []string, paths, boxes []string, types map[string]bool, method, orderBy, groupBy int) error {
	return FindReplaceContext(context.Background(), keyword, replacement, replaceTypes, ids, paths, boxes, types, method, orderBy, groupBy)
}

func replaceNodeTextMarkTextContent(n *ast.Node, method int, keyword, escapedKey string, replacement string, r *regexp.Regexp, typ string, luteEngine *lute.Lute, referenceCount func(string) error) (err error) {
	if 0 == method {
		if strings.Contains(typ, "tag") {
			keyword = strings.TrimPrefix(keyword, "#")
			keyword = strings.TrimSuffix(keyword, "#")
			escapedKey = strings.TrimPrefix(escapedKey, "#")
			escapedKey = strings.TrimSuffix(escapedKey, "#")
			if strings.HasPrefix(replacement, "#") && strings.HasSuffix(replacement, "#") {
				replacement = strings.TrimPrefix(replacement, "#")
				replacement = strings.TrimSuffix(replacement, "#")
			} else if n.TextMarkTextContent == keyword || n.TextMarkTextContent == escapedKey {
				// 将标签转换为纯文本

				if "tag" == n.TextMarkType { // 没有其他类型，仅是标签时直接转换
					content := n.TextMarkTextContent
					if strings.Contains(content, escapedKey) {
						content = strings.ReplaceAll(content, escapedKey, replacement)
					} else if strings.Contains(content, keyword) {
						content = strings.ReplaceAll(content, keyword, replacement)
					}
					content = strings.ReplaceAll(content, editor.Zwsp, "")

					tree := parse.Inline("", []byte(content), luteEngine.ParseOptions)
					if nil == tree.Root.FirstChild {
						return
					}
					parse.NestedInlines2FlattedSpans(tree, false)

					var replaceNodes []*ast.Node
					for rNode := tree.Root.FirstChild.FirstChild; nil != rNode; rNode = rNode.Next {
						replaceNodes = append(replaceNodes, rNode)
						if blockRefID, _, _ := treenode.GetBlockRef(rNode); "" != blockRefID {
							if err = referenceCount(blockRefID); err != nil {
								return
							}
						}
					}

					for _, rNode := range replaceNodes {
						n.InsertBefore(rNode)
					}
					n.TextMarkTextContent = ""
					return
				}

				// 存在其他类型时仅移除标签类型
				n.TextMarkType = strings.ReplaceAll(n.TextMarkType, "tag", "")
				n.TextMarkType = strings.TrimSpace(n.TextMarkType)
			} else if strings.Contains(n.TextMarkTextContent, keyword) || strings.Contains(n.TextMarkTextContent, escapedKey) { // 标签包含了部分关键字的情况
				if "tag" == n.TextMarkType { // 没有其他类型，仅是标签时保持标签类型不变，仅替换标签部分内容
					content := n.TextMarkTextContent
					if strings.Contains(content, escapedKey) {
						content = strings.ReplaceAll(content, escapedKey, replacement)
					} else if strings.Contains(content, keyword) {
						content = strings.ReplaceAll(content, keyword, replacement)
					}
					content = strings.ReplaceAll(content, editor.Zwsp, "")
					n.TextMarkTextContent = content
					return
				}
			}
		}

		if strings.Contains(n.TextMarkTextContent, escapedKey) {
			n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, escapedKey, util.EscapeHTML(replacement))
		} else if strings.Contains(n.TextMarkTextContent, keyword) {
			n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, keyword, replacement)
		}
		n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, editor.Zwsp, "")
	} else if 3 == method {
		if nil != r && r.MatchString(n.TextMarkTextContent) {
			n.TextMarkTextContent = r.ReplaceAllString(n.TextMarkTextContent, replacement)
		}
		n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, editor.Zwsp, "")
	}
	return
}

// replaceTextNode 替换文本节点为其他节点。
// Supports replacing text elements with other elements https://github.com/siyuan-note/siyuan/issues/11058
func replaceTextNode(text *ast.Node, method int, keyword string, replacement string, r *regexp.Regexp, luteEngine *lute.Lute, caseSensitive bool) bool {
	if 0 == method {
		newContent := text.Tokens
		if caseSensitive {
			if bytes.Contains(text.Tokens, []byte(keyword)) {
				newContent = bytes.ReplaceAll(text.Tokens, []byte(keyword), []byte(replacement))
			}
		} else {
			if "" != strings.TrimSpace(keyword) {
				// 当搜索结果中的文本元素包含大小写混合时替换失败
				// Replace fails when search results contain mixed case in text elements https://github.com/siyuan-note/siyuan/issues/9171
				keywords := strings.Split(keyword, " ")
				// keyword 可能是 "foo Foo" 使用空格分隔的大小写命中情况，这里统一转换小写后去重
				if 0 < len(keywords) {
					var lowerKeywords []string
					for _, k := range keywords {
						lowerKeywords = append(lowerKeywords, strings.ToLower(k))
					}
					keyword = strings.Join(lowerKeywords, " ")
				}
			}

			if bytes.Contains(bytes.ToLower(text.Tokens), []byte(keyword)) {
				newContent = replaceCaseInsensitive(text.Tokens, []byte(keyword), []byte(replacement))
			}
		}
		if !bytes.Equal(newContent, text.Tokens) {
			tree := parse.Inline("", newContent, luteEngine.ParseOptions)
			if nil == tree.Root.FirstChild {
				return false
			}
			parse.NestedInlines2FlattedSpans(tree, false)

			var replaceNodes []*ast.Node
			for rNode := tree.Root.FirstChild.FirstChild; nil != rNode; rNode = rNode.Next {
				replaceNodes = append(replaceNodes, rNode)
			}

			for _, rNode := range replaceNodes {
				text.InsertBefore(rNode)
			}
			return true
		}
	} else if 3 == method {
		if nil != r && r.MatchString(string(text.Tokens)) {
			newContent := []byte(r.ReplaceAllString(string(text.Tokens), replacement))
			tree := parse.Inline("", newContent, luteEngine.ParseOptions)
			if nil == tree.Root.FirstChild {
				return false
			}

			var replaceNodes []*ast.Node
			for rNode := tree.Root.FirstChild.FirstChild; nil != rNode; rNode = rNode.Next {
				replaceNodes = append(replaceNodes, rNode)
			}

			for _, rNode := range replaceNodes {
				text.InsertBefore(rNode)
			}
			return true
		}
	}
	return false
}

func replaceNodeTokens(n *ast.Node, method int, keyword string, replacement string, r *regexp.Regexp) {
	if 0 == method {
		if bytes.Contains(n.Tokens, []byte(keyword)) {
			n.Tokens = bytes.ReplaceAll(n.Tokens, []byte(keyword), []byte(replacement))
		}
	} else if 3 == method {
		if nil != r && r.MatchString(string(n.Tokens)) {
			n.Tokens = []byte(r.ReplaceAllString(string(n.Tokens), replacement))
		}
	}
}

func mergeSamePreNext(n *ast.Node) {
	prev, next := n.Previous, n.Next
	if nil != n.Parent && ast.NodeImage == n.Parent.Type {
		prev = n.Parent.Previous
		next = n.Parent.Next
	}

	if nil == prev || nil == next || prev.Type != next.Type || ast.NodeKramdownSpanIAL == prev.Type {
		return
	}

	switch prev.Type {
	case ast.NodeText:
		prev.Tokens = append(prev.Tokens, next.Tokens...)
		next.Unlink()
	case ast.NodeTextMark:
		if prev.TextMarkType != next.TextMarkType {
			break
		}

		switch prev.TextMarkType {
		case "em", "strong", "mark", "s", "u", "text":
			prev.TextMarkTextContent += next.TextMarkTextContent
			next.Unlink()
		}
	}
}

// FullTextSearchBlock 搜索内容块。
//
// method：0：关键字，1：查询语法，2：SQL，3：正则表达式
// orderBy: 0：按块类型（默认），1：按创建时间升序，2：按创建时间降序，3：按更新时间升序，4：按更新时间降序，5：按内容顺序（仅在按文档分组时），6：按相关度升序，7：按相关度降序
// groupBy：0：不分组，1：按文档分组
func FullTextSearchBlock(query string, boxes, paths []string, types map[string]bool, method, orderBy, groupBy, page, pageSize int) ([]*Block, int, int, int, bool) {
	blocks, matched, roots, pages, docs, e := FullTextSearchBlockContext(context.Background(), query, boxes, paths, types, method, orderBy, groupBy, page, pageSize)
	if e != nil {
		logging.LogErrorf("full text search: %s", e)
	}
	return blocks, matched, roots, pages, docs
}

func buildBoxesFilter(boxes []string) string {
	if 0 == len(boxes) {
		return ""
	}
	builder := bytes.Buffer{}
	builder.WriteString(" AND (")
	for i, box := range boxes {
		builder.WriteString(fmt.Sprintf("box = '%s'", box))
		if i < len(boxes)-1 {
			builder.WriteString(" OR ")
		}
	}
	builder.WriteString(")")
	return builder.String()
}

func buildPathsFilter(paths []string) string {
	if 0 == len(paths) {
		return ""
	}
	builder := bytes.Buffer{}
	builder.WriteString(" AND (")
	for i, path := range paths {
		builder.WriteString(fmt.Sprintf("path LIKE '%s%%'", path))
		if i < len(paths)-1 {
			builder.WriteString(" OR ")
		}
	}
	builder.WriteString(")")
	return builder.String()
}

func buildOrderBy(query string, method, orderBy int) string {
	switch orderBy {
	case 1:
		return "ORDER BY created ASC"
	case 2:
		return "ORDER BY created DESC"
	case 3:
		return "ORDER BY updated ASC"
	case 4:
		return "ORDER BY updated DESC"
	case 6:
		if 0 != method && 1 != method {
			// 只有关键字搜索和查询语法搜索才支持按相关度升序 https://github.com/siyuan-note/siyuan/issues/7861
			return "ORDER BY sort DESC, updated DESC"
		}
		return "ORDER BY rank DESC" // 默认是按相关度降序，所以按相关度升序要反过来使用 DESC
	case 7:
		if 0 != method && 1 != method {
			return "ORDER BY sort ASC, updated DESC"
		}
		return "ORDER BY rank" // 默认是按相关度降序
	default:
		clause := "ORDER BY CASE " +
			"WHEN name = '${keyword}' THEN 10 " +
			"WHEN alias = '${keyword}' THEN 20 " +
			"WHEN name LIKE '%${keyword}%' THEN 50 " +
			"WHEN alias LIKE '%${keyword}%' THEN 60 " +
			"ELSE 65535 END ASC, sort ASC, updated DESC"
		clause = strings.ReplaceAll(clause, "${keyword}", strings.ReplaceAll(query, "'", "''"))
		return clause
	}
}

func buildTypeFilter(types map[string]bool) string {
	s := conf.NewSearch()
	if err := copier.Copy(s, Conf.Search); err != nil {
		logging.LogErrorf("copy search conf failed: %s", err)
	}
	if nil != types {
		s.Document = types["document"]
		s.Heading = types["heading"]
		s.List = types["list"]
		s.ListItem = types["listItem"]
		s.CodeBlock = types["codeBlock"]
		s.MathBlock = types["mathBlock"]
		s.Table = types["table"]
		s.Blockquote = types["blockquote"]
		s.SuperBlock = types["superBlock"]
		s.Paragraph = types["paragraph"]
		s.HTMLBlock = types["htmlBlock"]
		s.EmbedBlock = types["embedBlock"]
		s.DatabaseBlock = types["databaseBlock"]
		s.AudioBlock = types["audioBlock"]
		s.VideoBlock = types["videoBlock"]
		s.IFrameBlock = types["iframeBlock"]
		s.WidgetBlock = types["widgetBlock"]
	} else {
		s.Document = Conf.Search.Document
		s.Heading = Conf.Search.Heading
		s.List = Conf.Search.List
		s.ListItem = Conf.Search.ListItem
		s.CodeBlock = Conf.Search.CodeBlock
		s.MathBlock = Conf.Search.MathBlock
		s.Table = Conf.Search.Table
		s.Blockquote = Conf.Search.Blockquote
		s.SuperBlock = Conf.Search.SuperBlock
		s.Paragraph = Conf.Search.Paragraph
		s.HTMLBlock = Conf.Search.HTMLBlock
		s.EmbedBlock = Conf.Search.EmbedBlock
		s.DatabaseBlock = Conf.Search.DatabaseBlock
		s.AudioBlock = Conf.Search.AudioBlock
		s.VideoBlock = Conf.Search.VideoBlock
		s.IFrameBlock = Conf.Search.IFrameBlock
		s.WidgetBlock = Conf.Search.WidgetBlock
	}
	return s.TypeFilter()
}

func searchBySQL(stmt string, beforeLen, page, pageSize int) (ret []*Block, matchedBlockCount, matchedRootCount int) {
	stmt = strings.TrimSpace(stmt)
	blocks := sql.SelectBlocksRawStmt(stmt, page, pageSize)
	ret = fromSQLBlocks(&blocks, "", beforeLen)
	if 1 > len(ret) {
		ret = []*Block{}
		return
	}

	stmt = strings.ToLower(stmt)
	stdQuery := !strings.Contains(stmt, "with recursive") && !strings.Contains(stmt, "union")
	if stdQuery {
		if strings.HasPrefix(stmt, "select a.* ") { // 多个搜索关键字匹配文档 https://github.com/siyuan-note/siyuan/issues/7350
			stmt = strings.ReplaceAll(stmt, "select a.* ", "select COUNT(a.id) AS `matches`, COUNT(DISTINCT(a.root_id)) AS `docs` ")
		} else {
			stmt = strings.ReplaceAll(stmt, "select * ", "select COUNT(id) AS `matches`, COUNT(DISTINCT(root_id)) AS `docs` ")
		}
	}
	stmt = removeLimitClause(stmt)
	result, _ := sql.QueryNoLimit(stmt)
	if 1 > len(result) {
		return
	}

	if !stdQuery {
		var rootIDs, blockIDs []string
		for _, queryResult := range result {
			rootIDs = append(rootIDs, queryResult["root_id"].(string))
			blockIDs = append(blockIDs, queryResult["id"].(string))
		}
		rootIDs = gulu.Str.RemoveDuplicatedElem(rootIDs)
		blockIDs = gulu.Str.RemoveDuplicatedElem(blockIDs)
		matchedRootCount = len(rootIDs)
		matchedBlockCount = len(blockIDs)
	} else {
		matchedBlockCount = int(result[0]["matches"].(int64))
		matchedRootCount = int(result[0]["docs"].(int64))
	}
	return
}

func removeLimitClause(stmt string) string {
	parsedStmt, err := sqlparser.Parse(stmt)
	if err != nil {
		return stmt
	}

	switch parsedStmt.(type) {
	case *sqlparser.Select:
		slct := parsedStmt.(*sqlparser.Select)
		if nil != slct.Limit {
			slct.Limit = nil
		}
		stmt = sqlparser.String(slct)
	}
	return stmt
}

func fullTextSearchRefBlock(keyword string, beforeLen int, onlyDoc bool) (ret []*Block) {
	keyword = filterQueryInvisibleChars(keyword)

	if id := extractID(keyword); "" != id {
		ret, _, _ = searchBySQL("SELECT * FROM `blocks` WHERE `id` = '"+id+"'", 36, 1, 32)
		return
	}

	quotedKeyword := stringQuery(keyword)
	table := "blocks_fts" // 大小写敏感
	if !Conf.Search.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}

	projections := "id, parent_id, root_id, hash, box, path, " +
		"snippet(" + table + ", 6, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS hpath, " +
		"snippet(" + table + ", 7, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS name, " +
		"snippet(" + table + ", 8, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS alias, " +
		"snippet(" + table + ", 9, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS memo, " +
		"snippet(" + table + ", 10, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS tag, " +
		"snippet(" + table + ", 11, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS content, " +
		"fcontent, markdown, length, type, subtype, ial, sort, created, updated"
	stmt := "SELECT " + projections + " FROM " + table + " WHERE " + table + " MATCH '" + columnFilter() + ":(" + quotedKeyword + ")' AND type"
	if onlyDoc {
		stmt += " = 'd'"
	} else {
		stmt += " IN " + Conf.Search.TypeFilter()
	}

	if ignoreLines := getRefSearchIgnoreLines(); 0 < len(ignoreLines) {
		// Support ignore search results https://github.com/siyuan-note/siyuan/issues/10089
		buf := bytes.Buffer{}
		for _, line := range ignoreLines {
			buf.WriteString(" AND ")
			buf.WriteString(line)
		}
		stmt += buf.String()
	}

	orderBy := ` ORDER BY CASE
             WHEN name = '${keyword}' THEN 10
             WHEN alias = '${keyword}' THEN 20
             WHEN memo = '${keyword}' THEN 30
             WHEN content = '${keyword}' and type = 'd' THEN 40
             WHEN content LIKE '%${keyword}%' and type = 'd' THEN 41
             WHEN name LIKE '%${keyword}%' THEN 50
             WHEN alias LIKE '%${keyword}%' THEN 60
             WHEN content = '${keyword}' and type = 'h' THEN 70
             WHEN content LIKE '%${keyword}%' and type = 'h' THEN 71
             WHEN fcontent = '${keyword}' and type = 'i' THEN 80
             WHEN fcontent LIKE '%${keyword}%' and type = 'i' THEN 81
             WHEN memo LIKE '%${keyword}%' THEN 90
             WHEN content LIKE '%${keyword}%' and type != 'i' and type != 'l' THEN 100
             ELSE 65535 END ASC, sort ASC, length ASC`
	orderBy = strings.ReplaceAll(orderBy, "${keyword}", strings.ReplaceAll(keyword, "'", "''"))
	stmt += orderBy + " LIMIT " + strconv.Itoa(Conf.Search.Limit)
	blocks := sql.SelectBlocksRawStmtNoParse(stmt, Conf.Search.Limit)
	ret = fromSQLBlocks(&blocks, "", beforeLen)
	if 1 > len(ret) {
		ret = []*Block{}
	}
	return
}

func extractID(content string) (ret string) {
	// Improve block ref search ID extraction https://github.com/siyuan-note/siyuan/issues/10848

	if 22 > len(content) {
		return
	}

	// 从第一个字符开始循环，直到找到一个合法的 ID 为止
	for i := 0; i < len(content)-21; i++ {
		if ast.IsNodeIDPattern(content[i : i+22]) {
			ret = content[i : i+22]
			return
		}
	}
	return
}

func fullTextSearchByRegexp(exp, boxFilter, pathFilter, typeFilter, ignoreFilter, orderBy string, beforeLen, page, pageSize int) (ret []*Block, matchedBlockCount, matchedRootCount int) {
	fieldFilter := fieldRegexp(exp)
	stmt := "SELECT * FROM `blocks` WHERE " + fieldFilter + " AND type IN " + typeFilter
	stmt += boxFilter + pathFilter + ignoreFilter + " " + orderBy
	regex, err := regexp.Compile(exp)
	if nil != err {
		util.PushErrMsg(err.Error(), 5000)
		return
	}

	blocks := sql.SelectBlocksRegex(stmt, regex, Conf.Search.Name, Conf.Search.Alias, Conf.Search.Memo, Conf.Search.IAL, page, pageSize)
	ret = fromSQLBlocks(&blocks, "", beforeLen)
	if 1 > len(ret) {
		ret = []*Block{}
	}

	matchedBlockCount, matchedRootCount = fullTextSearchCountByRegexp(exp, boxFilter, pathFilter, typeFilter, ignoreFilter)
	return
}

func fullTextSearchCountByRegexp(exp, boxFilter, pathFilter, typeFilter, ignoreFilter string) (matchedBlockCount, matchedRootCount int) {
	fieldFilter := fieldRegexp(exp)
	stmt := "SELECT COUNT(id) AS `matches`, COUNT(DISTINCT(root_id)) AS `docs` FROM `blocks` WHERE " + fieldFilter + " AND type IN " + typeFilter + ignoreFilter
	stmt += boxFilter + pathFilter
	result, _ := sql.QueryNoLimit(stmt)
	if 1 > len(result) {
		return
	}
	matchedBlockCount = int(result[0]["matches"].(int64))
	matchedRootCount = int(result[0]["docs"].(int64))
	return
}

func fullTextSearchByFTS(query, boxFilter, pathFilter, typeFilter, ignoreFilter, orderBy string, beforeLen, page, pageSize int) (ret []*Block, matchedBlockCount, matchedRootCount int) {
	table := "blocks_fts" // 大小写敏感
	if !Conf.Search.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}
	projections := "id, parent_id, root_id, hash, box, path, " +
		// Search result content snippet returns more text https://github.com/siyuan-note/siyuan/issues/10707
		"snippet(" + table + ", 6, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS hpath, " +
		"snippet(" + table + ", 7, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS name, " +
		"snippet(" + table + ", 8, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS alias, " +
		"snippet(" + table + ", 9, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS memo, " +
		"snippet(" + table + ", 10, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS tag, " +
		"snippet(" + table + ", 11, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS content, " +
		"fcontent, markdown, length, type, subtype, ial, sort, created, updated"
	stmt := "SELECT " + projections + " FROM " + table + " WHERE (`" + table + "` MATCH '" + columnFilter() + ":(" + query + ")'"
	stmt += ") AND type IN " + typeFilter
	stmt += boxFilter + pathFilter + ignoreFilter + " " + orderBy
	stmt += " LIMIT " + strconv.Itoa(pageSize) + " OFFSET " + strconv.Itoa((page-1)*pageSize)
	blocks := sql.SelectBlocksRawStmt(stmt, page, pageSize)
	ret = fromSQLBlocks(&blocks, "", beforeLen)
	if 1 > len(ret) {
		ret = []*Block{}
	}

	matchedBlockCount, matchedRootCount = fullTextSearchCountByFTS(query, boxFilter, pathFilter, typeFilter, ignoreFilter)
	return
}

func fullTextSearchCountByFTS(query, boxFilter, pathFilter, typeFilter, ignoreFilter string) (matchedBlockCount, matchedRootCount int) {
	table := "blocks_fts" // 大小写敏感
	if !Conf.Search.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}

	stmt := "SELECT COUNT(id) AS `matches`, COUNT(DISTINCT(root_id)) AS `docs` FROM `" + table + "` WHERE (`" + table + "` MATCH '" + columnFilter() + ":(" + query + ")'"
	stmt += ") AND type IN " + typeFilter
	stmt += boxFilter + pathFilter + ignoreFilter
	result, _ := sql.QueryNoLimit(stmt)
	if 1 > len(result) {
		return
	}
	matchedBlockCount = int(result[0]["matches"].(int64))
	matchedRootCount = int(result[0]["docs"].(int64))
	return
}

func fullTextSearchByLikeWithRoot(query, boxFilter, pathFilter, typeFilter, ignoreFilter, orderBy string, beforeLen, page, pageSize int) (ret []*Block, matchedBlockCount, matchedRootCount int) {
	query = strings.ReplaceAll(query, "'", "''") // 不需要转义双引号，因为条件都是通过单引号包裹的，只需要转义单引号即可
	keywords := strings.Split(query, " ")
	contentField := columnConcat()
	var likeFilter string
	orderByLike := "("
	for i, keyword := range keywords {
		likeFilter += "GROUP_CONCAT(" + contentField + ") LIKE '%" + keyword + "%'"
		orderByLike += "(docContent LIKE '%" + keyword + "%')"
		if i < len(keywords)-1 {
			likeFilter += " AND "
			orderByLike += " + "
		}
	}
	orderByLike += ")"
	dMatchStmt := "SELECT root_id, MAX(CASE WHEN type = 'd' THEN (" + contentField + ") END) AS docContent" +
		" FROM blocks WHERE type IN " + typeFilter + boxFilter + pathFilter + ignoreFilter +
		" GROUP BY root_id HAVING " + likeFilter + "ORDER BY " + orderByLike + " DESC, MAX(updated) DESC"
	cteStmt := "WITH docBlocks AS (" + dMatchStmt + ")"
	likeFilter = strings.ReplaceAll(likeFilter, "GROUP_CONCAT("+contentField+")", "concatContent")
	limit := " LIMIT " + strconv.Itoa(pageSize) + " OFFSET " + strconv.Itoa((page-1)*pageSize)
	selectStmt := cteStmt + "\nSELECT *, " +
		"(" + contentField + ") AS concatContent, " +
		"(SELECT COUNT(root_id) FROM docBlocks) AS docs, " +
		"(CASE WHEN (root_id IN (SELECT root_id FROM docBlocks) AND (" + strings.ReplaceAll(likeFilter, "concatContent", contentField) + ")) THEN 1 ELSE 0 END) AS blockSort" +
		" FROM blocks WHERE type IN " + typeFilter + boxFilter + pathFilter + ignoreFilter +
		" AND (id IN (SELECT root_id FROM docBlocks " + limit + ") OR" +
		"  (root_id IN (SELECT root_id FROM docBlocks" + limit + ") AND (" + likeFilter + ")))"
	if strings.Contains(orderBy, "ORDER BY rank DESC") {
		orderBy = buildOrderBy(query, 0, 0)
		selectStmt += " " + strings.Replace(orderBy, "END ASC, ", "END ASC, blockSort ASC, ", 1)
	} else if strings.Contains(orderBy, "ORDER BY rank") {
		orderBy = buildOrderBy(query, 0, 0)
		selectStmt += " " + strings.Replace(orderBy, "END ASC, ", "END ASC, blockSort DESC, ", 1)
	} else if strings.Contains(orderBy, "sort ASC") {
		selectStmt += " " + strings.Replace(orderBy, "END ASC, ", "END ASC, blockSort DESC, ", 1)
	} else {
		selectStmt += " " + orderBy
	}
	result, _ := sql.QueryNoLimit(selectStmt)
	resultBlocks := sql.ToBlocks(result)
	if 0 < len(resultBlocks) {
		matchedRootCount = int(result[0]["docs"].(int64))
		matchedBlockCount = matchedRootCount
	}

	keywords = gulu.Str.RemoveDuplicatedElem(keywords)
	terms := strings.Join(keywords, search.TermSep)
	terms = strings.ReplaceAll(terms, "''", "'")
	ret = fromSQLBlocks(&resultBlocks, terms, beforeLen)
	if 1 > len(ret) {
		ret = []*Block{}
	}
	return
}

func highlightByFTS(query, typeFilter, id string) (ret []string) {
	query = strings.ReplaceAll(query, " ", " OR ")
	const limit = 256
	table := "blocks_fts"
	if !Conf.Search.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}
	projections := "id, parent_id, root_id, hash, box, path, " +
		"highlight(" + table + ", 6, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS hpath, " +
		"highlight(" + table + ", 7, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS name, " +
		"highlight(" + table + ", 8, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS alias, " +
		"highlight(" + table + ", 9, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS memo, " +
		"highlight(" + table + ", 10, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS tag, " +
		"highlight(" + table + ", 11, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS content, " +
		"fcontent, markdown, length, type, subtype, " +
		"highlight(" + table + ", 17, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "') AS ial, " +
		"sort, created, updated"
	stmt := "SELECT " + projections + " FROM " + table + " WHERE (`" + table + "` MATCH '" + columnFilter() + ":(" + query + ")'"
	stmt += ") AND type IN " + typeFilter
	stmt += " AND root_id = '" + id + "'"
	stmt += " LIMIT " + strconv.Itoa(limit)
	sqlBlocks := sql.SelectBlocksRawStmt(stmt, 1, limit)
	for _, block := range sqlBlocks {
		keyword := gulu.Str.SubstringsBetween(block.HPath, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Name, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Alias, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Memo, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Tag, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.IAL, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Content, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
	}
	ret = gulu.Str.RemoveDuplicatedElem(ret)
	return
}

func highlightByRegexp(query, typeFilter, id string) (ret []string) {
	fieldFilter := fieldRegexp(query)
	stmt := "SELECT * FROM `blocks` WHERE " + fieldFilter + " AND type IN " + typeFilter
	stmt += " AND root_id = '" + id + "'"
	regex, _ := regexp.Compile(query)
	if nil == regex {
		return
	}
	sqlBlocks := sql.SelectBlocksRegex(stmt, regex, Conf.Search.Name, Conf.Search.Alias, Conf.Search.Memo, Conf.Search.IAL, 1, 256)
	for _, block := range sqlBlocks {
		keyword := gulu.Str.SubstringsBetween(block.HPath, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Name, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Alias, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Memo, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Tag, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
		keyword = gulu.Str.SubstringsBetween(block.Content, search.SearchMarkLeft, search.SearchMarkRight)
		if 0 < len(keyword) {
			ret = append(ret, keyword...)
		}
	}
	ret = gulu.Str.RemoveDuplicatedElem(ret)
	return
}

func markSearch(text, keyword string, beforeLen int) (string, float64) {
	return blockview.MarkSearch(text, keyword, beforeLen, Conf.Search.CaseSensitive)
}
func fromSQLBlocks(sqlBlocks *[]*sql.Block, terms string, beforeLen int) (ret []*Block) {
	for _, sqlBlock := range *sqlBlocks {
		ret = append(ret, fromSQLBlock(sqlBlock, terms, beforeLen))
	}
	return
}

func fromSQLBlock(b *sql.Block, terms string, beforeLen int) *Block {
	if b == nil {
		return nil
	}
	row := indexdb.BlockRecord(*b)
	return blockview.FromRow(&row, terms, beforeLen, Conf.Search.CaseSensitive)
}
func maxContent(content string, maxLen int) string { return blockview.MaxContent(content, maxLen) }
func fieldRegexp(regexp string) string {
	regexp = strings.ReplaceAll(regexp, "'", "''") // 不需要转义双引号，因为条件都是通过单引号包裹的，只需要转义单引号即可
	buf := bytes.Buffer{}
	buf.WriteString("(")
	buf.WriteString("content REGEXP '")
	buf.WriteString(regexp)
	buf.WriteString("'")
	if Conf.Search.Name {
		buf.WriteString(" OR name REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	if Conf.Search.Alias {
		buf.WriteString(" OR alias REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	if Conf.Search.Memo {
		buf.WriteString(" OR memo REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	if Conf.Search.IAL {
		buf.WriteString(" OR ial REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	buf.WriteString(" OR tag REGEXP '")
	buf.WriteString(regexp)
	buf.WriteString("')")
	return buf.String()
}

func columnFilter() string {
	buf := bytes.Buffer{}
	buf.WriteString("{content")
	if Conf.Search.Name {
		buf.WriteString(" name")
	}
	if Conf.Search.Alias {
		buf.WriteString(" alias")
	}
	if Conf.Search.Memo {
		buf.WriteString(" memo")
	}
	if Conf.Search.IAL {
		buf.WriteString(" ial")
	}
	buf.WriteString(" tag}")
	return buf.String()
}

func columnConcat() string {
	buf := bytes.Buffer{}
	buf.WriteString("content")
	if Conf.Search.Name {
		buf.WriteString("||name")
	}
	if Conf.Search.Alias {
		buf.WriteString("||alias")
	}
	if Conf.Search.Memo {
		buf.WriteString("||memo")
	}
	if Conf.Search.IAL {
		buf.WriteString("||ial")
	}
	buf.WriteString("||tag")
	return buf.String()
}

func stringQuery(query string) string {
	trimmedQuery := strings.TrimSpace(query)
	if "" == trimmedQuery {
		return "\"" + query + "\""
	}

	query = strings.ReplaceAll(query, "\"", "\"\"")
	query = strings.ReplaceAll(query, "'", "''")

	if strings.Contains(trimmedQuery, " ") {
		buf := bytes.Buffer{}
		parts := strings.Split(query, " ")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = "\"" + part + "\""
			buf.WriteString(part)
			buf.WriteString(" ")
		}
		return strings.TrimSpace(buf.String())
	}
	return "\"" + query + "\""
}

// markReplaceSpan 用于处理搜索高亮。
func markReplaceSpan(n *ast.Node, unlinks *[]*ast.Node, keywords []string, kind string, parser *lute.Lute) bool {
	return markReplaceSpanWithCase(n, unlinks, keywords, kind, parser, Conf.Search.CaseSensitive)
}
func markReplaceSpanWithCase(n *ast.Node, unlinks *[]*ast.Node, keywords []string, kind string, parser *lute.Lute, sensitive bool) bool {
	return docview.MarkReplaceSpan(n, unlinks, keywords, kind, parser, sensitive)
}
func markReplaceSpanWithSplit(text string, keywords []string, start, end string) string {
	return docview.MarkSplit(text, keywords, start, end, Conf.Search.CaseSensitive)
}

var (
	searchIgnoreLastModified int64
	searchIgnore             []string
	searchIgnoreLock         = sync.Mutex{}
)

func getSearchIgnoreLines() (ret []string) {
	// Support ignore search results https://github.com/siyuan-note/siyuan/issues/10089

	now := time.Now().UnixMilli()
	if now-searchIgnoreLastModified < 30*1000 {
		return searchIgnore
	}

	searchIgnoreLock.Lock()
	defer searchIgnoreLock.Unlock()

	searchIgnoreLastModified = now

	searchIgnorePath := filepath.Join(util.DataDir, ".siyuan", "searchignore")
	err := os.MkdirAll(filepath.Dir(searchIgnorePath), 0755)
	if err != nil {
		return
	}
	if !gulu.File.IsExist(searchIgnorePath) {
		if err = gulu.File.WriteFileSafer(searchIgnorePath, nil, 0644); err != nil {
			logging.LogErrorf("create searchignore [%s] failed: %s", searchIgnorePath, err)
			return
		}
	}
	data, err := os.ReadFile(searchIgnorePath)
	if err != nil {
		logging.LogErrorf("read searchignore [%s] failed: %s", searchIgnorePath, err)
		return
	}
	dataStr := string(data)
	dataStr = strings.ReplaceAll(dataStr, "\r\n", "\n")
	ret = strings.Split(dataStr, "\n")

	ret = gulu.Str.RemoveDuplicatedElem(ret)
	if 0 < len(ret) && "" == ret[0] {
		ret = ret[1:]
	}
	searchIgnore = nil
	for _, line := range ret {
		searchIgnore = append(searchIgnore, line)
	}
	return
}

var (
	refSearchIgnoreLastModified int64
	refSearchIgnore             []string
	refSearchIgnoreLock         = sync.Mutex{}
)

func getRefSearchIgnoreLines() (ret []string) {
	// Support ignore search results https://github.com/siyuan-note/siyuan/issues/10089

	now := time.Now().UnixMilli()
	if now-refSearchIgnoreLastModified < 30*1000 {
		return refSearchIgnore
	}

	refSearchIgnoreLock.Lock()
	defer refSearchIgnoreLock.Unlock()

	refSearchIgnoreLastModified = now

	searchIgnorePath := filepath.Join(util.DataDir, ".siyuan", "refsearchignore")
	err := os.MkdirAll(filepath.Dir(searchIgnorePath), 0755)
	if err != nil {
		return
	}
	if !gulu.File.IsExist(searchIgnorePath) {
		if err = gulu.File.WriteFileSafer(searchIgnorePath, nil, 0644); err != nil {
			logging.LogErrorf("create refsearchignore [%s] failed: %s", searchIgnorePath, err)
			return
		}
	}
	data, err := os.ReadFile(searchIgnorePath)
	if err != nil {
		logging.LogErrorf("read refsearchignore [%s] failed: %s", searchIgnorePath, err)
		return
	}
	dataStr := string(data)
	dataStr = strings.ReplaceAll(dataStr, "\r\n", "\n")
	ret = strings.Split(dataStr, "\n")

	ret = gulu.Str.RemoveDuplicatedElem(ret)
	if 0 < len(ret) && "" == ret[0] {
		ret = ret[1:]
	}
	refSearchIgnore = nil
	for _, line := range ret {
		refSearchIgnore = append(refSearchIgnore, line)
	}
	return
}

func filterQueryInvisibleChars(query string) string {
	query = strings.ReplaceAll(query, "　", "_@full_width_space@_")
	query = strings.ReplaceAll(query, "\t", "_@tab@_")
	query = strings.ReplaceAll(query, string(gulu.ZWJ), "__@ZWJ@__")
	query = util.RemoveInvalid(query)
	query = strings.ReplaceAll(query, "_@full_width_space@_", "　")
	query = strings.ReplaceAll(query, "_@tab@_", "\t")
	query = strings.ReplaceAll(query, "__@ZWJ@__", string(gulu.ZWJ))
	query = strings.ReplaceAll(query, string(gulu.ZWJ)+"#", "#")
	return query
}

func replaceCaseInsensitive(input, old, new []byte) []byte {
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(string(old)))
	return []byte(re.ReplaceAllString(string(input), string(new)))
}
