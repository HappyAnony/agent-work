// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type shorthandPorts struct {
	readDir func(string) ([]os.DirEntry, error)
	read    func(string) ([]byte, error)
	rename  func(string, string) error
	remove  func(string) error
	create  func(context.Context, string, string, string, string) (string, error)
	lookup  func(string, string) *treenode.BlockTree
	load    func(*treenode.BlockTree) (*parse.Tree, error)
	stage   func(*parse.Tree) error
	sort    func(*Box, string) error
}

type shorthandOwner struct {
	owner                      *AppConf
	workspace, data, shortcuts string
	boxID                      string
	box                        *Box
	boxIdentity                Box
	ports                      shorthandPorts
}

func legacyShorthandPorts() shorthandPorts {
	return shorthandPorts{
		readDir: os.ReadDir, read: os.ReadFile, rename: os.Rename, remove: os.Remove,
		create: func(ctx context.Context, box, hPath, content, parent string) (string, error) {
			return CreateWithMarkdownContext(ctx, "", box, hPath, content, parent, "", false, "")
		},
		lookup: treenode.GetBlockTreeRootByHPath,
		load:   loadTreeByBlockTree,
		stage: func(tree *parse.Tree) error {
			indexWriteTreeIndexQueue(tree)
			return nil
		},
		sort: func(box *Box, id string) error {
			box.setSortByConf("/", id)
			return nil
		},
	}
}

