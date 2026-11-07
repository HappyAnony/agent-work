// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"wiki-kernel/conf"
	"wiki-kernel/internal/graphview"
)

func SetGraphSettingContext(ctx context.Context, kind string, raw json.RawMessage) (value json.RawMessage, err error) {
	if kind != "graphLocal" && kind != "graphGlobal" {
		return nil, conf.ErrSetting
	}
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	err = o.run(ctx, func(op, life context.Context) error {
		var e error
		value, e = o.owner.ApplySetting(op, kind, raw, nil)
		return e
	})
	return
}

// Each request uses its own normalized graph settings even when another graph
// request publishes settings concurrently. No request temporarily swaps globals.
func GraphContext(ctx context.Context, id, query, kind string, raw json.RawMessage) (ret graphview.Result, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return ret, e
	}
	err = o.base.run(ctx, func(op, life context.Context) error {
		var data conf.AppData
		if e := o.base.owner.WithSettingSnapshot(op, func(v conf.AppData) error { var e error; data, e = v.Clone(); return e }); e != nil {
			return e
		}
		if kind != "" {
			if kind != "graphLocal" && kind != "graphGlobal" {
				return conf.ErrSetting
			}
			value, e := o.base.owner.ApplySetting(op, kind, raw, nil)
			if e != nil {
				return e
			}
			plan, e := data.PrepareSetting(kind, value)
			if e != nil {
				return e
			}
			data = plan.Data
		}
		if data.Graph == nil || data.Search == nil {
			return conf.ErrBusinessUnbound
		}
		root, e := os.OpenRoot(o.base.data)
		if e != nil {
			return e
		}
		defer root.Close()
		books, e := o.base.owner.business.Notebooks(op)
		if e != nil {
			return e
		}
		paths, e := graphview.DailyDirectories(op, books, (ownedResourceFiles{owner: o.base, root: root}).read)
		if e != nil {
			return e
		}
		ret, e = graphview.Build(op, id, query, graphview.Services{DB: o.r.main.DB, Graph: *data.Graph, Search: *data.Search, Parser: o.parser, DailyPaths: paths})
		return errors.Join(e, root.Close())
	})
	if err != nil {
		return graphview.Result{}, err
	}
	return
}

func BuildGraphContext(ctx context.Context, query string) (graphview.Result, error) {
	return GraphContext(ctx, "", query, "", nil)
}
func BuildTreeGraphContext(ctx context.Context, id, query string) (graphview.Result, error) {
	return GraphContext(ctx, id, query, "", nil)
}
