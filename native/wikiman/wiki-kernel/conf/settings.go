// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original setting normalization extracted from api/setting.go.
package conf

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"wiki-kernel/internal/textprefs"
)

var ErrSetting = errors.New("invalid_configuration_setting")

// SettingPlan preserves original controller normalization. Effects are ordered
// intent, never authority to persist source or call process-global services.
type SettingEffect struct {
	Kind string
	Bool bool
	Int  int
	Text string
}
type SettingPlan struct {
	Data        AppData
	Value       json.RawMessage
	Effects     []SettingEffect
	NoSave      bool // Original emoji/refresh/empty-keyword actions are memory/effect-only.
	EffectsOnly bool
}

// Trusted device capabilities, never loaded from preference JSON. Callbacks run
// under the owning setting gate, must be bounded and must not reenter the owner.
type SettingServices struct {
	ValidateSyncLocal     func(context.Context, string) error
	DisableSyncPerception bool
	CurrentPandoc         func(context.Context) (string, error)
	ValidatePandoc        func(context.Context, string) (bool, error)
	NormalizeAppearance   func(context.Context, *Appearance) (*Appearance, error)
	ThemeScriptExists     func(context.Context, string) (bool, error)
}

func (data AppData) PrepareSetting(kind string, raw json.RawMessage) (out SettingPlan, err error) {
	return data.PrepareSettingWithServices(context.Background(), kind, raw, SettingServices{})
}

