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

package conf

type Lang struct {
	Label string `json:"label"` // 简体中文
	Name  string `json:"name"`  // zh_CN
}

// PortableLanguages lists the language assets shipped with the original app.
func PortableLanguages() []*Lang {
	return []*Lang{
		{Name: "ar_SA", Label: "العربية"}, {Name: "de_DE", Label: "Deutsch"},
		{Name: "en_US", Label: "English"}, {Name: "es_ES", Label: "Español"},
		{Name: "fr_FR", Label: "Français"}, {Name: "he_IL", Label: "עברית"},
		{Name: "it_IT", Label: "Italiano"}, {Name: "ja_JP", Label: "日本語"},
		{Name: "pl_PL", Label: "Polski"}, {Name: "pt_BR", Label: "Português (Brasil)"},
		{Name: "ru_RU", Label: "Русский"}, {Name: "zh_CHT", Label: "繁體中文"},
		{Name: "zh_CN", Label: "简体中文"},
	}
}
