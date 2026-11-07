// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"net/url"
	"sort"
	"strings"
	"wiki-kernel/internal/assetcache"
	"wiki-kernel/internal/assetflow"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/wikistore"
)

func (l *Lease) NativeAssetReferences(ctx context.Context, missing bool) ([]string, error) {
	return l.nativeAssetReferences(ctx, missing, false)
}

// Integrity checks include closed notebooks. The original missing-assets
// command still limits its interactive results to open notebooks.
func (l *Lease) NativeMissingAssetsInAllNotebooks(ctx context.Context) ([]string, error) {
	return l.nativeAssetReferences(ctx, true, true)
}

func (l *Lease) nativeAssetReferences(ctx context.Context, missing, includeClosed bool) (out []string, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		source := indexrepair.Source{Data: w.data}
		boxes, e := r.configurationBusiness.Notebooks(ctx)
		if e != nil {
			return e
		}
		all, selected := []string{}, []string{}
		for _, box := range boxes {
			all = append(all, box.ID)
			if !missing || includeClosed || !box.Closed {
				selected = append(selected, box.ID)
			}
		}
		assets, e := assetflow.Inventory(ctx, source, all)
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		s := assetflow.ReferenceServices{Assets: assets, AV: func(id string) (*av.AttributeView, error) {
			raw, e := source.Read(ctx, "storage/av/"+id+".json")
			if e != nil {
				return nil, e
			}
			var a *av.AttributeView
			if e = json.Unmarshal(raw, &a); e != nil {
				return nil, e
			}
			if a == nil || a.ID != id {
				return nil, ErrDocument
			}
			return a, nil
		}, AVData: func(c context.Context) ([][]byte, error) { return assetflow.ReadAVData(c, source) }, Exists: func(c context.Context, name string) (bool, error) {
			e := source.Check(name, false)
			if indexrepair.IsSourceNotExist(e) {
				return false, nil
			}
			return e == nil, e
		}}
		scan := func(ids []string) ([]string, error) {
			s.Walk = func(c context.Context, visit func(*ast.Node) error) error {
				return docquery.WalkDocumentTrees(c, ids, source.ReadDir, source.Read, parser, func(t *parse.Tree) error { return visit(t.Root) })
			}
			return assetflow.CheckReferences(ctx, missing, s)
		}
		if missing {
			out = []string{}
			for _, id := range selected {
				v, e := scan([]string{id})
				if e != nil {
					return e
				}
				out = append(out, v...)
			}
			sort.Strings(out)
			return ctx.Err()
		}
		out, e = scan(selected)
		return e
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (l *Lease) NativeSearchAssetNames(ctx context.Context, keyword string, exts []string) (out []*assetcache.Asset, err error) {
	return l.nativeSearchAssetNames(ctx, keyword, exts, nil)
}

func (l *Lease) NativeSearchEntryAssetNames(ctx context.Context, keyword string, exts []string, entry wikistore.EntryDocument) ([]*assetcache.Asset, error) {
	return l.nativeSearchAssetNames(ctx, keyword, exts, &entry)
}

func (l *Lease) nativeSearchAssetNames(ctx context.Context, keyword string, exts []string, entry *wikistore.EntryDocument) (out []*assetcache.Asset, err error) {
	exts = append([]string(nil), exts...)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil || r.configuration.Search.Limit < 0 {
			return ErrDocument
		}
		if e := r.loadAssets(ctx, w); e != nil {
			return e
		}
		assets := r.nativeAssets.Snapshot()
		if entry != nil {
			tree, e := r.locatedTree(ctx, w, entry.BoxID, entry.DocID)
			if e != nil {
				return e
			}
			paths, e := assetflow.LinkDests(tree.Root, func(string) (*av.AttributeView, error) { return nil, ctx.Err() })
			if e != nil {
				return e
			}
			filtered := make(map[string]assetcache.Asset)
			for _, value := range paths {
				name, e := url.PathUnescape(strings.TrimPrefix(value, "/"))
				if asset, ok := assets[name]; e == nil && ok {
					filtered[name] = asset
				}
			}
			assets = filtered
		}
		out = assetcache.SearchNames(keyword, exts, assets, r.configuration.Search.Limit, r.configuration.Search.CaseSensitive)
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
