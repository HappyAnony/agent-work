// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package catalogue

import (
	"encoding/json"
	"github.com/88250/lute"
	"github.com/PuerkitoBio/goquery"
	"path"
	"strings"
	"wiki-kernel/internal/extensionprefs"
)

func localized(v any) map[string]string {
	raw, _ := json.Marshal(v)
	m := map[string]string{}
	json.Unmarshal(raw, &m)
	return m
}
func preferred(v any, language, fallback string) string {
	m := localized(v)
	out := m["default"]
	if text := m[language]; text != "" {
		out = text
	} else {
		switch language {
		case "ar_SA", "de_DE", "en_US", "es_ES", "fr_FR", "he_IL", "it_IT", "ja_JP", "pl_PL", "pt_BR", "ru_RU", "zh_CHT", "zh_CN":
		default:
			if m["en_US"] != "" {
				out = m["en_US"]
			}
		}
	}
	if out == "" {
		out = fallback
	}
	return out
}
func PreferredName(p *Package, language string) string {
	if p == nil {
		return ""
	}
	return extensionprefs.PreferredName(extensionprefs.Package{Name: p.Name, DisplayName: localized(p.DisplayName)}, language)
}
func PreferredDescription(p *Description, language string) string { return preferred(p, language, "") }
func PreferredReadme(p *Readme, language string) string           { return preferred(p, language, "README.md") }
func PreferredFunding(p *Funding) string {
	if p == nil {
		return ""
	}
	if p.OpenCollective != "" {
		return "https://opencollective.com/" + p.OpenCollective
	}
	if p.Patreon != "" {
		return "https://www.patreon.com/" + p.Patreon
	}
	if p.GitHub != "" {
		return "https://github.com/sponsors/" + p.GitHub
	}
	if len(p.Custom) > 0 {
		return p.Custom[0]
	}
	return ""
}
func Match(keyword string, p *Package) bool {
	keywords := strings.Fields(strings.ToLower(keyword))
	if len(keywords) == 0 {
		return true
	}
	if p == nil || p.DisplayName == nil || p.Description == nil {
		return false
	}
	values := []string{p.DisplayName.Default, p.DisplayName.ZhCN, p.DisplayName.ZhCHT, p.DisplayName.EnUS, p.Description.Default, p.Description.ZhCN, p.Description.ZhCHT, p.Description.EnUS, path.Base(p.RepoURL), p.Author}
	values = append(values, p.Keywords...)
	for _, keyword := range keywords {
		hit := false
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), keyword) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// Original README renderer and link-target rules, without util's logger or any
// implicit resource lookup. The caller supplies the link base and bytes.
func RenderREADME(base string, raw []byte) (string, error) {
	p := lute.New()
	p.SetSoftBreak2HardBreak(false)
	p.SetCodeSyntaxHighlight(false)
	p.SetLinkBase(base)
	doc, e := goquery.NewDocumentFromReader(strings.NewReader(p.Md2HTML(string(raw))))
	if e != nil {
		return "", e
	}
	doc.Find("a").Each(func(_ int, s *goquery.Selection) {
		if href, ok := s.Attr("href"); ok {
			if href == "" || href[0] != '/' && !strings.Contains(href, ":/") && !strings.Contains(href, ":\\") {
				s.SetAttr("href", base+href)
			}
			s.SetAttr("target", "_blank")
		}
	})
	return doc.Find("body").Html()
}
