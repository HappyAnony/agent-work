// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/util"
)

func (c *AppConf) catalogueService() *catalogue.Service {
	c.m.Lock()
	defer c.m.Unlock()
	if c.catalogueResources == nil {
		c.catalogueResources = &catalogue.Service{}
	}
	return c.catalogueResources
}

func CatalogueContext(ctx context.Context, q catalogue.Request) (out catalogue.Result, err error) {
	o, e := captureLegacyBazaarScanOwner(ctx)
	if e != nil {
		return out, e
	}
	err = o.run(ctx, func(op context.Context) (err error) {
		p := catalogue.Options{Version: util.Ver, OSS: util.BazaarOSSServer, Statistics: util.BazaarStatServer, Backend: runtime.GOOS, Now: time.Now, CurrentIcon: o.icon, LightTheme: o.lightTheme, DarkTheme: o.darkTheme}
		switch util.Container {
		case util.ContainerDocker, util.ContainerIOS, util.ContainerAndroid, util.ContainerHarmony:
			p.Backend = util.Container
		}
		if e := o.owner.WithSettingSnapshot(op, func(v conf.AppData) error {
			p.Language = v.Lang
			p.VersionURL = util.GetCloudServerForRegion(v.CloudRegion) + "/apis/siyuan/version?ver=" + url.QueryEscape(p.Version)
			if v.Appearance != nil {
				for _, theme := range v.Appearance.LightThemes {
					p.ThemeNames = append(p.ThemeNames, theme.Name)
				}
				for _, theme := range v.Appearance.DarkThemes {
					p.ThemeNames = append(p.ThemeNames, theme.Name)
				}
			}
			return o.guard(op)
		}); e != nil {
			return e
		}
		roots := map[string]string{"plugins": filepath.Join(o.data, "plugins"), "widgets": filepath.Join(o.data, "widgets"), "templates": filepath.Join(o.data, "templates"), "icons": o.icons, "themes": o.themes}
		selected, ok := roots[q.Kind]
		if !ok {
			return catalogue.ErrValue
		}
		files, e := catalogue.OpenFiles(map[string]string{q.Kind: selected}, true)
		if e != nil {
			return e
		}
		defer func() { err = errors.Join(err, files.Close()) }()
		p.Read, p.List, p.Stat = files.Read, files.List, files.Stat
		transport := http.DefaultTransport
		if original, ok := transport.(*http.Transport); ok {
			owned := original.Clone()
			defer owned.CloseIdleConnections()
			transport = owned
		}
		p.Fetch = catalogue.HTTPFetch(&http.Client{Transport: transport, Timeout: 30 * time.Second})
		if q.Kind == "plugins" && q.Action == "installed" {
			petals, e := GetPetalsContext(op)
			if e != nil {
				return e
			}
			p.PluginEnabled = map[string]bool{}
			for _, petal := range petals {
				p.PluginEnabled[petal.Name] = petal.Enabled
			}
		}
		out, e = o.owner.catalogueService().Do(op, q, p)
		if e != nil {
			return e
		}
		return errors.Join(o.guard(op), files.Check(op))
	})
	if err != nil {
		return catalogue.Result{}, err
	}
	return
}
