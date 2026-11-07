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

package nativeparser

import "github.com/88250/lute"

// Settings are passed by value; factories never read process or workspace globals.
func Defaults() Markdown {
	return Markdown{
		InlineAsterisk:      true,
		InlineUnderscore:    true,
		InlineSup:           true,
		InlineSub:           true,
		InlineTag:           true,
		InlineMath:          true,
		InlineStrikethrough: true,
		InlineMark:          true,
	}
}

type Markdown struct {
	InlineAsterisk      bool `json:"inlineAsterisk"`      // 是否启用行级 * 语法
	InlineUnderscore    bool `json:"inlineUnderscore"`    // 是否启用行级 _ 语法
	InlineSup           bool `json:"inlineSup"`           // 是否启用行级上标
	InlineSub           bool `json:"inlineSub"`           // 是否启用行级下标
	InlineTag           bool `json:"inlineTag"`           // 是否启用行级标签
	InlineMath          bool `json:"inlineMath"`          // 是否启用行级公式
	InlineStrikethrough bool `json:"inlineStrikethrough"` // 是否启用行级删除线
	InlineMark          bool `json:"inlineMark"`          // 是否启用行级标记
}

func NewLute(settings Markdown) (ret *lute.Lute) {
	ret = lute.New()
	detachEmojis(ret)
	ret.SetTextMark(true)
	ret.SetProtyleWYSIWYG(true)
	ret.SetBlockRef(true)
	ret.SetFileAnnotationRef(true)
	ret.SetKramdownIAL(true)
	ret.SetTag(true)
	ret.SetSuperBlock(true)
	ret.SetImgPathAllowSpace(true)
	ret.SetGitConflict(true)
	ret.SetInlineAsterisk(settings.InlineAsterisk)
	ret.SetInlineUnderscore(settings.InlineUnderscore)
	ret.SetSup(settings.InlineSup)
	ret.SetSub(settings.InlineSub)
	ret.SetTag(settings.InlineTag)
	ret.SetInlineMath(settings.InlineMath)
	ret.SetGFMStrikethrough(settings.InlineStrikethrough)
	ret.SetMark(settings.InlineMark)
	ret.SetInlineMathAllowDigitAfterOpenMarker(true)
	ret.SetGFMStrikethrough1(false)
	ret.SetFootnotes(false)
	ret.SetToC(false)
	ret.SetIndentCodeBlock(false)
	ret.SetParagraphBeginningSpace(true)
	ret.SetAutoSpace(false)
	ret.SetHeadingID(false)
	ret.SetSetext(false)
	ret.SetYamlFrontMatter(false)
	ret.SetLinkRef(false)
	ret.SetCodeSyntaxHighlight(false)
	ret.SetSanitize(true)
	ret.SetUnorderedListMarker("-")
	ret.SetCallout(true)
	return
}

func NewStdLute(settings Markdown) (ret *lute.Lute) {
	ret = lute.New()
	detachEmojis(ret)
	ret.SetFootnotes(false)
	ret.SetToC(false)
	ret.SetIndentCodeBlock(true) // 导入 Markdown 时支持缩进代码块语法 Support indented code block syntax when importing Markdown https://github.com/siyuan-note/siyuan/issues/14429
	ret.SetAutoSpace(false)
	ret.SetHeadingID(false)
	ret.SetSetext(false)
	ret.SetYamlFrontMatter(false)
	ret.SetLinkRef(false)
	ret.SetGFMAutoLink(false) // 导入 Markdown 时不自动转换超链接 https://github.com/siyuan-note/siyuan/issues/7682
	ret.SetImgPathAllowSpace(true)
	ret.SetInlineMathAllowDigitAfterOpenMarker(true) // Formula parsing supports $ followed by numbers when importing Markdown https://github.com/siyuan-note/siyuan/issues/8362

	// 导入 Markdown 时遵循编辑器 Markdown 语法设置
	// Follow editor Markdown syntax settings when importing Markdown https://github.com/siyuan-note/siyuan/issues/14731
	ret.SetInlineAsterisk(settings.InlineAsterisk)
	ret.SetInlineUnderscore(settings.InlineUnderscore)
	ret.SetSup(settings.InlineSup)
	ret.SetSub(settings.InlineSub)
	ret.SetTag(settings.InlineTag)
	ret.SetInlineMath(settings.InlineMath)
	ret.SetGFMStrikethrough(settings.InlineStrikethrough)
	ret.SetGFMStrikethrough1(false)
	return
}
