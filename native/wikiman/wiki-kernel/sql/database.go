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

package sql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"text/template"
	"unicode/utf8"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

var (
	db             *sql.DB
	historyDB      *sql.DB
	assetContentDB *sql.DB
)

var initDatabaseLock = sync.Mutex{}

func InitDatabase(forceRebuild bool) (err error) {
	initDatabaseLock.Lock()
	defer initDatabaseLock.Unlock()
	if err = OpenLegacyCaches(); err != nil {
		return err
	}

	ClearCache()
	disableCache()
	defer enableCache()

	util.IncBootProgress(2, "Initializing database...")

	if forceRebuild {
		ClearQueue()
	}

	initDBConnection()
	treenode.InitBlockTree(forceRebuild)

	if !forceRebuild {
		// 检查数据库结构版本，如果版本不一致的话说明改过表结构，需要重建
		if util.DatabaseVer == getDatabaseVer() {
			return
		}
		logging.LogInfof("the database structure is changed, rebuilding database...")
	}

	// 不存在库或者版本不一致都会走到这里

	closeDatabase()
	if gulu.File.IsExist(util.DBPath) {
		if err = removeDatabaseFile(); err != nil {
			logging.LogErrorf("remove database file [%s] failed: %s", util.DBPath, err)
			util.PushClearProgress()
			err = nil
		}
	}

	initDBConnection()
	initDBTables()

	logging.LogInfof("reinitialized database [%s]", util.DBPath)
	return
}

func initDBTables() {
	if err := indexdb.Reset(context.Background(), db, indexdb.Main, util.DatabaseVer); err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "initialize original index failed: %s", err)
	}
}

func initDBConnection() {
	if nil != db {
		closeDatabase()
	}

	util.LogDatabaseSize(util.DBPath)
	var err error
	db, err = indexdb.OpenFile(context.Background(), util.DBPath, 20)
	if err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "open original index failed: %s", err)
	}
}

var initHistoryDatabaseLock = sync.Mutex{}

func InitHistoryDatabase(forceRebuild bool) {
	if e := InitHistoryDatabaseContext(context.Background(), forceRebuild); e != nil {
		logging.LogErrorf("initialize history DB failed: %s", e)
	}
}

// A failed drain keeps the old DB alive. Once physical replacement starts,
// finish the new connection/schema before releasing its lifecycle owner.
func InitHistoryDatabaseContext(ctx context.Context, forceRebuild bool) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	initHistoryDatabaseLock.Lock()
	defer initHistoryDatabaseLock.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	existed := gulu.File.IsExist(util.HistoryDBPath)
	if e := initHistoryDBConnection(ctx); e != nil {
		return e
	}
	if !forceRebuild && existed {
		return ctx.Err()
	}
	if e := closeLegacyHistoryQueue(context.WithoutCancel(ctx)); e != nil {
		return e
	}
	if e := historyDB.Close(); e != nil {
		return e
	}
	if e := os.RemoveAll(util.HistoryDBPath); e != nil {
		return e
	}
	if e := initHistoryDBConnection(context.WithoutCancel(ctx)); e != nil {
		return e
	}
	if e := indexdb.Reset(context.WithoutCancel(ctx), historyDB, indexdb.History, util.DatabaseVer); e != nil {
		return e
	}
	return ctx.Err()
}

func initHistoryDBConnection(ctx context.Context) error {
	historyQueues.Lock()
	old := historyDB
	historyQueues.Unlock()
	if old != nil {
		if e := closeLegacyHistoryQueue(ctx); e != nil {
			return e
		}
		_ = old.Close()
	}
	util.LogDatabaseSize(util.HistoryDBPath)
	opened, e := indexdb.OpenFile(context.WithoutCancel(ctx), util.HistoryDBPath, 3)
	if e != nil {
		return e
	}
	historyQueues.Lock()
	historyDB = opened
	delete(historyQueues.byDB, old)
	delete(historyQueues.eventOwners, old)
	historyQueues.Unlock()
	return nil
}

func initHistoryDBTables() {
	if err := indexdb.Reset(context.Background(), historyDB, indexdb.History, util.DatabaseVer); err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "initialize original index failed: %s", err)
	}
}

var initAssetContentDatabaseLock = sync.Mutex{}

func InitAssetContentDatabase(forceRebuild bool) {
	if err := InitAssetContentDatabaseContext(context.Background(), forceRebuild); err != nil {
		logging.LogErrorf("initialize assets DB failed: %s", err)
	}
}

// InitAssetContentDatabaseContext mirrors the history database replacement
// barrier: cancellation is honored before replacement, while an accepted
// replacement always finishes opening and initializing its new database.
func InitAssetContentDatabaseContext(ctx context.Context, forceRebuild bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	initAssetContentDatabaseLock.Lock()
	defer initAssetContentDatabaseLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	existed := gulu.File.IsExist(util.AssetContentDBPath)
	if err := initAssetContentDBConnectionContext(ctx); err != nil {
		return err
	}

	if !forceRebuild && existed {
		return ctx.Err()
	}

	if err := assetContentDB.Close(); err != nil {
		return err
	}
	if err := os.RemoveAll(util.AssetContentDBPath); err != nil {
		return err
	}

	if err := initAssetContentDBConnectionContext(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	if err := indexdb.Reset(context.WithoutCancel(ctx), assetContentDB, indexdb.Assets, util.DatabaseVer); err != nil {
		return err
	}
	return ctx.Err()
}

func initAssetContentDBConnection() {
	if err := initAssetContentDBConnectionContext(context.Background()); err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "open original assets index failed: %s", err)
	}
}

