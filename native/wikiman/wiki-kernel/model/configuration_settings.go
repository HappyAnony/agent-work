// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/siyuan-note/filelock"
	"os"
	"wiki-kernel/bazaar"
	"wiki-kernel/conf"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

var ErrSettingReadOnly = errors.New("configuration_owner_read_only")
var ErrSettingEffects = errors.New("configuration_saved_effect_failed")
var ErrSettingRuntimeEffects = errors.New("configuration_runtime_effect_failed")

// SettingPorts are explicit post-save effects. Prepare validates every required
// port before touching disk; no missing binding falls back to global services.
type SettingPorts map[string]func(context.Context, conf.SettingPlan, conf.SettingEffect) error

func (c *AppConf) saveDataLocked(data conf.AppData) error {
	if c.confPath == "" {
		return ErrUnboundAppConf
	}
	if c.persistenceReadOnly {
		return nil
	}
	raw, e := gulu.JSON.MarshalIndentJSON(data, "", "  ")
	if e != nil {
		return e
	}
	old, e := filelock.ReadFile(c.confPath)
	if e == nil && bytes.Equal(raw, old) {
		return nil
	}
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	return filelock.WriteFile(c.confPath, raw)
}

// ApplySetting serializes this owner's setting operations, persists its private
// path, then publishes memory and ordered effects. A post-save effect error is
// reported without pretending the already committed configuration was undone.
func (c *AppConf) ApplySetting(ctx context.Context, kind string, raw json.RawMessage, ports SettingPorts) (json.RawMessage, error) {
	return c.ApplySettingWithServices(ctx, kind, raw, ports, conf.SettingServices{})
}

// WithSettingSnapshot serializes runtime readers with setting publication.
// The callback gets detached data, runs outside m, and must not reenter this gate.
func (c *AppConf) WithSettingSnapshot(ctx context.Context, fn func(conf.AppData) error) error {
	if fn == nil {
		return conf.ErrBusinessUnbound
	}
	c.m.Lock()
	if c.settingsGate == nil {
		c.settingsGate = make(chan struct{}, 1)
	}
	gate := c.settingsGate
	c.m.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	c.m.Lock()
	data, e := c.AppData.Clone()
	c.m.Unlock()
	if e != nil {
		return e
	}
	return fn(data)
}

func (c *AppConf) ApplySettingWithServices(ctx context.Context, kind string, raw json.RawMessage, ports SettingPorts, services conf.SettingServices) (json.RawMessage, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	c.m.Lock()
	if c.settingsGate == nil {
		c.settingsGate = make(chan struct{}, 1)
	}
	gate := c.settingsGate
	c.m.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.m.Lock()
	if c.persistenceReadOnly {
		c.m.Unlock()
		return nil, ErrSettingReadOnly
	}
	if c.confPath == "" {
		c.m.Unlock()
		return nil, ErrUnboundAppConf
	}
	plan, e := c.AppData.PrepareSettingWithServices(ctx, kind, raw, services)
	bound := make(SettingPorts, len(plan.Effects))
	if e == nil {
		for _, effect := range plan.Effects {
			fn := ports[effect.Kind]
			if fn == nil {
				e = conf.ErrBusinessUnbound
				break
			}
			bound[effect.Kind] = fn
		}
	}
	var published conf.AppData
	if e == nil {
		published, e = plan.Data.Clone()
	}
	if e == nil {
		e = ctx.Err()
	}
	if e == nil && !plan.NoSave {
		e = c.saveDataLocked(published)
	}
	if e == nil && !plan.EffectsOnly {
		c.AppData = published
	}
	c.m.Unlock()
	if e != nil {
		return nil, e
	}
	// Publication is accepted (durable or explicitly transient). Preserve request
	// identity values and finish effects even if the HTTP caller disconnects now.
	effectCtx := context.WithoutCancel(ctx)
	for _, effect := range plan.Effects {
		if e = bound[effect.Kind](effectCtx, plan, effect); e != nil {
			marker := ErrSettingEffects
			if plan.NoSave {
				marker = ErrSettingRuntimeEffects
			}
			return nil, errors.Join(marker, e)
		}
	}
	return append(json.RawMessage(nil), plan.Value...), nil
}

// The old HTTP handler is the only caller of this compatibility adapter. Native
// services plan against a Lease and never import it or these process effects.
func SetLegacyConfiguration(ctx context.Context, kind string, raw json.RawMessage) (json.RawMessage, error) {
	return setLegacyConfiguration(ctx, kind, raw, true)
}

func SetLegacyConfigurationWithPorts(ctx context.Context, kind string, raw json.RawMessage, extra SettingPorts) (json.RawMessage, error) {
	return setLegacyConfigurationPorts(ctx, kind, raw, true, extra)
}

