package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"io/fs"
	"path"
	"runtime"
	"wiki-kernel/conf"
	"wiki-kernel/internal/extensionprefs"
	"wiki-kernel/internal/indexdb"
)

type NativeExtensionPlan struct {
	Value        json.RawMessage
	SourceWrites map[string]json.RawMessage
}

func (l *Lease) PlanNativeExtensions(ctx context.Context, q extensionprefs.Request) (out NativeExtensionPlan, err error) {
	q.Value = append(json.RawMessage(nil), q.Value...)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		cfg := r.configuration
		if cfg.Bazaar == nil && (q.Action == "petal.list" || q.Action == "petal.set") {
			return conf.ErrBusinessUnbound
		}
		container := ""
		if cfg.System != nil {
			container = cfg.System.Container
		}
		backend := runtime.GOOS
		switch container {
		case "docker", "ios", "android", "harmony":
			backend = container
		}
		p := extensionprefs.Ports{NewID: ast.NewNodeID, Language: cfg.Lang, Backend: backend, RequireTrust: container == "" || container == "std" || container == "docker", Repair: !q.Publish}
		if cfg.Bazaar != nil {
			p.Trusted, p.Disabled = cfg.Bazaar.Trust, cfg.Bazaar.PetalDisabled
		}
		out.SourceWrites = map[string]json.RawMessage{}
		p.Read = func(ctx context.Context, name string) ([]byte, error) {
			if w.data == nil {
				return nil, fs.ErrNotExist
			}
			f, e := openRootFile(w.data, name)
			if e != nil {
				return nil, e
			}
			raw, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
			e = errors.Join(e, f.Close())
			if e != nil {
				return nil, e
			}
			if len(raw) > 16<<20 {
				return nil, ErrBudget
			}
			return raw, ctx.Err()
		}
		p.ReadDir = func(ctx context.Context, name string) ([]fs.DirEntry, error) {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if w.data == nil {
				return nil, fs.ErrNotExist
			}
			entries, e := fs.ReadDir(w.data.FS(), name)
			if e != nil {
				return nil, e
			}
			return entries, ctx.Err()
		}
		p.Write = func(ctx context.Context, name string, raw []byte) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			if name != "snippets/conf.json" && name != "storage/petal/petals.json" {
				return ErrIdentity
			}
			out.SourceWrites[path.Clean(name)] = append(json.RawMessage(nil), raw...)
			return nil
		}
		v, e := r.extensionPreferences.Do(ctx, q, p)
		if e != nil {
			return e
		}
		out.Value, e = json.Marshal(v)
		return e
	})
	if err != nil {
		return NativeExtensionPlan{}, err
	}
	return out, nil
}