func initAssetContentDBConnectionContext(ctx context.Context) error {
	old := assetContentDB
	if nil != assetContentDB {
		if err := assetContentDB.Close(); err != nil {
			return err
		}
	}
	legacyAssetEventOwners.Lock()
	delete(legacyAssetEventOwners.byDB, old)
	legacyAssetEventOwners.Unlock()

	util.LogDatabaseSize(util.AssetContentDBPath)
	opened, err := indexdb.OpenFile(context.WithoutCancel(ctx), util.AssetContentDBPath, 3)
	if err != nil {
		return err
	}
	assetContentDB = opened
	return nil
}

func initAssetContentDBTables() {
	if err := indexdb.Reset(context.Background(), assetContentDB, indexdb.Assets, util.DatabaseVer); err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "initialize original index failed: %s", err)
	}
}

var (
	caseSensitive  bool
	indexAssetPath bool
)

func SetCaseSensitive(b bool) {
	caseSensitive = b
	if b {
		db.Exec("PRAGMA case_sensitive_like = ON;")
	} else {
		db.Exec("PRAGMA case_sensitive_like = OFF;")
	}
}

func SetIndexAssetPath(b bool) {
	indexAssetPath = b
}

func (projection *treeProjection) referenceRows(tree *parse.Tree) ([]indexdb.RefRecord, []indexdb.AnnotationRecord, error) {
	return indexdb.ProjectReferences(context.Background(), tree, projection.parser, func(_ context.Context, id string) (indexdb.BlockLocation, bool, error) {
		b := treenode.GetBlockTree(id)
		if b == nil {
			return indexdb.BlockLocation{}, false, nil
		}
		return indexdb.BlockLocation{ParentID: b.ParentID, RootID: b.RootID, Path: b.Path}, true, nil
	})
}
func (projection *treeProjection) referencePointers(tree *parse.Tree) (refs []*Ref, annotations []*FileAnnotationRef, err error) {
	rows, ann, err := projection.referenceRows(tree)
	if err != nil {
		return nil, nil, err
	}
	for i := range rows {
		refs = append(refs, &rows[i])
	}
	for i := range ann {
		annotations = append(annotations, &ann[i])
	}
	return refs, annotations, nil
}
func (projection *treeProjection) refsFromTree(tree *parse.Tree) (refs []*Ref, annotations []*FileAnnotationRef) {
	refs, annotations, err := projection.referencePointers(tree)
	if err != nil {
		logging.LogErrorf("project references failed: %s", err)
	}
	return refs, annotations
}

func isRepeatedRef(refs []*Ref, ref *Ref) bool {
	// Repeated references to the same block within a block only count as one reference https://github.com/siyuan-note/siyuan/issues/9670
	for _, r := range refs {
		if r.DefBlockID == ref.DefBlockID && r.BlockID == ref.BlockID {
			return true
		}
	}
	return false
}

func (projection *treeProjection) buildRef(tree *parse.Tree, refNode *ast.Node) *Ref {
	ref, err := indexdb.ProjectReference(context.Background(), tree, refNode, projection.parser, func(ctx context.Context, id string) (indexdb.BlockLocation, bool, error) {
		def := treenode.GetBlockTree(id)
		if def == nil {
			return indexdb.BlockLocation{}, false, nil
		}
		return indexdb.BlockLocation{ParentID: def.ParentID, RootID: def.RootID, Path: def.Path}, true, nil
	})
	if err != nil {
		logging.LogErrorf("reference projection failed: %s", err)
		return nil
	}
	return &ref
}

func buildEmbedRef(tree *parse.Tree, embedNode *ast.Node) *Ref {
	defBlockID := getEmbedRef(embedNode)
	var defBlockParentID, defBlockRootID, defBlockPath string
	defBlock := treenode.GetBlockTree(defBlockID)
	if nil != defBlock {
		defBlockParentID = defBlock.ParentID
		defBlockRootID = defBlock.RootID
		defBlockPath = defBlock.Path
	}

	return &Ref{
		ID:               ast.NewNodeID(),
		DefBlockID:       defBlockID,
		DefBlockParentID: defBlockParentID,
		DefBlockRootID:   defBlockRootID,
		DefBlockPath:     defBlockPath,
		BlockID:          embedNode.ID,
		RootID:           tree.ID,
		Box:              tree.Box,
		Path:             tree.Path,
		Content:          "", // 通过嵌入块构建引用时定义块可能还没有入库，所以这里统一不填充内容
		Markdown:         "",
		Type:             treenode.TypeAbbr(embedNode.Type.String()),
	}
}

