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
	"crypto/sha1"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/lex"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/filesys"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func ExportAv2CSV(avID, blockID string) (string, error) {
	return ExportAv2CSVContext(context.Background(), avID, blockID)
}

func Export2Liandi(id string) error { return Export2LiandiContext(context.Background(), id) }
func Export2LiandiContext(ctx context.Context, id string) error {
	return export2LiandiContextWithPorts(ctx, id, liandiPorts{})
}

func ExportSystemLog() string {
	v, e := ExportSystemLogContext(context.Background())
	if e != nil {
		logOwnedTaskError("export system log", e)
	}
	return v
}

func ExportNotebookSY(id string, contexts ...context.Context) (zipPath string) {
	return exportBoxSYZip(id, contexts...)
}
func ExportSY(id string, contexts ...context.Context) (name, zipPath string) {
	e, err := captureArchiveExport(legacyAVContext(contexts))
	if err != nil {
		logging.LogErrorf("capture archive export: %s", err)
		return
	}
	selected, err := e.single(id)
	if err != nil {
		logging.LogErrorf("select archive export: %s", err)
		return
	}
	zipPath = exportSYZipWithContext(e, selected.box, selected.rootDir, selected.folder, selected.paths)
	name = selected.name
	return
}

func ExportDataInFolder(folder string) (string, error) {
	return ExportDataInFolderContext(context.Background(), folder)
}
func ExportData() (string, error) { return ExportDataContext(context.Background()) }
func ExportResources(paths []string, name string) (string, error) {
	return ExportResourcesContext(context.Background(), paths, name)
}

func Preview(id string, fillCSSVar bool) string {
	html, err := PreviewContext(context.Background(), id, fillCSSVar)
	if err != nil {
		logging.LogErrorf("export HTML preview: %s", err)
		return ""
	}
	return html
}

func ExportDocx(id, folder string, removeAssets, merge bool) (string, error) {
	return ExportDocxContext(context.Background(), id, folder, removeAssets, merge)
}

func ExportMarkdownHTML(id, savePath string, docx, merge bool) (name, dom string) {
	bt := treenode.GetBlockTree(id)
	if nil == bt {
		return
	}

	tree := prepareExportTree(bt)

	if merge {
		var mergeErr error
		tree, mergeErr = mergeSubDocs(tree)
		if nil != mergeErr {
			logging.LogErrorf("merge sub docs failed: %s", mergeErr)
			return
		}
	}

	blockRefMode := Conf.Export.BlockRefMode
	tree = exportTree(tree, true, false, true,
		blockRefMode, Conf.Export.BlockEmbedMode, Conf.Export.FileAnnotationRefMode,
		Conf.Export.TagOpenMarker, Conf.Export.TagCloseMarker,
		Conf.Export.BlockRefTextLeft, Conf.Export.BlockRefTextRight,
		Conf.Export.AddTitle, Conf.Export.InlineMemo, true, true, map[string]*parse.Tree{})
	name = path.Base(tree.HPath)
	name = util.FilterFileName(name) // 导出 PDF、HTML 和 Word 时未移除不支持的文件名符号 https://github.com/siyuan-note/siyuan/issues/5614
	savePath = strings.TrimSpace(savePath)

	if err := os.MkdirAll(savePath, 0755); err != nil {
		logging.LogErrorf("mkdir [%s] failed: %s", savePath, err)
		return
	}

	assets := getAssetsLinkDests(tree.Root)
	for _, asset := range assets {
		if strings.HasPrefix(asset, "assets/") {
			if strings.Contains(asset, "?") {
				asset = asset[:strings.LastIndex(asset, "?")]
			}

			srcAbsPath, err := GetAssetAbsPath(asset)
			if err != nil {
				logging.LogWarnf("resolve path of asset [%s] failed: %s", asset, err)
				continue
			}
			targetAbsPath := filepath.Join(savePath, asset)
			if err = filelock.Copy(srcAbsPath, targetAbsPath); err != nil {
				logging.LogWarnf("copy asset from [%s] to [%s] failed: %s", srcAbsPath, targetAbsPath, err)
			}
		}
	}

	srcs := []string{"stage/build/export", "stage/protyle"}
	for _, src := range srcs {
		from := filepath.Join(util.WorkingDir, src)
		to := filepath.Join(savePath, src)
		if err := filelock.Copy(from, to); err != nil {
			logging.LogWarnf("copy stage from [%s] to [%s] failed: %s", from, savePath, err)
		}
	}

	theme := Conf.Appearance.ThemeLight
	if 1 == Conf.Appearance.Mode {
		theme = Conf.Appearance.ThemeDark
	}
	// 复制主题文件夹
	srcs = []string{"themes/" + theme}
	appearancePath := util.AppearancePath
	if util.IsSymlinkPath(util.AppearancePath) {
		// Support for symlinked theme folder when exporting HTML https://github.com/siyuan-note/siyuan/issues/9173
		var readErr error
		appearancePath, readErr = filepath.EvalSymlinks(util.AppearancePath)
		if nil != readErr {
			logging.LogErrorf("readlink [%s] failed: %s", util.AppearancePath, readErr)
			return
		}
	}

	for _, src := range srcs {
		from := filepath.Join(appearancePath, src)
		to := filepath.Join(savePath, "appearance", src)
		if err := filelock.Copy(from, to); err != nil {
			logging.LogErrorf("copy appearance from [%s] to [%s] failed: %s", from, savePath, err)
			return
		}
	}

	// 只复制图标文件夹中的 icon.js 文件
	iconName := Conf.Appearance.Icon
	// 如果使用的不是内建图标（ant 或 material），需要复制 material 作为后备
	if iconName != "ant" && iconName != "material" && iconName != "" {
		srcIconFile := filepath.Join(appearancePath, "icons", "material", "icon.js")
		toIconDir := filepath.Join(savePath, "appearance", "icons", "material")
		if err := os.MkdirAll(toIconDir, 0755); err != nil {
			logging.LogErrorf("mkdir [%s] failed: %s", toIconDir, err)
			return
		}
		toIconFile := filepath.Join(toIconDir, "icon.js")
		if err := filelock.Copy(srcIconFile, toIconFile); err != nil {
			logging.LogWarnf("copy icon file from [%s] to [%s] failed: %s", srcIconFile, toIconFile, err)
		}
	}
	// 复制当前使用的图标文件
	if iconName != "" {
		srcIconFile := filepath.Join(appearancePath, "icons", iconName, "icon.js")
		toIconDir := filepath.Join(savePath, "appearance", "icons", iconName)
		if err := os.MkdirAll(toIconDir, 0755); err != nil {
			logging.LogErrorf("mkdir [%s] failed: %s", toIconDir, err)
			return
		}
		toIconFile := filepath.Join(toIconDir, "icon.js")
		if err := filelock.Copy(srcIconFile, toIconFile); err != nil {
			logging.LogWarnf("copy icon file from [%s] to [%s] failed: %s", srcIconFile, toIconFile, err)
		}
	}

	// 复制自定义表情图片
	emojis := emojisInTree(tree)
	for _, emoji := range emojis {
		from := filepath.Join(util.DataDir, emoji)
		to := filepath.Join(savePath, emoji)
		if err := filelock.Copy(from, to); err != nil {
			logging.LogErrorf("copy emojis from [%s] to [%s] failed: %s", from, to, err)
		}
	}

	if docx {
		processIFrame(tree)
	}

	var renderErr error
	dom, renderErr = renderMarkdownExportHTML(tree, NewLute(), docx, func() error { return nil })
	if renderErr != nil {
		logging.LogErrorf("render exported Markdown HTML: %s", renderErr)
	}
	return
}

