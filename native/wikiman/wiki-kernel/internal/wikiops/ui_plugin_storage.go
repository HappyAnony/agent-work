// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"slices"
	"unicode/utf8"

	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/internal/workspacefile"
)

const (
	uiPluginStorageEndpoint = "/api/plugin/sandboxedStorage"
	scopedPluginFileLimit   = 64 << 10
)

func runUISandboxPluginStorage(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	policy, _ := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	if view.Mode != "wikiDesktop" || policy == nil || len(policy.pluginStorage) == 0 {
		return nil, ErrUnauthorized
	}
	var q struct {
		Plugin, File, Action string
		Value                *string
	}
	if err := Decode(bytes.NewReader(r.Data), &q); err != nil {
		return nil, err
	}
	if !policy.pluginStorage[q.Plugin] {
		return nil, ErrUnauthorized
	}
	if !scriptPart.MatchString(q.File) || q.Action != "load" && q.Action != "save" && q.Action != "remove" ||
		(q.Action == "save") != (q.Value != nil) || q.Value != nil && len(*q.Value) > scopedPluginFileLimit ||
		(q.Action == "load") != (r.OperationID == "") {
		return nil, ErrArgument
	}

	var mutation sourcetx.Request
	var result any = ""
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		l, err := m.Acquire(c, r.StoreID, r.PageID)
		if err != nil {
			return err
		}
		defer l.Release()
		petalsBytes, err := scopedPluginRead(c, l, scopedPetalsPath, 256<<10)
		if err != nil {
			return err
		}
		petals, err := extensionprefs.DecodePetals(petalsBytes)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(petals, func(petal *extensionprefs.Petal) bool {
			return petal.Name == q.Plugin && petal.Enabled && !petal.Incompatible
		}) {
			return ErrUnauthorized
		}
		hash := policy.plugins[q.Plugin]
		_, manifest, err := scopedPluginCode(c, l, q.Plugin, hash)
		if err != nil {
			return err
		}
		dir := "storage/petal/" + q.Plugin
		name := dir + "/" + q.File
		raw, err := scopedPluginRead(c, l, name, scopedPluginFileLimit)
		exists := err == nil
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if q.Action == "load" {
			if json.Valid(raw) {
				result = json.RawMessage(raw)
			} else if utf8.Valid(raw) {
				result = string(raw)
			} else {
				return ErrArgument
			}
			return nil
		}
		cfg, err := l.Configuration(c)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		canonical, err := json.Marshal(q)
		if err != nil {
			return err
		}
		clientHash := sourcetx.Hash(append([]byte("plugin-data:"+r.PageID+":"), canonical...))
		if previous, found, err := m.LookupReceipt(c, r.StoreID, r.OperationID, clientHash); err != nil || found {
			if err == nil && !previous.SourceCommitted {
				return sourcetx.ErrConflict
			}
			return err
		}
		tree, err := m.Tree(c, r.StoreID)
		if err != nil {
			return err
		}
		base := "pages/" + r.PageID + "/data/"
		guards := &sourcetx.ReadGuards{Files: map[string]string{
			base + scopedPetalsPath:                       sourcetx.Hash(petalsBytes),
			base + "plugins/" + q.Plugin + "/plugin.json": sourcetx.Hash(manifest),
			base + "plugins/" + q.Plugin + "/index.js":    hash,
		}}
		prefs, err := sourcetx.WorkspacePreferencesPath(r.PageID)
		if err != nil {
			return err
		}
		if content, err := m.SourceFile(c, r.StoreID, prefs); err == nil {
			guards.Files[prefs] = sourcetx.Hash(content)
		} else if errors.Is(err, fs.ErrNotExist) {
			guards.Missing = append(guards.Missing, prefs)
		} else {
			return err
		}
		mutation = sourcetx.Request{OperationID: r.OperationID, ClientHash: clientHash,
			ExpectedTreeDigest: tree.SourceDigest(), ReadGuards: guards}
		if q.Action == "remove" && !exists {
			mutation.Noop = true
			guards.Missing = append(guards.Missing, base+name)
			return nil
		}
		change := sourcetx.Change{Path: base + name, Delete: q.Action == "remove"}
		if exists {
			change.ExpectedHash = sourcetx.Hash(raw)
		} else if err := guardPluginStorageCapacity(c, l, guards, dir, base+dir); err != nil {
			return err
		}
		if q.Value != nil {
			change.Data = []byte(*q.Value)
		}
		mutation.Changes = []sourcetx.Change{change}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if mutation.OperationID != "" {
		if _, err := m.Apply(ctx, r.StoreID, mutation); err != nil {
			return nil, err
		}
	}
	if q.Action != "load" {
		return map[string]int{"code": 0}, nil
	}
	return result, nil
}

func guardPluginStorageCapacity(ctx context.Context, l *workspace.Lease, guards *sourcetx.ReadGuards, dir, absolute string) error {
	out, err := l.NativeFileRead(ctx, workspacefile.Request{Action: "list", Path: dir})
	if errors.Is(err, fs.ErrNotExist) {
		guards.Missing = append(guards.Missing, absolute)
		return nil
	}
	if err != nil {
		return err
	}
	if len(out.Entries) >= 64 {
		return ErrExportLimit
	}
	names := make([]string, 0, len(out.Entries))
	for _, entry := range out.Entries {
		names = append(names, entry.Name)
	}
	slices.Sort(names)
	guards.Directories = map[string][]string{absolute: names}
	return nil
}