func getEmbedRef(embedNode *ast.Node) (queryBlockID string) {
	queryBlockID = treenode.GetEmbedBlockRef(embedNode)
	return
}

func (projection *treeProjection) fromTree(node *ast.Node, tree *parse.Tree) (blocks []*Block, spans []*Span, assets []*Asset, attributes []*Attribute) {
	rootID := tree.Root.ID
	boxID := tree.Box
	p := tree.Path
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		// 构造行级元素
		spanBlocks, spanSpans, spanAssets, spanAttrs, walkStatus := projection.buildSpanFromNode(n, tree, rootID, boxID, p)
		if 0 < len(spanBlocks) {
			blocks = append(blocks, spanBlocks...)
		}
		if 0 < len(spanSpans) {
			spans = append(spans, spanSpans...)
		}
		if 0 < len(spanAssets) {
			assets = append(assets, spanAssets...)
		}
		if 0 < len(spanAttrs) {
			attributes = append(attributes, spanAttrs...)
		}

		// 构造属性
		attrs := buildAttributeFromNode(n, rootID, boxID, p)
		if 0 < len(attrs) {
			attributes = append(attributes, attrs...)
		}
		if -1 != walkStatus {
			return walkStatus
		}

		// 构造块级元素
		if "" == n.ID || !n.IsBlock() {
			return ast.WalkContinue
		}

		b, attrs := projection.buildBlockFromNode(n, tree)
		blocks = append(blocks, b)
		if 0 < len(attrs) {
			attributes = append(attributes, attrs...)
		}
		return ast.WalkContinue
	})
	return
}

func buildAttributeFromNode(n *ast.Node, rootID, boxID, p string) (attributes []*Attribute) {
	switch n.Type {
	case ast.NodeKramdownSpanIAL:
		parentBlock := treenode.ParentBlock(n)
		attrs := parse.IALValMap(n)
		for name, val := range attrs {
			if !isAttr(name) {
				continue
			}

			attr := &Attribute{
				ID:      ast.NewNodeID(),
				Name:    name,
				Value:   val,
				Type:    "s",
				BlockID: parentBlock.ID,
				RootID:  rootID,
				Box:     boxID,
				Path:    p,
			}
			attributes = append(attributes, attr)
		}
	case ast.NodeKramdownBlockIAL:
		attrs := parse.IALValMap(n)
		for name, val := range attrs {
			if !isAttr(name) {
				continue
			}

			attr := &Attribute{
				ID:      ast.NewNodeID(),
				Name:    name,
				Value:   val,
				Type:    "b",
				BlockID: n.ID,
				RootID:  rootID,
				Box:     boxID,
				Path:    p,
			}
			attributes = append(attributes, attr)
		}
	}
	return
}

func isAttr(name string) bool {
	return strings.HasPrefix(name, "custom-") || "name" == name || "alias" == name || "memo" == name || "bookmark" == name || "fold" == name || "heading-fold" == name || "style" == name
}