func ExportHTML(id, savePath string, pdf, image, keepFold, merge bool) (name, dom string, node *ast.Node) {
	savePath = strings.TrimSpace(savePath)

	bt := treenode.GetBlockTree(id)
	if nil == bt {
		return
	}

	tree := prepareExportTree(bt)
	node = treenode.GetNodeInTree(tree, id)

	if merge {
		var mergeErr error
		tree, mergeErr = mergeSubDocs(tree)
		if nil != mergeErr {
			logging.LogErrorf("merge sub docs failed: %s", mergeErr)
			return
		}
	}

	blockRefMode := Conf.Export.BlockRefMode
	if pdf {
		if err := addExportPDFOutline(tree, func() error { return nil }); err != nil {
			logging.LogErrorf("prepare PDF outline: %s", err)
			return
		}
	}

	tree = exportTree(tree, true, keepFold, true,
		blockRefMode, Conf.Export.BlockEmbedMode, Conf.Export.FileAnnotationRefMode,
		Conf.Export.TagOpenMarker, Conf.Export.TagCloseMarker,
		Conf.Export.BlockRefTextLeft, Conf.Export.BlockRefTextRight,
		Conf.Export.AddTitle, Conf.Export.InlineMemo, true, true, map[string]*parse.Tree{})
	adjustHeadingLevel(bt, tree)
	name = path.Base(tree.HPath)
	name = util.FilterFileName(name) // 导出 PDF、HTML 和 Word 时未移除不支持的文件名符号 https://github.com/siyuan-note/siyuan/issues/5614

	if "" != savePath {
		if err := os.MkdirAll(savePath, 0755); err != nil {
			logging.LogErrorf("mkdir [%s] failed: %s", savePath, err)
			return
		}

		assets := getAssetsLinkDests(tree.Root)
		for _, asset := range assets {
			if strings.Contains(asset, "?") {
				asset = asset[:strings.LastIndex(asset, "?")]
			}

			srcAbsPath, err := GetAssetAbsPath(asset)
			if err != nil {
				logging.LogWarnf("resolve path of asset [%s] failed: %s", asset, err)
				continue
			}
			targetAbsPath := filepath.Join(savePath, asset)
			if err = filelock.Copy(srcAbsPath, targetAbsPath); err != nil {
				logging.LogWarnf("copy asset from [%s] to [%s] failed: %s", srcAbsPath, targetAbsPath, err)
			}
		}
	}

	if !pdf && "" != savePath { // 导出 HTML 需要复制静态资源
		srcs := []string{"stage/build/export", "stage/protyle"}
		for _, src := range srcs {
			from := filepath.Join(util.WorkingDir, src)
			to := filepath.Join(savePath, src)
			if err := filelock.Copy(from, to); err != nil {
				logging.LogErrorf("copy stage from [%s] to [%s] failed: %s", from, savePath, err)
				return
			}
		}

		theme := Conf.Appearance.ThemeLight
		if 1 == Conf.Appearance.Mode {
			theme = Conf.Appearance.ThemeDark
		}
		// 复制主题文件夹
		srcs = []string{"themes/" + theme}
		appearancePath := util.AppearancePath
		if util.IsSymlinkPath(util.AppearancePath) {
			// Support for symlinked theme folder when exporting HTML https://github.com/siyuan-note/siyuan/issues/9173
			var readErr error
			appearancePath, readErr = filepath.EvalSymlinks(util.AppearancePath)
			if nil != readErr {
				logging.LogErrorf("readlink [%s] failed: %s", util.AppearancePath, readErr)
				return
			}
		}
		for _, src := range srcs {
			from := filepath.Join(appearancePath, src)
			to := filepath.Join(savePath, "appearance", src)
			if err := filelock.Copy(from, to); err != nil {
				logging.LogErrorf("copy appearance from [%s] to [%s] failed: %s", from, savePath, err)
			}
		}

		// 只复制图标文件夹中的 icon.js 文件
		iconName := Conf.Appearance.Icon
		// 如果使用的不是内建图标（ant 或 material），需要复制 material 作为后备
		if iconName != "ant" && iconName != "material" && iconName != "" {
			srcIconFile := filepath.Join(appearancePath, "icons", "material", "icon.js")
			toIconDir := filepath.Join(savePath, "appearance", "icons", "material")
			if err := os.MkdirAll(toIconDir, 0755); err != nil {
				logging.LogErrorf("mkdir [%s] failed: %s", toIconDir, err)
				return
			}
			toIconFile := filepath.Join(toIconDir, "icon.js")
			if err := filelock.Copy(srcIconFile, toIconFile); err != nil {
				logging.LogWarnf("copy icon file from [%s] to [%s] failed: %s", srcIconFile, toIconFile, err)
			}
		}
		// 复制当前使用的图标文件
		if iconName != "" {
			srcIconFile := filepath.Join(appearancePath, "icons", iconName, "icon.js")
			toIconDir := filepath.Join(savePath, "appearance", "icons", iconName)
			if err := os.MkdirAll(toIconDir, 0755); err != nil {
				logging.LogErrorf("mkdir [%s] failed: %s", toIconDir, err)
				return
			}
			toIconFile := filepath.Join(toIconDir, "icon.js")
			if err := filelock.Copy(srcIconFile, toIconFile); err != nil {
				logging.LogWarnf("copy icon file from [%s] to [%s] failed: %s", srcIconFile, toIconFile, err)
			}
		}

		// 复制自定义表情图片
		emojis := emojisInTree(tree)
		for _, emoji := range emojis {
			from := filepath.Join(util.DataDir, emoji)
			to := filepath.Join(savePath, emoji)
			if err := filelock.Copy(from, to); err != nil {
				logging.LogErrorf("copy emojis from [%s] to [%s] failed: %s", from, to, err)
			}
		}
	}

	if pdf {
		processIFrame(tree)
	}

	dom, _ = renderExportHTML(tree, NewLute(), func() error { return nil })
	return
}

func prepareExportTree(bt *treenode.BlockTree) (ret *parse.Tree) {
	luteEngine := NewLute()
	ret, _ = filesys.LoadTree(bt.BoxID, bt.Path, luteEngine)
	return selectExportTree(bt, ret, luteEngine)
}

// Shared original selection algorithm; callers own loading and identity policy.
func selectExportTree(bt *treenode.BlockTree, ret *parse.Tree, luteEngine *lute.Lute) *parse.Tree {
	if "d" != bt.Type {
		node := treenode.GetNodeInTree(ret, bt.ID)
		nodes := []*ast.Node{node}
		if "h" == bt.Type {
			children := treenode.HeadingChildren(node)
			for _, child := range children {
				nodes = append(nodes, child)
			}
		}

		oldRoot := ret.Root
		ret = parse.Parse("", []byte(""), luteEngine.ParseOptions)
		first := ret.Root.FirstChild
		for _, n := range nodes {
			first.InsertBefore(n)
		}
		ret.Root.KramdownIAL = oldRoot.KramdownIAL
	}
	ret.Path = bt.Path
	ret.HPath = bt.HPath
	ret.Box = bt.BoxID
	ret.ID = bt.RootID
	return ret
}

func processIFrame(tree *parse.Tree) {
	if err := processExportIFrames(tree, func() error { return nil }); err != nil {
		logging.LogErrorf("process exported iframe: %s", err)
	}
}

func ProcessPDF(id, p string, merge, removeAssets, watermark bool) error {
	return ProcessPDFContext(context.Background(), id, p, merge, removeAssets, watermark)
}

func processPDFWatermark(pdfCtx *model.Context, watermark bool) {
	if !watermark {
		return
	}
	snapshot, err := captureLegacyPDFWatermark()
	if err == nil {
		err = applyCapturedPDFWatermark(pdfCtx, snapshot, true, compatibilityPDFWatermarkDevice(), func() error { return nil })
	}
	if err != nil {
		logging.LogErrorf("PDF watermark failed: %s", err)
		util.PushErrMsg(err.Error(), 7000)
	}
}

func processPDFBookmarks(pdfCtx *model.Context, headings []*ast.Node) {
	selected := make([]pdfExportHeading, 0, len(headings))
	for _, h := range headings {
		selected = append(selected, pdfExportHeading{id: h.ID, level: h.HeadingLevel})
	}
	err := applyPDFBookmarks(pdfCtx, selected, func(id string) (string, bool, error) {
		b := sql.GetBlock(id)
		if b == nil {
			logging.LogWarnf("pdf outline block [%s] not found", id)
			return "", false, nil
		}
		return b.Content, true, nil
	}, func() error { return nil })
	if err != nil {
		logging.LogErrorf("add bookmark failed: %s", err)
	}
}

// processPDFLinkEmbedAssets 处理资源文件超链接，根据 removeAssets 参数决定是否将资源文件嵌入到 PDF 中。
// 导出 PDF 时支持将资源文件作为附件嵌入 https://github.com/siyuan-note/siyuan/issues/7414
func processPDFLinkEmbedAssets(pdfCtx *model.Context, assetDests []string, removeAssets bool) {
	origin := "http://" + util.LocalHost + ":" + util.ServerPort
	// Source destinations retain URL encoding in native AST. Normalize that
	// initial existence check once, just as hyperlink paths are decoded later.
	destinations := make([]string, 0, len(assetDests))
	for _, dest := range assetDests {
		if i := strings.IndexByte(dest, '#'); i >= 0 {
			dest = dest[:i]
		}
		if normalized, e := normalizeExportAsset(dest); e == nil {
			dest = normalized
		}
		destinations = append(destinations, dest)
	}
	err := applyPDFAssetLinks(pdfCtx, destinations, removeAssets, pdfAssetServices{
		guard: func() error { return nil },
		classify: func(uri string) bool {
			return strings.HasPrefix(uri, "http://127.0.0.1:") && strings.Contains(uri, "/assets/")
		},
		normalize: func(uri string) (pdfAssetURL, error) {
			uri = strings.ReplaceAll(uri, origin+"/export/temp/", "")
			uri = strings.ReplaceAll(uri, origin+"/", "")
			uri, _ = url.PathUnescape(uri)
			if i := strings.IndexByte(uri, '?'); i > 0 {
				uri = uri[:i]
			}
			return pdfAssetURL{uri, uri}, nil
		},
		resolve: func(asset string) (pdfAssetFile, bool, error) {
			p, e := GetAssetAbsPath(asset)
			if e != nil || p == "" {
				return pdfAssetFile{}, false, nil
			}
			return pdfAssetFile{p, filepath.Base(p)}, true, nil
		},
		embed: func(pdf *model.Context, f pdfAssetFile) (*types.IndirectRef, error) {
			info, err := os.Stat(f.path)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, errors.New("pdf_asset_not_regular")
			}
			return pdf.NewEmbeddedFileStreamDict(f.path)
		}, now: time.Now(), decodeOther: true, skipEmbedFailure: true,
	})
	if err != nil {
		logging.LogErrorf("process PDF assets: %s", err)
	}
}

func ExportStdMarkdown(id string, assetsDestSpace2Underscore, fillCSSVar, adjustHeadingLevel, imgTag bool) string {
	content, err := ExportStdMarkdownContext(context.Background(), id, MarkdownContentOptions{AssetsDestSpace2Underscore: assetsDestSpace2Underscore, FillCSSVar: fillCSSVar, AdjustHeadingLevel: adjustHeadingLevel, ImgTag: imgTag})
	if err != nil {
		logging.LogErrorf("export standard Markdown: %s", err)
		return ""
	}
	return content
}

