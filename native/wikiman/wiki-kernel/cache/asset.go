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

package cache

import (
	"context"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/internal/assetcache"
	"wiki-kernel/util"
)

type AssetHash = assetcache.Hash
type Asset = assetcache.Asset

var originalAssets assetcache.Cache

func RemoveAssetHash(hash string) { originalAssets.RemoveHash(hash) }
func AssetHashCandidate(hash string) (AssetHash, bool) {
	return originalAssets.GetHash(hash, func(string) bool { return true })
}
func SetAssetHash(hash, path string) { originalAssets.SetHash(hash, path) }
func GetAssetHash(hash string) *AssetHash {
	v, ok := originalAssets.GetHash(hash, func(p string) bool { return filelock.IsExist(filepath.Join(util.DataDir, p)) })
	if !ok {
		return nil
	}
	return &v
}
func GetAssets() map[string]*Asset {
	ret := map[string]*Asset{}
	for k, v := range originalAssets.Snapshot() {
		v := v
		ret[k] = &v
	}
	return ret
}
func RemoveAsset(path string)     { originalAssets.Remove(path) }
func ExistAsset(path string) bool { return originalAssets.Exists(path) }
func LoadAssets() {
	root := util.GetDataAssetsAbsPath()
	if err := originalAssets.Load(context.Background(), os.DirFS(root), func(name string) bool { return filelock.IsHidden(filepath.Join(root, name)) }); err != nil {
		logging.LogErrorf("load assets failed: %s", err)
	}
}

func LoadAssetsContext(ctx context.Context, root *os.Root) error {
	if root == nil {
		return fs.ErrInvalid
	}
	info, e := root.Lstat("assets")
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if e == nil && (!info.IsDir() || info.Mode()&fs.ModeSymlink != 0) {
		return fs.ErrInvalid
	}
	sub, e := fs.Sub(root.FS(), "assets")
	if e != nil {
		return e
	}
	return originalAssets.Load(ctx, sub, nil)
}
