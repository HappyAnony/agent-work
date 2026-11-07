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

package model

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/internal/assetflow"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/disintegration/imaging"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func GetAssetPathByHash(hash string) string {
	assetHash := cache.GetAssetHash(hash)
	if nil == assetHash {
		sqlAsset := sql.QueryAssetByHash(hash)
		if nil == sqlAsset {
			return ""
		}
		cache.SetAssetHash(sqlAsset.Hash, sqlAsset.Path)
		return sqlAsset.Path
	}
	return assetHash.Path
}

func HandleAssetsWatchEvent(ctx context.Context, e assetflow.Event) error {
	w, err := captureAssetWatchContext(ctx)
	if err != nil {
		return err
	}
	return w.dispatch(ctx, e)
}

func HandleAssetsRemoveEvent(assetAbsPath string) {
	removeIndexAssetContent(assetAbsPath)
	removeAssetThumbnail(assetAbsPath)
}

func HandleAssetsChangeEvent(assetAbsPath string) {
	indexAssetContent(assetAbsPath)
	removeAssetThumbnail(assetAbsPath)
}

func removeAssetThumbnail(assetAbsPath string) {
	if util.IsCompressibleAssetImage(assetAbsPath) {
		p := filepath.ToSlash(assetAbsPath)
		idx := strings.Index(p, "assets/")
		if -1 == idx {
			return
		}
		thumbnailPath := filepath.Join(util.TempDir, "thumbnails", "assets", p[idx+7:])
		os.RemoveAll(thumbnailPath)
	}
}

func NeedGenerateAssetsThumbnail(sourceImgPath string) bool {
	info, err := os.Stat(sourceImgPath)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	return info.Size() > 1024*1024
}

func GenerateAssetsThumbnail(sourceImgPath, resizedImgPath string) (err error) {
	start := time.Now()
	img, err := imaging.Open(sourceImgPath)
	if err != nil {
		return
	}

	// 获取原图宽高
	originalWidth := img.Bounds().Dx()
	originalHeight := img.Bounds().Dy()

	// 固定最大宽度为 520，计算缩放比例
	maxWidth := 520
	scale := float64(maxWidth) / float64(originalWidth)

	// 按比例计算新的宽高
	newWidth := maxWidth
	newHeight := int(float64(originalHeight) * scale)

	// 缩放图片
	resizedImg := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)

	// 保存缩放后的图片
	err = os.MkdirAll(filepath.Dir(resizedImgPath), 0755)
	if err != nil {
		return
	}
	err = imaging.Save(resizedImg, resizedImgPath)
	if err != nil {
		return
	}
	logging.LogDebugf("generated thumbnail image [%s] to [%s], cost [%d]ms", sourceImgPath, resizedImgPath, time.Since(start).Milliseconds())
	return
}

func DocImageAssets(id string) ([]string, error) {
	return DocImageAssetsContext(context.Background(), id)
}
func DocAssets(id string) ([]string, error) { return DocAssetsContext(context.Background(), id) }

func NetAssets2LocalAssets(id string, onlyImg bool, originalURL string) error {
	return NetAssets2LocalAssetsContext(context.Background(), id, onlyImg, originalURL)
}

func SearchAssetsByName(keyword string, exts []string) []*cache.Asset {
	v, e := SearchAssetsByNameContext(context.Background(), keyword, exts)
	if e != nil {
		logging.LogErrorf("search assets: %s", e)
	}
	return v
}

func GetAssetAbsPath(relativePath string) (string, error) {
	return GetAssetAbsPathContext(context.Background(), relativePath)
}
func GetAssetAbsPathContext(ctx context.Context, relative string) (string, error) {
	return resolveAssetContext(ctx, relative)
}

func UploadAssets2Cloud(id string) (int, error) {
	return UploadAssets2CloudContext(context.Background(), id)
}
func UploadAssets2CloudContext(ctx context.Context, id string) (count int, err error) {
	owner := Conf
	if owner == nil {
		return 0, conf.ErrBusinessUnbound
	}
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	if !(conf.AppData{User: owner.GetUser()}).IsSubscriber() {
		return
	}

	tree, err := LoadTreeByBlockID(id)
	if err != nil {
		return
	}

	node := treenode.GetNodeInTree(tree, id)
	if nil == node {
		err = ErrBlockNotFound
		return
	}

	nodes := []*ast.Node{node}
	if ast.NodeHeading == node.Type {
		nodes = append(nodes, treenode.HeadingChildren(node)...)
	}

	var assets []string
	for _, n := range nodes {
		assets = append(assets, getAssetsLinkDests(n)...)
		assets = append(assets, getQueryEmbedNodesAssetsLinkDests(n)...)
	}
	assets = gulu.Str.RemoveDuplicatedElem(assets)
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	count, err = owner.uploadCloudAssets(ctx, assets, bizTypeUploadAssets)
	if err != nil {
		return
	}
	return
}