func ExportPandocConvertZip(ids []string, pandocTo, ext string, contexts ...context.Context) (name, zipPath string) {
	if len(ids) == 0 {
		return
	}
	m, err := captureMarkdownExport(legacyAVContext(contexts))
	if err != nil {
		logging.LogErrorf("capture Markdown export: %s", err)
		return
	}
	selections, err := m.selectDocuments(ids)
	if err != nil {
		logging.LogErrorf("select Markdown documents: %s", err)
		return
	}
	defs, trees, paths, err := m.prepare(selections)
	if err != nil {
		logging.LogErrorf("prepare Markdown documents: %s", err)
		return
	}
	zipPath = exportPandocConvertZip(selections[0].folder, paths, defs, "gfm+footnotes+hard_line_breaks", pandocTo, ext, trees, m)
	name = selections[0].name
	return
}
func ExportNotebookMarkdown(boxID string, contexts ...context.Context) (zipPath string) {
	m, err := captureMarkdownExport(legacyAVContext(contexts))
	if err != nil {
		logging.LogErrorf("capture notebook Markdown export: %s", err)
		return
	}
	util.PushEndlessProgress(m.read.binding.scope.commit.r.language(65))
	defer util.ClearPushProgress(100)
	selected, err := m.read.notebook(boxID)
	if err != nil {
		logging.LogErrorf("select Markdown notebook: %s", err)
		return
	}
	defs, trees, paths, err := m.prepare([]archiveExportSelection{selected})
	if err != nil {
		logging.LogErrorf("prepare Markdown notebook: %s", err)
		return
	}
	return exportPandocConvertZip(selected.folder, paths, defs, "", "", ".md", trees, m)
}

func yfm(docIAL map[string]string) string {
	// 导出 Markdown 文件时开头附上一些元数据 https://github.com/siyuan-note/siyuan/issues/6880

	buf := bytes.Buffer{}
	buf.WriteString("---\n")
	var title, created, updated, tags string
	for k, v := range docIAL {
		if "id" == k {
			createdTime, parseErr := time.Parse("20060102150405", util.TimeFromID(v))
			if nil == parseErr {
				created = createdTime.Format(time.RFC3339)
			}
			continue
		}
		if "title" == k {
			title = v
			continue
		}
		if "updated" == k {
			updatedTime, parseErr := time.Parse("20060102150405", v)
			if nil == parseErr {
				updated = updatedTime.Format(time.RFC3339)
			}
			continue
		}
		if "tags" == k {
			tags = v
			continue
		}
	}
	if "" != title {
		buf.WriteString("title: ")
		buf.WriteString(title)
		buf.WriteString("\n")
	}
	if "" == updated {
		updated = time.Now().Format(time.RFC3339)
	}
	if "" == created {
		created = updated
	}
	buf.WriteString("date: ")
	buf.WriteString(created)
	buf.WriteString("\n")
	buf.WriteString("lastmod: ")
	buf.WriteString(updated)
	buf.WriteString("\n")
	if "" != tags {
		buf.WriteString("tags: [")
		buf.WriteString(tags)
		buf.WriteString("]\n")
	}
	buf.WriteString("---\n\n")
	return buf.String()
}

func exportBoxSYZip(boxID string, contexts ...context.Context) (zipPath string) {
	e, err := captureArchiveExport(legacyAVContext(contexts))
	if err != nil {
		logging.LogErrorf("capture notebook export: %s", err)
		return
	}
	defer util.ClearPushProgress(100)
	util.PushEndlessProgress(e.binding.scope.commit.r.language(65))
	selected, err := e.notebook(boxID)
	if err != nil {
		logging.LogErrorf("select notebook export: %s", err)
		return
	}
	return exportSYZipWithContext(e, selected.box, selected.rootDir, selected.folder, selected.paths)
}
func exportSYZip(boxID, rootDirPath, baseFolderName string, docPaths []string, contexts ...context.Context) string {
	e, err := captureArchiveExport(legacyAVContext(contexts))
	if err != nil {
		logging.LogErrorf("capture archive export: %s", err)
		return ""
	}
	return exportSYZipWithContext(e, boxID, rootDirPath, baseFolderName, docPaths)
}
func exportSYZipWithContext(e *archiveExportContext, boxID, rootDirPath, baseFolderName string, docPaths []string) (zipPath string) {
	if err := e.guard(); err != nil {
		return ""
	}
	if _, err := e.box(boxID); err != nil {
		return ""
	}
	archiveBinding := e.binding
	defer util.ClearPushProgress(100)

	output, err := e.newArchiveOutput(baseFolderName)
	if err != nil {
		logging.LogErrorf("create archive output: %s", err)
		return ""
	}
	defer func() {
		if err := output.finish(zipPath != ""); err != nil {
			logging.LogErrorf("finish archive output: %s", err)
			zipPath = ""
		}
	}()
	exportDir := output.folder

	trees, err := e.selectedTrees(boxID, docPaths)
	if err != nil {
		logging.LogErrorf("read selected export trees: %s", err)
		return ""
	}
	refTrees := map[string]*parse.Tree{}

	count := 1
	treeCache := map[string]*parse.Tree{}
	for _, tree := range trees {
		if err := e.guard(); err != nil {
			return ""
		}
		util.PushEndlessProgress(e.binding.scope.commit.r.language(65) + " " + fmt.Sprintf(e.binding.scope.commit.r.language(70), fmt.Sprintf("%d/%d %s", count, len(docPaths), tree.Root.IALAttr("title"))))

		refs := map[string]*parse.Tree{}
		if err := e.references(tree, &[]string{}, refs, treeCache); err != nil {
			logging.LogErrorf("read archive references: %s", err)
			return ""
		}
		for refTreeID, refTree := range refs {
			if nil == trees[refTreeID] {
				refTrees[refTreeID] = refTree
			}
		}
		count++
	}

	util.PushEndlessProgress(e.binding.scope.commit.r.language(65))
	count = 0

	// 按文件夹结构复制选择的树
	total := len(trees) + len(refTrees)
	for _, tree := range trees {
		if err := e.guard(); err != nil {
			return ""
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(tree.Path, rootDirPath), "/")
		if err := e.writeDocument(exportDir, relative, tree); err != nil {
			logging.LogErrorf("write selected export document: %s", err)
			return ""
		}
		count++

		util.PushEndlessProgress(e.binding.scope.commit.r.language(65) + " " + fmt.Sprintf(e.binding.scope.commit.r.language(66), fmt.Sprintf("%d/%d ", count, total)+tree.HPath))
	}

	count = 0
	// 引用树放在导出文件夹根路径下
	for treeID, tree := range refTrees {
		if err := e.guard(); err != nil {
			return ""
		}
		if err := e.writeDocument(exportDir, treeID+util.DocExt, tree); err != nil {
			logging.LogErrorf("write referenced export document: %s", err)
			return ""
		}
		count++

		util.PushEndlessProgress(e.binding.scope.commit.r.language(65) + " " + fmt.Sprintf(e.binding.scope.commit.r.language(66), fmt.Sprintf("%d/%d ", count, total)+tree.HPath))
	}

	// 将引用树合并到选择树中，以便后面一次性导出资源文件
	for treeID, tree := range refTrees {
		if err := e.guard(); err != nil {
			return ""
		}
		trees[treeID] = tree
	}

	if err := e.exportResources(exportDir, trees); err != nil {
		logging.LogErrorf("export archive resources: %s", err)
		return ""
	}

	// Export only cards from the selected trees, excluding reference-only trees.
	flashcardTrees := make([]*parse.Tree, 0, len(trees))
	for _, tree := range trees {
		if err := e.guard(); err != nil {
			return ""
		}
		flashcardTrees = append(flashcardTrees, tree)
	}
	if _, archiveErr := runLegacyFlashcardArchive(archiveBinding, nil, nil, flashcardTrees, exportDir, true, func() (*os.Root, error) { return e.openExportFolder(exportDir) }); archiveErr != nil {
		logging.LogErrorf("export flashcard archive: %s", archiveErr)
		return ""
	}

	if err = e.writeSort(exportDir, boxID, trees); err != nil {
		logging.LogErrorf("export sort: %s", err)
		return ""
	}

	if err = output.pack(); err != nil {
		logging.LogErrorf("pack archive output: %s", err)
		return ""
	}
	return "/export/" + url.PathEscape(output.filename)

}

func ExportMarkdownContent(id string, refMode, embedMode int, addYfm, fillCSSVar, adjustHeadingLv, imgTag bool) (hPath, exportedMd string) {
	hPath, exportedMd, err := ExportMarkdownContentContext(context.Background(), id, MarkdownContentOptions{RefMode: &refMode, EmbedMode: &embedMode, YFM: addYfm, FillCSSVar: fillCSSVar, AdjustHeadingLevel: adjustHeadingLv, ImgTag: imgTag})
	if err != nil {
		logging.LogErrorf("export Markdown content: %s", err)
		return "", ""
	}
	return hPath, exportedMd
}

func exportMarkdownContent(id, ext string, exportRefMode int, defBlockIDs []string, singleFile bool, treeCache map[string]*parse.Tree, m *markdownExportContext) (tree *parse.Tree, exportedMd string, isEmpty bool, err error) {
	if err = m.guard(); err != nil {
		return
	}
	tree = treeCache[id]
	if tree == nil {
		err = ErrBlockNotFound
		return
	}
	refCount, queryErr := m.rootChildrenRefCounts(tree.ID)
	if queryErr != nil {
		err = queryErr
		return
	}
	if !m.settings.MarkdownYFM && treenode.ContainOnlyDefaultIAL(tree) && 1 > len(refCount) {
		for c := tree.Root.FirstChild; nil != c; c = c.Next {
			if ast.NodeParagraph == c.Type {
				isEmpty = nil == c.FirstChild
				if !isEmpty {
					break
				}
			} else {
				isEmpty = false
				break
			}
		}
	}

	refs := m.referenceRenderer()
	exportedMd = exportMarkdownContent0(id, tree, "", false, false, false,
		ext, exportRefMode, m.settings.BlockEmbedMode, m.settings.FileAnnotationRefMode,
		m.settings.TagOpenMarker, m.settings.TagCloseMarker,
		m.settings.BlockRefTextLeft, m.settings.BlockRefTextRight,
		m.settings.AddTitle, m.settings.InlineMemo, defBlockIDs, singleFile, false, treeCache, refs)
	if err = refs.check(); err != nil {
		return tree, "", false, err
	}
	docIAL := parse.IAL2Map(tree.Root.KramdownIAL)
	if m.settings.MarkdownYFM {
		// 导出 Markdown 时在文档头添加 YFM 开关 https://github.com/siyuan-note/siyuan/issues/7727
		exportedMd = yfm(docIAL) + exportedMd
	}
	err = m.guard()
	return
}

