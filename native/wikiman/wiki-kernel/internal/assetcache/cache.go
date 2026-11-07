// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original cache/asset.go algorithms, without a global data path.
package assetcache

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/88250/lute/ast"
)

type Asset struct {
	HName   string `json:"hName"`
	Path    string `json:"path"`
	Updated int64  `json:"updated"`
}
type Hash struct {
	Hash string `json:"hash"`
	Path string `json:"path"`
}

var ErrClosed = errors.New("asset_cache_closed")
var ErrSourceChanged = errors.New("asset_cache_source_changed")

type loadIO struct {
	readDir func(fs.FS, string) ([]fs.DirEntry, error)
	stat    func(fs.FS, string) (fs.FileInfo, error)
}

type manifestEntry struct {
	name  string
	info  fs.FileInfo
	asset *Asset
}

type Cache struct {
	mu     sync.Mutex
	assets map[string]Asset
	hashes map[string]Hash
	closed bool
}

func RemoveID(name string) string {
	ext := path.Ext(name)
	if ext == "." {
		ext = ""
	}
	base := strings.TrimSuffix(name, ext)
	if len(base) > 23 && ast.IsNodeIDPattern(base[len(base)-22:]) {
		base = base[:len(base)-23]
	}
	return base + ext
}

// Load accepts an FS rooted at assets/, not a process-global workspace. Failed
// scans never replace a valid snapshot. Hidden/.sya exclusions preserve the UI.
func (c *Cache) Load(ctx context.Context, root fs.FS, hidden func(string) bool) error {
	return c.loadWithIO(ctx, root, hidden, loadIO{})
}

func sameManifestInfo(a, b fs.FileInfo) bool {
	if a == nil || b == nil || a.IsDir() != b.IsDir() || a.Mode().Type() != b.Mode().Type() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	if a.Sys() != nil && b.Sys() != nil {
		return os.SameFile(a, b)
	}
	return true
}

func assetManifest(ctx context.Context, root fs.FS, hidden func(string) bool, op loadIO) ([]manifestEntry, error) {
	if root == nil {
		return nil, fs.ErrInvalid
	}
	if op.readDir == nil {
		op.readDir = fs.ReadDir
	}
	if op.stat == nil {
		op.stat = fs.Stat
	}
	rootInfo, err := op.stat(root, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return []manifestEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, ErrSourceChanged
	}
	manifest := []manifestEntry{{name: ".", info: rootInfo}}
	count := 1
	var scan func(string) error
	scan = func(folder string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := op.stat(root, folder)
		if err != nil {
			return err
		}
		if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
			return ErrSourceChanged
		}
		entries, err := op.readDir(root, folder)
		if err != nil {
			return err
		}
		after, err := op.stat(root, folder)
		if err != nil {
			return err
		}
		if !sameManifestInfo(before, after) {
			return ErrSourceChanged
		}
		for _, entry := range entries {
			if err = ctx.Err(); err != nil {
				return err
			}
			count++
			if count > 100000 {
				return errors.New("asset_cache_budget")
			}
			name := path.Join(folder, entry.Name())
			if entry.Type()&fs.ModeSymlink != 0 {
				return errors.New("asset_symlink_rejected")
			}
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				info, statErr := op.stat(root, name)
				if statErr != nil {
					return statErr
				}
				if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || info.Mode().Type() != entry.Type() {
					return ErrSourceChanged
				}
				manifest = append(manifest, manifestEntry{name: name, info: info})
				if err = scan(name); err != nil {
					return err
				}
				continue
			}
			if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), ".sya") || (hidden != nil && hidden(name)) {
				continue
			}
			info, statErr := op.stat(root, name)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return errors.New("asset_symlink_rejected")
			}
			if !info.Mode().IsRegular() || info.Mode().Type() != entry.Type() {
				return errors.New("asset_not_regular")
			}
			key := "assets/" + name
			asset := Asset{HName: RemoveID(entry.Name()), Path: key, Updated: info.ModTime().Unix()}
			manifest = append(manifest, manifestEntry{name: name, info: info, asset: &asset})
		}
		return ctx.Err()
	}
	if err = scan("."); err != nil {
		return nil, err
	}
	for _, entry := range manifest {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		info, statErr := op.stat(root, entry.name)
		if statErr != nil {
			return nil, statErr
		}
		if !sameManifestInfo(info, entry.info) {
			return nil, ErrSourceChanged
		}
	}
	return manifest, ctx.Err()
}

func (c *Cache) loadWithIO(ctx context.Context, root fs.FS, hidden func(string) bool, op loadIO) error {
	manifest, err := assetManifest(ctx, root, hidden, op)
	if err != nil {
		return err
	}
	next := map[string]Asset{}
	for _, entry := range manifest {
		if entry.asset != nil {
			next[entry.asset.Path] = *entry.asset
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}
	c.assets = next
	c.hashes = map[string]Hash{}
	return nil
}
func (c *Cache) Snapshot() map[string]Asset {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := map[string]Asset{}
	if c.closed {
		return ret
	}
	for k, v := range c.assets {
		ret[k] = v
	}
	return ret
}
func (c *Cache) Exists(p string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.assets[p]
	return !c.closed && ok
}
func (c *Cache) Remove(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.assets, p)
	for h, v := range c.hashes {
		if v.Path == p {
			delete(c.hashes, h)
		}
	}
}
func (c *Cache) SetHash(hash, p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.hashes == nil {
		c.hashes = map[string]Hash{}
	}
	if len(c.hashes) >= 100000 {
		clear(c.hashes)
	}
	c.hashes[hash] = Hash{Hash: hash, Path: p}
}
func (c *Cache) RemoveHash(hash string) { c.mu.Lock(); defer c.mu.Unlock(); delete(c.hashes, hash) }
func (c *Cache) GetHash(hash string, exists func(string) bool) (Hash, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Hash{}, false
	}
	v, ok := c.hashes[hash]
	if ok && !exists(v.Path) {
		delete(c.hashes, hash)
		return Hash{}, false
	}
	return v, ok
}
func (c *Cache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	clear(c.assets)
	clear(c.hashes)
}