func captureShorthandOwner(ctx context.Context, boxID string) (*shorthandOwner, error) {
	if ctx == nil {
		return nil, errors.New("shorthand_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o := &shorthandOwner{owner: Conf, workspace: util.WorkspaceDir, data: util.DataDir, shortcuts: util.ShortcutsPath, boxID: boxID, ports: legacyShorthandPorts()}
	if o.owner == nil || !filepath.IsAbs(o.workspace) || !filepath.IsAbs(o.data) || !filepath.IsAbs(o.shortcuts) {
		return nil, errors.New("shorthand_owner_unbound")
	}
	o.box = o.owner.Box(boxID)
	if o.box == nil {
		return nil, errors.New("shorthand_box_unbound")
	}
	o.boxIdentity = *o.box
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || !o.current() {
		return nil, errors.New("shorthand_owner_changed")
	}
	return o, nil
}

func (o *shorthandOwner) current() bool {
	if o == nil || o.owner == nil || o.owner != Conf || o.workspace != util.WorkspaceDir || o.data != util.DataDir || o.shortcuts != util.ShortcutsPath {
		return false
	}
	box := o.owner.Box(o.boxID)
	return box != nil && *box == o.boxIdentity
}

func (o *shorthandOwner) guard(ctx context.Context) error {
	if o == nil || o.owner == nil || ctx == nil {
		return errors.New("shorthand_owner_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || !o.current() {
		return errors.New("shorthand_owner_changed")
	}
	return ctx.Err()
}

func (o *shorthandOwner) call(ctx context.Context, fn func() error) error {
	if err := o.guard(ctx); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return o.guard(ctx)
}

type shorthandResult struct {
	ids []string
	err error
}

func (o *shorthandOwner) run(ctx context.Context, hPath, parentID string) ([]string, error) {
	result := make(chan shorthandResult, 1)
	accepted := o.owner.startRepoBackground(func(life context.Context) {
		work, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(life, cancel)
		defer func() {
			stop()
			cancel()
			if recovered := recover(); recovered != nil {
				result <- shorthandResult{err: errors.New("shorthand_operation_panicked")}
			}
		}()
		ids, err := o.move(work, hPath, parentID)
		result <- shorthandResult{ids: ids, err: err}
	})
	if !accepted {
		return nil, errors.New("shorthand_owner_closed")
	}
	r := <-result
	return r.ids, r.err
}

func MoveLocalShorthandsContext(ctx context.Context, boxID, hPath, parentID string) ([]string, error) {
	o, err := captureShorthandOwner(ctx, boxID)
	if err != nil {
		return nil, err
	}
	return o.run(ctx, hPath, parentID)
}

func (o *shorthandOwner) move(ctx context.Context, hPath, parentID string) (retIDs []string, err error) {
	shorthandsDir := filepath.Join(o.shortcuts, "shorthands")
	assetsDir := filepath.Join(o.data, "assets")
	if !util.IsSubPath(o.shortcuts, shorthandsDir) || !util.IsSubPath(o.data, assetsDir) {
		return nil, errors.New("shorthand_path_invalid")
	}
	var entries []os.DirEntry
	err = o.call(ctx, func() (readErr error) { entries, readErr = o.ports.readDir(shorthandsDir); return })
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Name() != "assets" || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		assetRoot := filepath.Join(shorthandsDir, entry.Name())
		var assets []os.DirEntry
		readErr := o.call(ctx, func() (e error) { assets, e = o.ports.readDir(assetRoot); return })
		if readErr != nil {
			if ctx.Err() != nil || o.guard(ctx) != nil {
				return nil, errors.Join(readErr, ctx.Err())
			}
			logging.LogErrorf("read dir [%s] failed: %s", assetRoot, readErr)
			continue
		}
		for _, asset := range assets {
			if asset.IsDir() || asset.Type()&os.ModeSymlink != 0 || !filepath.IsLocal(asset.Name()) || filepath.Base(asset.Name()) != asset.Name() {
				continue
			}
			src, dst := filepath.Join(assetRoot, asset.Name()), filepath.Join(assetsDir, asset.Name())
			renameErr := o.call(ctx, func() error { return o.ports.rename(src, dst) })
			if renameErr != nil {
				if ctx.Err() != nil || o.guard(ctx) != nil {
					return nil, errors.Join(renameErr, ctx.Err())
				}
				logging.LogErrorf("rename file [%s] to [%s] failed: %s", src, dst, renameErr)
			}
		}
	}

	var toRemove []string
	readMarkdown := func(entry os.DirEntry) (string, string, bool, error) {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".md" || !filepath.IsLocal(entry.Name()) || filepath.Base(entry.Name()) != entry.Name() {
			return "", "", false, nil
		}
		p := filepath.Join(shorthandsDir, entry.Name())
		var data []byte
		readErr := o.call(ctx, func() (e error) { data, e = o.ports.read(p); return })
		if readErr != nil {
			if ctx.Err() != nil || o.guard(ctx) != nil {
				return "", "", false, errors.Join(readErr, ctx.Err())
			}
			logging.LogErrorf("read file [%s] failed: %s", p, readErr)
			return "", "", false, nil
		}
		return p, string(bytes.TrimSpace(data)), true, nil
	}

	if hPath == "" {
		for _, entry := range entries {
			p, content, ok, readErr := readMarkdown(entry)
			if readErr != nil {
				return retIDs, readErr
			}
			if !ok {
				continue
			}
			if content == "" {
				toRemove = append(toRemove, p)
				continue
			}
			millis, parseErr := strconv.ParseInt(strings.TrimSuffix(entry.Name(), ".md"), 10, 64)
			if parseErr != nil {
				logging.LogErrorf("parse [%s] to int failed: %s", entry.Name(), parseErr)
				continue
			}
			docPath := "/" + time.UnixMilli(millis).Format("2006-01-02 15:04:05")
			var id string
			if err = o.call(ctx, func() (e error) { id, e = o.ports.create(ctx, o.boxID, docPath, content, parentID); return }); err != nil {
				return retIDs, err
			}
			retIDs = append(retIDs, id)
			toRemove = append(toRemove, p)
			if err = o.call(ctx, func() error { return o.ports.sort(o.box, id) }); err != nil {
				return retIDs, err
			}
		}
	} else {
		if !strings.HasPrefix(hPath, "/") {
			hPath = "/" + hPath
		}
		var buff bytes.Buffer
		for _, entry := range entries {
			p, content, ok, readErr := readMarkdown(entry)
			if readErr != nil {
				return retIDs, readErr
			}
			if !ok {
				continue
			}
			toRemove = append(toRemove, p)
			if content != "" {
				buff.WriteString(content)
				buff.WriteString("\n\n")
			}
		}
		if buff.Len() > 0 {
			var bt *treenode.BlockTree
			if err = o.call(ctx, func() error { bt = o.ports.lookup(o.boxID, hPath); return nil }); err != nil {
				return nil, err
			}
			if bt == nil {
				var id string
				if err = o.call(ctx, func() (e error) { id, e = o.ports.create(ctx, o.boxID, hPath, buff.String(), parentID); return }); err != nil {
					return nil, err
				}
				retIDs = append(retIDs, id)
			} else {
				var tree *parse.Tree
				if err = o.call(ctx, func() (e error) { tree, e = o.ports.load(bt); return }); err != nil {
					return nil, err
				}
				if tree == nil || tree.Root == nil {
					return nil, errors.New("shorthand_tree_invalid")
				}
				input := parse.Parse("", buff.Bytes(), util.NewStdLute().ParseOptions)
				if input != nil {
					var nodes []*ast.Node
					for n := input.Root.FirstChild; n != nil; n = n.Next {
						nodes = append(nodes, n)
					}
					last := tree.Root.LastChild
					if last == nil {
						for _, n := range nodes {
							tree.Root.AppendChild(n)
						}
					} else {
						slices.Reverse(nodes)
						for _, n := range nodes {
							last.InsertAfter(n)
						}
					}
				}
				if err = o.call(ctx, func() error { return o.ports.stage(tree) }); err != nil {
					return nil, err
				}
			}
		}
	}

	for _, p := range toRemove {
		removeErr := o.call(ctx, func() error { return o.ports.remove(p) })
		if removeErr != nil {
			if ctx.Err() != nil || o.guard(ctx) != nil {
				return retIDs, errors.Join(removeErr, ctx.Err())
			}
			logging.LogErrorf("remove file [%s] failed: %s", p, removeErr)
		}
	}
	return retIDs, nil
}
