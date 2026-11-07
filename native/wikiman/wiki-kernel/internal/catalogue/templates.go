// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/template.go catalogue matching/ranking; explicit directory ports.
package catalogue

import (
	"context"
	"errors"
	"github.com/xrash/smetrics"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/search"
)

func SearchTemplates(ctx context.Context, templates, keyword string, caseSensitive bool, readDir func(string) ([]os.DirEntry, error), walk func(string, fs.WalkDirFunc) error) (ret []*blockview.Block, err error) {
	ret = []*blockview.Block{}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	groups, err := readDir(templates)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ret, nil
		}
		return nil, err
	}

	keyword = strings.TrimSpace(keyword)
	type result struct {
		block *blockview.Block
		score float64
	}
	var results []*result
	keywords := strings.Fields(keyword)
	for _, group := range groups {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if strings.HasPrefix(group.Name(), ".") {
			continue
		}

		if group.IsDir() {
			templateDir := filepath.Join(templates, group.Name())
			err = walk(templateDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err = ctx.Err(); err != nil {
					return err
				}
				name := strings.ToLower(d.Name())
				if strings.HasPrefix(name, ".") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				if d.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, "readme") {
					return nil
				}

				content := strings.TrimPrefix(path, templates)
				content = strings.TrimSuffix(content, ".md")
				p := filepath.Join(group.Name(), content)
				score := 0.0
				hit := true
				for _, k := range keywords {
					if strings.Contains(strings.ToLower(p), strings.ToLower(k)) {
						score += smetrics.JaroWinkler(name, k, 0.7, 4)
					} else {
						hit = false
						break
					}
				}
				if hit {
					content = strings.TrimPrefix(path, templates)
					content = strings.TrimSuffix(content, ".md")
					content = filepath.ToSlash(content)
					_, content = search.MarkText(content, strings.Join(keywords, search.TermSep), 32, caseSensitive)
					b := &blockview.Block{Path: path, Content: content}
					results = append(results, &result{block: b, score: score})
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			name := strings.ToLower(group.Name())
			if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".md") || "readme.md" == name {
				continue
			}

			content := group.Name()
			content = strings.TrimSuffix(content, ".md")
			score := 0.0
			hit := true
			for _, k := range keywords {
				if strings.Contains(strings.ToLower(content), strings.ToLower(k)) {
					score += smetrics.JaroWinkler(name, k, 0.7, 4)
				} else {
					hit = false
					break
				}
			}
			if hit {
				content = filepath.ToSlash(content)
				_, content = search.MarkText(content, strings.Join(keywords, search.TermSep), 32, caseSensitive)
				b := &blockview.Block{Path: filepath.Join(templates, group.Name()), Content: content}
				results = append(results, &result{block: b, score: score})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	for _, r := range results {
		ret = append(ret, r.block)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