const (
	bizTypeUploadAssets  = "upload-assets"
	bizTypeExport2Liandi = "export-liandi"
)

// uploadAssets2Cloud 将资源文件上传到云端图床。
func uploadAssets2Cloud(paths []string, biz string) (int, error) {
	return uploadAssets2CloudContext(context.Background(), paths, biz)
}

func RemoveUnusedAssets() []string {
	v, e := RemoveUnusedAssetsContext(context.Background())
	if e != nil {
		logging.LogErrorf("remove unused assets: %s", e)
	}
	return v
}
func RemoveUnusedAsset(p string) string {
	v, e := RemoveUnusedAssetContext(context.Background(), p)
	if e != nil {
		logging.LogErrorf("remove unused asset: %s", e)
	}
	return v
}

func RenameAsset(old, name string) (string, error) {
	return RenameAssetContext(context.Background(), old, name)
}

func UnusedAssets() []string {
	v, e := UnusedAssetsContext(context.Background())
	if e != nil {
		logging.LogErrorf("unused assets: %s", e)
	}
	return v
}
func MissingAssets() []string {
	v, e := MissingAssetsContext(context.Background())
	if e != nil {
		logging.LogErrorf("missing assets: %s", e)
	}
	return v
}

func emojisInTree(tree *parse.Tree) (ret []string) {
	if icon := tree.Root.IALAttr("icon"); "" != icon {
		if !strings.Contains(icon, "://") && !strings.HasPrefix(icon, "api/icon/") && !util.NativeEmojiChars[icon] {
			ret = append(ret, "/emojis/"+icon)
		}
	}

	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}
		if ast.NodeEmojiImg == n.Type {
			tokens := n.Tokens
			idx := bytes.Index(tokens, []byte("src=\""))
			if -1 == idx {
				return ast.WalkContinue
			}
			src := tokens[idx+len("src=\""):]
			src = src[:bytes.Index(src, []byte("\""))]
			ret = append(ret, string(src))
		}
		return ast.WalkContinue
	})
	ret = gulu.Str.RemoveDuplicatedElem(ret)
	return
}

func getQueryEmbedNodesAssetsLinkDests(node *ast.Node) (ret []string) {
	// The images in the embed blocks are not uploaded to the community hosting https://github.com/siyuan-note/siyuan/issues/10042

	ret = []string{}
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || ast.NodeBlockQueryEmbedScript != n.Type {
			return ast.WalkContinue
		}

		stmt := n.TokensStr()
		stmt = html.UnescapeString(stmt)
		stmt = strings.ReplaceAll(stmt, editor.IALValEscNewLine, "\n")
		sqlBlocks := sql.SelectBlocksRawStmt(stmt, 1, Conf.Search.Limit)
		for _, sqlBlock := range sqlBlocks {
			subtree, _ := LoadTreeByBlockID(sqlBlock.ID)
			if nil == subtree {
				continue
			}
			embedNode := treenode.GetNodeInTree(subtree, sqlBlock.ID)
			if nil == embedNode {
				continue
			}

			ret = append(ret, getAssetsLinkDests(embedNode)...)
		}
		return ast.WalkContinue
	})
	ret = gulu.Str.RemoveDuplicatedElem(ret)
	return
}

func getAssetsLinkDests(node *ast.Node) []string {
	ret, _ := getAssetsLinkDestsWithAV(node, func(id string) (*av.AttributeView, error) {
		a, _ := av.ParseAttributeView(id)
		return a, nil
	})
	return ret
}
func getAssetsLinkDestsWithAV(node *ast.Node, read func(string) (*av.AttributeView, error)) ([]string, error) {
	return assetflow.LinkDests(node, read)
}

func setAssetsLinkDest(node *ast.Node, oldDest, dest string) {
	if ast.NodeLinkDest == node.Type {
		if bytes.HasPrefix(node.Tokens, []byte("//")) {
			node.Tokens = append([]byte("https:"), node.Tokens...)
		}
		node.Tokens = bytes.ReplaceAll(node.Tokens, []byte(oldDest), []byte(dest))
	} else if node.IsTextMarkType("a") {
		if strings.HasPrefix(node.TextMarkAHref, "//") {
			node.TextMarkAHref = "https:" + node.TextMarkAHref
		}
		node.TextMarkAHref = strings.ReplaceAll(node.TextMarkAHref, oldDest, dest)
	} else if ast.NodeAudio == node.Type || ast.NodeVideo == node.Type {
		if strings.HasPrefix(node.TextMarkAHref, "//") {
			node.TextMarkAHref = "https:" + node.TextMarkAHref
		}
		node.Tokens = bytes.ReplaceAll(node.Tokens, []byte(oldDest), []byte(dest))
	} else if ast.NodeAttributeView == node.Type {
		needWrite := false
		attrView, _ := av.ParseAttributeView(node.AttributeViewID)
		if nil == attrView {
			return
		}

		for _, keyValues := range attrView.KeyValues {
			if av.KeyTypeMAsset != keyValues.Key.Type {
				continue
			}

			for _, value := range keyValues.Values {
				if 1 > len(value.MAsset) {
					continue
				}

				for _, asset := range value.MAsset {
					if oldDest == asset.Content && oldDest != dest {
						asset.Content = dest
						needWrite = true
					}
				}
			}
		}

		if needWrite {
			av.SaveAttributeView(attrView)
		}
	}
}

