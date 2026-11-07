// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/internal/workspacefile"
)

const scopedPetalsPath = "storage/petal/petals.json"

type scopedPlugin struct {
	Name          string            `json:"name"`
	Code          string            `json:"code"`
	Storage       bool              `json:"storage"`
	Hotkeys       map[string]string `json:"hotkeys,omitempty"`
	SystemHotkeys map[string]string `json:"systemHotkeys,omitempty"`
}

var scopedPluginChord = regexp.MustCompile(`^⌃?⌥?⇧?⌘?[A-Z0-9]$`)

func WithScopedPluginHotkeys(ctx context.Context, overrides map[string]map[string]string) (context.Context, error) {
	return withScopedHotkeys(ctx, overrides, false)
}

func WithScopedSystemHotkeys(ctx context.Context, overrides map[string]map[string]string) (context.Context, error) {
	return withScopedHotkeys(ctx, overrides, true)
}

func withScopedHotkeys(ctx context.Context, overrides map[string]map[string]string, system bool) (context.Context, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	if p == nil || len(overrides) > len(p.plugins) {
		return ctx, ErrArgument
	}
	profile := make(map[string]map[string]string, len(overrides))
	seen := map[string]bool{}
	for name, keys := range overrides {
		if p.plugins[name] == "" || len(keys) > 12 {
			return ctx, ErrArgument
		}
		checked := make(map[string]string, len(keys))
		for command, chord := range keys {
			if len(command) == 0 || len(command) > 96 || !scopedPluginChord.MatchString(chord) ||
				!strings.ContainsAny(chord, "⌃⌘") || !strings.ContainsAny(chord, "⌥⇧") || seen[chord] {
				return ctx, ErrArgument
			}
			if system && (!strings.Contains(chord, "⇧") || !strings.Contains(chord, "⌥") ||
				!strings.Contains(chord, "⌘") || strings.Contains(chord, "⌃")) {
				return ctx, ErrArgument
			}
			seen[chord] = true
			checked[command] = chord
		}
		profile[name] = checked
	}
	if system {
		p.pluginSystemHotkeys = profile
	} else {
		p.pluginHotkeys = profile
	}
	return ctx, nil
}

func WithScopedPluginStorage(ctx context.Context, names []string) (context.Context, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	if p == nil || len(names) > len(p.plugins) {
		return ctx, ErrArgument
	}
	p.pluginStorage = make(map[string]bool, len(names))
	for _, name := range names {
		if p.plugins[name] == "" || p.pluginStorage[name] {
			return ctx, ErrArgument
		}
		p.pluginStorage[name] = true
	}
	return ctx, nil
}

func scopedPluginRead(ctx context.Context, l *workspace.Lease, name string, limit int64) ([]byte, error) {
	out, err := l.NativeFileRead(ctx, workspacefile.Request{Action: "read", Path: name, ReadLimit: limit})
	return out.Data, err
}

func scopedPluginCode(ctx context.Context, l *workspace.Lease, name, hash string) (code, manifest []byte, err error) {
	manifest, err = scopedPluginRead(ctx, l, "plugins/"+name+"/plugin.json", 128<<10)
	var pkg extensionprefs.Package
	if err != nil || json.Unmarshal(manifest, &pkg) != nil || pkg.Name != name ||
		extensionprefs.Incompatible(pkg, runtime.GOOS, "desktop") {
		return nil, nil, ErrUnauthorized
	}
	code, err = scopedPluginRead(ctx, l, "plugins/"+name+"/index.js", 512<<10)
	if err != nil || !utf8.Valid(code) || sourcetx.Hash(code) != hash {
		return nil, nil, ErrUnauthorized
	}
	return code, manifest, nil
}

func runUISandboxPlugins(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	p, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	plugins := []scopedPlugin{}
	if view.Mode != "wikiDesktop" || p == nil || len(p.plugins) == 0 {
		return plugins, nil
	}
	if err := Decode(bytes.NewReader(r.Data), &struct{}{}); err != nil {
		return nil, err
	}
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		l, err := m.Acquire(c, r.StoreID, r.PageID)
		if err != nil {
			return err
		}
		defer l.Release()
		value, err := scopedPluginRead(c, l, scopedPetalsPath, 256<<10)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		petals, err := extensionprefs.DecodePetals(value)
		if err != nil {
			return err
		}
		for _, petal := range petals {
			hash := p.plugins[petal.Name]
			if hash == "" || !petal.Enabled || petal.Incompatible {
				continue
			}
			code, _, err := scopedPluginCode(c, l, petal.Name, hash)
			if err == nil {
				plugins = append(plugins, scopedPlugin{Name: petal.Name, Code: string(code), Storage: p.pluginStorage[petal.Name],
					Hotkeys: p.pluginHotkeys[petal.Name], SystemHotkeys: p.pluginSystemHotkeys[petal.Name]})
			}
		}
		return nil
	})
	return plugins, err
}
