package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/filelock"
	"io/fs"
	"os"
	"path/filepath"
	"wiki-kernel/conf"
)

// The catalogue binds a directory handle for the whole operation. A workspace
// close cancels and joins its actual return before process paths can change.
type templateCatalogOwner struct {
	base          *legacyReindexOwner
	caseSensitive bool
}

func captureTemplateCatalog(ctx context.Context) (*templateCatalogOwner, error) {
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	o := &templateCatalogOwner{base: base}
	err = base.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Search == nil {
			return conf.ErrBusinessUnbound
		}
		o.caseSensitive = v.Search.CaseSensitive
		return base.guard(ctx)
	})
	return o, err
}

func (o *templateCatalogOwner) run(ctx context.Context, fn func(context.Context) error) error {
	return o.base.run(ctx, func(op, life context.Context) error { return fn(op) })
}

func (o *templateCatalogOwner) open() (*os.Root, error) {
	data, err := os.OpenRoot(o.base.data)
	if err != nil {
		return nil, err
	}
	defer data.Close()
	return data.OpenRoot("templates")
}

func SearchTemplateContext(ctx context.Context, keyword string) (out []*Block, err error) {
	o, err := captureTemplateCatalog(ctx)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(op context.Context) error {
		root, e := o.open()
		if errors.Is(e, os.ErrNotExist) {
			out = []*Block{}
			return nil
		}
		if e != nil {
			return e
		}
		defer root.Close()
		base := filepath.Join(o.base.data, "templates")
		read := func(string) ([]os.DirEntry, error) { return fs.ReadDir(root.FS(), ".") }
		walk := func(start string, fn fs.WalkDirFunc) error {
			rel, e := filepath.Rel(base, start)
			if e != nil {
				return e
			}
			return fs.WalkDir(root.FS(), filepath.ToSlash(rel), func(p string, d fs.DirEntry, e error) error {
				return fn(filepath.Join(base, filepath.FromSlash(p)), d, e)
			})
		}
		out, e = searchTemplateCatalog(op, base, keyword, o.caseSensitive, read, walk)
		return e
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func RemoveTemplateContext(ctx context.Context, p string) error {
	o, err := captureTemplateCatalog(ctx)
	if err != nil {
		return err
	}
	return o.run(ctx, func(op context.Context) error {
		o.base.owner.m.Lock()
		readOnly := o.base.owner.persistenceReadOnly
		o.base.owner.m.Unlock()
		if readOnly {
			return ErrSettingReadOnly
		}
		base := filepath.Join(o.base.data, "templates")
		rel, err := filepath.Rel(base, p)
		if err != nil || !filepath.IsLocal(rel) || rel == "." {
			return errors.New("template_path_invalid")
		}
		root, err := o.open()
		if err != nil {
			return err
		}
		defer root.Close()
		filelock.Lock(p)
		defer filelock.Unlock(p)
		if err = o.base.guard(op); err != nil {
			return err
		}
		return root.RemoveAll(rel)
	})
}