func setLegacyConfiguration(ctx context.Context, kind string, raw json.RawMessage, notify bool) (json.RawMessage, error) {
	return setLegacyConfigurationPorts(ctx, kind, raw, notify, nil)
}
func setLegacyConfigurationPorts(ctx context.Context, kind string, raw json.RawMessage, notify bool, extra SettingPorts) (out json.RawMessage, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	raw = append(json.RawMessage(nil), raw...)
	err = base.run(ctx, func(c, _ context.Context) error {
		var e error
		out, e = applyLegacyConfigurationPorts(c, base.owner, kind, raw, notify, extra)
		return e
	})
	if err != nil {
		out = nil
	}
	return
}
func applyLegacyConfigurationPorts(ctx context.Context, owner *AppConf, kind string, raw json.RawMessage, notify bool, extra SettingPorts) (json.RawMessage, error) {
	if owner == nil {
		return nil, ErrUnboundAppConf
	}
	wrap := func(fn func(conf.SettingPlan, conf.SettingEffect)) func(context.Context, conf.SettingPlan, conf.SettingEffect) error {
		return func(ctx context.Context, p conf.SettingPlan, e conf.SettingEffect) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if Conf != owner {
				return conf.ErrBusinessUnbound
			}
			fn(p, e)
			return nil
		}
	}
	ports := SettingPorts{
		"pandoc": wrap(func(p conf.SettingPlan, _ conf.SettingEffect) { util.PandocBinPath = p.Data.Export.PandocBin }),
		"confNotify": wrap(func(p conf.SettingPlan, _ conf.SettingEffect) {
			if notify {
				util.BroadcastByType("main", "setConf", 0, "", p.Data)
			}
		}),
		"history": wrap(func(_ conf.SettingPlan, e conf.SettingEffect) { ChangeHistoryTick(e.Int) }),
		"virtual": func(ctx context.Context, plan conf.SettingPlan, _ conf.SettingEffect) error {
			if Conf != owner {
				return conf.ErrBusinessUnbound
			}
			if plan.Data.Editor != nil && !plan.Data.Editor.VirtualBlockRef {
				virtualBlockRefCache.Clear()
				return nil
			}
			cache, err := virtualCacheFromSnapshot(ctx, plan.Data)
			if err != nil {
				return err
			}
			return cache.rebuild(ctx)
		},
		"readonly": wrap(func(_ conf.SettingPlan, e conf.SettingEffect) {
			util.BroadcastByType("protyle", "readonly", 0, "", e.Bool)
			util.BroadcastByType("main", "readonly", 0, "", e.Bool)
		}),
		"snippet": wrap(func(p conf.SettingPlan, _ conf.SettingEffect) {
			util.BroadcastByType("main", "setSnippet", 0, "", p.Data.Snippet)
		}),
		"markdown": wrap(func(p conf.SettingPlan, _ conf.SettingEffect) {
			settings := *p.Data.Editor.Markdown
			util.MarkdownSettings = &settings
		}),
		"fileTree": wrap(func(p conf.SettingPlan, _ conf.SettingEffect) {
			util.UseSingleLineSave = p.Data.FileTree.UseSingleLineSave
			util.LargeFileWarningSize = p.Data.FileTree.LargeFileWarningSize
		}),
		"search": wrap(func(p conf.SettingPlan, _ conf.SettingEffect) {
			sql.SetCaseSensitive(p.Data.Search.CaseSensitive)
			sql.SetIndexAssetPath(p.Data.Search.IndexAssetPath)
		}),
		"reindex": wrap(func(conf.SettingPlan, conf.SettingEffect) { FullReindex() }),
	}
	if kind == "appearance" {
		ports["appearanceRuntime"] = func(ctx context.Context, p conf.SettingPlan, _ conf.SettingEffect) error {
			if Conf != owner {
				return conf.ErrBusinessUnbound
			}
			resources, e := legacyAppearanceOwner(owner)
			if e != nil {
				return e
			}
			if e = resources.Apply(ctx, p.Data.Appearance); e != nil {
				return e
			}
			util.StatusBarCfg = p.Data.Appearance.StatusBar
			util.Lang = p.Data.Lang
			util.InitEmojiChars()
			return nil
		}
		ports["appearanceMarket"] = wrap(func(conf.SettingPlan, conf.SettingEffect) {
			owner.catalogueService().Clear()
			bazaar.CleanBazaarPackageCache()
		})
		ports["appearanceNotify"] = wrap(func(p conf.SettingPlan, _ conf.SettingEffect) {
			util.BroadcastByType("main", "setAppearance", 0, "", p.Data.Appearance)
		})
	}
	if owner.business != nil {
		ports["pandocWarning"] = wrap(func(_ conf.SettingPlan, e conf.SettingEffect) {
			util.PushErrMsg(fmt.Sprintf(owner.Language(117), e.Text), 5000)
		})
	}
	for kind, fn := range extra {
		if _, exists := ports[kind]; exists {
			return nil, conf.ErrBusinessUnbound
		}
		ports[kind] = fn
	}
	return owner.ApplySettingWithServices(ctx, kind, raw, ports, conf.SettingServices{
		NormalizeAppearance: func(ctx context.Context, input *conf.Appearance) (*conf.Appearance, error) {
			resources, e := legacyAppearanceOwner(owner)
			if e != nil {
				return nil, e
			}
			return resources.Prepare(ctx, input)
		},
		CurrentPandoc: func(ctx context.Context) (string, error) {
			if e := ctx.Err(); e != nil {
				return "", e
			}
			if Conf != owner {
				return "", conf.ErrBusinessUnbound
			}
			return util.PandocBinPath, nil
		},
		ValidatePandoc: func(ctx context.Context, path string) (bool, error) {
			if Conf != owner {
				return false, conf.ErrBusinessUnbound
			}
			return util.ValidatePandocBin(ctx, path)
		},
	})
}
