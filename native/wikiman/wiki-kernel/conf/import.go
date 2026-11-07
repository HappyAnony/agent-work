// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
)

var ErrImport = errors.New("invalid_configuration_import")

const MaxImportJSON = 8 << 20

// ReadImport accepts the original JSON or single root-file ZIP export. No
// upload filename is used as a local path and no extraction directory exists.
func ReadImport(ctx context.Context, name string, input io.Reader) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	raw, e := io.ReadAll(io.LimitReader(input, (16<<20)+1))
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if len(raw) > 16<<20 {
		return nil, ErrImport
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".json":
		if len(raw) > MaxImportJSON {
			return nil, ErrImport
		}
		return raw, nil
	case ".zip":
		z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if e != nil || len(z.File) != 1 {
			return nil, ErrImport
		}
		f := z.File[0]
		if f.FileInfo().IsDir() || !f.Mode().IsRegular() || strings.ContainsAny(f.Name, "/\\") || strings.HasPrefix(f.Name, ".") || f.UncompressedSize64 > MaxImportJSON {
			return nil, ErrImport
		}
		r, e := f.Open()
		if e != nil {
			return nil, ErrImport
		}
		defer r.Close()
		raw, e = io.ReadAll(io.LimitReader(r, MaxImportJSON+1))
		if e != nil {
			return nil, ErrImport
		}
		if len(raw) > MaxImportJSON {
			return nil, ErrImport
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		return raw, nil
	default:
		return nil, ErrImport
	}
}

// PrepareImport retains the original twelve imported groups and no runtime
// effects. Omitted groups remain owned instead of becoming nil; explicit null
// is rejected except for the originally optional UI layout. Full exports retain
// replacement semantics. Paths, device identity and private authority survive.
func (data AppData) PrepareImport(ctx context.Context, raw []byte) (SettingPlan, error) {
	if e := ctx.Err(); e != nil {
		return SettingPlan{}, e
	}
	if len(raw) > MaxImportJSON || uniqueJSON(raw) != nil {
		return SettingPlan{}, ErrImport
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return SettingPlan{}, ErrImport
	}
	out, e := data.Clone()
	if e != nil {
		return SettingPlan{}, e
	}
	count := 0
	for _, key := range []string{"fileTree", "tag", "editor", "export", "graph", "uiLayout", "system", "keymap", "search", "flashcard", "ai", "bazaar"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		count++
		if string(bytes.TrimSpace(value)) == "null" && key != "uiLayout" {
			return SettingPlan{}, ErrImport
		}
		one, _ := json.Marshal(map[string]json.RawMessage{key: value})
		// Clear the selected group first: json.Unmarshal otherwise merges pointer
		// members, changing the original group-replacement behavior.
		switch key {
		case "fileTree":
			out.FileTree = nil
		case "tag":
			out.Tag = nil
		case "editor":
			out.Editor = nil
		case "export":
			out.Export = nil
		case "graph":
			out.Graph = nil
		case "uiLayout":
			out.UILayout = nil
		case "system":
			out.System = nil
		case "keymap":
			out.Keymap = nil
		case "search":
			out.Search = nil
		case "flashcard":
			out.Flashcard = nil
		case "ai":
			out.AI = nil
		case "bazaar":
			out.Bazaar = nil
		}
		if json.Unmarshal(one, &out) != nil {
			return SettingPlan{}, ErrImport
		}
		if key == "editor" && (out.Editor == nil || out.Editor.Markdown == nil) {
			return SettingPlan{}, ErrImport
		}
		if key == "ai" && (out.AI == nil || out.AI.OpenAI == nil) {
			return SettingPlan{}, ErrImport
		}
		if key == "system" {
			if out.System == nil || out.System.NetworkProxy == nil {
				return SettingPlan{}, ErrImport
			}
			v := out.System
			// Only original user-facing System preferences are imported. An export
			// never supplies authority to relocate the running owner's paths.
			base, e := data.Clone()
			if e != nil {
				return SettingPlan{}, e
			}
			if base.System == nil {
				base.System = &System{}
			}
			out.System = base.System
			out.System.NetworkServe = v.NetworkServe
			out.System.NetworkProxy = v.NetworkProxy
			out.System.DownloadInstallPkg = v.DownloadInstallPkg
			out.System.AutoLaunch2 = v.AutoLaunch2
			out.System.LockScreenMode = v.LockScreenMode
			out.System.MicrosoftDefenderExcluded = v.MicrosoftDefenderExcluded
		}
	}
	if count == 0 {
		return SettingPlan{}, ErrImport
	}
	if out.Editor != nil {
		out.Editor.ReadOnly = false
		if data.Editor != nil {
			out.Editor.ReadOnly = data.Editor.ReadOnly
		}
	}
	if out.Export != nil {
		out.Export.PandocBin = ""
		if data.Export != nil {
			out.Export.PandocBin = data.Export.PandocBin
		}
	}
	return SettingPlan{Data: out}, nil
}
