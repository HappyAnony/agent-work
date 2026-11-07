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
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/html/atom"
	"github.com/88250/lute/parse"
	util2 "github.com/88250/lute/util"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func ImportSY(zipPath, boxID, toPath string, contexts ...context.Context) error {
	e, err := CaptureArchiveImport(legacyAVContext(contexts))
	if err != nil {
		return err
	}
	return e.importFile(zipPath, boxID, toPath)
}

func (e *ArchiveImportContext) importFile(zipPath, boxID, toPath string) (err error) {
	if err = validateArchiveDestination(boxID, toPath); err != nil {
		return
	}
	if err = e.guard(); err != nil {
		return
	}
	util.PushEndlessProgress(e.language(73))
	defer util.ClearPushProgress(100)
	// Lock and unlock the same captured owner even if selection changes while waiting.
	runtime := &e.binding.scope.commit.r.owner.syncRuntime
	runtime.lock.Lock()
	runtime.running.Store(true)
	defer func() { runtime.running.Store(false); runtime.lock.Unlock() }()
	if err = e.guard(); err != nil {
		return
	}
	prepared, err := e.prepare(zipPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, prepared.close()) }()
	if err = e.guard(); err != nil {
		return
	}
	unzipRootPath, luteEngine, blockIDs, trees := prepared.root, prepared.engine, prepared.ids, prepared.trees

	avIDs, hasAVs, err := e.remapScratchAVs(unzipRootPath, blockIDs, ast.NewNodeID)
	if err != nil {
		return err
	}
	if hasAVs {
		if err = e.mergeArchiveDirectory(filepath.Join(unzipRootPath, "storage/av"), "storage/av"); err != nil {
			return
		}

		// 重新指向数据库属性值
		for _, tree := range trees {
			if err = e.guard(); err != nil {
				return
			}
			util.PushEndlessProgress(e.language(73) + " " + fmt.Sprintf(e.language(70), tree.Root.IALAttr("title")))
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !entering || "" == n.ID {
					return ast.WalkContinue
				}

				ial := parse.IAL2Map(n.KramdownIAL)
				for k, v := range ial {
					if strings.HasPrefix(k, av.NodeAttrNameAvs) {
						newKey, newVal := k, v
						for oldAvID, newAvID := range avIDs {
							newKey = strings.ReplaceAll(newKey, oldAvID, newAvID)
							newVal = strings.ReplaceAll(newVal, oldAvID, newAvID)
						}
						n.RemoveIALAttr(k)
						n.SetIALAttr(newKey, newVal)
					}
				}

				if ast.NodeAttributeView == n.Type {
					n.AttributeViewID = avIDs[n.AttributeViewID]
				}
				return ast.WalkContinue
			})

		}

		// 如果数据库中绑定的块不在导入的文档中，则需要单独更新这些绑定块的属性
		var attrViewIDs []string
		for _, avID := range avIDs {
			attrViewIDs = append(attrViewIDs, avID)
		}
		importedTrees := make([]*parse.Tree, 0, len(trees))
		for _, tree := range trees {
			importedTrees = append(importedTrees, tree)
		}
		if err = finishImportedAttributeViewsWithEnvironment(attrViewIDs, importedTrees, importWithBinding(nil, e.binding, e.ctx)); err != nil {
			return err
		}
	}

	if err = e.guard(); err != nil {
		return
	}
	// The flashcard phase uses explicit owner ports; imported trees are already ID-remapped.
	archiveFiles, archiveErr := e.readLegacyFlashcardArchive(unzipRootPath)
	if archiveErr != nil {
		return archiveErr
	}
	importedFlashcardTrees := make([]*parse.Tree, 0, len(trees))
	for _, tree := range trees {
		importedFlashcardTrees = append(importedFlashcardTrees, tree)
	}
	if _, archiveErr = runLegacyFlashcardArchive(e.binding, archiveFiles, blockIDs, importedFlashcardTrees, "", false); archiveErr != nil {
		return archiveErr
	}

	if err = e.removeScratchStorage(unzipRootPath); err != nil {
		return
	}

	if 1 > len(avIDs) { // 如果本次没有导入数据库，则清理掉文档中的数据库属性 https://github.com/siyuan-note/siyuan/issues/13011
		for _, tree := range trees {
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !entering || !n.IsBlock() {
					return ast.WalkContinue
				}

				n.RemoveIALAttr(av.NodeAttrNameAvs)
				return ast.WalkContinue
			})
		}
	}

	if err = e.writeScratchDocuments(unzipRootPath, trees, luteEngine); err != nil {
		return
	}

	if err = e.mergeArchiveSort(unzipRootPath, boxID, blockIDs); err != nil {
		return
	}

	if err = e.renameScratchDirectories(unzipRootPath, blockIDs); err != nil {
		return
	}

	if err = e.publishArchiveResources(unzipRootPath, "assets"); err != nil {
		return
	}
	if err = e.publishArchiveResources(unzipRootPath, "emojis"); err != nil {
		return
	}
	if err = e.guard(); err != nil {
		return
	}

	return e.finishArchive(unzipRootPath, boxID, toPath, luteEngine, blockIDs)
}