func (projection *treeProjection) buildSpanFromNode(n *ast.Node, tree *parse.Tree, rootID, boxID, p string) (blocks []*Block, spans []*Span, assets []*Asset, attributes []*Attribute, walkStatus ast.WalkStatus) {
	boxLocalPath := filepath.Join(util.DataDir, boxID)
	docDirLocalPath := filepath.Join(boxLocalPath, p)
	switch n.Type {
	case ast.NodeImage:
		text := n.Text()
		markdown := treenode.ExportNodeStdMd(n, projection.parser)
		parentBlock := treenode.ParentBlock(n)
		span := &Span{
			ID:       ast.NewNodeID(),
			BlockID:  parentBlock.ID,
			RootID:   rootID,
			Box:      boxID,
			Path:     p,
			Content:  text,
			Markdown: markdown,
			Type:     treenode.TypeAbbr(n.Type.String()),
			IAL:      treenode.IALStr(n),
		}
		spans = append(spans, span)
		walkStatus = ast.WalkSkipChildren

		destNode := n.ChildByType(ast.NodeLinkDest)
		if nil == destNode {
			return
		}

		// assetsLinkDestsInTree

		if !util.IsAssetLinkDest(destNode.Tokens) {
			return
		}

		dest := gulu.Str.FromBytes(destNode.Tokens)
		var title string
		if titleNode := n.ChildByType(ast.NodeLinkTitle); nil != titleNode {
			title = gulu.Str.FromBytes(titleNode.Tokens)
		}

		var hash string
		var hashErr error
		if lp := assetLocalPath(dest, boxLocalPath, docDirLocalPath); "" != lp {
			if !gulu.File.IsDir(lp) {
				hash, hashErr = util.GetEtag(lp)
				if nil != hashErr {
					logging.LogErrorf("calc asset [%s] hash failed: %s", lp, hashErr)
				}
			}
		}
		name, _ := util.LastID(dest)
		asset := &Asset{
			ID:      ast.NewNodeID(),
			BlockID: parentBlock.ID,
			RootID:  rootID,
			Box:     boxID,
			DocPath: p,
			Path:    dest,
			Name:    name,
			Title:   title,
			Hash:    hash,
		}
		assets = append(assets, asset)
		return
	case ast.NodeTextMark:
		typ := treenode.TypeAbbr(n.Type.String()) + " " + n.TextMarkType
		text := strings.TrimSuffix(n.Content(), string(gulu.ZWJ))
		markdown := treenode.ExportNodeStdMd(n, projection.parser)
		markdown = strings.ReplaceAll(markdown, string(gulu.ZWJ)+"#", "#")
		parentBlock := treenode.ParentBlock(n)
		span := &Span{
			ID:       ast.NewNodeID(),
			BlockID:  parentBlock.ID,
			RootID:   rootID,
			Box:      boxID,
			Path:     p,
			Content:  text,
			Markdown: markdown,
			Type:     typ,
			IAL:      treenode.IALStr(n),
		}
		spans = append(spans, span)

		if n.IsTextMarkType("a") {
			dest := n.TextMarkAHref
			if util.IsAssetLinkDest([]byte(dest)) {
				var title string
				if titleNode := n.ChildByType(ast.NodeLinkTitle); nil != titleNode {
					title = gulu.Str.FromBytes(titleNode.Tokens)
				}

				var hash string
				var hashErr error
				if lp := assetLocalPath(dest, boxLocalPath, docDirLocalPath); "" != lp {
					if !gulu.File.IsDir(lp) {
						hash, hashErr = util.GetEtag(lp)
						if nil != hashErr {
							logging.LogErrorf("calc asset [%s] hash failed: %s", lp, hashErr)
						}
					}
				}
				name, _ := util.LastID(dest)
				asset := &Asset{
					ID:      ast.NewNodeID(),
					BlockID: parentBlock.ID,
					RootID:  rootID,
					Box:     boxID,
					DocPath: p,
					Path:    dest,
					Name:    name,
					Title:   title,
					Hash:    hash,
				}
				assets = append(assets, asset)
			}
		}
		walkStatus = ast.WalkSkipChildren
		return
	case ast.NodeDocument:
		if asset := docTitleImgAsset(n, boxLocalPath, docDirLocalPath); nil != asset {
			assets = append(assets, asset)
		}
		if tags := docTagSpans(n); 0 < len(tags) {
			spans = append(spans, tags...)
		}
	case ast.NodeInlineHTML, ast.NodeHTMLBlock, ast.NodeIFrame, ast.NodeWidget, ast.NodeAudio, ast.NodeVideo:
		nodes, err := html.ParseFragment(bytes.NewReader(n.Tokens), &html.Node{Type: html.ElementNode})
		if err != nil {
			logging.LogErrorf("parse HTML failed: %s", err)
			walkStatus = ast.WalkContinue
			return
		}
		if 1 > len(nodes) &&
			ast.NodeHTMLBlock != n.Type { // HTML 块若内容为空时无法在数据库中查询到 https://github.com/siyuan-note/siyuan/issues/4691
			walkStatus = ast.WalkContinue
			return
		}

		if ast.NodeHTMLBlock == n.Type || ast.NodeIFrame == n.Type || ast.NodeWidget == n.Type || ast.NodeAudio == n.Type || ast.NodeVideo == n.Type {
			b, attrs := projection.buildBlockFromNode(n, tree)
			blocks = append(blocks, b)
			attributes = append(attributes, attrs...)
		}

		if ast.NodeInlineHTML == n.Type {
			// 没有行级 HTML，只有块级 HTML，这里转换为块
			n.ID = ast.NewNodeID()
			n.SetIALAttr("id", n.ID)
			n.SetIALAttr("updated", n.ID[:14])
			b, attrs := projection.buildBlockFromNode(n, tree)
			b.Type = ast.NodeHTMLBlock.String()
			blocks = append(blocks, b)
			attributes = append(attributes, attrs...)
			walkStatus = ast.WalkContinue
			logging.LogWarnf("inline HTML [%s] is converted to HTML block ", n.Tokens)
			return
		}

		if 1 > len(nodes) {
			walkStatus = ast.WalkContinue
			return
		}

		var src []byte
		for _, attr := range nodes[0].Attr {
			if "src" == attr.Key || strings.HasPrefix(attr.Key, "data-assets") || strings.HasPrefix(attr.Key, "custom-data-assets") {
				src = gulu.Str.ToBytes(attr.Val)
				break
			}
		}
		if 1 > len(src) {
			walkStatus = ast.WalkContinue
			return
		}

		if !util.IsAssetLinkDest(src) {
			walkStatus = ast.WalkContinue
			return
		}

		dest := string(src)
		var hash string
		var hashErr error
		if lp := assetLocalPath(dest, boxLocalPath, docDirLocalPath); "" != lp {
			hash, hashErr = util.GetEtag(lp)
			if nil != hashErr {
				logging.LogErrorf("calc asset [%s] hash failed: %s", lp, hashErr)
			}
		}

		parentBlock := treenode.ParentBlock(n)
		if ast.NodeInlineHTML != n.Type {
			parentBlock = n
		}
		name, _ := util.LastID(dest)
		asset := &Asset{
			ID:      ast.NewNodeID(),
			BlockID: parentBlock.ID,
			RootID:  rootID,
			Box:     boxID,
			DocPath: p,
			Path:    dest,
			Name:    name,
			Title:   "",
			Hash:    hash,
		}
		assets = append(assets, asset)
		walkStatus = ast.WalkSkipChildren
		return
	}
	walkStatus = -1
	return
}

