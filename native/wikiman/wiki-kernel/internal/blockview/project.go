// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model search presentation shared by UI and Context.
package blockview

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/88250/lute/parse"
	"github.com/xrash/smetrics"
	"strings"
	"unicode/utf8"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/markup"
	"wiki-kernel/internal/nodetype"
	"wiki-kernel/search"
)

func MarkSearch(text string, keyword string, beforeLen int, caseSensitive bool) (marked string, score float64) {
	if 0 == len(keyword) {
		if strings.Contains(text, search.SearchMarkLeft) { // 使用 FTS snippet() 处理过高亮片段，这里简单替换后就返回
			marked = markup.EscapeHTML(text)
			marked = strings.ReplaceAll(marked, search.SearchMarkLeft, "<mark>")
			marked = strings.ReplaceAll(marked, search.SearchMarkRight, "</mark>")
			return
		}

		keywords := gulu.Str.SubstringsBetween(text, search.SearchMarkLeft, search.SearchMarkRight)
		keywords = gulu.Str.RemoveDuplicatedElem(keywords)
		keyword = strings.Join(keywords, search.TermSep)
		marked = strings.ReplaceAll(text, search.SearchMarkLeft, "")
		marked = strings.ReplaceAll(marked, search.SearchMarkRight, "")
		_, marked = search.MarkText(marked, keyword, beforeLen, caseSensitive)
		marked = markup.EscapeHTML(marked)
		return
	}

	pos, marked := search.MarkText(text, keyword, beforeLen, caseSensitive)
	if -1 < pos {
		if 0 == pos {
			score = 1
		}
		score += float64(strings.Count(marked, "<mark>"))
		winkler := smetrics.JaroWinkler(text, keyword, 0.7, 4)
		score += winkler
	}
	score = -score // 分越小排序越靠前
	return
}

func FromRows(sqlBlocks *[]*indexdb.BlockRecord, terms string, beforeLen int, caseSensitive bool) (ret []*Block) {
	for _, sqlBlock := range *sqlBlocks {
		ret = append(ret, FromRow(sqlBlock, terms, beforeLen, caseSensitive))
	}
	return
}

func FromRow(sqlBlock *indexdb.BlockRecord, terms string, beforeLen int, caseSensitive bool) (block *Block) {
	if nil == sqlBlock {
		return
	}

	id := sqlBlock.ID
	content := sqlBlock.Content
	if 1 < strings.Count(content, search.SearchMarkRight) && strings.HasSuffix(content, search.SearchMarkRight+"...") {
		// 返回多个关键字命中时需要检查最后一个关键字是否被截断
		firstKeyword := gulu.Str.SubStringBetween(content, search.SearchMarkLeft, search.SearchMarkRight)
		lastKeyword := gulu.Str.LastSubStringBetween(content, search.SearchMarkLeft, search.SearchMarkRight)
		if firstKeyword != lastKeyword {
			// 如果第一个关键字和最后一个关键字不相同，说明最后一个关键字被截断了
			// 此时需要将 content 中的最后一个关键字替换为完整的关键字
			content = strings.TrimSuffix(content, search.SearchMarkLeft+lastKeyword+search.SearchMarkRight+"...")
			content += search.SearchMarkLeft + firstKeyword + search.SearchMarkRight + "..."
		}
	}

	content, _ = MarkSearch(content, terms, beforeLen, caseSensitive)
	content = MaxContent(content, 5120)
	tag, _ := MarkSearch(sqlBlock.Tag, terms, beforeLen, caseSensitive)
	markdown := MaxContent(sqlBlock.Markdown, 5120)
	fContent := sqlBlock.FContent
	block = &Block{
		Box:      sqlBlock.Box,
		Path:     sqlBlock.Path,
		ID:       id,
		RootID:   sqlBlock.RootID,
		ParentID: sqlBlock.ParentID,
		Alias:    sqlBlock.Alias,
		Name:     sqlBlock.Name,
		Memo:     sqlBlock.Memo,
		Tag:      tag,
		Content:  content,
		FContent: fContent,
		Markdown: markdown,
		Type:     nodetype.FromAbbrType(sqlBlock.Type),
		SubType:  sqlBlock.SubType,
		Sort:     sqlBlock.Sort,
	}
	if "" != sqlBlock.IAL {
		block.IAL = map[string]string{}
		ialStr := strings.TrimPrefix(sqlBlock.IAL, "{:")
		ialStr = strings.TrimSuffix(ialStr, "}")
		ial := parse.Tokens2IAL([]byte(ialStr))
		for _, kv := range ial {
			block.IAL[kv[0]] = kv[1]
		}
	}

	hPath, _ := MarkSearch(sqlBlock.HPath, "", 18, caseSensitive)
	if !strings.HasPrefix(hPath, "/") {
		hPath = "/" + hPath
	}
	block.HPath = hPath

	if "" != block.Name {
		block.Name, _ = MarkSearch(block.Name, terms, 256, caseSensitive)
	}
	if "" != block.Alias {
		block.Alias, _ = MarkSearch(block.Alias, terms, 256, caseSensitive)
	}
	if "" != block.Memo {
		block.Memo, _ = MarkSearch(block.Memo, terms, 256, caseSensitive)
	}
	return
}

func MaxContent(content string, maxLen int) string {
	idx := strings.Index(content, "<mark>")
	if 128 < maxLen && maxLen <= idx {
		head := bytes.Buffer{}
		for i := 0; i < 512; i++ {
			r, size := utf8.DecodeLastRuneInString(content[:idx])
			head.WriteRune(r)
			idx -= size
			if 64 < head.Len() {
				break
			}
		}

		content = reverse(head.String()) + content[idx:]
	}

	if maxLen < utf8.RuneCountInString(content) {
		return gulu.Str.SubStr(content, maxLen) + "..."
	}
	return content
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
