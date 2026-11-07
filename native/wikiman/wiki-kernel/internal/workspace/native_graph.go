// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/graphview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

func (l *Lease) NativeGraph(ctx context.Context, id, query string) (graphview.Result, error) {
	return l.NativeGraphWithOptions(ctx, id, query, nil, nil)
}

// A component's graph filter applies to this projection only.
func (l *Lease) NativeGraphWithOptions(ctx context.Context, id, query string, global *conf.GlobalGraph, local *conf.LocalGraph) (ret graphview.Result, err error) {
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		data, e := r.configuration.Clone()
		if e != nil {
			return e
		}
		if data.Graph == nil || data.Search == nil || r.configurationBusiness == nil {
			return conf.ErrBusinessUnbound
		}
		if global != nil {
			data.Graph.Global = global
		}
		if local != nil {
			data.Graph.Local = local
		}
		books, e := r.configurationBusiness.Notebooks(ctx)
		if e != nil {
			return e
		}
		paths, e := graphview.DailyDirectories(ctx, books, func(c context.Context, p string) ([]byte, error) { return readNativeFile(c, w, p, 1<<20) })
		if e != nil {
			return e
		}
		options, e := r.configuredParserOptions(ctx, w)
		if e != nil {
			return e
		}
		main, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = graphview.Build(ctx, id, query, graphview.Services{DB: main, Graph: *data.Graph, Search: *data.Search, Parser: nativeparser.NewEditor(options), DailyPaths: paths})
		return e
	})
	if err != nil {
		return graphview.Result{}, err
	}
	return
}