func BuildBlockFromNode(n *ast.Node, tree *parse.Tree) (block *Block) {
	block, _ = legacyTreeProjection().buildBlockFromNode(n, tree)
	return
}

func (projection *treeProjection) buildBlockFromNode(n *ast.Node, tree *parse.Tree) (block *Block, attributes []*Attribute) {
	boxID := tree.Box
	p := tree.Path
	rootID := tree.Root.ID
	name := html.UnescapeString(n.IALAttr("name"))
	alias := html.UnescapeString(n.IALAttr("alias"))
	memo := html.UnescapeString(n.IALAttr("memo"))
	tag := tagFromNode(n)

	var content, fcontent, markdown, parentID string
	ialContent := treenode.IALStr(n)
	hash := treenode.NodeHash(n, tree, projection.parser)
	var length int
	if ast.NodeDocument == n.Type {
		content = n.IALAttr("title")
		fcontent = content
		length = utf8.RuneCountInString(fcontent)
	} else if n.IsContainerBlock() {
		markdown = treenode.ExportNodeStdMd(n, projection.parser)
		if !treenode.IsNodeOCRed(n) {
			util.PushNodeOCRQueue(n)
		}
		content = NodeStaticContent(n, nil, true, indexAssetPath, true)

		fc := treenode.FirstLeafBlock(n)
		if !treenode.IsNodeOCRed(fc) {
			util.PushNodeOCRQueue(fc)
		}
		fcontent = NodeStaticContent(fc, nil, true, false, true)

		parentID = n.Parent.ID
		if h := treenode.HeadingParent(n); nil != h { // 如果在标题块下方，则将标题块作为父节点
			parentID = h.ID
		}
		length = utf8.RuneCountInString(fcontent)
	} else {
		markdown = treenode.ExportNodeStdMd(n, projection.parser)
		if !treenode.IsNodeOCRed(n) {
			util.PushNodeOCRQueue(n)
		}
		content = NodeStaticContent(n, nil, true, indexAssetPath, true)

		parentID = n.Parent.ID
		if h := treenode.HeadingParent(n); nil != h {
			parentID = h.ID
		}
		length = utf8.RuneCountInString(content)
	}

	// 剔除零宽空格 Database index content/markdown values no longer contain zero-width spaces https://github.com/siyuan-note/siyuan/issues/15204
	fcontent = strings.ReplaceAll(fcontent, editor.Zwsp, "")
	content = strings.ReplaceAll(content, editor.Zwsp, "")
	markdown = strings.ReplaceAll(markdown, editor.Zwsp, "")

	// 剔除标签结尾处的零宽连字符 Improve search for emojis in tags https://github.com/siyuan-note/siyuan/issues/15391
	fcontent = strings.ReplaceAll(fcontent, string(gulu.ZWJ)+"#", "#")
	content = strings.ReplaceAll(content, string(gulu.ZWJ)+"#", "#")
	markdown = strings.ReplaceAll(markdown, string(gulu.ZWJ)+"#", "#")

	block = &Block{
		ID:       n.ID,
		ParentID: parentID,
		RootID:   rootID,
		Hash:     hash,
		Box:      boxID,
		Path:     p,
		HPath:    tree.HPath,
		Name:     name,
		Alias:    alias,
		Memo:     memo,
		Tag:      tag,
		Content:  content,
		FContent: fcontent,
		Markdown: markdown,
		Length:   length,
		Type:     treenode.TypeAbbr(n.Type.String()),
		SubType:  treenode.SubTypeAbbr(n),
		IAL:      ialContent,
		Sort:     nSort(n),
		Created:  util.TimeFromID(n.ID),
		Updated:  n.IALAttr("updated"),
	}

	attrs := parse.IAL2Map(n.KramdownIAL)
	for attrName, attrVal := range attrs {
		if !isAttr(attrName) {
			continue
		}

		attr := &Attribute{
			ID:      ast.NewNodeID(),
			Name:    attrName,
			Value:   attrVal,
			Type:    "b",
			BlockID: n.ID,
			RootID:  rootID,
			Box:     boxID,
			Path:    p,
		}
		attributes = append(attributes, attr)
	}
	return
}

func tagFromNode(node *ast.Node) (ret string) {
	tagBuilder := bytes.Buffer{}

	if ast.NodeDocument == node.Type {
		tagIAL := html.UnescapeString(node.IALAttr("tags"))
		tags := strings.Split(tagIAL, ",")
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if "" == t {
				continue
			}
			tagBuilder.WriteString("#")
			tagBuilder.WriteString(t)
			tagBuilder.WriteString("# ")
		}
		return strings.TrimSpace(tagBuilder.String())
	}

	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if n.IsTextMarkType("tag") {
			tagBuilder.WriteString("#")
			tagBuilder.WriteString(n.Content())
			tagBuilder.WriteString("# ")
		}
		return ast.WalkContinue
	})
	return strings.TrimSpace(tagBuilder.String())
}

