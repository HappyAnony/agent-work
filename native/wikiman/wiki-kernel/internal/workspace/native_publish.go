// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/publishsvc"
)

// StartNativePublish applies only the acquired owner's existing configuration
// to an explicitly bound local runtime. It does not save preferences or source.
// Native setting candidates still require the later formal publication path.
func (l *Lease) StartNativePublish(ctx context.Context) (port uint16, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.publish == nil {
			return publishsvc.ErrDependency
		}
		var e error
		port, e = r.publish.Apply(ctx, r.configuration.Publish)
		return e
	})
	return
}
func (l *Lease) StopNativePublish(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.publish == nil {
			return publishsvc.ErrDependency
		}
		if r.configuration.Publish == nil {
			return publishsvc.ErrDependency
		}
		p := *r.configuration.Publish
		p.Enable = false
		_, e := r.publish.Apply(ctx, &p)
		return e
	})
}
