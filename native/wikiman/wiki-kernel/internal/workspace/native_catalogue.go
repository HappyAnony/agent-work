// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"time"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeCatalogue(ctx context.Context, q catalogue.Request) (out catalogue.Result, err error) {
	result := make(chan catalogue.Result, 1)
	task, e := l.StartTask(ctx, func(c context.Context, pin *Lease) error {
		return pin.WithNativeIndexes(c, func(*indexdb.Bundle) (err error) {
			r, w := pin.workspace.resources, pin.workspace
			p := r.catalogueOptions
			p.Language = r.configuration.Lang
			p.PluginEnabled = map[string]bool{}
			p.ThemeNames = append([]string(nil), p.ThemeNames...)
			if p.Now == nil {
				p.Now = time.Now
			}
			if p.Backend == "" {
				p.Backend = runtime.GOOS
			}
			if r.configuration.System != nil {
				switch r.configuration.System.Container {
				case "docker", "ios", "android", "harmony":
					p.Backend = r.configuration.System.Container
				}
			}
			if a := r.configuration.Appearance; a != nil {
				p.CurrentIcon, p.LightTheme, p.DarkTheme = a.Icon, a.ThemeLight, a.ThemeDark
				for _, v := range a.LightThemes {
					p.ThemeNames = append(p.ThemeNames, v.Name)
				}
				for _, v := range a.DarkThemes {
					p.ThemeNames = append(p.ThemeNames, v.Name)
				}
			}
			var files *catalogue.Files
			if p.Read == nil && p.List == nil && p.Stat == nil {
				switch q.Kind {
				case "plugins", "widgets", "templates":
				default:
					return catalogue.ErrDependency
				}
				files, err = catalogue.OpenFiles(map[string]string{q.Kind: filepath.Join(w.paths.DataDir, q.Kind)}, false)
				if err != nil {
					return err
				}
				defer func() { err = errors.Join(err, files.Close()) }()
				p.Read, p.List, p.Stat = files.Read, files.List, files.Stat
			}
			if q.Kind == "plugins" && q.Action == "installed" {
				raw, e := readNativeFile(c, w, "storage/petal/petals.json", 1<<20)
				if e != nil && !errors.Is(e, fs.ErrNotExist) {
					return e
				}
				if e == nil {
					petals, e := extensionprefs.DecodePetals(raw)
					if e != nil {
						return e
					}
					for _, v := range petals {
						p.PluginEnabled[v.Name] = v.Enabled
					}
				}
			}
			v, e := r.catalogueResources.Do(c, q, p)
			if e != nil {
				return e
			}
			if files != nil {
				if e = files.Check(c); e != nil {
					return e
				}
			}
			result <- v
			return nil
		})
	})
	if e != nil {
		return out, e
	}
	if e = task.Wait(context.Background()); e != nil {
		return catalogue.Result{}, e
	}
	return <-result, nil
}
