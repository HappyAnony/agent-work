package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"runtime"
	"wiki-kernel/conf"
	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/util"
)

func ExtensionPreferencesContext(ctx context.Context, q extensionprefs.Request, after func()) (out any, err error) {
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	q.Value = append(json.RawMessage(nil), q.Value...)
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	var cfg conf.AppData
	if err = o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error { cfg = v; return o.guard(ctx) }); err != nil {
		return nil, err
	}
	if (q.Action == "petal.list" || q.Action == "petal.set") && cfg.Bazaar == nil {
		return nil, conf.ErrBusinessUnbound
	}
	o.owner.m.Lock()
	if o.owner.extensionPreferences == nil {
		o.owner.extensionPreferences = &extensionprefs.Service{}
	}
	s := o.owner.extensionPreferences
	readonly := o.owner.persistenceReadOnly
	o.owner.m.Unlock()
	backend := runtime.GOOS
	switch util.Container {
	case util.ContainerDocker, util.ContainerIOS, util.ContainerAndroid, util.ContainerHarmony:
		backend = util.Container
	}
	ports := extensionprefs.Ports{NewID: ast.NewNodeID, Language: cfg.Lang, Backend: backend, RequireTrust: util.Container == util.ContainerStd || util.Container == util.ContainerDocker, Repair: !readonly}
	if q.Publish {
		ports.Repair = false
	}
	if cfg.Bazaar != nil {
		ports.Trusted, ports.Disabled = cfg.Bazaar.Trust, cfg.Bazaar.PetalDisabled
	}
	err = o.run(ctx, func(op, life context.Context) error {
		root, e := os.OpenRoot(o.data)
		if os.IsNotExist(e) {
			switch q.Action {
			case "petal.metadata", "petal.list":
				out = []*Petal{}
				return nil
			case "snippet.list":
				out = []*conf.Snippet{}
				return nil
			case "petal.replace":
				var v []*Petal
				if json.Unmarshal(q.Value, &v) == nil && len(v) == 0 {
					out = []*Petal{}
					return nil
				}
			}
		}
		if e != nil {
			return e
		}
		defer root.Close()
		files := ownedResourceFiles{owner: o, root: root}
		ports.Read, ports.Write, ports.ReadDir = files.read, files.write, files.readDir
		out, e = s.Do(op, q, ports)
		if errors.Is(e, extensionprefs.ErrIncompatible) && o.owner.business != nil {
			e = errors.New(o.owner.business.Language(cfg.Lang, 205, true, cfg.CloudRegion))
		}
		if e != nil {
			return e
		}
		if after != nil {
			o.owner.publishSyncUI(life, after)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
func LoadSnippetsContext(ctx context.Context) ([]*conf.Snippet, error) {
	v, e := ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "snippet.list"}, nil)
	if e != nil {
		return nil, e
	}
	return v.([]*conf.Snippet), nil
}

func LoadSnippetsForRoleContext(ctx context.Context, publish bool) ([]*conf.Snippet, error) {
	v, e := ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "snippet.list", Publish: publish}, nil)
	if e != nil {
		return nil, e
	}
	return v.([]*conf.Snippet), nil
}
func SetSnippetContext(ctx context.Context, s []*conf.Snippet) error {
	raw, e := json.Marshal(s)
	if e != nil {
		return e
	}
	_, e = ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "snippet.set", Value: raw}, nil)
	return e
}
func RemoveSnippetContext(ctx context.Context, id string) (*conf.Snippet, error) {
	v, e := ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "snippet.remove", Name: id}, nil)
	if e != nil {
		return nil, e
	}
	return v.(*conf.Snippet), nil
}
func LoadPetalsContext(ctx context.Context, frontend string, publish bool) ([]*Petal, error) {
	v, e := ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "petal.list", Frontend: frontend, Publish: publish}, nil)
	if e != nil {
		return nil, e
	}
	return v.([]*Petal), nil
}
func GetPetalsContext(ctx context.Context) ([]*Petal, error) {
	v, e := ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "petal.metadata"}, nil)
	if e != nil {
		return nil, e
	}
	return v.([]*Petal), nil
}
func SavePetalsContext(ctx context.Context, petals []*Petal) error {
	raw, e := json.Marshal(petals)
	if e != nil {
		return e
	}
	_, e = ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "petal.replace", Value: raw}, nil)
	return e
}
func SetPetalEnabledContext(ctx context.Context, name string, enabled bool, frontend, app string, notify bool) (*Petal, error) {
	var after func()
	if notify {
		after = func() {
			if enabled {
				pushReloadPlugin0([]string{name}, []string{}, []string{}, app)
			} else {
				pushReloadPlugin0([]string{}, []string{}, []string{name}, app)
			}
		}
	}
	v, e := ExtensionPreferencesContext(ctx, extensionprefs.Request{Action: "petal.set", Name: name, Enabled: enabled, Frontend: frontend}, after)
	if e != nil {
		return nil, e
	}
	return v.(*Petal), nil
}
