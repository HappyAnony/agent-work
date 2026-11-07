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

// NewIndex retains the original SQL Markdown rendering rule, including span IAL.
func NewIndex(markdown Markdown) *lute.Lute {
	engine := NewLute(markdown)
	engine.RenderOptions.KramdownBlockIAL = false
	return engine
}

// NewConfiguredIndex preserves index rendering while owning its emoji map.
func NewConfiguredIndex(options EditorOptions) *lute.Lute {
	engine := NewIndex(options.Markdown)
	emojis := make(map[string]string, len(options.Emojis))
	for name, src := range options.Emojis {
		emojis[name] = src
	}
	engine.PutEmojis(emojis)
	return engine
}

// EditorOptions is a detached view of original AppConf parser-facing settings.
// Device/editor preferences are supplied by the caller, not fetched or saved by
// the parser. This does not replace the full AppConf/UI configuration contract.
type EditorOptions struct {
	Markdown                   Markdown
	CodeSyntaxHighlightLineNum bool
	ParagraphBeginningSpace    bool
	DisplayNetImgMark          bool
	Spellcheck                 bool
	Emojis                     map[string]string
}

func NewEditor(options EditorOptions) *lute.Lute {
	engine := NewLute(options.Markdown)
	engine.SetCodeSyntaxHighlightLineNum(options.CodeSyntaxHighlightLineNum)
	engine.SetChineseParagraphBeginningSpace(options.ParagraphBeginningSpace)
	engine.SetProtyleMarkNetImg(options.DisplayNetImgMark)
	engine.SetSpellcheck(options.Spellcheck)
	emojis := make(map[string]string, len(options.Emojis))
	for k, v := range options.Emojis {
		emojis[k] = v
	}
	engine.PutEmojis(emojis)
	return engine
}