// Compatibility wrapper for callers with an explicit local ZIP path.
func ImportData(zipPath string) error {
	d, err := CaptureDataImport(context.Background())
	if err != nil {
		return err
	}
	return d.ImportFile(zipPath)
}

func ImportFromLocalPath(boxID, localPath string, toPath string, contexts ...context.Context) (err error) {
	m, err := captureMarkdownImportPreparation(legacyAVContext(contexts))
	if err != nil {
		return err
	}
	return m.importFromLocalPath(boxID, localPath, toPath, "", true)
}

// ZIP imports reuse their pre-upload capture and defer completion until their
// owned upload/extraction directories have closed successfully.
func (m *markdownImportPreparation) importFromLocalPath(boxID, localPath, toPath, rootName string, finish bool) (err error) {
	if m == nil || m.archive == nil {
		return errors.New("markdown_import_source_unbound")
	}
	m.completion = nil
	defer func() {
		m.discard()
		if err != nil {
			m.completion = nil
		}
	}()
	err = func() (err error) {
		source, err := m.openLocalSource(localPath)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, source.close()) }()
		return m.importLocalSource(boxID, source, toPath, rootName)
	}()
	if err != nil {
		return err
	}
	if finish {
		return m.completeSync()
	}
	return nil
}
func (m *markdownImportPreparation) importLocalSource(boxID string, source *markdownImportSource, toPath, rootName string) (err error) {
	localPath := source.local

	if m == nil || m.archive == nil {
		return errors.New("markdown_import_source_unbound")
	}
	if err = source.guard(); err != nil {
		return err
	}
	if rootName == "" {
		rootName = filepath.Base(localPath)
	}
	defer m.discard()
	util.PushEndlessProgress(m.language(73))
	defer func() {
		util.PushClearProgress()

		if e := recover(); nil != e {
			stack := debug.Stack()
			msg := fmt.Sprintf("PANIC RECOVERED: %v\n\t%s\n", e, stack)
			logging.LogErrorf("import from local path failed: %s", msg)
			err = errors.New("import from local path failed, please check kernel log for details")
		}
	}()

	runtime := &m.archive.binding.scope.commit.r.owner.syncRuntime
	runtime.lock.Lock()
	runtime.running.Store(true)
	defer func() { runtime.running.Store(false); runtime.lock.Unlock() }()
	if err = source.guard(); err != nil {
		return err
	}

	if err = FlushTxQueueContext(m.ctx); err != nil {
		return err
	}
	if err = source.guard(); err != nil {
		return err
	}

	baseHPath, baseTargetPath, err := m.localTarget(boxID, toPath)
	if err != nil {
		return err
	}
	assets, err := source.openAssets()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, assets.close()) }()

	hPathsIDs := map[string]string{}
	idPaths := map[string]string{}
	moveIDs := map[string]string{}
	assetsDone := map[string]string{}
	if source.directory { // 导入文件夹
		targetPaths := map[string]string{}
		count := 0
		// md 转换 sy
		walkErr := source.walk(func(currentPath string, d fs.DirEntry, scanErr error) error {
			if scanErr != nil {
				return scanErr
			}
			if err := source.guard(); err != nil {
				return err
			}
			if d == nil {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if !d.IsDir() && !strings.HasSuffix(currentPath, ".md") && !strings.HasSuffix(currentPath, ".markdown") {
				// 非 Markdown 文件作为资源文件处理 https://github.com/siyuan-note/siyuan/issues/13817
				existName := assetsDone[currentPath]
				var name string
				if "" == existName {
					name, err = assets.copy(currentPath, "assets")
					if err != nil {
						return err
					}
					assetsDone[currentPath] = name
				}
				return nil
			}

			var tree *parse.Tree
			var ext string
			title := d.Name()
			if !d.IsDir() {
				ext = util.Ext(d.Name())
				title = strings.TrimSuffix(d.Name(), ext)
			}
			id := ast.NewNodeID()

			curRelPath := filepath.ToSlash(strings.TrimPrefix(currentPath, localPath))
			targetPath := path.Join(baseTargetPath, id)
			hPath := path.Join(baseHPath, rootName, filepath.ToSlash(strings.TrimPrefix(currentPath, localPath)))
			hPath = strings.TrimSuffix(hPath, ext)
			if "" == curRelPath {
				title = rootName
				curRelPath = "/"
				hPath = path.Join(baseHPath, title)
			} else {
				dirPath := targetPaths[path.Dir(curRelPath)]
				targetPath = path.Join(dirPath, id)
			}

			targetPath = strings.ReplaceAll(targetPath, util.DocExt+"/", "/")
			targetPath += util.DocExt
			if _, ok := targetPaths[curRelPath]; !ok {
				targetPaths[curRelPath] = targetPath
			} else {
				targetPath = targetPaths[curRelPath]
				id = util.GetTreeID(targetPath)
			}

			if d.IsDir() {
				if "assets" == d.Name() {
					// 如果是 assets 文件夹则跳过，里面的 Markdown 文件算作资源文件 https://github.com/siyuan-note/siyuan/issues/13817
					return nil
				}

				hasMarkdown, scanErr := source.hasMarkdown(currentPath)
				if scanErr != nil {
					return scanErr
				}
				if !hasMarkdown {
					// 如果该文件夹中不包含 Markdown 文件则不处理 https://github.com/siyuan-note/siyuan/issues/11567
					return nil
				}

				// 如果当前文件夹路径下包含同名的 Markdown 文件，则不创建空文档 https://github.com/siyuan-note/siyuan/issues/13149
				sibling, siblingErr := source.siblingMarkdown(currentPath)
				if siblingErr != nil {
					return siblingErr
				}
				if sibling {
					targetPaths[curRelPath+".md"] = targetPath
					return nil
				}

				tree = treenode.NewTree(boxID, targetPath, hPath, title)
				m.trees = append(m.trees, tree)
				return nil
			}

			if !strings.HasSuffix(d.Name(), ".md") && !strings.HasSuffix(d.Name(), ".markdown") {
				return nil
			}

			data, readErr := source.read(currentPath)
			if nil != readErr {
				err = readErr
				return readErr
			}

			tree, yfmRootID, yfmTitle, yfmUpdated, parseErr := m.parse(data)
			if parseErr != nil {
				err = parseErr
				return parseErr
			}
			if nil == tree {
				logging.LogErrorf("parse tree [%s] failed", currentPath)
				return nil
			}

			if "" != yfmRootID {
				moveIDs[id] = yfmRootID
				id = yfmRootID
			}
			if "" != yfmTitle {
				title = yfmTitle
			}
			unescapedTitle, unescapeErr := url.PathUnescape(title)
			if nil == unescapeErr {
				title = unescapedTitle
			}
			hPath = path.Join(path.Dir(hPath), title)
			updated := yfmUpdated
			fname := path.Base(targetPath)
			targetPath = strings.ReplaceAll(targetPath, fname, id+util.DocExt)
			targetPaths[curRelPath] = targetPath

			tree.ID = id
			tree.Root.ID = id
			tree.Root.SetIALAttr("id", tree.Root.ID)
			tree.Root.SetIALAttr("title", title)
			tree.Box = boxID
			targetPath = path.Join(path.Dir(targetPath), tree.Root.ID+util.DocExt)
			tree.Path = targetPath
			targetPaths[curRelPath] = targetPath
			tree.HPath = hPath
			tree.Root.Spec = "1"

			folder, assetErr := assets.directory(boxID, targetPath)
			if assetErr != nil {
				return assetErr
			}
			if err = assets.rewrite(tree, currentPath, folder, assetsDone); err != nil {
				return err
			}

			reassignIDUpdated(tree, id, updated)
			m.trees = append(m.trees, tree)

			hPathsIDs[tree.HPath] = tree.ID
			idPaths[tree.ID] = tree.Path

			count++
			if 0 == count%4 {
				util.PushEndlessProgress(fmt.Sprintf(m.language(70), fmt.Sprintf("%s", tree.HPath)))
			}
			return nil
		})
		if walkErr != nil {
			return walkErr
		}
		if err != nil {
			return err
		}
	} else { // 导入单个文件
		fileName := filepath.Base(localPath)
		if !strings.HasSuffix(fileName, ".md") && !strings.HasSuffix(fileName, ".markdown") {
			return errors.New(m.language(79))
		}

		title := strings.TrimSuffix(fileName, ".markdown")
		title = strings.TrimSuffix(title, ".md")
		targetPath := util.StripDocExt(toPath)
		id := ast.NewNodeID()
		targetPath = path.Join(targetPath, id+util.DocExt)
		var data []byte
		data, err = source.read(localPath)
		if err != nil {
			return err
		}
		tree, yfmRootID, yfmTitle, yfmUpdated, parseErr := m.parse(data)
		if parseErr != nil {
			err = parseErr
			return parseErr
		}
		if nil == tree {
			msg := fmt.Sprintf("parse tree [%s] failed", localPath)
			logging.LogErrorf("%s", msg)
			return errors.New(msg)
		}

		if "" != yfmRootID {
			id = yfmRootID
		}
		if "" != yfmTitle {
			title = yfmTitle
		}
		unescapedTitle, unescapeErr := url.PathUnescape(title)
		if nil == unescapeErr {
			title = unescapedTitle
		}
		updated := yfmUpdated
		fname := path.Base(targetPath)
		targetPath = strings.ReplaceAll(targetPath, fname, id+util.DocExt)

		tree.ID = id
		tree.Root.ID = id
		tree.Root.SetIALAttr("id", tree.Root.ID)
		tree.Root.SetIALAttr("title", title)
		tree.Box = boxID
		tree.Path = targetPath
		tree.HPath = path.Join(baseHPath, title)
		tree.Root.Spec = "1"

		folder, assetErr := assets.directory(boxID, targetPath)
		if assetErr != nil {
			return assetErr
		}
		if err = assets.rewrite(tree, localPath, folder, assetsDone); err != nil {
			return err
		}

		reassignIDUpdated(tree, id, updated)
		m.trees = append(m.trees, tree)
	}

	if 0 < len(m.trees) {
		for id, newID := range moveIDs {
			for _, importTree := range m.trees {
				importTree.ID = strings.ReplaceAll(importTree.ID, id, newID)
				importTree.Path = strings.ReplaceAll(importTree.Path, id, newID)
			}
		}

		if err = m.prepareLinks(); err != nil {
			return err
		}

		if err = m.publishDocuments(source, boxID); err != nil {
			return err
		}
		util.PushClearProgress()

		// 按照路径排序 Improve sort when importing markdown files https://github.com/siyuan-note/siyuan/issues/11390
		var hPaths []string
		for hPath := range hPathsIDs {
			hPaths = append(hPaths, hPath)
		}
		sort.Strings(hPaths)
		paths := map[string][]string{}
		for _, hPath := range hPaths {
			p := idPaths[hPathsIDs[hPath]]
			parent := path.Dir(p)
			for {
				if baseTargetPath == parent {
					break
				}

				if ps, ok := paths[parent]; !ok {
					paths[parent] = []string{p}
				} else {
					ps = append(ps, p)
					ps = gulu.Str.RemoveDuplicatedElem(ps)
					paths[parent] = ps
				}
				p = parent
				parent = path.Dir(parent)
			}
		}

		sortIDVals := map[string]int{}
		for _, ps := range paths {
			sortVal := 0
			for _, p := range ps {
				sortIDVals[util.GetTreeID(p)] = sortVal
				sortVal++
			}
		}
		m.completion, err = assets.persistSort(boxID, sortIDVals)
		if err != nil {
			return err
		}
	}

	if err = source.guard(); err != nil {
		return err
	}
	m.discard()
	debug.FreeOSMemory()
	return
}