// allAssetAbsPaths 返回 asset 相对路径（assets/xxx）到绝对路径（F:\SiYuan\data\assets\xxx）的映射。
func allAssetAbsPaths() (assetsAbsPathMap map[string]string, err error) {
	notebooks, err := ListNotebooks()
	if err != nil {
		return
	}

	assetsAbsPathMap = map[string]string{}
	// 笔记本 assets
	for _, notebook := range notebooks {
		notebookAbsPath := filepath.Join(util.DataDir, notebook.ID)
		filelock.Walk(notebookAbsPath, func(path string, d fs.DirEntry, err error) error {
			if notebookAbsPath == path {
				return nil
			}
			if isSkipFile(d.Name()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if filelock.IsHidden(path) {
				// 清理资源文件时忽略隐藏文件 Ignore hidden files when cleaning unused assets https://github.com/siyuan-note/siyuan/issues/12172
				return nil
			}

			if d.IsDir() && "assets" == d.Name() {
				filelock.Walk(path, func(assetPath string, d fs.DirEntry, err error) error {
					if path == assetPath {
						return nil
					}
					if isSkipFile(d.Name()) {
						if d.IsDir() {
							return filepath.SkipDir
						}
						return nil
					}
					relPath := filepath.ToSlash(assetPath)
					relPath = relPath[strings.Index(relPath, "assets/"):]
					if d.IsDir() {
						relPath += "/"
					}
					assetsAbsPathMap[relPath] = assetPath
					return nil
				})
				return filepath.SkipDir
			}
			return nil
		})
	}

	// 全局 assets
	dataAssetsAbsPath := util.GetDataAssetsAbsPath()
	filelock.Walk(dataAssetsAbsPath, func(assetPath string, d fs.DirEntry, err error) error {
		if dataAssetsAbsPath == assetPath {
			return nil
		}

		if isSkipFile(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if filelock.IsHidden(assetPath) {
			// 清理资源文件时忽略隐藏文件 Ignore hidden files when cleaning unused assets https://github.com/siyuan-note/siyuan/issues/12172
			return nil
		}

		relPath := filepath.ToSlash(assetPath)
		relPath = relPath[strings.Index(relPath, "assets/"):]
		if d.IsDir() {
			relPath += "/"
		}
		assetsAbsPathMap[relPath] = assetPath
		return nil
	})
	return
}

// copyBoxAssetsToDataAssets 将笔记本路径下所有（包括子文档）的 assets 复制一份到 data/assets 中。
func copyBoxAssetsToDataAssets(boxID string) {
	boxLocalPath := filepath.Join(util.DataDir, boxID)
	copyAssetsToDataAssets(boxLocalPath)
}

// copyDocAssetsToDataAssets 将文档路径下所有（包括子文档）的 assets 复制一份到 data/assets 中。
func copyDocAssetsToDataAssets(boxID, parentDocPath string) {
	boxLocalPath := filepath.Join(util.DataDir, boxID)
	parentDocDirAbsPath := filepath.Dir(filepath.Join(boxLocalPath, parentDocPath))
	copyAssetsToDataAssets(parentDocDirAbsPath)
}

func copyAssetsToDataAssets(rootPath string) {
	var assetsDirPaths []string
	filelock.Walk(rootPath, func(path string, d fs.DirEntry, err error) error {
		if nil != err || rootPath == path || nil == d {
			return nil
		}

		isDir, name := d.IsDir(), d.Name()
		if isSkipFile(name) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}

		if "assets" == name && isDir {
			assetsDirPaths = append(assetsDirPaths, path)
		}
		return nil
	})

	dataAssetsPath := filepath.Join(util.DataDir, "assets")
	for _, assetsDirPath := range assetsDirPaths {
		if err := filelock.Copy(assetsDirPath, dataAssetsPath); err != nil {
			logging.LogErrorf("copy tree assets from [%s] to [%s] failed: %s", assetsDirPaths, dataAssetsPath, err)
		}
	}
}
