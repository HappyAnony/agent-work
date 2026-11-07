// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/appearance"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeAppearance(ctx context.Context) (out *conf.Appearance, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.appearance == nil {
			return appearance.ErrDependency
		}
		var e error
		out, e = r.appearance.Prepare(ctx, r.configuration.Appearance)
		return e
	})
	return
}

// Installs only explicitly bound device resources, not source/preferences.
func (l *Lease) StartNativeAppearance(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.appearance == nil {
			return appearance.ErrDependency
		}
		value, e := r.appearance.Prepare(ctx, r.configuration.Appearance)
		if e != nil {
			return e
		}
		if e = r.appearance.Apply(ctx, value); e != nil {
			return e
		}
		r.configuration.Appearance = value
		r.configuration.Lang = value.Lang
		return nil
	})
}
func (l *Lease) StopNativeAppearance(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.appearance == nil {
			return appearance.ErrDependency
		}
		return r.appearance.Stop(ctx)
	})
}
