// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/workspacefile"
)

// NativeFileRead shares compatibility file algorithms against this page's
// selected data root. Holding a reader never grants source write authority.
func (l *Lease) NativeFileRead(ctx context.Context, q workspacefile.Request) (out workspacefile.Result, err error) {
	if q.Action != "read" && q.Action != "list" {
		return out, indexrepair.ErrSourceWrite
	}
	pin, e := l.pin(true)
	if e != nil {
		return out, e
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			if pin.workspace.data == nil {
				return ErrDocument
			}
			var e error
			out, e = workspacefile.Execute(ctx, indexrepair.Source{Data: pin.workspace.data}, q)
			return e
		})
	})
	if err != nil {
		out = workspacefile.Result{}
	}
	return
}
