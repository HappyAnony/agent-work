// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/ClarkThan/ahocorasick"
	"regexp"
	"slices"
	"sort"
	"strings"
	"wiki-kernel/internal/textprefs"
	"wiki-kernel/search"
)

type VirtualOptions struct {
	Enabled, CaseSensitive bool
	Include, Exclude       string
}

func FilterVirtualKeywords(root *ast.Node, keywords []string, options VirtualOptions) (ret []string) {
	if !options.Enabled {
		return
	}

	ret = slices.Clone(keywords)

	includes := ParseKeywords(options.Include)
	if 0 < len(includes) {
		ret = append(ret, includes...)
		ret = gulu.Str.RemoveDuplicatedElem(ret)
	}

	excludes := ParseKeywords(options.Exclude)
	if 0 < len(excludes) {
		var tmp, regexps []string
		for _, e := range excludes {
			if strings.HasPrefix(e, "/") && strings.HasSuffix(e, "/") {
				regexps = append(regexps, e[1:len(e)-1])
			} else {
				tmp = append(tmp, e)
			}
		}
		excludes = tmp
		ret = gulu.Str.ExcludeElem(ret, excludes)
		if 0 < len(regexps) {
			tmp = nil
			for _, str := range ret {
				matchExclude := false
				for _, re := range regexps {
					if ok, _ := regexp.MatchString(re, str); ok {
						matchExclude = true
						break
					}
				}
				if !matchExclude {
					tmp = append(tmp, str)
				}
			}
			ret = tmp
		}
	}

	// 虚拟引用排除当前文档名 https://github.com/siyuan-note/siyuan/issues/4537
	// Virtual references exclude the name and aliases from the current document https://github.com/siyuan-note/siyuan/issues/9204
	title := root.IALAttr("title")
	ret = gulu.Str.ExcludeElem(ret, []string{title})
	if name := root.IALAttr("name"); "" != name {
		ret = gulu.Str.ExcludeElem(ret, []string{name})
	}
	if alias := root.IALAttr("alias"); "" != alias {
		for _, a := range strings.Split(alias, ",") {
			ret = gulu.Str.ExcludeElem(ret, []string{a})
		}
	}

	ret = PrepareKeywords(ret)
	return
}
func ProcessVirtual(ctx context.Context, n *ast.Node, unlinks *[]*ast.Node, virtualBlockRefKeywords []string, refCount map[string]int, luteEngine *lute.Lute, options VirtualOptions, text func(context.Context, *ast.Node) (string, error)) (bool, error) {
	if ctx == nil {
		return false, errors.New("get_doc_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !options.Enabled || 1 > len(virtualBlockRefKeywords) {
		return false, nil
	}

	if ast.NodeText != n.Type {
		return false, nil
	}

	parentBlock := ParentBlock(n)
	if nil == parentBlock {
		return false, nil
	}

	if 0 < refCount[parentBlock.ID] {
		if text == nil {
			return false, errors.New("get_doc_dependencies_unbound")
		}
		// 如果块被引用过，则将其自身的文本排除在虚拟引用关键字之外
		// Referenced blocks support rendering virtual references https://github.com/siyuan-note/siyuan/issues/10960
		parentText, err := text(ctx, parentBlock)
		if err != nil {
			return false, err
		}
		virtualBlockRefKeywords = gulu.Str.RemoveElem(virtualBlockRefKeywords, parentText)
	}
	// Excluding the last self keyword must not compile the empty regexp `()`.
	if len(virtualBlockRefKeywords) == 0 {
		return false, nil
	}

	content := string(n.Tokens)
	tmp := gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(content))
	tmp = strings.TrimSpace(tmp)
	if "" == tmp {
		return false, nil
	}

	newContent := MarkSplit(content, slices.Clone(virtualBlockRefKeywords), search.GetMarkSpanStart(search.VirtualBlockRefDataType), search.GetMarkSpanEnd(), options.CaseSensitive)
	if content != newContent {
		// 虚拟引用排除命中自身块命名和别名的情况 https://github.com/siyuan-note/siyuan/issues/3185
		var blockKeys []string
		if name := parentBlock.IALAttr("name"); "" != name {
			blockKeys = append(blockKeys, name)
		}
		if alias := parentBlock.IALAttr("alias"); "" != alias {
			blockKeys = append(blockKeys, alias)
		}
		if 0 < len(blockKeys) {
			keys := gulu.Str.SubstringsBetween(newContent, search.GetMarkSpanStart(search.VirtualBlockRefDataType), search.GetMarkSpanEnd())
			for _, k := range keys {
				if gulu.Str.Contains(k, blockKeys) {
					return true, nil
				}
			}
		}

		n.Tokens = []byte(newContent)
		linkTree := parse.Inline("", n.Tokens, luteEngine.ParseOptions)
		var children []*ast.Node
		for c := linkTree.Root.FirstChild.FirstChild; nil != c; c = c.Next {
			children = append(children, c)
		}
		for _, c := range children {
			n.InsertBefore(c)
		}
		*unlinks = append(*unlinks, n)
		return true, nil
	}
	return false, nil
}

// parseKeywords 将字符串转换为关键字切片，并剔除前后的空白字符
func MatchVirtualKeywords(blockContent string, keywords []string, caseSensitive bool) (ret []string) {
	if 1 > len(keywords) {
		return
	}

	contentTmp := blockContent
	var keywordsTmp []string
	if !caseSensitive {
		contentTmp = strings.ToLower(blockContent)
		for _, keyword := range keywords {
			keywordsTmp = append(keywordsTmp, strings.ToLower(keyword))
		}
	} else {
		for _, keyword := range keywords {
			keywordsTmp = append(keywordsTmp, keyword)
		}
	}

	m := ahocorasick.NewMatcher()
	m.BuildWithPatterns(keywordsTmp)
	hits := m.Search(contentTmp)
	for _, hit := range hits {
		ret = append(ret, hit)
	}

	if 1 > len(ret) {
		return
	}

	ret = gulu.Str.RemoveDuplicatedElem(ret)
	return
}
func ParseKeywords(s string) []string { return textprefs.ParseKeywords(s) }
func PrepareKeywords(keywords []string) (ret []string) {
	ret = gulu.Str.RemoveDuplicatedElem(keywords)
	var tmp []string
	for _, k := range ret {
		if "" != k && "*" != k { // 提及和虚引排除 * Ignore `*` back mentions and virtual references https://github.com/siyuan-note/siyuan/issues/10873
			tmp = append(tmp, k)
		}
	}
	ret = tmp

	sort.SliceStable(ret, func(i, j int) bool {
		return len(ret[i]) > len(ret[j])
	})
	return
}