func exportMarkdownContent0(id string, tree *parse.Tree, cloudAssetsBase string, assetsDestSpace2Underscore, adjustHeadingLv, imgTag bool,
	ext string, blockRefMode, blockEmbedMode, fileAnnotationRefMode int,
	tagOpenMarker, tagCloseMarker string, blockRefTextLeft, blockRefTextRight string,
	addTitle, inlineMemo bool, defBlockIDs []string, singleFile, fillCSSVar bool, treeCache map[string]*parse.Tree, ports ...*exportReferenceRenderer) (ret string) {
	tree = exportTree(tree, false, false, false,
		blockRefMode, blockEmbedMode, fileAnnotationRefMode,
		tagOpenMarker, tagCloseMarker,
		blockRefTextLeft, blockRefTextRight,
		addTitle, inlineMemo, 0 < len(defBlockIDs), singleFile, treeCache, ports...)
	if adjustHeadingLv {
		bt := exportRenderLookup(id, ports)
		if !exportRenderReady(ports) {
			return ""
		}
		if bt == nil && len(ports) > 0 {
			exportRenderFailure(ports, ErrBlockNotFound)
			return ""
		}
		adjustExportHeading(bt, tree, addTitle, ports)
	}

	if !exportRenderReady(ports) {
		return ""
	}
	luteEngine := exportRenderParser(ports)
	luteEngine.SetFootnotes(true)
	luteEngine.SetKramdownIAL(false)
	if "" != cloudAssetsBase {
		luteEngine.RenderOptions.LinkBase = cloudAssetsBase
	}
	if assetsDestSpace2Underscore { // 上传到社区图床的资源文件会将空格转为下划线，所以这里也需要将文档内容做相应的转换
		ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering {
				return ast.WalkContinue
			}

			if ast.NodeLinkDest == n.Type {
				if util.IsAssetLinkDest(n.Tokens) {
					n.Tokens = bytes.ReplaceAll(n.Tokens, []byte(" "), []byte("_"))
				}
			} else if n.IsTextMarkType("a") {
				href := n.TextMarkAHref
				if util.IsAssetLinkDest([]byte(href)) {
					n.TextMarkAHref = strings.ReplaceAll(href, " ", "_")
				}
			} else if ast.NodeIFrame == n.Type || ast.NodeAudio == n.Type || ast.NodeVideo == n.Type {
				dest := treenode.GetNodeSrcTokens(n)
				if util.IsAssetLinkDest([]byte(dest)) {
					setAssetsLinkDest(n, dest, strings.ReplaceAll(dest, " ", "_"))
				}
			}
			return ast.WalkContinue
		})
	}

	currentDocDir := path.Dir(tree.HPath)
	currentDocDir = util.FilterFilePath(currentDocDir)

	var unlinks []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeBr == n.Type {
			if !n.ParentIs(ast.NodeTableCell) {
				// When exporting Markdown, `<br />` nodes in non-tables are replaced with `\n` text nodes https://github.com/siyuan-note/siyuan/issues/9509
				n.InsertBefore(&ast.Node{Type: ast.NodeText, Tokens: []byte("\n")})
				unlinks = append(unlinks, n)
			}
		}

		if 4 == blockRefMode { // 脚注+锚点哈希
			if n.IsBlock() && gulu.Str.Contains(n.ID, defBlockIDs) {
				// 如果是定义块，则在开头处添加锚点
				anchorSpan := treenode.NewSpanAnchor(n.ID)
				if ast.NodeDocument != n.Type {
					firstLeaf := treenode.FirstLeafBlock(n)
					if nil != firstLeaf {
						if ast.NodeTable == firstLeaf.Type {
							firstLeaf.InsertBefore(anchorSpan)
							firstLeaf.InsertBefore(&ast.Node{Type: ast.NodeHardBreak})
						} else {
							if nil != firstLeaf.FirstChild {
								firstLeaf.FirstChild.InsertBefore(anchorSpan)
							} else {
								firstLeaf.AppendChild(anchorSpan)
							}
						}
					} else {
						n.AppendChild(anchorSpan)
					}
				}
			}

			if treenode.IsBlockRef(n) {
				// 如果是引用元素，则将其转换为超链接，指向 xxx.md#block-id
				defID, linkText := getExportBlockRefLinkText(n, blockRefTextLeft, blockRefTextRight, ports...)
				if gulu.Str.Contains(defID, defBlockIDs) {
					var href string
					bt := exportRenderLookup(defID, ports)
					if nil != bt {
						href += bt.HPath + ext
						if "d" != bt.Type {
							href += "#" + defID
						}
						if tree.ID == bt.RootID {
							href = "#" + defID
						}
					}

					sameDir := path.Dir(href) == currentDocDir
					if strings.HasPrefix(href, "#") {
						sameDir = true
					}
					href = util.FilterFilePath(href)
					if !sameDir {
						var relErr error
						href, relErr = filepath.Rel(currentDocDir, href)
						if nil != relErr {
							logging.LogWarnf("get relative path from [%s] to [%s] failed: %s", currentDocDir, href, relErr)
						}
						href = filepath.ToSlash(href)
					} else {
						href = strings.TrimPrefix(href, currentDocDir+"/")
					}
					blockRefLink := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "a", TextMarkTextContent: linkText, TextMarkAHref: href}
					blockRefLink.KramdownIAL = n.KramdownIAL
					n.InsertBefore(blockRefLink)
					unlinks = append(unlinks, n)
				}
			}
		}
		return ast.WalkContinue
	})
	for _, unlink := range unlinks {
		unlink.Unlink()
	}

	if fillCSSVar {
		if len(ports) == 0 {
			fillThemeStyleVar(tree)
		} else if exportRenderReady(ports) {
			if ports[0].theme == nil {
				exportRenderFailure(ports, errors.New("export_theme_port_unbound"))
			} else {
				exportRenderFailure(ports, ports[0].theme(tree))
			}
		}
		if !exportRenderReady(ports) {
			return ""
		}
	}

	luteEngine.SetUnorderedListMarker("-")
	luteEngine.SetImgTag(imgTag)
	renderer := render.NewProtyleExportMdRenderer(tree, luteEngine.RenderOptions)
	ret = gulu.Str.FromBytes(renderer.Render())
	return
}

