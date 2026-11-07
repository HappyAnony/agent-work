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

package nodetype

var typeAbbrMap = map[string]string{
	// 块级元素
	"NodeDocument":         "d",
	"NodeHeading":          "h",
	"NodeList":             "l",
	"NodeListItem":         "i",
	"NodeCodeBlock":        "c",
	"NodeMathBlock":        "m",
	"NodeTable":            "t",
	"NodeBlockquote":       "b",
	"NodeSuperBlock":       "s",
	"NodeParagraph":        "p",
	"NodeHTMLBlock":        "html",
	"NodeBlockQueryEmbed":  "query_embed",
	"NodeAttributeView":    "av",
	"NodeKramdownBlockIAL": "ial",
	"NodeIFrame":           "iframe",
	"NodeWidget":           "widget",
	"NodeThematicBreak":    "tb",
	"NodeVideo":            "video",
	"NodeAudio":            "audio",
	// 行级元素
	"NodeText":     "text",
	"NodeImage":    "img",
	"NodeLinkText": "link_text",
	"NodeLinkDest": "link_dest",
	"NodeTextMark": "textmark",
}

var abbrTypeMap = map[string]string{
	"d":           "NodeDocument",
	"h":           "NodeHeading",
	"l":           "NodeList",
	"i":           "NodeListItem",
	"c":           "NodeCodeBlock",
	"m":           "NodeMathBlock",
	"t":           "NodeTable",
	"b":           "NodeBlockquote",
	"s":           "NodeSuperBlock",
	"p":           "NodeParagraph",
	"html":        "NodeHTMLBlock",
	"query_embed": "NodeBlockQueryEmbed",
	"av":          "NodeAttributeView",
	"ial":         "NodeKramdownBlockIAL",
	"iframe":      "NodeIFrame",
	"widget":      "NodeWidget",
	"tb":          "NodeThematicBreak",
	"video":       "NodeVideo",
	"audio":       "NodeAudio",
	"text":        "NodeText",
	"img":         "NodeImage",
	"link_text":   "NodeLinkText",
	"link_dest":   "NodeLinkDest",
	"textmark":    "NodeTextMark",
}

func TypeAbbr(nodeType string) string { return typeAbbrMap[nodeType] }
func FromAbbrType(abbr string) string { return abbrTypeMap[abbr] }