func (data AppData) PrepareSettingWithServices(ctx context.Context, kind string, raw json.RawMessage, services SettingServices) (out SettingPlan, err error) {
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if len(raw) == 0 || len(raw) > 4<<20 || strings.TrimSpace(string(raw)) == "null" {
		return out, ErrSetting
	}
	out.Data, err = data.Clone()
	if err != nil {
		return SettingPlan{}, err
	}
	var target any
	effect := func(kind string, b bool, n int) {
		out.Effects = append(out.Effects, SettingEffect{Kind: kind, Bool: b, Int: n})
	}
	decode := func(v any) error { return json.Unmarshal(raw, v) }
	switch kind {
	case "repoIndexRetention", "repoRetentionDaily":
		if out.Data.Repo == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		var args struct {
			Days    *int `json:"days"`
			Indexes *int `json:"indexes"`
		}
		if err = decode(&args); err != nil {
			return SettingPlan{}, err
		}
		v := args.Days
		if kind == "repoRetentionDaily" {
			v = args.Indexes
		}
		if v == nil {
			return SettingPlan{}, ErrSetting
		}
		value := *v
		if value < 1 {
			value = 180
		}
		if kind == "repoIndexRetention" {
			out.Data.Repo.IndexRetentionDays = value
		} else {
			out.Data.Repo.RetentionIndexesDaily = value
		}
		target = value
	case "uiLayout", "apiToken", "accessAuthCode", "lockScreenMode", "appearanceMode", "networkServe", "autoLaunch", "downloadInstallPkg", "networkProxy", "defenderExcluded":
		return prepareDeviceSetting(ctx, out, kind, raw, services)
	case "graphLocal":
		v := NewLocalGraph()
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.TypeFilter == nil || v.D3 == nil || v.NodeSize < 0 {
			return SettingPlan{}, ErrSetting
		}
		if out.Data.Graph == nil {
			out.Data.Graph = NewGraph()
		}
		out.Data.Graph.Local = v
		target = v
	case "graphGlobal":
		v := NewGlobalGraph()
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.TypeFilter == nil || v.D3 == nil || v.NodeSize < 0 || v.MinRefs < 0 {
			return SettingPlan{}, ErrSetting
		}
		if out.Data.Graph == nil {
			out.Data.Graph = NewGraph()
		}
		out.Data.Graph.Global = v
		target = v
	case "syncEnabled", "syncConflict", "syncMode", "syncProvider", "syncInterval", "syncPerception", "syncS3", "syncWebDAV", "syncLocal":
		target, out.Effects, err = out.Data.prepareSyncSetting(ctx, kind, raw, services)
	case "appearance":
		v := &Appearance{}
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.StatusBar == nil {
			return SettingPlan{}, ErrSetting
		}
		if services.NormalizeAppearance == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		v, err = services.NormalizeAppearance(ctx, v)
		if err != nil {
			return SettingPlan{}, err
		}
		if v == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		out.Data.Appearance = v
		out.Data.Lang = v.Lang
		target = v
		effect("appearanceRuntime", false, 0)
		if data.Lang != v.Lang {
			effect("appearanceMarket", false, 0)
		}
		effect("appearanceNotify", false, 0)
	case "publish":
		v := &Publish{}
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.Auth == nil {
			return SettingPlan{}, ErrSetting
		}
		for _, a := range v.Auth.Accounts {
			if a == nil {
				return SettingPlan{}, ErrSetting
			}
		}
		out.Data.Publish = v
		target = v
		effect("publish", false, 0)
	case "export":
		v := &Export{}
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.PandocBin != "" {
			if services.ValidatePandoc == nil {
				return SettingPlan{}, ErrBusinessUnbound
			}
			requested := v.PandocBin
			valid, e := services.ValidatePandoc(ctx, requested)
			if e != nil {
				return SettingPlan{}, e
			}
			if e = ctx.Err(); e != nil {
				return SettingPlan{}, e
			}
			if valid {
				effect("pandoc", false, 0)
			} else {
				if services.CurrentPandoc == nil {
					return SettingPlan{}, ErrBusinessUnbound
				}
				v.PandocBin, e = services.CurrentPandoc(ctx)
				if e != nil {
					return SettingPlan{}, e
				}
				out.Effects = append(out.Effects, SettingEffect{Kind: "pandocWarning", Text: requested})
			}
		}
		out.Data.Export = v
		target = v
	case "virtualInclude", "virtualExclude", "virtualRefresh", "emoji":
		if out.Data.Editor == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		if kind == "virtualRefresh" {
			out.NoSave = true
			out.EffectsOnly = true
			effect("virtual", false, 0)
			effect("confNotify", false, 0)
			break
		}
		var args map[string]json.RawMessage
		if err = decode(&args); err != nil {
			return SettingPlan{}, err
		}
		key := "keywords"
		if kind == "emoji" {
			key = "emoji"
		}
		var entries []*string
		v, ok := args[key]
		if !ok || string(v) == "null" {
			return SettingPlan{}, ErrSetting
		}
		if err = json.Unmarshal(v, &entries); err != nil {
			return SettingPlan{}, err
		}
		values := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry == nil {
				return SettingPlan{}, ErrSetting
			}
			values = append(values, *entry)
		}
		if kind == "emoji" {
			out.NoSave = true
			var emojis []string
			for _, value := range values {
				if strings.Contains(value, ".") {
					value = textprefs.FilterUploadEmojiFileName(value)
				}
				emojis = append(emojis, value)
			}
			out.Data.Editor.Emoji = emojis
			break
		}
		if len(values) > 0 {
			if kind == "virtualInclude" {
				out.Data.Editor.VirtualBlockRefInclude = textprefs.AddKeywords(out.Data.Editor.VirtualBlockRefInclude, values)
			} else {
				out.Data.Editor.VirtualBlockRefExclude = textprefs.AddKeywords(out.Data.Editor.VirtualBlockRefExclude, values)
			}
			effect("virtual", false, 0)
		} else {
			out.NoSave = true
			out.EffectsOnly = true
		}
		effect("confNotify", false, 0)
	case "editorReadOnly":
		if out.Data.Editor == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		var q struct {
			ReadOnly *bool `json:"readonly"`
		}
		if err = decode(&q); err != nil {
			return SettingPlan{}, err
		}
		if q.ReadOnly == nil {
			return SettingPlan{}, ErrSetting
		}
		if out.Data.Editor.ReadOnly != *q.ReadOnly {
			effect("readonly", *q.ReadOnly, 0)
		}
		out.Data.Editor.ReadOnly = *q.ReadOnly
	case "snippet":
		v := &Snpt{}
		err = decode(v)
		out.Data.Snippet = v
		target = v
		effect("snippet", false, 0)
	case "bazaar":
		v := &Bazaar{}
		err = decode(v)
		out.Data.Bazaar = v
		target = v
	case "account":
		v := &Account{}
		err = decode(v)
		out.Data.Account = v
		target = v
	case "keymap":
		v := &Keymap{}
		err = decode(v)
		out.Data.Keymap = v // Original API has no response data.
	case "ai":
		v := &AI{}
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.OpenAI == nil {
			return SettingPlan{}, ErrSetting
		}
		v.OpenAI.APITimeout = max(5, min(600, v.OpenAI.APITimeout))
		v.OpenAI.APIMaxTokens = max(0, v.OpenAI.APIMaxTokens)
		if v.OpenAI.APITemperature <= 0 || v.OpenAI.APITemperature > 2 {
			v.OpenAI.APITemperature = 1
		}
		if v.OpenAI.APIMaxContexts < 1 || v.OpenAI.APIMaxContexts > 64 {
			v.OpenAI.APIMaxContexts = 7
		}
		out.Data.AI = v
		target = v
	case "flashcard":
		v := &Flashcard{}
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.NewCardLimit < 0 {
			v.NewCardLimit = 20
		}
		if v.ReviewCardLimit < 0 {
			v.ReviewCardLimit = 200
		}
		out.Data.Flashcard = v
		target = v
	case "editor":
		if data.Editor == nil || data.Editor.Markdown == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		v := NewEditorWithMarkdown(*data.Editor.Markdown)
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		if v.Markdown == nil {
			return SettingPlan{}, ErrSetting
		}
		if v.PlantUMLServePath == "" {
			v.PlantUMLServePath = "https://www.plantuml.com/plantuml/svg/~1"
		}
		if v.KaTexMacros == "" {
			v.KaTexMacros = "{}"
		}
		if v.HistoryRetentionDays < 1 {
			v.HistoryRetentionDays = 30
		}
		v.HistoryRetentionDays = min(3650, v.HistoryRetentionDays)
		old := data.Editor
		if old.GenerateHistoryInterval != v.GenerateHistoryInterval {
			effect("history", false, v.GenerateHistoryInterval)
		}
		if old.VirtualBlockRef != v.VirtualBlockRef || old.VirtualBlockRefInclude != v.VirtualBlockRefInclude || old.VirtualBlockRefExclude != v.VirtualBlockRefExclude {
			effect("virtual", false, 0)
		}
		if old.ReadOnly != v.ReadOnly {
			effect("readonly", v.ReadOnly, 0)
		}
		effect("markdown", false, 0)
		out.Data.Editor = v
		target = v
	case "fileTree":
		if data.FileTree == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		v := NewFileTreeWithSettings(data.FileTree.UseSingleLineSave, data.FileTree.LargeFileWarningSize)
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		trim := func(p string) string {
			parts := strings.Split(p, "/")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return strings.Join(parts, "/")
		}
		v.RefCreateSavePath = trim(v.RefCreateSavePath)
		if v.RefCreateSavePath != "" && !strings.HasSuffix(v.RefCreateSavePath, "/") {
			v.RefCreateSavePath += "/"
		}
		v.DocCreateSavePath = trim(v.DocCreateSavePath)
		if v.MaxOpenTabCount < 1 {
			v.MaxOpenTabCount = 8
		}
		v.MaxOpenTabCount = min(32, v.MaxOpenTabCount)
		out.Data.FileTree = v
		target = v
		effect("fileTree", false, 0)
	case "search":
		if data.Search == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		v := &Search{}
		if err = decode(v); err != nil {
			return SettingPlan{}, err
		}
		v.Limit = max(32, v.Limit)
		old := data.Search
		effect("search", false, 0)
		if old.CaseSensitive != v.CaseSensitive || old.IndexAssetPath != v.IndexAssetPath {
			effect("reindex", false, 0)
		}
		if old.VirtualRefName != v.VirtualRefName || old.VirtualRefAlias != v.VirtualRefAlias || old.VirtualRefAnchor != v.VirtualRefAnchor || old.VirtualRefDoc != v.VirtualRefDoc {
			effect("virtual", false, 0)
		}
		out.Data.Search = v
		target = v
	default:
		return SettingPlan{}, ErrSetting
	}
	if err != nil {
		return SettingPlan{}, err
	}
	if err = ctx.Err(); err != nil {
		return SettingPlan{}, err
	}
	if target != nil {
		out.Value, err = json.Marshal(target)
		if err != nil {
			return SettingPlan{}, err
		}
	}
	return out, nil
}

// Patches are classified preview deltas, not replacement preference documents.
// Consumers must merge against current documents and use their owner CAS gate.
type PreferencePatch struct {
	Set    map[string]json.RawMessage
	Remove []string
}

func (p SettingPlan) PreferencePatches(before AppData) (map[Scope]PreferencePatch, error) {
	out := map[Scope]PreferencePatch{}
	if p.NoSave {
		return out, nil
	}
	for _, scope := range []Scope{DevicePreference, StorePreference, WorkspacePreference} {
		old, e := before.Preferences(scope)
		if e != nil {
			return nil, e
		}
		next, e := p.Data.Preferences(scope)
		if e != nil {
			return nil, e
		}
		patch := PreferencePatch{Set: map[string]json.RawMessage{}}
		for k, v := range next.Values {
			if string(old.Values[k]) != string(v) {
				patch.Set[k] = append(json.RawMessage(nil), v...)
			}
		}
		for k := range old.Values {
			if _, ok := next.Values[k]; !ok {
				patch.Remove = append(patch.Remove, k)
			}
		}
		sort.Strings(patch.Remove)
		if len(patch.Set) > 0 || len(patch.Remove) > 0 {
			out[scope] = patch
		}
	}
	return out, nil
}
