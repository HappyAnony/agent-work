package workspace

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/wikistore"
)

// Paths are presentation/selection values, never filesystem authority. Actual
// IO continues through the Context's pinned roots, not paths imported in JSON.
func bindConfigurationPaths(data conf.AppData, p wikistore.WorkspacePaths) conf.AppData {
	if data.System == nil {
		data.System = &conf.System{}
	}
	data.System.WorkspaceDir = filepath.Dir(p.DataDir)
	data.System.DataDir = p.DataDir
	data.System.ConfDir = p.ConfDir
	return data
}
func (l *Lease) RuntimePaths(ctx context.Context) (out wikistore.WorkspacePaths, err error) {
	pin, e := l.pin(true)
	if e != nil {
		return out, e
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error { out = pin.workspace.paths; return nil })
	})
	return
}
func (l *Lease) RuntimePath(ctx context.Context, kind, relative string) (string, error) {
	if relative != "" && (!fs.ValidPath(relative) || strings.Contains(relative, "\\")) {
		return "", ErrDocument
	}
	p, e := l.RuntimePaths(ctx)
	if e != nil {
		return "", e
	}
	var root string
	switch kind {
	case "data":
		root = p.DataDir
	case "runtime":
		root = p.RuntimeDir
	case "conf":
		root = p.ConfDir
	case "temp":
		root = p.TempDir
	case "history":
		root = filepath.Join(p.RuntimeDir, "history")
	case "assets", "emojis":
		root = filepath.Join(p.DataDir, kind)
	default:
		return "", ErrIdentity
	}
	if relative == "" {
		return root, nil
	}
	return filepath.Join(root, filepath.FromSlash(relative)), nil
}
func (l *Lease) ConfigurationPresentation(ctx context.Context, restricted, publish bool) (out conf.AppData, err error) {
	pin, e := l.pin(true)
	if e != nil {
		return out, e
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			r := pin.workspace.resources
			if r.configurationBusiness == nil {
				return conf.ErrBusinessUnbound
			}
			var e error
			out, e = r.configuration.Presentation(conf.PresentationOptions{Restricted: restricted, Publish: publish, DisabledSyncText: r.configurationBusiness.Language(r.configuration.Lang, 53, true, r.configuration.CloudRegion)})
			return e
		})
	})
	return
}
