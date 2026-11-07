// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"html"
	"maps"
	"sort"
	"strings"
	av "wiki-kernel/internal/avmodel"
)

type ReferenceServices struct {
	Assets map[string]string
	Walk   func(context.Context, func(*ast.Node) error) error
	AV     func(string) (*av.AttributeView, error)
	AVData func(context.Context) ([][]byte, error)
	Exists func(context.Context, string) (bool, error)
}

func TitleImage(root *ast.Node) string {
	if root == nil {
		return ""
	}
	const prefix = "background-image: url("
	s := strings.ReplaceAll(html.UnescapeString(strings.TrimSpace(root.IALAttr("title-img"))), "background-image:url(", prefix)
	i := strings.Index(s, prefix)
	j := strings.LastIndex(s, ")")
	if i < 0 || j < i+len(prefix) {
		return ""
	}
	return strings.Trim(s[i+len(prefix):j], "\"'")
}
func CheckReferences(ctx context.Context, missing bool, s ReferenceServices) ([]string, error) {
	if ctx == nil || s.Walk == nil || s.AV == nil || s.AVData == nil || s.Exists == nil {
		return nil, errors.New("asset_reference_unbound")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	assets := maps.Clone(s.Assets)
	refs := map[string]bool{}
	e := s.Walk(ctx, func(root *ast.Node) error {
		links, e := LinkDests(root, s.AV)
		if e != nil {
			return e
		}
		if title := TitleImage(root); IsAssetLinkDest([]byte(title)) {
			links = append(links, title)
		}
		for _, dest := range links {
			if !strings.HasPrefix(dest, "assets/") {
				continue
			}
			if i := strings.Index(dest, "?"); i > 0 {
				dest = dest[:i]
			}
			refs[dest] = true
		}
		return ctx.Err()
	})
	if e != nil {
		return nil, e
	}
	out := []string{}
	if missing {
		for dest := range refs {
			if strings.HasSuffix(dest, "/") {
				continue
			}
			if strings.Contains(strings.ToLower(dest), ".pdf/") {
				if i := strings.LastIndex(dest, "/"); i >= 0 && ast.IsNodeIDPattern(dest[i+1:]) {
					continue
				}
			}
			if assets[dest] != "" {
				continue
			}
			if strings.HasPrefix(dest, "assets/.") {
				exists, e := s.Exists(ctx, dest)
				if e != nil {
					return nil, e
				}
				if exists {
					continue
				}
			}
			out = append(out, dest)
		}
	} else {
		avs, e := s.AVData(ctx)
		if e != nil {
			return nil, e
		}
		for asset, rel := range assets {
			if strings.HasSuffix(asset, "ocr-texts.json") || strings.HasSuffix(asset, "android-notification-texts.txt") {
				continue
			}
			used := false
			for dest := range refs {
				if strings.HasSuffix(dest, "/") {
					if assets[dest] != "" && strings.HasPrefix(asset, dest) {
						used = true
						break
					}
				} else if assets[dest] != "" && (strings.HasPrefix(dest, asset) || dest == asset || strings.HasSuffix(dest, ".pdf") && dest+".sya" == asset) {
					used = true
					break
				}
			}
			if !used {
				for _, raw := range avs {
					if bytes.Contains(raw, []byte(asset)) {
						used = true
						break
					}
				}
			}
			if !used {
				out = append(out, rel)
			}
		}
	}
	sort.Strings(out)
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