func deleteByBoxTx(tx *sql.Tx, box string) (err error) {
	if err = indexdb.ApplyMainMutation(nativeSQLContext(), tx, indexdb.MainMutation{Action: "delete_box", Box: box}, caseSensitive); err != nil {
		return err
	}
	ClearCache()
	return nil

}

func deleteBlocksByIDs(tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		removeBlockCache(id)
	}
	return indexdb.DeleteIndexedBlocksByIDs(context.Background(), tx, ids, caseSensitive)
}

func deleteBlocksByBoxTx(tx *sql.Tx, box string) (err error) {
	stmt := "DELETE FROM blocks WHERE box = ?"
	if err = execStmtTx(tx, stmt, box); err != nil {
		return
	}
	stmt = "DELETE FROM blocks_fts WHERE box = ?"
	if err = execStmtTx(tx, stmt, box); err != nil {
		return
	}
	if !caseSensitive {
		stmt = "DELETE FROM blocks_fts_case_insensitive WHERE box = ?"
		if err = execStmtTx(tx, stmt, box); err != nil {
			return
		}
	}
	ClearCache()
	return
}

func deleteSpansByRootID(tx *sql.Tx, rootID string) (err error) {
	stmt := "DELETE FROM spans WHERE root_id =?"
	err = execStmtTx(tx, stmt, rootID)
	return
}

func deleteSpansByBoxTx(tx *sql.Tx, box string) (err error) {
	stmt := "DELETE FROM spans WHERE box = ?"
	err = execStmtTx(tx, stmt, box)
	return
}

func deleteAssetsByRootID(tx *sql.Tx, rootID string) (err error) {
	stmt := "DELETE FROM assets WHERE root_id = ?"
	err = execStmtTx(tx, stmt, rootID)
	return
}

func deleteAssetsByBoxTx(tx *sql.Tx, box string) (err error) {
	stmt := "DELETE FROM assets WHERE box = ?"
	err = execStmtTx(tx, stmt, box)
	return
}

func deleteAttributesByRootID(tx *sql.Tx, rootID string) (err error) {
	stmt := "DELETE FROM attributes WHERE root_id = ?"
	err = execStmtTx(tx, stmt, rootID)
	return

}

func deleteAttributesByBoxTx(tx *sql.Tx, box string) (err error) {
	stmt := "DELETE FROM attributes WHERE box = ?"
	err = execStmtTx(tx, stmt, box)
	return
}

func deleteRefsByPath(tx *sql.Tx, box, path string) (err error) {
	stmt := "DELETE FROM refs WHERE box = ? AND path = ?"
	err = execStmtTx(tx, stmt, box, path)
	return
}

func deleteRefsByPathTx(tx *sql.Tx, box, path string) (err error) {
	stmt := "DELETE FROM refs WHERE box = ? AND path = ?"
	err = execStmtTx(tx, stmt, box, path)
	return
}

func deleteRefsByBoxTx(tx *sql.Tx, box string) (err error) {
	if err = indexdb.ApplyMainMutation(nativeSQLContext(), tx, indexdb.MainMutation{Action: "delete_box_refs", Box: box}, caseSensitive); err != nil {
		return err
	}
	return nil

}

func deleteBlockRefsByBoxTx(tx *sql.Tx, box string) (err error) {
	stmt := "DELETE FROM refs WHERE box = ?"
	err = execStmtTx(tx, stmt, box)
	return
}

func deleteFileAnnotationRefsByPath(tx *sql.Tx, box, path string) (err error) {
	stmt := "DELETE FROM file_annotation_refs WHERE box = ? AND path = ?"
	err = execStmtTx(tx, stmt, box, path)
	return
}

func deleteFileAnnotationRefsByPathTx(tx *sql.Tx, box, path string) (err error) {
	stmt := "DELETE FROM file_annotation_refs WHERE box = ? AND path = ?"
	err = execStmtTx(tx, stmt, box, path)
	return
}

func deleteFileAnnotationRefsByBoxTx(tx *sql.Tx, box string) (err error) {
	stmt := "DELETE FROM file_annotation_refs WHERE box = ?"
	err = execStmtTx(tx, stmt, box)
	return
}

func deleteByRootID(tx *sql.Tx, rootID string, context map[string]interface{}) (err error) {
	if err = indexdb.ApplyMainMutation(nativeSQLContext(), tx, indexdb.MainMutation{Action: "delete_id", ID: rootID}, caseSensitive); err != nil {
		return err
	}
	ClearCache()
	eventbus.Publish(eventbus.EvtSQLDeleteBlocks, context, rootID)
	return nil

}