func exportTree(tree *parse.Tree, wysiwyg, keepFold, avHiddenCol bool,
	blockRefMode, blockEmbedMode, fileAnnotationRefMode int,
	tagOpenMarker, tagCloseMarker string,
	blockRefTextLeft, blockRefTextRight string,
	addTitle, inlineMemo, addDocAnchorSpan, singleFile bool, treeCache map[string]*parse.Tree, ports ...*exportReferenceRenderer) (ret *parse.Tree) {
	if !exportRenderReady(ports) {
		return tree
	}
	luteEngine := exportRenderParser(ports)
	ret = tree
	id := tree.Root.ID
	treeCache[tree.ID] = tree

	// 解析查询嵌入节点
	depth := 0
	resolveEmbedR(ret.Root, blockEmbedMode, luteEngine, &[]string{}, &depth, ports...)
	if !exportRenderReady(ports) {
		return ret
	}

	// 将块超链接转换为引用
	depth = 0
	blockLink2Ref(ret, ret.ID, treeCache, &depth, ports...)
	if !exportRenderReady(ports) {
		return ret
	}

	// 收集引用转脚注+锚点哈希
	var refFootnotes []*refAsFootnotes
	if 4 == blockRefMode && singleFile {
		depth = 0
		collectFootnotesDefs(ret, ret.ID, &refFootnotes, treeCache, &depth, ports...)
	}

	currentTreeNodeIDs := map[string]bool{}
	ast.Walk(ret.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if "" != n.ID {
			currentTreeNodeIDs[n.ID] = true
		}
		return ast.WalkContinue
	})

	var unlinks []*ast.Node
	ast.Walk(ret.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		switch n.Type {
		case ast.NodeSuperBlockOpenMarker, ast.NodeSuperBlockLayoutMarker, ast.NodeSuperBlockCloseMarker:
			if !wysiwyg {
				unlinks = append(unlinks, n)
				return ast.WalkContinue
			}
		case ast.NodeHeading:
			n.SetIALAttr("id", n.ID)
		case ast.NodeMathBlockContent:
			n.Tokens = bytes.TrimSpace(n.Tokens) // 导出 Markdown 时去除公式内容中的首尾空格 https://github.com/siyuan-note/siyuan/issues/4666
			return ast.WalkContinue
		case ast.NodeTextMark:
			if n.IsTextMarkType("inline-memo") {
				if !inlineMemo {
					n.TextMarkInlineMemoContent = ""
				}
			}

			if n.IsTextMarkType("inline-math") {
				n.TextMarkInlineMathContent = strings.TrimSpace(n.TextMarkInlineMathContent)
				return ast.WalkContinue
			} else if treenode.IsFileAnnotationRef(n) {
				refID := n.TextMarkFileAnnotationRefID
				if !strings.Contains(refID, "/") {
					if len(ports) > 0 {
						exportRenderFailure(ports, errors.New("export_annotation_id_invalid"))
						return ast.WalkStop
					}
					return ast.WalkSkipChildren
				}

				status := processFileAnnotationRef(refID, n, fileAnnotationRefMode, ports...)
				unlinks = append(unlinks, n)
				return status
			} else if n.IsTextMarkType("tag") {
				if !wysiwyg {
					n.Type = ast.NodeText
					n.Tokens = []byte(tagOpenMarker + n.TextMarkTextContent + tagCloseMarker)
					return ast.WalkContinue
				}
			}
		}

		if !treenode.IsBlockRef(n) {
			return ast.WalkContinue
		}

		// 处理引用节点
		defID, linkText := getExportBlockRefLinkText(n, blockRefTextLeft, blockRefTextRight, ports...)

		switch blockRefMode {
		case 2: // 锚文本块链
			blockRefLink := &ast.Node{Type: ast.NodeTextMark, TextMarkTextContent: linkText, TextMarkAHref: "siyuan://blocks/" + defID}
			blockRefLink.KramdownIAL = n.KramdownIAL
			blockRefLink.TextMarkType = "a " + n.TextMarkType
			blockRefLink.TextMarkInlineMemoContent = n.TextMarkInlineMemoContent
			n.InsertBefore(blockRefLink)
			unlinks = append(unlinks, n)
		case 3: // 仅锚文本
			blockRefLink := &ast.Node{Type: ast.NodeTextMark, TextMarkType: strings.TrimSpace(strings.ReplaceAll(n.TextMarkType, "block-ref", "")), TextMarkTextContent: linkText}
			blockRefLink.KramdownIAL = n.KramdownIAL
			blockRefLink.TextMarkInlineMemoContent = n.TextMarkInlineMemoContent
			n.InsertBefore(blockRefLink)
			unlinks = append(unlinks, n)
		case 4: // 脚注+锚点哈希
			if currentTreeNodeIDs[defID] {
				// 当前文档内不转换脚注，直接使用锚点哈希 https://github.com/siyuan-note/siyuan/issues/13283
				n.TextMarkType = "a " + n.TextMarkType
				n.TextMarkTextContent = linkText
				n.TextMarkAHref = "#" + defID
				return ast.WalkContinue
			}

			refFoot := getRefAsFootnotes(defID, &refFootnotes)
			if nil == refFoot {
				return ast.WalkContinue
			}

			text := &ast.Node{Type: ast.NodeText, Tokens: []byte(linkText)}
			if "block-ref" != n.TextMarkType {
				text.Type = ast.NodeTextMark
				text.TextMarkType = strings.TrimSpace(strings.ReplaceAll(n.TextMarkType, "block-ref", ""))
				text.TextMarkTextContent = linkText
				text.TextMarkInlineMemoContent = n.TextMarkInlineMemoContent
			}
			n.InsertBefore(text)
			n.InsertBefore(&ast.Node{Type: ast.NodeFootnotesRef, Tokens: []byte("^" + refFoot.refNum), FootnotesRefId: refFoot.refNum, FootnotesRefLabel: []byte("^" + refFoot.refNum)})
			unlinks = append(unlinks, n)
		}
		return ast.WalkSkipChildren
	})
	for _, n := range unlinks {
		n.Unlink()
	}

	if 4 == blockRefMode { // 脚注+锚点哈希
		unlinks = nil
		footnotesDefBlock := resolveFootnotesDefs(&refFootnotes, ret, currentTreeNodeIDs, blockRefTextLeft, blockRefTextRight, treeCache, ports...)
		if nil != footnotesDefBlock {
			// 如果是聚焦导出，可能存在没有使用的脚注定义块，在这里进行清理
			// Improve focus export conversion of block refs to footnotes https://github.com/siyuan-note/siyuan/issues/10647
			footnotesRefs := ret.Root.ChildrenByType(ast.NodeFootnotesRef)
			for footnotesDef := footnotesDefBlock.FirstChild; nil != footnotesDef; footnotesDef = footnotesDef.Next {
				fnRefsInDef := footnotesDef.ChildrenByType(ast.NodeFootnotesRef)
				footnotesRefs = append(footnotesRefs, fnRefsInDef...)
			}

			for footnotesDef := footnotesDefBlock.FirstChild; nil != footnotesDef; footnotesDef = footnotesDef.Next {
				exist := false
				for _, ref := range footnotesRefs {
					if ref.FootnotesRefId == footnotesDef.FootnotesRefId {
						exist = true
						break
					}
				}
				if !exist {
					unlinks = append(unlinks, footnotesDef)
				}
			}

			for _, n := range unlinks {
				n.Unlink()
			}

			ret.Root.AppendChild(footnotesDefBlock)
		}
	}

	if !exportRenderReady(ports) {
		return ret
	}

	if addTitle {
		if root := exportRenderDocumentTitle(id, tree, ports); root != nil {
			root.IAL["type"] = "doc"
			title := &ast.Node{Type: ast.NodeHeading, HeadingLevel: 1}
			for k, v := range root.IAL {
				if "type" == k {
					continue
				}
				title.SetIALAttr(k, v)
			}
			title.InsertAfter(&ast.Node{Type: ast.NodeKramdownBlockIAL, Tokens: parse.IAL2Tokens(title.KramdownIAL)})
			content := html.UnescapeString(root.Content)
			title.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(content)})
			ret.Root.PrependChild(title)
		}
	} else {
		if 4 == blockRefMode { // 脚注+锚点哈希
			refRoot := false

			for _, refFoot := range refFootnotes {
				if id == refFoot.defID {
					refRoot = true
					break
				}
			}

			footnotesDefs := tree.Root.ChildrenByType(ast.NodeFootnotesDef)
			for _, footnotesDef := range footnotesDefs {
				ast.Walk(footnotesDef, func(n *ast.Node, entering bool) ast.WalkStatus {
					if !exportRenderReady(ports) {
						return ast.WalkStop
					}
					if !entering {
						return ast.WalkContinue
					}

					if id == n.TextMarkBlockRefID {
						refRoot = true
						return ast.WalkStop
					}
					return ast.WalkContinue
				})
			}

			if refRoot && addDocAnchorSpan {
				anchorSpan := treenode.NewSpanAnchor(id)
				ret.Root.PrependChild(anchorSpan)
			}
		}
	}

	if !exportRenderReady(ports) {
		return ret
	}

	// 导出时支持导出题头图 https://github.com/siyuan-note/siyuan/issues/4372
	titleImgPath := treenode.GetDocTitleImgPath(ret.Root)
	if "" != titleImgPath {
		p := &ast.Node{Type: ast.NodeParagraph}
		titleImg := &ast.Node{Type: ast.NodeImage}
		titleImg.AppendChild(&ast.Node{Type: ast.NodeBang})
		titleImg.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
		titleImg.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte("image")})
		titleImg.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
		titleImg.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
		titleImg.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(titleImgPath)})
		titleImg.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
		p.AppendChild(titleImg)
		ret.Root.PrependChild(p)
	}

	unlinks = nil
	var emptyParagraphs []*ast.Node
	ast.Walk(ret.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		// 支持按照现有折叠状态导出 PDF https://github.com/siyuan-note/siyuan/issues/5941
		if !keepFold {
			// 块折叠以后导出 HTML/PDF 固定展开 https://github.com/siyuan-note/siyuan/issues/4064
			n.RemoveIALAttr("fold")
			n.RemoveIALAttr("heading-fold")
		} else {
			if "1" == n.IALAttr("heading-fold") {
				unlinks = append(unlinks, n)
				return ast.WalkContinue
			}
		}

		// 导出时去掉内容块闪卡样式 https://github.com/siyuan-note/siyuan/issues/7374
		if n.IsBlock() {
			n.RemoveIALAttr("custom-riff-decks")
		}

		switch n.Type {
		case ast.NodeParagraph:
			if nil == n.FirstChild {
				// 空的段落块需要补全文本展位，否则后续格式化后再解析树会语义不一致 https://github.com/siyuan-note/siyuan/issues/5806
				emptyParagraphs = append(emptyParagraphs, n)
			}
		case ast.NodeWidget:
			// 挂件块导出 https://github.com/siyuan-note/siyuan/issues/3834 https://github.com/siyuan-note/siyuan/issues/6188

			if wysiwyg {
				exportHtmlVal := n.IALAttr("data-export-html")
				if "" != exportHtmlVal {
					htmlBlock := &ast.Node{Type: ast.NodeHTMLBlock, Tokens: []byte(exportHtmlVal)}
					n.InsertBefore(htmlBlock)
					unlinks = append(unlinks, n)
					return ast.WalkContinue
				}
			}

			exportMdVal := n.IALAttr("data-export-md")
			exportMdVal = html.UnescapeString(exportMdVal) // 导出 `data-export-md` 时未解析代码块与行内代码内的转义字符 https://github.com/siyuan-note/siyuan/issues/4180
			if "" != exportMdVal {
				luteEngine0 := exportRenderMarkdownParser(ports)
				luteEngine0.SetYamlFrontMatter(true) // 挂件导出属性 `data-export-md` 支持 YFM https://github.com/siyuan-note/siyuan/issues/7752
				exportMdTree := parse.Parse("", []byte(exportMdVal), luteEngine0.ParseOptions)
				var insertNodes []*ast.Node
				for c := exportMdTree.Root.FirstChild; nil != c; c = c.Next {
					if ast.NodeKramdownBlockIAL != c.Type {
						insertNodes = append(insertNodes, c)
					}
				}
				for _, insertNode := range insertNodes {
					n.InsertBefore(insertNode)
				}
				unlinks = append(unlinks, n)
			}
		case ast.NodeSuperBlockOpenMarker, ast.NodeSuperBlockLayoutMarker, ast.NodeSuperBlockCloseMarker:
			if !wysiwyg {
				unlinks = append(unlinks, n)
			}
		}

		if ast.NodeText != n.Type {
			return ast.WalkContinue
		}

		// Shift+Enter 换行在导出为 Markdown 时使用硬换行 https://github.com/siyuan-note/siyuan/issues/3458
		n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("\n"), []byte("  \n"))
		return ast.WalkContinue
	})
	for _, n := range unlinks {
		n.Unlink()
	}
	for _, emptyParagraph := range emptyParagraphs {
		emptyParagraph.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(editor.Zwj)})
	}

	unlinks = nil
	// Attribute View export https://github.com/siyuan-note/siyuan/issues/8710
	ast.Walk(ret.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeAttributeView != n.Type {
			return ast.WalkContinue
		}

		attrView, table := exportRenderAttributeTable(n.AttributeViewID, n.IALAttr(av.NodeAttrView), ports)
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if table == nil {
			return ast.WalkContinue
		}

		var aligns []int
		for range table.Columns {
			aligns = append(aligns, 0)
		}
		mdTable := &ast.Node{Type: ast.NodeTable, TableAligns: aligns}
		mdTableHead := &ast.Node{Type: ast.NodeTableHead}
		mdTable.AppendChild(mdTableHead)
		mdTableHeadRow := &ast.Node{Type: ast.NodeTableRow, TableAligns: aligns}
		mdTableHead.AppendChild(mdTableHeadRow)
		for _, col := range table.Columns {
			if avHiddenCol && col.Hidden {
				// 按需跳过隐藏列 Improve database table view exporting https://github.com/siyuan-note/siyuan/issues/12232
				continue
			}

			cell := &ast.Node{Type: ast.NodeTableCell}
			name := col.Name
			if !wysiwyg {
				name = string(lex.EscapeProtyleMarkers([]byte(col.Name)))
				name = strings.ReplaceAll(name, "\\|", "|")
				name = strings.ReplaceAll(name, "|", "\\|")
			}
			cell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(name)})
			mdTableHeadRow.AppendChild(cell)
		}

		rowNum := 1
		for _, row := range table.Rows {
			mdTableRow := &ast.Node{Type: ast.NodeTableRow, TableAligns: aligns}
			mdTable.AppendChild(mdTableRow)
			for _, cell := range row.Cells {
				if avHiddenCol && nil != cell.Value {
					if col := table.GetColumn(cell.Value.KeyID); nil != col && col.Hidden {
						continue
					}
				}

				mdTableCell := &ast.Node{Type: ast.NodeTableCell}
				mdTableRow.AppendChild(mdTableCell)
				var val string
				if nil != cell.Value {
					if av.KeyTypeBlock == cell.Value.Type {
						if nil != cell.Value.Block {
							val = cell.Value.Block.Content
							if !wysiwyg {
								val = string(lex.EscapeProtyleMarkers([]byte(val)))
								val = strings.ReplaceAll(val, "\\|", "|")
								val = strings.ReplaceAll(val, "|", "\\|")
							}
							col := table.GetColumn(cell.Value.KeyID)
							if nil != col && col.Wrap {
								lines := strings.Split(val, "\n")
								for _, line := range lines {
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(line)})
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeHardBreak})
								}
							} else {
								val = strings.ReplaceAll(val, "\n", " ")
								mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
							}
							continue
						}
					} else if av.KeyTypeText == cell.Value.Type {
						if nil != cell.Value.Text {
							val = cell.Value.Text.Content
							if !wysiwyg {
								val = string(lex.EscapeProtyleMarkers([]byte(val)))
								val = strings.ReplaceAll(val, "\\|", "|")
								val = strings.ReplaceAll(val, "|", "\\|")
							}
							col := table.GetColumn(cell.Value.KeyID)
							if nil != col && col.Wrap {
								lines := strings.Split(val, "\n")
								for _, line := range lines {
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(line)})
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeHardBreak})
								}
							} else {
								val = strings.ReplaceAll(val, "\n", " ")
								mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
							}
							continue
						}
					} else if av.KeyTypeTemplate == cell.Value.Type {
						if nil != cell.Value.Template {
							val = cell.Value.Template.Content
							val = strings.ReplaceAll(val, "\\|", "|")
							val = strings.ReplaceAll(val, "|", "\\|")
							col := table.GetColumn(cell.Value.KeyID)
							if nil != col && col.Wrap {
								lines := strings.Split(val, "\n")
								for _, line := range lines {
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(line)})
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeHardBreak})
								}
							} else {
								val = strings.ReplaceAll(val, "\n", " ")
								mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
							}
							continue
						}
					} else if av.KeyTypeDate == cell.Value.Type {
						if nil != cell.Value.Date {
							cell.Value.Date = av.NewFormattedValueDate(cell.Value.Date.Content, cell.Value.Date.Content2, av.DateFormatNone, cell.Value.Date.IsNotTime, cell.Value.Date.HasEndDate)
						}
					} else if av.KeyTypeCreated == cell.Value.Type {
						if nil != cell.Value.Created {
							key, _ := attrView.GetKey(cell.Value.KeyID)
							isNotTime := false
							if nil != key && nil != key.Created {
								isNotTime = !key.Created.IncludeTime
							}

							cell.Value.Created = av.NewFormattedValueCreated(cell.Value.Created.Content, 0, av.CreatedFormatNone, isNotTime)
						}
					} else if av.KeyTypeUpdated == cell.Value.Type {
						if nil != cell.Value.Updated {
							key, _ := attrView.GetKey(cell.Value.KeyID)
							isNotTime := false
							if nil != key && nil != key.Updated {
								isNotTime = !key.Updated.IncludeTime
							}

							cell.Value.Updated = av.NewFormattedValueUpdated(cell.Value.Updated.Content, 0, av.UpdatedFormatNone, isNotTime)
						}
					} else if av.KeyTypeURL == cell.Value.Type {
						if nil != cell.Value.URL {
							if "" != strings.TrimSpace(cell.Value.URL.Content) {
								link := &ast.Node{Type: ast.NodeLink}
								link.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
								link.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(cell.Value.URL.Content)})
								link.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
								link.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
								link.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(cell.Value.URL.Content)})
								link.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
								mdTableCell.AppendChild(link)
							}
							continue
						}
					} else if av.KeyTypeMAsset == cell.Value.Type {
						if nil != cell.Value.MAsset {
							for i, a := range cell.Value.MAsset {
								if av.AssetTypeImage == a.Type {
									img := &ast.Node{Type: ast.NodeImage}
									img.AppendChild(&ast.Node{Type: ast.NodeBang})
									img.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
									img.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(a.Name)})
									img.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
									img.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
									img.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(a.Content)})
									img.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
									mdTableCell.AppendChild(img)
								} else if av.AssetTypeFile == a.Type {
									linkText := strings.TrimSpace(a.Name)
									if "" == linkText {
										linkText = a.Content
									}

									if "" != strings.TrimSpace(a.Content) {
										file := &ast.Node{Type: ast.NodeLink}
										file.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
										file.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(linkText)})
										file.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
										file.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
										file.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(a.Content)})
										file.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
										mdTableCell.AppendChild(file)
									} else {
										mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(linkText)})
									}
								}
								if i < len(cell.Value.MAsset)-1 {
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(" ")})
								}
							}
							continue
						}
					} else if av.KeyTypeLineNumber == cell.Value.Type {
						val = strconv.Itoa(rowNum)
						rowNum++
					} else if av.KeyTypeRelation == cell.Value.Type {
						for i, v := range cell.Value.Relation.Contents {
							if nil == v {
								continue
							}

							if av.KeyTypeBlock == v.Type && nil != v.Block {
								val = v.Block.Content
								if !wysiwyg {
									val = string(lex.EscapeProtyleMarkers([]byte(val)))
									val = strings.ReplaceAll(val, "\\|", "|")
									val = strings.ReplaceAll(val, "|", "\\|")
								}

								col := table.GetColumn(cell.Value.KeyID)
								if nil != col && col.Wrap {
									lines := strings.Split(val, "\n")
									for _, line := range lines {
										mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(line)})
										mdTableCell.AppendChild(&ast.Node{Type: ast.NodeHardBreak})
									}
								} else {
									val = strings.ReplaceAll(val, "\n", " ")
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
								}
							}
							if i < len(cell.Value.Relation.Contents)-1 {
								mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(", ")})
							}
						}
						continue
					} else if av.KeyTypeRollup == cell.Value.Type {
						for i, v := range cell.Value.Rollup.Contents {
							if nil == v {
								continue
							}

							if av.KeyTypeBlock == v.Type {
								if nil != v.Block {
									val = v.Block.Content
									if !wysiwyg {
										val = string(lex.EscapeProtyleMarkers([]byte(val)))
										val = strings.ReplaceAll(val, "\\|", "|")
										val = strings.ReplaceAll(val, "|", "\\|")
									}

									col := table.GetColumn(cell.Value.KeyID)
									if nil != col && col.Wrap {
										lines := strings.Split(val, "\n")
										for _, line := range lines {
											mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(line)})
											mdTableCell.AppendChild(&ast.Node{Type: ast.NodeHardBreak})
										}
									} else {
										val = strings.ReplaceAll(val, "\n", " ")
										mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
									}
								}
							} else if av.KeyTypeText == v.Type {
								val = v.Text.Content
								if !wysiwyg {
									val = string(lex.EscapeProtyleMarkers([]byte(val)))
									val = strings.ReplaceAll(val, "\\|", "|")
									val = strings.ReplaceAll(val, "|", "\\|")
								}

								col := table.GetColumn(cell.Value.KeyID)
								if nil != col && col.Wrap {
									lines := strings.Split(val, "\n")
									for _, line := range lines {
										mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(line)})
										mdTableCell.AppendChild(&ast.Node{Type: ast.NodeHardBreak})
									}
								} else {
									val = strings.ReplaceAll(val, "\n", " ")
									mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
								}
							} else {
								mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(v.String(true))})
							}

							if i < len(cell.Value.Rollup.Contents)-1 {
								mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(", ")})
							}
						}
						continue
					}

					if "" == val {
						val = cell.Value.String(true)
					}
				}
				mdTableCell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(val)})
			}
		}

		n.InsertBefore(mdTable)
		unlinks = append(unlinks, n)
		return ast.WalkContinue
	})
	for _, n := range unlinks {
		n.Unlink()
	}
	return ret
}

