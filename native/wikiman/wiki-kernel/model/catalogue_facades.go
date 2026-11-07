// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/bazaar"
	"wiki-kernel/internal/catalogue"
)

func GetPackageREADMEContext(ctx context.Context, repo, hash, kind string) (string, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "readme", Kind: kind, RepoURL: repo, RepoHash: hash})
	return r.README, e
}
func SearchWidgetContext(ctx context.Context, keyword string) ([]*Block, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "search", Kind: "widgets", Keyword: keyword})
	if e != nil {
		return nil, e
	}
	out := []*Block{}
	for _, item := range r.Items {
		out = append(out, &Block{Name: item.PreferredName, Content: item.Name})
	}
	return out, nil
}
func BazaarPluginsContext(ctx context.Context, frontend, keyword string) ([]*bazaar.Plugin, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "remote", Kind: "plugins", Keyword: keyword, CheckUpdate: false, Frontend: frontend})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Plugin{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Plugin{Package: item.Package, Enabled: item.Enabled})
	}
	return out, nil
}
func InstalledPluginsContext(ctx context.Context, frontend, keyword string) ([]*bazaar.Plugin, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "installed", Kind: "plugins", Keyword: keyword, CheckUpdate: true, Frontend: frontend})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Plugin{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Plugin{Package: item.Package, Enabled: item.Enabled})
	}
	return out, nil
}
func BazaarWidgetsContext(ctx context.Context, keyword string) ([]*bazaar.Widget, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "remote", Kind: "widgets", Keyword: keyword, CheckUpdate: false})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Widget{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Widget{Package: item.Package})
	}
	return out, nil
}
func InstalledWidgetsContext(ctx context.Context, keyword string) ([]*bazaar.Widget, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "installed", Kind: "widgets", Keyword: keyword, CheckUpdate: true})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Widget{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Widget{Package: item.Package})
	}
	return out, nil
}
func BazaarIconsContext(ctx context.Context, keyword string) ([]*bazaar.Icon, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "remote", Kind: "icons", Keyword: keyword, CheckUpdate: false})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Icon{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Icon{Package: item.Package})
	}
	return out, nil
}
func InstalledIconsContext(ctx context.Context, keyword string) ([]*bazaar.Icon, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "installed", Kind: "icons", Keyword: keyword, CheckUpdate: true})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Icon{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Icon{Package: item.Package})
	}
	return out, nil
}
func BazaarThemesContext(ctx context.Context, keyword string) ([]*bazaar.Theme, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "remote", Kind: "themes", Keyword: keyword, CheckUpdate: false})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Theme{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Theme{Package: item.Package, Modes: item.Modes})
	}
	return out, nil
}
func InstalledThemesContext(ctx context.Context, keyword string) ([]*bazaar.Theme, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "installed", Kind: "themes", Keyword: keyword, CheckUpdate: true})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Theme{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Theme{Package: item.Package, Modes: item.Modes})
	}
	return out, nil
}
func BazaarTemplatesContext(ctx context.Context, keyword string) ([]*bazaar.Template, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "remote", Kind: "templates", Keyword: keyword, CheckUpdate: false})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Template{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Template{Package: item.Package})
	}
	return out, nil
}
func InstalledTemplatesContext(ctx context.Context, keyword string) ([]*bazaar.Template, error) {
	r, e := CatalogueContext(ctx, catalogue.Request{Action: "installed", Kind: "templates", Keyword: keyword, CheckUpdate: true})
	if e != nil {
		return nil, e
	}
	out := []*bazaar.Template{}
	for _, item := range r.Items {
		out = append(out, &bazaar.Template{Package: item.Package})
	}
	return out, nil
}
