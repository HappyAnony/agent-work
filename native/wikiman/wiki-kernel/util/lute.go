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

package util

import (
	"strings"

	"github.com/88250/lute"
	"github.com/PuerkitoBio/goquery"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/nativeparser"
)

// MarkdownSettings 运行时 Markdown 配置。
var MarkdownSettings = &Markdown{
	InlineAsterisk:      true,
	InlineUnderscore:    true,
	InlineSup:           true,
	InlineSub:           true,
	InlineTag:           true,
	InlineMath:          true,
	InlineStrikethrough: true,
	InlineMark:          true,
}

type Markdown = nativeparser.Markdown

func NewLute() *lute.Lute    { return nativeparser.NewLute(*MarkdownSettings) }
func NewStdLute() *lute.Lute { return nativeparser.NewStdLute(*MarkdownSettings) }

func LinkTarget(htmlStr, linkBase string) (ret string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		logging.LogErrorf("parse HTML failed: %s", err)
		return
	}

	doc.Find("a").Each(func(i int, selection *goquery.Selection) {
		if href, ok := selection.Attr("href"); ok {
			if IsRelativePath(href) {
				selection.SetAttr("href", linkBase+href)
			}

			// The hyperlink in the marketplace package README fails to jump to the browser to open https://github.com/siyuan-note/siyuan/issues/8452
			selection.SetAttr("target", "_blank")
		}
	})

	ret, _ = doc.Find("body").Html()
	return
}