func resolveFootnotesDefs(refFootnotes *[]*refAsFootnotes, currentTree *parse.Tree, currentTreeNodeIDs map[string]bool, blockRefTextLeft, blockRefTextRight string, treeCache map[string]*parse.Tree, ports ...*exportReferenceRenderer) (footnotesDefBlock *ast.Node) {
	if !exportRenderReady(ports) {
		return nil
	}
	if 1 > len(*refFootnotes) {
		return nil
	}

	footnotesDefBlock = &ast.Node{Type: ast.NodeFootnotesDefBlock}
	var rendered []string
	for _, foot := range *refFootnotes {
		if !exportRenderReady(ports) {
			return nil
		}
		t, err := exportFootnoteLoad(foot.defID, treeCache, ports)
		if nil != err {
			if len(ports) > 0 {
				return nil
			}
			return
		}

		defNode := treenode.GetNodeInTree(t, foot.defID)
		if defNode == nil && len(ports) > 0 {
			exportRenderFailure(ports, errors.New("export_footnote_node_missing"))
			return nil
		}
		docID := util.GetTreeID(defNode.Path)
		var nodes []*ast.Node
		if ast.NodeHeading == defNode.Type {
			nodes = append(nodes, defNode)
			if currentTree.ID != docID {
				// 同文档块引转脚注缩略定义考虑容器块和标题块 https://github.com/siyuan-note/siyuan/issues/5917
				children := treenode.HeadingChildren(defNode)
				nodes = append(nodes, children...)
			}
		} else if ast.NodeDocument == defNode.Type {
			docTitle := &ast.Node{ID: defNode.ID, Type: ast.NodeHeading, HeadingLevel: 1}
			docTitle.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(defNode.IALAttr("title"))})
			nodes = append(nodes, docTitle)
			for c := defNode.FirstChild; nil != c; c = c.Next {
				nodes = append(nodes, c)
			}
		} else {
			nodes = append(nodes, defNode)
		}

		var newNodes []*ast.Node
		for _, node := range nodes {
			if !exportRenderReady(ports) {
				return nil
			}
			var unlinks []*ast.Node

			ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !exportRenderReady(ports) {
					return ast.WalkStop
				}
				if !entering {
					return ast.WalkContinue
				}

				if treenode.IsBlockRef(n) {
					defID, _, _ := treenode.GetBlockRef(n)
					if f := getRefAsFootnotes(defID, refFootnotes); nil != f {
						n.InsertBefore(&ast.Node{Type: ast.NodeText, Tokens: []byte(blockRefTextLeft + f.refAnchorText + blockRefTextRight)})
						n.InsertBefore(&ast.Node{Type: ast.NodeFootnotesRef, Tokens: []byte("^" + f.refNum), FootnotesRefId: f.refNum, FootnotesRefLabel: []byte("^" + f.refNum)})
						unlinks = append(unlinks, n)
					} else {
						if isNodeInTree(defID, currentTree) {
							if currentTreeNodeIDs[defID] {
								// 当前文档内不转换脚注，直接使用锚点哈希 https://github.com/siyuan-note/siyuan/issues/13283
								n.TextMarkType = "a"
								n.TextMarkTextContent = blockRefTextLeft + n.TextMarkTextContent + blockRefTextRight
								n.TextMarkAHref = "#" + defID
								return ast.WalkSkipChildren
							}
						}
					}
					return ast.WalkSkipChildren
				} else if ast.NodeBlockQueryEmbed == n.Type {
					script := n.ChildByType(ast.NodeBlockQueryEmbedScript)
					if script == nil && len(ports) > 0 {
						exportRenderFailure(ports, errors.New("export_footnote_script_missing"))
						return ast.WalkStop
					}
					stmt := script.TokensStr()
					stmt = html.UnescapeString(stmt)
					stmt = strings.ReplaceAll(stmt, editor.IALValEscNewLine, "\n")
					sqlBlocks := exportEmbedQuery(stmt, ports)
					for _, b := range sqlBlocks {
						subNodes := renderBlockMarkdownR(b.ID, &rendered, ports...)
						for _, subNode := range subNodes {
							if ast.NodeListItem == subNode.Type {
								parentList := &ast.Node{Type: ast.NodeList, ListData: &ast.ListData{Typ: subNode.ListData.Typ}}
								parentList.AppendChild(subNode)
								newNodes = append(newNodes, parentList)
							} else {
								newNodes = append(newNodes, subNode)
							}
						}
					}
					unlinks = append(unlinks, n)
					return ast.WalkSkipChildren
				}
				return ast.WalkContinue
			})
			for _, n := range unlinks {
				n.Unlink()
			}

			if ast.NodeBlockQueryEmbed != node.Type {
				if ast.NodeListItem == node.Type {
					parentList := &ast.Node{Type: ast.NodeList, ListData: &ast.ListData{Typ: node.ListData.Typ}}
					parentList.AppendChild(node)
					newNodes = append(newNodes, parentList)
				} else {
					newNodes = append(newNodes, node)
				}
			}
		}

		footnotesDef := &ast.Node{Type: ast.NodeFootnotesDef, Tokens: []byte("^" + foot.refNum), FootnotesRefId: foot.refNum, FootnotesRefLabel: []byte("^" + foot.refNum)}
		for _, node := range newNodes {
			ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !exportRenderReady(ports) {
					return ast.WalkStop
				}
				if !entering {
					return ast.WalkContinue
				}
				if ast.NodeParagraph != n.Type {
					return ast.WalkContinue
				}

				docID := util.GetTreeID(n.Path)
				if currentTree.ID == docID {
					// 同文档块引转脚注缩略定义 https://github.com/siyuan-note/siyuan/issues/3299
					if text := exportRenderText(n.ID, ports); 64 < utf8.RuneCountInString(text) {
						var unlinkChildren []*ast.Node
						for c := n.FirstChild; nil != c; c = c.Next {
							unlinkChildren = append(unlinkChildren, c)
						}
						for _, c := range unlinkChildren {
							c.Unlink()
						}
						text = gulu.Str.SubStr(text, 64) + "..."
						n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(text)})
						return ast.WalkSkipChildren
					}
				}
				return ast.WalkContinue
			})

			footnotesDef.AppendChild(node)
		}
		footnotesDefBlock.AppendChild(footnotesDef)
	}
	if !exportRenderReady(ports) {
		return nil
	}
	return
}

