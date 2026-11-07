package model

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/88250/lute/parse"
	"github.com/xrash/smetrics"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
	"wiki-kernel/conf"
	"wiki-kernel/search"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func oracleMarkSearch(text string, keyword string, beforeLen int) (marked string, score float64) {
	if 0 == len(keyword) {
		if strings.Contains(text, search.SearchMarkLeft) { // 使用 FTS snippet() 处理过高亮片段，这里简单替换后就返回
			marked = util.EscapeHTML(text)
			marked = strings.ReplaceAll(marked, search.SearchMarkLeft, "<mark>")
			marked = strings.ReplaceAll(marked, search.SearchMarkRight, "</mark>")
			return
		}

		keywords := gulu.Str.SubstringsBetween(text, search.SearchMarkLeft, search.SearchMarkRight)
		keywords = gulu.Str.RemoveDuplicatedElem(keywords)
		keyword = strings.Join(keywords, search.TermSep)
		marked = strings.ReplaceAll(text, search.SearchMarkLeft, "")
		marked = strings.ReplaceAll(marked, search.SearchMarkRight, "")
		_, marked = search.MarkText(marked, keyword, beforeLen, Conf.Search.CaseSensitive)
		marked = util.EscapeHTML(marked)
		return
	}

	pos, marked := search.MarkText(text, keyword, beforeLen, Conf.Search.CaseSensitive)
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

func oracleFromSQLBlocks(sqlBlocks *[]*sql.Block, terms string, beforeLen int) (ret []*Block) {
	for _, sqlBlock := range *sqlBlocks {
		ret = append(ret, oracleFromSQLBlock(sqlBlock, terms, beforeLen))
	}
	return
}

func oracleFromSQLBlock(sqlBlock *sql.Block, terms string, beforeLen int) (block *Block) {
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

	content, _ = oracleMarkSearch(content, terms, beforeLen)
	content = oracleMaxContent(content, 5120)
	tag, _ := oracleMarkSearch(sqlBlock.Tag, terms, beforeLen)
	markdown := oracleMaxContent(sqlBlock.Markdown, 5120)
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
		Type:     treenode.FromAbbrType(sqlBlock.Type),
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

	hPath, _ := oracleMarkSearch(sqlBlock.HPath, "", 18)
	if !strings.HasPrefix(hPath, "/") {
		hPath = "/" + hPath
	}
	block.HPath = hPath

	if "" != block.Name {
		block.Name, _ = oracleMarkSearch(block.Name, terms, 256)
	}
	if "" != block.Alias {
		block.Alias, _ = oracleMarkSearch(block.Alias, terms, 256)
	}
	if "" != block.Memo {
		block.Memo, _ = oracleMarkSearch(block.Memo, terms, 256)
	}
	return
}

func oracleMaxContent(content string, maxLen int) string {
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

		content = util.Reverse(head.String()) + content[idx:]
	}

	if maxLen < utf8.RuneCountInString(content) {
		return gulu.Str.SubStr(content, maxLen) + "..."
	}
	return content
}

func TestOriginalSearchPresentationByteEquivalent(t *testing.T) {
	old := Conf
	Conf = NewAppConfWithBusiness(nil)
	Conf.Search = conf.NewSearch()
	defer func() { Conf = old }()
	for _, sensitive := range []bool{true, false} {
		Conf.Search.CaseSensitive = sensitive
		for _, kind := range []string{"d", "p", "h", "i", "av"} {
			for _, content := range []string{"中文 <html> & alpha", "__@mark__alpha__mark@__ ... __@mark__alp__mark@__...", strings.Repeat("中文abcdef", 900) + "<mark>x</mark>"} {
				for _, term := range []string{"", "alpha"} {
					b := &sql.Block{ID: "root", RootID: "root", Content: content, Type: kind, IAL: `{: updated="20261022" custom-test="keep"}`, HPath: "/Folder/<name>", Tag: "AGI <tag>", Name: "alpha", Alias: "ALPHA", Memo: "中文alpha", Markdown: content}
					got, want := fromSQLBlock(b, term, 36), oracleFromSQLBlock(b, term, 36)
					if got.IAL["custom-test"] != "keep" {
						t.Fatal("IAL fixture lost custom attribute")
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("%s %q sensitive=%v: %#v != %#v", kind, term, sensitive, got, want)
					}
				}
			}
		}
	}
}
