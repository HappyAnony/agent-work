// SPDX-License-Identifier: AGPL-3.0-or-later
package workspacestat

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

type Result struct {
	TreeCount, CTreeCount, BlockCount, CBlockCount int
	DataSize, CDataSize, AssetsSize, CAssetsSize   int64
}

func CeilTrees(count int) int {
	if count < 100 {
		return 100
	}
	for i := 1; i < 40; i++ {
		if count < i*500 {
			return i * 500
		}
	}
	return 20001
}
func CeilBlocks(count int) int {
	if count < 5000 {
		return 5000
	}
	for i := 1; i < 100; i++ {
		if count < i*10000 {
			return i * 10000
		}
	}
	return 1000001
}
func CeilSize(size int64) int64 {
	if size < 100*1024*1024 {
		return 100 * 1024 * 1024
	}
	for i := int64(1); i < 40; i++ {
		if size < 200*1024*1024*i {
			return 200 * 1024 * 1024 * i
		}
	}
	return 200*1024*1024*40 + 1
}

func Measure(ctx context.Context, source indexrepair.Source, count func(context.Context, bool) (int, error)) (out Result, err error) {
	if ctx == nil || source.Data == nil || count == nil {
		return out, errors.New("workspace_statistics_unbound")
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	out.TreeCount, err = count(ctx, true)
	if err != nil {
		return Result{}, err
	}
	out.BlockCount, err = count(ctx, false)
	if err != nil {
		return Result{}, err
	}
	manifest := map[string]fs.FileInfo{}
	entries := 0
	var walk func(string, int) error
	walk = func(name string, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		entries++
		if depth > 255 || entries > 1000000 {
			return errors.New("workspace_statistics_budget")
		}
		if name != "." {
			if e := source.Check(name, false); e != nil {
				return e
			}
		}
		info, e := source.Data.Lstat(name)
		if e != nil {
			return e
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
		manifest[name] = info
		size := info.Size()
		if info.IsDir() {
			size = 4096
		}
		if size < 0 || out.DataSize > math.MaxInt64-size {
			return errors.New("workspace_statistics_overflow")
		}
		out.DataSize += size
		if !info.IsDir() {
			if strings.Contains(name, "assets") {
				out.AssetsSize += size
			}
			return nil
		}
		children, e := source.ReadDir(ctx, name)
		if e != nil {
			return e
		}
		for _, child := range children {
			if e = walk(path.Join(name, child.Name()), depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	if err = walk(".", 0); err != nil {
		return Result{}, err
	}
	for name, before := range manifest {
		if e := ctx.Err(); e != nil {
			return Result{}, e
		}
		if name != "." {
			if e := source.Check(name, false); e != nil {
				return Result{}, e
			}
		}
		after, e := source.Data.Lstat(name)
		if e != nil {
			return Result{}, e
		}
		if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return Result{}, indexrepair.ErrSourceChanged
		}
	}
	out.CTreeCount = CeilTrees(out.TreeCount)
	out.CBlockCount = CeilBlocks(out.BlockCount)
	out.CDataSize = CeilSize(out.DataSize)
	out.CAssetsSize = CeilSize(out.AssetsSize)
	if e := ctx.Err(); e != nil {
		return Result{}, e
	}
	return out, nil
}
