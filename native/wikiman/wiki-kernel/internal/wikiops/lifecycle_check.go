// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"path"
	"strings"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

func checkLifecycleContent(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest, files []sourcetx.FileInfo, out *ContentResult, live bool) error {
	prefix := "pages/" + r.PageID + "/data/"
	active := map[string]string{}
	docs := 0
	for _, f := range files {
		name := strings.TrimPrefix(f.Path, prefix)
		active[name] = f.Hash
		if activeDoc(name) {
			docs++
		}
	}
	for name := range active {
		if !activeDoc(name) {
			continue
		}
		parts := strings.Split(name, "/")
		for i := 1; i < len(parts)-1; i++ {
			if !nativeID.MatchString(parts[i]) || active[strings.Join(parts[:i+1], "/")+".docj"] == "" {
				out.Diagnostics = append(out.Diagnostics, "missing_document_parent:"+name)
				break
			}
		}
	}
	records, e := m.ArchiveFiles(ctx, r.StoreID, "tombstones", r.PageID, "", "")
	if e != nil {
		return e
	}
	retained, e := m.ArchiveFiles(ctx, r.StoreID, "deleted", r.PageID, "", "")
	if e != nil {
		return e
	}
	available := map[string]string{}
	for _, f := range retained {
		available[f.Path] = f.Hash
	}
	for _, record := range records {
		raw, e := m.SourceFile(ctx, r.StoreID, record.Path)
		if e != nil {
			return e
		}
		if sourcetx.Hash(raw) != record.Hash {
			return sourcetx.ErrConflict
		}
		t, e := sourcetx.DecodeDocumentTombstone(raw, record.Path)
		if e != nil {
			out.Diagnostics = append(out.Diagnostics, "invalid_tombstone:"+record.Path)
			continue
		}
		for _, f := range t.Files {
			name := t.ArchivePath(f.Path)
			if available[name] != f.Hash {
				out.Diagnostics = append(out.Diagnostics, "invalid_retained_file:"+name)
			}
			delete(available, name)
			if active[f.Path] != "" {
				out.Diagnostics = append(out.Diagnostics, "deleted_path_reused:"+f.Path)
			}
		}
		for _, effect := range t.Effects {
			name := t.EffectPath(effect.Path)
			if available[name] != effect.BeforeHash {
				out.Diagnostics = append(out.Diagnostics, "invalid_retained_effect:"+name)
			}
			delete(available, name)
		}
	}
	for name := range available {
		out.Diagnostics = append(out.Diagnostics, "unowned_retained_file:"+name)
	}
	versions, e := m.ArchiveFiles(ctx, r.StoreID, "versions", r.PageID, "", "")
	if e != nil {
		return e
	}
	for _, version := range versions {
		raw, e := m.SourceFile(ctx, r.StoreID, version.Path)
		if e != nil {
			return e
		}
		if sourcetx.Hash(raw) != version.Hash {
			return sourcetx.ErrConflict
		}
		if strings.HasSuffix(version.Path, ".json") {
			var av struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &av) != nil || av.ID+".json" != path.Base(version.Path) {
				out.Diagnostics = append(out.Diagnostics, "invalid_version_attribute_view:"+version.Path)
			}
			continue
		}
		id := strings.TrimSuffix(path.Base(version.Path), ".docj")
		if _, _, e = (&docast.Parser{}).Parse(ctx, raw, id); e != nil {
			out.Diagnostics = append(out.Diagnostics, "invalid_document_version:"+version.Path)
		}
	}
	for name := range active {
		if !strings.HasPrefix(name, "storage/av/") || !strings.HasSuffix(name, ".json") {
			continue
		}
		raw, e := m.SourceFile(ctx, r.StoreID, prefix+name)
		if e != nil {
			return e
		}
		var av struct {
			ID string `json:"id"`
		}
		if sourcetx.Hash(raw) != active[name] {
			return sourcetx.ErrConflict
		}
		if json.Unmarshal(raw, &av) != nil || av.ID+".json" != path.Base(name) {
			out.Diagnostics = append(out.Diagnostics, "invalid_attribute_view:"+name)
		}
	}
	if live && docs > 0 && len(out.Diagnostics) == 0 {
		l, e := m.Acquire(ctx, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		missing, e := l.NativeMissingAssetsInAllNotebooks(ctx)
		l.Release()
		if e != nil {
			return e
		}
		for _, name := range missing {
			out.Diagnostics = append(out.Diagnostics, "missing_asset:"+name)
		}
	}
	return nil
}

func (p *lifecycleEdit) checkVersionAsset(name string) error {
	files, e := p.guardArchive("versions")
	if e != nil {
		return e
	}
	for _, f := range files {
		raw, e := p.read(f.Path)
		if e != nil {
			return e
		}
		if sourcetx.Hash(raw) != f.Hash || retainedAssetReference(raw, name) {
			return sourcetx.ErrConflict
		}
	}
	return nil
}

func (p *lifecycleEdit) guardArchive(namespace string) ([]sourcetx.FileInfo, error) {
	r := p.request
	files, guards, e := p.m.ArchiveReadGuards(p.ctx, r.StoreID, namespace, r.PageID)
	if e != nil {
		return nil, e
	}
	maps.Copy(p.tx.ReadGuards.Files, guards.Files)
	maps.Copy(p.tx.ReadGuards.Directories, guards.Directories)
	p.tx.ReadGuards.Missing = append(p.tx.ReadGuards.Missing, guards.Missing...)
	return files, nil
}

// Be conservative with retained source. JSON and URL escaping can hide a
// reference from a literal byte search.
func retainedAssetReference(raw []byte, name string) bool {
	if bytes.Contains(raw, []byte(name)) {
		return true
	}
	var data any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil {
		return true
	}
	var contains func(any) bool
	contains = func(value any) bool {
		switch value := value.(type) {
		case string:
			unescaped, e := url.PathUnescape(value)
			return strings.Contains(value, name) || e == nil && strings.Contains(unescaped, name)
		case []any:
			for _, child := range value {
				if contains(child) {
					return true
				}
			}
		case map[string]any:
			for _, child := range value {
				if contains(child) {
					return true
				}
			}
		}
		return false
	}
	return contains(data)
}
