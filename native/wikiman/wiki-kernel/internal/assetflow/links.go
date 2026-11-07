// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"strings"
	av "wiki-kernel/internal/avmodel"
)

func IsAssetLinkDest(b []byte) bool { return bytes.HasPrefix(b, []byte("assets/")) }
func NodeSrc(n *ast.Node) string {
	if i := bytes.Index(n.Tokens, []byte(`src="`)); i > 0 {
		s := n.Tokens[i+5:]
		if j := bytes.IndexByte(s, '"'); j > 0 {
			return strings.TrimSpace(string(s[:j]))
		}
	}
	return ""
}
func LinkDests(node *ast.Node, read func(string) (*av.AttributeView, error)) (ret []string, err error) {
	if node == nil || read == nil {
		return nil, errors.New("asset_links_unbound")
	}
	ret = []string{}
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if n.IsBlock() {
			// 以 custom-data-assets 开头的块属性值可能是多个资源文件链接，需要计入
			// Ignore assets associated with the `custom-data-assets` block attribute when cleaning unreferenced assets https://github.com/siyuan-note/siyuan/issues/12574
			for _, kv := range n.KramdownIAL {
				k := kv[0]
				if strings.HasPrefix(k, "custom-data-assets") {
					dest := kv[1]
					if "" == dest || !IsAssetLinkDest([]byte(dest)) {
						continue
					}
					ret = append(ret, dest)
				}
			}
		}

		// 修改以下代码时需要同时修改 database 构造行级元素实现，增加必要的类型
		if !entering || (ast.NodeLinkDest != n.Type && ast.NodeHTMLBlock != n.Type && ast.NodeInlineHTML != n.Type &&
			ast.NodeIFrame != n.Type && ast.NodeWidget != n.Type && ast.NodeAudio != n.Type && ast.NodeVideo != n.Type &&
			ast.NodeAttributeView != n.Type && !n.IsTextMarkType("a") && !n.IsTextMarkType("file-annotation-ref")) {
			return ast.WalkContinue
		}

		if ast.NodeLinkDest == n.Type {
			if !IsAssetLinkDest(n.Tokens) {
				return ast.WalkContinue
			}

			dest := strings.TrimSpace(string(n.Tokens))
			ret = append(ret, dest)
		} else if n.IsTextMarkType("a") {
			if !IsAssetLinkDest(gulu.Str.ToBytes(n.TextMarkAHref)) {
				return ast.WalkContinue
			}

			dest := strings.TrimSpace(n.TextMarkAHref)
			ret = append(ret, dest)
		} else if n.IsTextMarkType("file-annotation-ref") {
			if !IsAssetLinkDest(gulu.Str.ToBytes(n.TextMarkFileAnnotationRefID)) {
				return ast.WalkContinue
			}

			if !strings.Contains(n.TextMarkFileAnnotationRefID, "/") {
				return ast.WalkContinue
			}

			dest := n.TextMarkFileAnnotationRefID[:strings.LastIndexByte(n.TextMarkFileAnnotationRefID, '/')]
			dest = strings.TrimSpace(dest)
			ret = append(ret, dest)
		} else if ast.NodeAttributeView == n.Type {
			var attrView *av.AttributeView
			attrView, err = read(n.AttributeViewID)
			if err != nil {
				return ast.WalkStop
			}
			if nil == attrView {
				return ast.WalkContinue
			}

			for _, keyValues := range attrView.KeyValues {
				if keyValues == nil || keyValues.Key == nil {
					err = errors.New("asset_links_invalid_av")
					return ast.WalkStop
				}
				if av.KeyTypeMAsset == keyValues.Key.Type {
					for _, value := range keyValues.Values {
						if value == nil {
							err = errors.New("asset_links_invalid_av")
							return ast.WalkStop
						}
						if 1 > len(value.MAsset) {
							continue
						}

						for _, asset := range value.MAsset {
							if asset == nil {
								err = errors.New("asset_links_invalid_av")
								return ast.WalkStop
							}
							dest := asset.Content
							if !IsAssetLinkDest([]byte(dest)) {
								continue
							}
							ret = append(ret, strings.TrimSpace(dest))
						}
					}
				} else if av.KeyTypeURL == keyValues.Key.Type {
					for _, value := range keyValues.Values {
						if value == nil {
							err = errors.New("asset_links_invalid_av")
							return ast.WalkStop
						}
						if nil != value.URL {
							dest := value.URL.Content
							if !IsAssetLinkDest([]byte(dest)) {
								continue
							}
							ret = append(ret, strings.TrimSpace(dest))
						}
					}
				}
			}
		} else {
			if ast.NodeWidget == n.Type {
				dataAssets := n.IALAttr("custom-data-assets")
				if "" == dataAssets {
					// 兼容两种属性名 custom-data-assets 和 data-assets https://github.com/siyuan-note/siyuan/issues/4122#issuecomment-1154796568
					dataAssets = n.IALAttr("data-assets")
				}
				if !IsAssetLinkDest([]byte(dataAssets)) {
					return ast.WalkContinue
				}
				ret = append(ret, dataAssets)
			} else { // HTMLBlock/InlineHTML/IFrame/Audio/Video
				dest := NodeSrc(n)
				if !IsAssetLinkDest([]byte(dest)) {
					return ast.WalkContinue
				}
				ret = append(ret, dest)
			}
		}
		return ast.WalkContinue
	})
	if err != nil {
		return nil, err
	}
	ret = gulu.Str.RemoveDuplicatedElem(ret)
	for i, dest := range ret {
		// 对于 macOS 的 rtfd 文件夹格式需要特殊处理，为其加上结尾 /
		if strings.HasSuffix(dest, ".rtfd") {
			ret[i] = dest + "/"
		}
	}
	return
}

// ImageDests is the original document-image scan, shared by legacy and native
// read orchestration. It does not resolve or rewrite the captured destinations.
func ImageDests(ctx context.Context, root *ast.Node) (ret []string, err error) {
	ast.Walk(root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if e := ctx.Err(); e != nil {
			err = e
			return ast.WalkStop
		}
		if enter && n.Type == ast.NodeImage {
			if dest := n.ChildByType(ast.NodeLinkDest); dest != nil && len(dest.Tokens) > 0 {
				ret = append(ret, string(dest.Tokens))
			}
		}
		return ast.WalkContinue
	})
	if err != nil {
		return nil, err
	}
	return ret, ctx.Err()
}
