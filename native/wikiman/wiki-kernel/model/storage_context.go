package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/filelock"
	"io"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/treenode"
)

func StorageContext(ctx context.Context, q pageprefs.Request) (any, error) {
	return StorageContextWithAfter(ctx, q, nil)
}

func (c *AppConf) preferencesService() *pageprefs.Service {
	c.m.Lock()
	defer c.m.Unlock()
	if c.pagePreferences == nil {
		c.pagePreferences = &pageprefs.Service{}
	}
	return c.pagePreferences
}

func StorageContextWithAfter(ctx context.Context, q pageprefs.Request, after func()) (out any, err error) {
	q.Value = append([]byte(nil), q.Value...)
	q.Keys = append([]string(nil), q.Keys...)
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	registry := treenode.CaptureLegacyRepairRegistry()
	o.owner.m.Lock()
	if o.owner.pagePreferences == nil {
		o.owner.pagePreferences = &pageprefs.Service{}
	}
	service := o.owner.pagePreferences
	readonly := o.owner.persistenceReadOnly
	o.owner.m.Unlock()
	err = o.run(ctx, func(op, life context.Context) (runErr error) {
		backend, closeBackend, e := o.preferenceBackend(op, readonly, registry)
		if e != nil {
			return e
		}
		defer func() { runErr = errors.Join(runErr, closeBackend()) }()
		out, e = service.Do(op, q, backend)
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

func (o *legacyReindexOwner) preferenceBackend(op context.Context, readonly bool, registry treenode.RepairRegistry) (pageprefs.Backend, func() error, error) {
	root, e := os.OpenRoot(o.data)
	if e != nil {
		return pageprefs.Backend{}, nil, e
	}
	backend := pageprefs.Backend{Now: time.Now, RepairRecent: !readonly, RefreshRecent: true, BadName: func() error { return errors.New(o.owner.Language(142)) }}
	backend.Read = func(c context.Context, name string) (raw []byte, err error) {
		if err = o.guard(c); err != nil {
			return nil, err
		}
		f, err := root.Open(filepath.Join("storage", name))
		if err != nil {
			return nil, err
		}
		raw, err = io.ReadAll(io.LimitReader(f, (16<<20)+1))
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, err
		}
		if len(raw) > 16<<20 {
			return nil, errors.New("storage_budget_exceeded")
		}
		return raw, c.Err()
	}
	backend.Write = func(c context.Context, name string, raw []byte) error {
		if e := o.guard(c); e != nil {
			return e
		}
		o.owner.m.Lock()
		ro := o.owner.persistenceReadOnly
		o.owner.m.Unlock()
		if ro {
			return ErrSettingReadOnly
		}
		p := filepath.Join(o.data, "storage", name)
		filelock.Lock(p)
		defer filelock.Unlock(p)
		if e := o.guard(c); e != nil {
			return e
		}
		if e := root.Mkdir("storage", 0755); e != nil && !os.IsExist(e) {
			return e
		}
		f, e := root.OpenFile(filepath.Join("storage", name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		_, e = f.Write(raw)
		return errors.Join(e, f.Close())
	}
	backend.Paths = func(c context.Context, ids []string) (map[string]string, error) {
		if registry.DB != treenode.CaptureLegacyRepairRegistry().DB {
			return nil, conf.ErrBusinessUnbound
		}
		rows, e := registry.Query(c, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
		if e != nil {
			return nil, e
		}
		out := map[string]string{}
		for _, r := range rows {
			out[r.ID] = r.HPath
		}
		return out, nil
	}
	return backend, root.Close, nil
}