func htmlBlock2Inline(tree *parse.Tree) {
	imgHtmlBlocks := map[*ast.Node]*html.Node{}
	aHtmlBlocks := map[*ast.Node]*html.Node{}
	var unlinks []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeHTMLBlock == n.Type || (ast.NodeText == n.Type && bytes.HasPrefix(bytes.ToLower(n.Tokens), []byte("<img "))) {
			tokens := bytes.TrimSpace(n.Tokens)
			if bytes.HasPrefix(tokens, []byte("<div>")) {
				tokens = bytes.TrimPrefix(tokens, []byte("<div>"))
			}
			if bytes.HasSuffix(tokens, []byte("</div>")) {
				tokens = bytes.TrimSuffix(tokens, []byte("</div>"))
			}
			tokens = bytes.TrimSpace(tokens)

			htmlNodes, pErr := html.ParseFragment(bytes.NewReader(tokens), &html.Node{Type: html.ElementNode})
			if nil != pErr {
				logging.LogErrorf("parse html block [%s] failed: %s", n.Tokens, pErr)
				return ast.WalkContinue
			}
			if 1 > len(htmlNodes) {
				return ast.WalkContinue
			}

			for _, htmlNode := range htmlNodes {
				if atom.Img == htmlNode.DataAtom {
					imgHtmlBlocks[n] = htmlNode
					break
				}
			}
		}
		if ast.NodeHTMLBlock == n.Type || (ast.NodeText == n.Type && bytes.HasPrefix(bytes.ToLower(n.Tokens), []byte("<a "))) {
			tokens := bytes.TrimSpace(n.Tokens)
			if bytes.HasPrefix(tokens, []byte("<div>")) {
				tokens = bytes.TrimPrefix(tokens, []byte("<div>"))
			}
			if bytes.HasSuffix(tokens, []byte("</div>")) {
				tokens = bytes.TrimSuffix(tokens, []byte("</div>"))
			}
			tokens = bytes.TrimSpace(tokens)

			if ast.NodeHTMLBlock != n.Type && nil != n.Next && nil != n.Next.Next {
				if ast.NodeText == n.Next.Next.Type && bytes.Equal(n.Next.Next.Tokens, []byte("</a>")) {
					tokens = append(tokens, n.Next.Tokens...)
					tokens = append(tokens, []byte("</a>")...)
					unlinks = append(unlinks, n.Next)
					unlinks = append(unlinks, n.Next.Next)
				}
			}

			htmlNodes, pErr := html.ParseFragment(bytes.NewReader(tokens), &html.Node{Type: html.ElementNode})
			if nil != pErr {
				logging.LogErrorf("parse html block [%s] failed: %s", n.Tokens, pErr)
				return ast.WalkContinue
			}
			if 1 > len(htmlNodes) {
				return ast.WalkContinue
			}

			for _, htmlNode := range htmlNodes {
				if atom.A == htmlNode.DataAtom {
					aHtmlBlocks[n] = htmlNode
					break
				}
			}
		}
		return ast.WalkContinue
	})

	for n, htmlImg := range imgHtmlBlocks {
		src := domAttrValue(htmlImg, "src")
		alt := domAttrValue(htmlImg, "alt")
		title := domAttrValue(htmlImg, "title")

		p := treenode.NewParagraph(n.ID)
		img := &ast.Node{Type: ast.NodeImage}
		p.AppendChild(img)
		img.AppendChild(&ast.Node{Type: ast.NodeBang})
		img.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
		img.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(alt)})
		img.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
		img.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
		img.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(src)})
		if "" != title {
			img.AppendChild(&ast.Node{Type: ast.NodeLinkSpace})
			img.AppendChild(&ast.Node{Type: ast.NodeLinkTitle})
		}
		img.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
		if width := domAttrValue(htmlImg, "width"); "" != width {
			if util2.IsDigit(width) {
				width += "px"
			}
			style := "width: " + width + ";"
			ial := &ast.Node{Type: ast.NodeKramdownSpanIAL, Tokens: parse.IAL2Tokens([][]string{{"style", style}})}
			img.SetIALAttr("style", style)
			img.InsertAfter(ial)
		} else if height := domAttrValue(htmlImg, "height"); "" != height {
			if util2.IsDigit(height) {
				height += "px"
			}
			style := "height: " + height + ";"
			ial := &ast.Node{Type: ast.NodeKramdownSpanIAL, Tokens: parse.IAL2Tokens([][]string{{"style", style}})}
			img.SetIALAttr("style", style)
			img.InsertAfter(ial)
		}

		if ast.NodeHTMLBlock == n.Type {
			n.InsertBefore(p)
		} else if ast.NodeText == n.Type {
			if nil != n.Parent {
				if n.Parent.IsContainerBlock() {
					n.InsertBefore(p)
				} else {
					n.InsertBefore(img)
				}
			} else {
				n.InsertBefore(p)
			}
		}
		unlinks = append(unlinks, n)
	}

	for n, htmlA := range aHtmlBlocks {
		href := domAttrValue(htmlA, "href")
		title := domAttrValue(htmlA, "title")
		anchor := util2.DomText(htmlA)

		p := treenode.NewParagraph(n.ID)
		a := &ast.Node{Type: ast.NodeLink}
		p.AppendChild(a)
		a.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
		a.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte(anchor)})
		a.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
		a.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
		a.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(href)})
		if "" != title {
			a.AppendChild(&ast.Node{Type: ast.NodeLinkSpace})
			a.AppendChild(&ast.Node{Type: ast.NodeLinkTitle, Tokens: []byte(title)})
		}
		a.AppendChild(&ast.Node{Type: ast.NodeCloseParen})

		if ast.NodeHTMLBlock == n.Type || (nil == n.Previous && (nil != n.Next && nil != n.Next.Next && nil == n.Next.Next.Next)) {
			n.InsertBefore(p)
		} else {
			n.InsertBefore(a)
		}
		unlinks = append(unlinks, n)
	}

	for _, n := range unlinks {
		n.Unlink()
	}
	return
}

func reassignIDUpdated(tree *parse.Tree, rootID, updated string) {
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || "" == n.ID {
			return ast.WalkContinue
		}

		n.ID = ast.NewNodeID()
		if ast.NodeDocument == n.Type && "" != rootID {
			n.ID = rootID
		}

		n.SetIALAttr("id", n.ID)
		if "" != updated {
			n.SetIALAttr("updated", updated)
			if "" == rootID {
				n.ID = updated + "-" + gulu.Rand.String(7)
				n.SetIALAttr("id", n.ID)
			}
		} else {
			n.SetIALAttr("updated", util.TimeFromID(n.ID))
		}
		return ast.WalkContinue
	})
	tree.ID = tree.Root.ID
	tree.Path = path.Join(path.Dir(tree.Path), tree.ID+util.DocExt)
	tree.Root.SetIALAttr("id", tree.Root.ID)
}

func domAttrValue(n *html.Node, attrName string) string {
	if nil == n {
		return ""
	}

	for _, attr := range n.Attr {
		if attr.Key == attrName {
			return attr.Val
		}
	}
	return ""
}