func blockLink2Ref(currentTree *parse.Tree, id string, treeCache map[string]*parse.Tree, depth *int, ports ...*exportReferenceRenderer) {
	if !exportRenderReady(ports) {
		return
	}
	*depth++
	if 4096 < *depth {
		return
	}

	b := exportRenderLookup(id, ports)
	if nil == b {
		return
	}
	requested := b.RootID
	if len(ports) > 0 {
		requested = b.ID
	}
	t, err := exportRenderLoad(requested, treeCache, ports)
	if nil != err {
		return
	}

	node := treenode.GetNodeInTree(t, b.ID)
	if nil == node {
		exportRenderFailure(ports, errors.New("export_reference_node_missing"))
		logging.LogErrorf("not found node [%s] in tree [%s]", b.ID, t.Root.ID)
		return
	}
	blockLink2Ref0(currentTree, node, treeCache, depth, ports...)
	if ast.NodeHeading == node.Type {
		children := treenode.HeadingChildren(node)
		for _, c := range children {
			blockLink2Ref0(currentTree, c, treeCache, depth, ports...)
		}
	}
	return
}

func blockLink2Ref0(currentTree *parse.Tree, node *ast.Node, treeCache map[string]*parse.Tree, depth *int, ports ...*exportReferenceRenderer) {
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if treenode.IsBlockLink(n) {
			n.TextMarkType = strings.TrimSpace(strings.TrimPrefix(n.TextMarkType, "a") + " block-ref")
			n.TextMarkBlockRefID = strings.TrimPrefix(n.TextMarkAHref, "siyuan://blocks/")
			n.TextMarkBlockRefSubtype = "s"

			blockLink2Ref(currentTree, n.TextMarkBlockRefID, treeCache, depth, ports...)
			return ast.WalkSkipChildren
		} else if treenode.IsBlockRef(n) {
			defID, _, _ := treenode.GetBlockRef(n)
			blockLink2Ref(currentTree, defID, treeCache, depth, ports...)
		}
		return ast.WalkContinue
	})
}

func collectFootnotesDefs(currentTree *parse.Tree, id string, refFootnotes *[]*refAsFootnotes, treeCache map[string]*parse.Tree, depth *int, ports ...*exportReferenceRenderer) {
	if !exportRenderReady(ports) {
		return
	}
	*depth++
	if 4096 < *depth {
		return
	}
	b := exportRenderLookup(id, ports)
	if nil == b {
		return
	}
	requested := b.RootID
	if len(ports) > 0 {
		requested = b.ID
	}
	t, err := exportRenderLoad(requested, treeCache, ports)
	if nil != err {
		return
	}

	node := treenode.GetNodeInTree(t, b.ID)
	if nil == node {
		exportRenderFailure(ports, errors.New("export_footnote_node_missing"))
		logging.LogErrorf("not found node [%s] in tree [%s]", b.ID, t.Root.ID)
		return
	}
	collectFootnotesDefs0(currentTree, node, refFootnotes, treeCache, depth, ports...)
	if ast.NodeHeading == node.Type {
		children := treenode.HeadingChildren(node)
		for _, c := range children {
			collectFootnotesDefs0(currentTree, c, refFootnotes, treeCache, depth, ports...)
		}
	}
	return
}

func collectFootnotesDefs0(currentTree *parse.Tree, node *ast.Node, refFootnotes *[]*refAsFootnotes, treeCache map[string]*parse.Tree, depth *int, ports ...*exportReferenceRenderer) {
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !exportRenderReady(ports) {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if treenode.IsBlockRef(n) {
			defID, refText, _ := treenode.GetBlockRef(n)
			if nil == getRefAsFootnotes(defID, refFootnotes) {
				if isNodeInTree(defID, currentTree) {
					// 当前文档内不转换脚注，直接使用锚点哈希 https://github.com/siyuan-note/siyuan/issues/13283
					return ast.WalkSkipChildren
				}
				anchorText := refText
				if max := exportRenderAnchorMax(ports); max < utf8.RuneCountInString(anchorText) {
					anchorText = gulu.Str.SubStr(anchorText, max) + "..."
				}
				*refFootnotes = append(*refFootnotes, &refAsFootnotes{
					defID:         defID,
					refNum:        strconv.Itoa(len(*refFootnotes) + 1),
					refAnchorText: anchorText,
				})
				collectFootnotesDefs(currentTree, defID, refFootnotes, treeCache, depth, ports...)
			}
			return ast.WalkSkipChildren
		}
		return ast.WalkContinue
	})
}

