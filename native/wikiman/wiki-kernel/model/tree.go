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
	"context"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type resetTreeServices struct {
	NewID    func() string
	Now      func() time.Time
	Untitled func() string
	Bind     func(context.Context, string, string) error
}

func resetTree(tree *parse.Tree, titleSuffix string, removeAvBinding bool) {
	if e := resetTreeWithServices(context.Background(), tree, titleSuffix, removeAvBinding, resetTreeServices{NewID: ast.NewNodeID, Now: time.Now, Untitled: func() string { return Conf.language(16) }, Bind: func(ctx context.Context, a, b string) error { av.UpsertBlockRel(a, b); return nil }}); e != nil {
		logging.LogErrorf("reset tree failed: %s", e)
	}
}
func resetTreeWithServices(ctx context.Context, tree *parse.Tree, titleSuffix string, removeAvBinding bool, s resetTreeServices) (err error) {
	if s.NewID == nil || s.Now == nil || s.Untitled == nil || s.Bind == nil {
		return errors.New("reset_tree_unbound")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	tree.ID = s.NewID()
	tree.Root.ID = tree.ID
	title := tree.Root.IALAttr("title")
	if "" != titleSuffix {
		if t, parseErr := time.Parse("20060102150405", util.TimeFromID(tree.ID)); nil == parseErr {
			titleSuffix += " " + t.Format("2006-01-02 15:04:05")
		} else {
			titleSuffix = "Duplicated " + s.Now().Format("2006-01-02 15:04:05")
		}
		titleSuffix = "(" + titleSuffix + ")"
		titleSuffix = " " + titleSuffix
		if s.Untitled() == title {
			titleSuffix = ""
		}
	}
	tree.Root.SetIALAttr("id", tree.ID)
	tree.Root.SetIALAttr("title", title+titleSuffix)
	tree.Root.RemoveIALAttr("scroll")
	p := path.Join(path.Dir(tree.Path), tree.ID) + util.DocExt
	tree.Path = p
	tree.HPath = tree.HPath + " " + titleSuffix

	// 收集所有引用
	refIDs := map[string]string{}
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || !treenode.IsBlockRef(n) {
			return ast.WalkContinue
		}
		defID, _, _ := treenode.GetBlockRef(n)
		if "" == defID {
			return ast.WalkContinue
		}
		refIDs[defID] = "1"
		return ast.WalkContinue
	})

	// 重置块 ID
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || ast.NodeDocument == n.Type {
			return ast.WalkContinue
		}
		if n.IsBlock() && "" != n.ID {
			newID := s.NewID()
			if "1" == refIDs[n.ID] {
				// 如果是文档自身的内部引用
				refIDs[n.ID] = newID
			}
			n.ID = newID
			n.SetIALAttr("id", n.ID)
		}
		return ast.WalkContinue
	})

	// 重置内部引用
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || !treenode.IsBlockRef(n) {
			return ast.WalkContinue
		}
		defID, _, _ := treenode.GetBlockRef(n)
		if "" == defID {
			return ast.WalkContinue
		}
		if "1" != refIDs[defID] {
			if ast.NodeTextMark == n.Type {
				n.TextMarkBlockRefID = refIDs[defID]
			}
		}
		return ast.WalkContinue
	})

	var attrViewIDs []string
	// 绑定镜像数据库
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeAttributeView == n.Type {
			if err = s.Bind(ctx, n.AttributeViewID, n.ID); err != nil {
				return ast.WalkStop
			}
			attrViewIDs = append(attrViewIDs, n.AttributeViewID)
		}
		return ast.WalkContinue
	})

	if removeAvBinding {
		// 清空文档绑定的数据库
		tree.Root.RemoveIALAttr(av.NodeAttrNameAvs)
	}
	return err
}

func pagedPaths(localPath string, pageSize int) (ret map[int][]string) {
	ret = map[int][]string{}
	page := 1
	filelock.Walk(localPath, func(path string, d fs.DirEntry, err error) error {
		if nil != err || nil == d {
			return nil
		}

		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !util.HasDocExt(d.Name()) {
			return nil
		}

		ret[page] = append(ret[page], path)
		if pageSize <= len(ret[page]) {
			page++
		}
		return nil
	})
	return
}

func loadTree(localPath string, luteEngine *lute.Lute) (ret *parse.Tree, err error) {
	data, err := filelock.ReadFile(localPath)
	if err != nil {
		logging.LogErrorf("get data [path=%s] failed: %s", localPath, err)
		return
	}

	ret, err = dataparser.ParseJSONWithoutFix(data, luteEngine.ParseOptions)
	if err != nil {
		logging.LogErrorf("parse json to tree [%s] failed: %s", localPath, err)
		return
	}
	return
}

var (
	ErrBoxNotFound   = errors.New("notebook not found")
	ErrBlockNotFound = editcore.ErrBlockNotFound
	ErrTreeNotFound  = errors.New("tree not found")
	ErrIndexing      = errors.New("indexing")
)

func LoadTreeByBlockIDWithReindex(id string) (*parse.Tree, error) {
	return LoadTreeByBlockIDWithReindexContext(context.Background(), id)
}
func LoadTreeByBlockID(id string) (*parse.Tree, error) {
	return LoadTreeByBlockIDContext(context.Background(), id)
}

func loadTreeByBlockTree(bt *treenode.BlockTree) (ret *parse.Tree, err error) {
	luteEngine := util.NewLute()
	ret, err = filesys.LoadTree(bt.BoxID, bt.Path, luteEngine)
	return
}

func indexTreeInFilesystem(id string) {
	if _, err := LoadTreeByBlockIDWithReindexContext(context.Background(), id); err != nil {
		logging.LogErrorf("filesystem tree lookup: %s", err)
	}
}

func loadParentTree(tree *parse.Tree) (ret *parse.Tree) {
	boxDir := filepath.Join(util.DataDir, tree.Box)
	parentDir := path.Dir(tree.Path)
	if parentDir == boxDir || parentDir == "/" {
		return
	}

	luteEngine := lute.New()
	parentPath := parentDir + util.DocExt
	ret, _ = filesys.LoadTree(tree.Box, parentPath, luteEngine)
	return
}