func batchDeleteByRootIDs(tx *sql.Tx, rootIDs []string, context map[string]interface{}) (err error) {
	if err = indexdb.ApplyMainMutation(nativeSQLContext(), tx, indexdb.MainMutation{Action: "delete_ids", IDs: rootIDs}, caseSensitive); err != nil {
		return err
	}
	if len(rootIDs) == 0 {
		return nil
	}
	ClearCache()
	eventbus.Publish(eventbus.EvtSQLDeleteBlocks, context, fmt.Sprintf("%d", len(rootIDs)))
	return nil

}

func batchDeleteByPathPrefix(tx *sql.Tx, boxID, pathPrefix string) (err error) {
	if err = indexdb.ApplyMainMutation(nativeSQLContext(), tx, indexdb.MainMutation{Action: "delete", Box: boxID, Path: pathPrefix}, caseSensitive); err != nil {
		return err
	}
	ClearCache()
	return nil

}

func batchUpdatePath(tx *sql.Tx, tree *parse.Tree, context map[string]interface{}) (err error) {
	if err = indexdb.UpdateBlockPaths(nativeSQLContext(), tx, indexdb.MainMutation{Action: "rename_sub_tree", ID: tree.ID, Box: tree.Box, Path: tree.Path, HPath: tree.HPath, IAL: treenode.IALStr(tree.Root)}, caseSensitive); err != nil {
		return err
	}
	ClearCache()
	evtHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tree.ID)))[:7]
	eventbus.Publish(eventbus.EvtSQLUpdateBlocksHPaths, context, 1, evtHash)
	return nil

}

func batchUpdateHPath(tx *sql.Tx, tree *parse.Tree, context map[string]interface{}) (err error) {
	if err = indexdb.UpdateBlockPaths(nativeSQLContext(), tx, indexdb.MainMutation{Action: "rename", ID: tree.ID, Box: tree.Box, Path: tree.Path, HPath: tree.HPath, IAL: treenode.IALStr(tree.Root)}, caseSensitive); err != nil {
		return err
	}
	ClearCache()
	evtHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tree.ID)))[:7]
	eventbus.Publish(eventbus.EvtSQLUpdateBlocksHPaths, context, 1, evtHash)
	return nil

}

func CloseDatabase() {
	if e := closeLegacyHistoryQueue(context.Background()); e != nil {
		logging.LogErrorf("close history queue failed: %s", e)
		return
	}
	defer CloseLegacyCaches()
	if err := closeDatabase(); err != nil {
		logging.LogErrorf("close database failed: %s", err)
		return
	}
	if err := historyDB.Close(); err != nil {
		logging.LogErrorf("close history database failed: %s", err)
		return
	}
	if err := assetContentDB.Close(); err != nil {
		logging.LogErrorf("close asset content database failed: %s", err)
		return
	}
	treenode.CloseDatabase()
	logging.LogInfof("closed database")
}

func queryRow(query string, args ...interface{}) *sql.Row {
	query = strings.TrimSpace(query)
	if "" == query {
		logging.LogErrorf("statement is empty")
		return nil
	}
	if nil == db {
		return nil
	}
	row, err := indexdb.QueryRow(context.Background(), db, query, args...)
	if err != nil {
		logging.LogErrorf("query row failed: %s", err)
		return nil
	}
	return row
}

func query(query string, args ...interface{}) (*sql.Rows, error) {
	query = strings.TrimSpace(query)
	if "" == query {
		return nil, errors.New("statement is empty")
	}
	if nil == db {
		return nil, errors.New("database is nil")
	}
	return indexdb.Query(context.Background(), db, query, args...)
}

func beginTx() (tx *sql.Tx, err error) {
	if tx, err = indexdb.Begin(context.Background(), db); err != nil {
		logging.LogErrorf("begin tx failed: %s\n  %s", err, logging.ShortStack())
		if strings.Contains(err.Error(), "database is locked") {
			os.Exit(logging.ExitCodeReadOnlyDatabase)
		}
	}
	return
}

// Only the legacy facade supplies a process background context.
func nativeSQLContext() context.Context { return context.Background() }

func commitTx(tx *sql.Tx) (err error) {
	if nil == tx {
		logging.LogErrorf("tx is nil")
		return errors.New("tx is nil")
	}

	if err = indexdb.Commit(tx); err != nil {
		logging.LogErrorf("commit tx failed: %s\n  %s", err, logging.ShortStack())
	}
	return
}

func beginHistoryTx() (tx *sql.Tx, err error) {
	if tx, err = indexdb.Begin(context.Background(), historyDB); err != nil {
		logging.LogErrorf("begin history tx failed: %s\n  %s", err, logging.ShortStack())
		if strings.Contains(err.Error(), "database is locked") {
			os.Exit(logging.ExitCodeReadOnlyDatabase)
		}
	}
	return
}

func commitHistoryTx(tx *sql.Tx) (err error) {
	if nil == tx {
		logging.LogErrorf("tx is nil")
		return errors.New("tx is nil")
	}

	if err = indexdb.Commit(tx); err != nil {
		logging.LogErrorf("commit tx failed: %s\n  %s", err, logging.ShortStack())
	}
	return
}

