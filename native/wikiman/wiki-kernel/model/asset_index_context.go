// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func IndexAssetContentContext(ctx context.Context, p string, removed bool) error {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	return indexAssetContentOwned(ctx, p, removed, o, sql.CaptureLegacyAssetQueue())
}
func indexAssetContentOwned(ctx context.Context, p string, removed bool, o *legacyReindexOwner, queue sql.LegacyAssetQueue) error {
	if e := o.guard(ctx); e != nil {
		return e
	}
	rel, e := filepath.Rel(filepath.Join(o.data, "assets"), p)
	if e != nil || !filepath.IsLocal(rel) || rel == "." {
		return errors.New("asset_index_path_invalid")
	}
	check := func(c context.Context) error {
		if e := o.guard(c); e != nil {
			return e
		}
		if queue.DB != sql.CaptureLegacyAssetQueue().DB {
			return conf.ErrBusinessUnbound
		}
		return nil
	}
	var extraction assetExtractionEnvironment
	var parser AssetParser
	if !removed {
		parser = assetContentSearcher.GetParser(filepath.Ext(p))
		if parser == nil {
			return nil
		}
		extraction, e = captureAssetExtraction(o.tempDir)
		if e != nil {
			return e
		}
	}
	return o.run(ctx, func(op, life context.Context) error {
		if e := check(op); e != nil {
			return e
		}
		c := o.owner
		c.m.Lock()
		ro := c.persistenceReadOnly
		c.m.Unlock()
		if ro {
			return ErrSettingReadOnly
		}
		name := "assets/" + filepath.ToSlash(rel)
		var items []indexdb.AssetContentEntry
		if !removed {
			root, e := os.OpenRoot(filepath.Join(o.data, "assets"))
			if e != nil {
				return e
			}
			result, e := extraction.read(op, root, rel, assetParserKind(parser))
			e = errors.Join(e, root.Close())
			if e != nil {
				return e
			}
			if result == nil {
				return nil
			}
			items = []indexdb.AssetContentEntry{{ID: ast.NewNodeID(), Name: util.RemoveID(filepath.Base(p)), Ext: strings.ToLower(filepath.Ext(p)), Path: name, Size: result.Size, Updated: result.Updated, Content: result.Content}}
		}
		if e := check(op); e != nil {
			return e
		}
		critical := context.WithoutCancel(op)
		if e := queue.DeletePath(critical, name); e != nil {
			return e
		}
		e := queue.Index(critical, items)
		return errors.Join(e, queue.Flush(critical, nil))
	})
}
