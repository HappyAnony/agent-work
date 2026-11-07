package workspace

import (
	"context"
	"github.com/88250/lute"
	"maps"
	"wiki-kernel/conf"
	"wiki-kernel/internal/nativeparser"
)

// ConfigurationNotebooks runs the same original configuration business reader
// as AppConf, under the identity's source read admission and resource gate.
func (l *Lease) ConfigurationNotebooks(ctx context.Context) (ret []conf.Notebook, err error) {
	pin, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			ret, err = pin.workspace.resources.configurationBusiness.Notebooks(ctx)
			return err
		})
	})
	return
}

func (l *Lease) ConfigurationLanguage(ctx context.Context, number int) (ret string, err error) {
	pin, err := l.pin(true)
	if err != nil {
		return "", err
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			r := pin.workspace.resources
			if r.configurationBusiness == nil {
				return conf.ErrBusinessUnbound
			}
			ret = r.configurationBusiness.Language(r.configuration.Lang, number, true, r.configuration.CloudRegion)
			return nil
		})
	})
	return
}

// Configuration returns a detached full original configuration snapshot, not
// persistence authority or an arbitrary source file loader. Saved configuration
// changes must later use the source/device services, not mutate this copy.
func (l *Lease) Configuration(ctx context.Context) (conf.AppData, error) {
	pin, err := l.pin(true)
	if err != nil {
		return conf.AppData{}, err
	}
	defer pin.Release()
	var ret conf.AppData
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			var err error
			ret, err = pin.workspace.resources.configuration.Clone()
			return err
		})
	})
	return ret, err
}

// WithConfiguredParser consumes this Context's actual settings using the same
// AppData.ParserOptions factory as the original model.NewLute. No global Conf,
// process environment or current-workspace switch is read.
func (l *Lease) WithConfiguredParser(ctx context.Context, emojis map[string]string, fn func(*lute.Lute) error) error {
	if fn == nil {
		return ErrIdentity
	}
	supplied := maps.Clone(emojis)
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	return pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			r, w := pin.workspace.resources, pin.workspace
			owned, e := r.ownedEmojis(ctx, w)
			if e != nil {
				return e
			}
			for name, src := range supplied {
				owned[name] = src
			}
			options, e := r.configuration.ParserOptions(owned)
			if e != nil {
				return e
			}
			return fn(nativeparser.NewEditor(options))
		})
	})
}