func isNodeInTree(id string, tree *parse.Tree) (ret bool) {
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if n.ID == id {
			ret = true
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return
}

func getRefAsFootnotes(defID string, slice *[]*refAsFootnotes) *refAsFootnotes {
	for _, e := range *slice {
		if e.defID == defID {
			return e
		}
	}
	return nil
}

type refAsFootnotes struct {
	defID         string
	refNum        string
	refAnchorText string
}

func processFileAnnotationRef(refID string, n *ast.Node, fileAnnotationRefMode int, ports ...*exportReferenceRenderer) ast.WalkStatus {
	if len(ports) > 0 {
		return processOwnedFileAnnotationRef(refID, n, fileAnnotationRefMode, ports[0])
	}
	p := refID[:strings.LastIndex(refID, "/")]
	absPath, err := GetAssetAbsPath(p)
	if err != nil {
		logging.LogWarnf("get assets abs path by rel path [%s] failed: %s", p, err)
		return ast.WalkSkipChildren
	}
	sya := absPath + ".sya"
	syaData, err := os.ReadFile(sya)
	if err != nil {
		logging.LogErrorf("read file [%s] failed: %s", sya, err)
		return ast.WalkSkipChildren
	}
	syaJSON := map[string]interface{}{}
	if err = gulu.JSON.UnmarshalJSON(syaData, &syaJSON); err != nil {
		logging.LogErrorf("unmarshal file [%s] failed: %s", sya, err)
		return ast.WalkSkipChildren
	}
	annotationID := refID[strings.LastIndex(refID, "/")+1:]
	annotationData := syaJSON[annotationID]
	if nil == annotationData {
		logging.LogErrorf("not found annotation [%s] in .sya", annotationID)
		return ast.WalkSkipChildren
	}
	pages := annotationData.(map[string]interface{})["pages"].([]interface{})
	page := int(pages[0].(map[string]interface{})["index"].(float64)) + 1
	pageStr := strconv.Itoa(page)

	refText := n.TextMarkTextContent
	ext := filepath.Ext(p)
	file := p[7:len(p)-23-len(ext)] + ext
	return insertFileAnnotationLink(p, file, refText, pageStr, n, fileAnnotationRefMode)
}

func insertFileAnnotationLink(p, file, refText, pageStr string, n *ast.Node, fileAnnotationRefMode int) ast.WalkStatus {
	fileAnnotationRefLink := &ast.Node{Type: ast.NodeLink}
	fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
	if 0 == fileAnnotationRefMode {
		fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(file + " - p" + pageStr + " - " + refText)})
	} else {
		fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(refText)})
	}
	fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
	fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
	dest := p + "#page=" + pageStr // https://github.com/siyuan-note/siyuan/issues/11780
	fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(dest)})
	fileAnnotationRefLink.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
	n.InsertBefore(fileAnnotationRefLink)
	return ast.WalkSkipChildren
}

func exportPandocConvertZip(baseFolderName string, docPaths, defBlockIDs []string,
	pandocFrom, pandocTo, ext string, treeCache map[string]*parse.Tree, m *markdownExportContext) (zipPath string) {
	defer util.ClearPushProgress(100)
	if err := m.guard(); err != nil {
		return ""
	}

	o, err := m.newOutput(baseFolderName, ext)
	if err != nil {
		logging.LogErrorf("create Markdown output: %s", err)
		return ""
	}
	defer func() {
		if closeErr := o.finish(zipPath != ""); closeErr != nil {
			logging.LogErrorf("finish Markdown output: %s", closeErr)
			zipPath = ""
		}
	}()
	resources, err := m.openResources(o)
	if err != nil {
		logging.LogErrorf("open Markdown resources: %s", err)
		return ""
	}
	// Close resources before output finalization, and suppress the URL on error.
	defer func() {
		if closeErr := resources.close(); closeErr != nil {
			logging.LogErrorf("close Markdown resources: %s", closeErr)
			zipPath = ""
		}
	}()

	exportRefMode := m.settings.BlockRefMode
	wrotePathHash := map[string]string{}

	luteEngine := m.markdownParser()
	for i, p := range docPaths {
		id := util.GetTreeID(p)
		if err := m.guard(); err != nil {
			return ""
		}
		tree, md, isEmpty, renderErr := exportMarkdownContent(id, ext, exportRefMode, defBlockIDs, false, treeCache, m)
		if renderErr != nil {
			logging.LogErrorf("render prepared Markdown: %s", renderErr)
			return ""
		}
		if nil == tree {
			continue
		}

		p, err = markdownExportPath(tree.HPath, ext)
		if err != nil {
			logging.LogErrorf("Markdown path: %s", err)
			return ""
		}
		hash := fmt.Sprintf("%x", sha1.Sum([]byte(md)))
		if _, statErr := o.payload.Lstat(p); statErr == nil && hash != wrotePathHash[p] {
			p = strings.TrimSuffix(p, ext) + "-" + id + ext
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return ""
		}
		if err = resources.target.Check(p, true); err != nil {
			return ""
		}
		if err = o.payload.MkdirAll(path.Dir(p), 0755); err != nil {
			return ""
		}
		name := path.Base(p)
		if isEmpty {
			hasChildren, readErr := resources.hasChildren(tree.Box, tree.Path)
			if readErr != nil {
				logging.LogErrorf("Markdown child directory: %s", readErr)
				return ""
			}
			if hasChildren {
				continue
			}
		}

		// 解析导出后的标准 Markdown，汇总 assets
		tree = parse.Parse("", gulu.Str.ToBytes(md), luteEngine.ParseOptions)

		assets, assetErr := getAssetsLinkDestsWithAV(tree.Root, m.read.readExportView)
		if assetErr != nil {
			logging.LogErrorf("Markdown assets: %s", assetErr)
			return ""
		}
		for _, asset := range assets {
			if err = resources.copyAsset(asset, path.Dir(p)); err != nil {
				logging.LogErrorf("copy Markdown resource: %s", err)
				return ""
			}
		}
		if err = m.convert(o, p, pandocFrom, pandocTo, md); err != nil {
			logging.LogErrorf("convert Markdown: %s", err)
			return ""
		}

		wrotePathHash[p] = hash
		util.PushEndlessProgress(m.read.binding.scope.commit.r.language(65) + " " + fmt.Sprintf(m.read.binding.scope.commit.r.language(70), fmt.Sprintf("%d/%d %s", i+1, len(docPaths), name)))
	}

	if err = o.pack(); err != nil {
		logging.LogErrorf("pack Markdown: %s", err)
		return ""
	}
	return "/export/" + url.PathEscape(o.filename)
}

func getExportBlockRefLinkText(blockRef *ast.Node, blockRefTextLeft, blockRefTextRight string, ports ...*exportReferenceRenderer) (defID, linkText string) {
	defID, linkText, _ = treenode.GetBlockRef(blockRef)
	if "" == linkText {
		linkText = exportRenderText(defID, ports)
	}
	linkText = util.UnescapeHTML(linkText) // 块引锚文本导出时 `&` 变为实体 `&amp;` https://github.com/siyuan-note/siyuan/issues/7659
	if max := exportRenderAnchorMax(ports); max < utf8.RuneCountInString(linkText) {
		linkText = gulu.Str.SubStr(linkText, max) + "..."
	}
	linkText = blockRefTextLeft + linkText + blockRefTextRight
	return
}

func loadTreeWithCache(id string, treeCache map[string]*parse.Tree) (tree *parse.Tree, err error) {
	if tree = treeCache[id]; nil != tree {
		return
	}
	tree, err = LoadTreeByBlockID(id)
	if nil == err && nil != tree {
		treeCache[id] = tree
	}
	return
}

func getAttrViewTable(attrView *av.AttributeView, view *av.View, query string) (ret *av.Table) {
	projectAttributeViewTable(view)
	depth := 1
	ret = sql.RenderAttributeViewTable(attrView, view, query, &depth, map[string]*av.AttributeView{})
	return
}

// adjustHeadingLevel 聚焦导出（即非文档块）的情况下，将第一个标题层级提升为一级（如果开启了添加文档标题的话提升为二级）。
// Export preview mode supports focus use https://github.com/siyuan-note/siyuan/issues/15340
func adjustHeadingLevel(bt *treenode.BlockTree, tree *parse.Tree) {
	adjustHeadingLevelWithTitle(bt, tree, Conf.Export.AddTitle)
}

func adjustHeadingLevelWithTitle(bt *treenode.BlockTree, tree *parse.Tree, addTitle bool) {
	if "d" == bt.Type {
		return
	}

	level := 1
	var firstHeading *ast.Node
	if !addTitle {
		for n := tree.Root.FirstChild; nil != n; n = n.Next {
			if ast.NodeHeading == n.Type && !n.ParentIs(ast.NodeBlockquote) {
				firstHeading = n
				break
			}
		}
	} else {
		for n := tree.Root.FirstChild.Next; nil != n; n = n.Next {
			if ast.NodeHeading == n.Type && !n.ParentIs(ast.NodeBlockquote) {
				firstHeading = n
				break
			}
		}
		level = 2
	}
	if nil != firstHeading {
		hLevel := firstHeading.HeadingLevel
		diff := level - hLevel
		var children, childrenHeadings []*ast.Node
		children = append(children, firstHeading)
		children = append(children, treenode.HeadingChildren(firstHeading)...)
		for _, c := range children {
			ccH := c.ChildrenByType(ast.NodeHeading)
			childrenHeadings = append(childrenHeadings, ccH...)
		}
		for _, h := range childrenHeadings {
			h.HeadingLevel += diff
			if 6 < h.HeadingLevel {
				h.HeadingLevel = 6
			} else if 1 > h.HeadingLevel {
				h.HeadingLevel = 1
			}
		}
	}
}
