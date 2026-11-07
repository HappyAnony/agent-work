// SPDX-License-Identifier: AGPL-3.0-or-later
package assetcache

import (
	"path/filepath"
	"sort"
	"strings"
	"wiki-kernel/search"
)

func SearchNames(keyword string, exts []string, assets map[string]Asset, limit int, sensitive bool) (ret []*Asset) {
	ret = []*Asset{}
	var keywords []string
	keywords = append(keywords, keyword)
	if "" != keyword {
		keywords = append(keywords, strings.Split(keyword, " ")...)
	}
	pathHitCount := map[string]int{}
	filterByExt := 0 < len(exts)
	for _, asset := range assets {
		if filterByExt {
			ext := filepath.Ext(asset.HName)
			includeExt := false
			for _, e := range exts {
				if strings.ToLower(ext) == strings.ToLower(e) {
					includeExt = true
					break
				}
			}
			if !includeExt {
				continue
			}
		}

		lowerHName := strings.ToLower(asset.HName)
		lowerPath := strings.ToLower(asset.Path)
		var hitNameCount, hitPathCount int
		for i, k := range keywords {
			lowerKeyword := strings.ToLower(k)
			if 0 == i {
				// 第一个是完全匹配，权重最高
				if strings.Contains(lowerHName, lowerKeyword) {
					hitNameCount += 64
				}
				if strings.Contains(lowerPath, lowerKeyword) {
					hitPathCount += 64
				}
			}

			hitNameCount += strings.Count(lowerHName, lowerKeyword)
			hitPathCount += strings.Count(lowerPath, lowerKeyword)
			if 1 > hitNameCount && 1 > hitPathCount {
				continue
			}
		}

		if 1 > hitNameCount+hitPathCount {
			continue
		}
		pathHitCount[asset.Path] += hitNameCount + hitPathCount

		hName := asset.HName
		if 0 < hitNameCount {
			_, hName = search.MarkText(asset.HName, strings.Join(keywords, search.TermSep), 64, sensitive)
		}
		ret = append(ret, &Asset{
			HName:   hName,
			Path:    asset.Path,
			Updated: asset.Updated,
		})
	}

	if 0 < len(pathHitCount) {
		sort.Slice(ret, func(i, j int) bool {
			return pathHitCount[ret[i].Path] > pathHitCount[ret[j].Path]
		})
	} else {
		sort.Slice(ret, func(i, j int) bool {
			return ret[i].Updated > ret[j].Updated
		})
	}

	if limit <= len(ret) {
		ret = ret[:limit]
	}
	return
}