func beginAssetContentTx() (tx *sql.Tx, err error) {
	if tx, err = indexdb.Begin(context.Background(), assetContentDB); err != nil {
		logging.LogErrorf("begin asset content tx failed: %s\n  %s", err, logging.ShortStack())
		if strings.Contains(err.Error(), "database is locked") {
			os.Exit(logging.ExitCodeReadOnlyDatabase)
		}
	}
	return
}

func commitAssetContentTx(tx *sql.Tx) (err error) {
	if nil == tx {
		logging.LogErrorf("tx is nil")
		return errors.New("tx is nil")
	}

	if err = indexdb.Commit(tx); err != nil {
		logging.LogErrorf("commit tx failed: %s\n  %s", err, logging.ShortStack())
	}
	return
}

func prepareExecInsertTx(tx *sql.Tx, stmtSQL string, args []interface{}) (err error) {
	stmt, err := tx.Prepare(stmtSQL)
	if err != nil {
		return
	}
	if _, err = stmt.Exec(args...); err != nil {
		logging.LogErrorf("exec database stmt [%s] failed: %s", stmtSQL, err)
		return
	}
	return
}

func execStmtTx(tx *sql.Tx, stmt string, args ...interface{}) (err error) {
	if _, err = tx.Exec(stmt, args...); err != nil {
		if strings.Contains(err.Error(), "database disk image is malformed") {
			tx.Rollback()
			closeDatabase()
			removeDatabaseFile()
			logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "database disk image [%s] is malformed, please restart SiYuan kernel to rebuild it", util.DBPath)
		}
		logging.LogErrorf("exec database stmt [%s] failed: %s\n  %s", stmt, err, logging.ShortStack())
		return
	}
	return
}

func nSort(n *ast.Node) int {
	switch n.Type {
	// 以下为块级元素
	case ast.NodeHeading:
		return 5
	case ast.NodeParagraph:
		return 10
	case ast.NodeCodeBlock:
		return 10
	case ast.NodeMathBlock:
		return 10
	case ast.NodeTable:
		return 10
	case ast.NodeHTMLBlock:
		return 10
	case ast.NodeList:
		return 20
	case ast.NodeListItem:
		return 20
	case ast.NodeBlockquote:
		return 20
	case ast.NodeSuperBlock:
		return 30
	case ast.NodeAttributeView:
		return 30
	case ast.NodeDocument:
		return 0
	case ast.NodeText, ast.NodeTextMark:
		if n.IsTextMarkType("tag") {
			return 205
		}
		return 200
	}
	return 100
}

func ialAttr(ial, name string) (ret string) {
	idx := strings.Index(ial, name)
	if 0 > idx {
		return ""
	}
	ret = ial[idx+len(name)+2:]
	ret = ret[:strings.Index(ret, "\"")]
	return
}

func removeDatabaseFile() (err error) {
	err = os.RemoveAll(util.DBPath)
	if err != nil {
		return
	}
	err = os.RemoveAll(util.DBPath + "-shm")
	if err != nil {
		return
	}
	err = os.RemoveAll(util.DBPath + "-wal")
	if err != nil {
		return
	}
	return
}

func closeDatabase() (err error) {
	if nil == db {
		return
	}

	err = db.Close()
	debug.FreeOSMemory()
	runtime.GC() // 没有这句的话文件句柄不会释放，后面就无法删除文件
	return
}

func SQLTemplateFuncs(templateFuncMap *template.FuncMap) {
	(*templateFuncMap)["queryBlocks"] = func(stmt string, args ...string) (retBlocks []*Block) {
		for _, arg := range args {
			stmt = strings.Replace(stmt, "?", arg, 1)
		}
		retBlocks = SelectBlocksRawStmt(stmt, 1, 512)
		return
	}
	(*templateFuncMap)["getBlock"] = func(arg any) (retBlock *Block) {
		switch v := arg.(type) {
		case string:
			retBlock = GetBlock(v)
		case map[string]interface{}:
			if id, ok := v["id"]; ok {
				retBlock = GetBlock(id.(string))
			}
		}
		return
	}
	(*templateFuncMap)["querySpans"] = func(stmt string, args ...string) (retSpans []*Span) {
		for _, arg := range args {
			stmt = strings.Replace(stmt, "?", arg, 1)
		}
		retSpans = SelectSpansRawStmt(stmt, 512)
		return
	}
	(*templateFuncMap)["querySQL"] = func(stmt string) (ret []map[string]interface{}) {
		ret, _ = Query(stmt, 1024)
		return
	}
}

func Vacuum() {
	if nil != db {
		if _, err := db.Exec("VACUUM"); nil != err {
			logging.LogErrorf("vacuum database failed: %s", err)
		}
	}
	if nil != historyDB {
		if _, err := historyDB.Exec("VACUUM"); nil != err {
			logging.LogErrorf("vacuum history database failed: %s", err)
		}
	}
	if nil != assetContentDB {
		if _, err := assetContentDB.Exec("VACUUM"); nil != err {
			logging.LogErrorf("vacuum asset content database failed: %s", err)
		}
	}
	return
}
