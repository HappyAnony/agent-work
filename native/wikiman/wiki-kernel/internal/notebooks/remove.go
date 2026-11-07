// SPDX-License-Identifier: AGPL-3.0-or-later
package notebooks

import (
	"context"
	"errors"
)

type RemovePorts struct {
	Backup, Assets, Unmount, Delete, Changed func(context.Context, string) error
}

// Remove preserves original backup/asset retention/unmount/delete order. Ports
// carry explicit source authority; failures retain earlier successful effects.
func Remove(ctx context.Context, id string, p RemovePorts) error {
	if ctx == nil || !idPattern.MatchString(id) {
		return ErrPath
	}
	if p.Backup == nil || p.Assets == nil || p.Unmount == nil || p.Delete == nil || p.Changed == nil {
		return errors.New("notebook_remove_unbound")
	}
	actions := []func(context.Context, string) error{}
	if !Guide(id) {
		actions = append(actions, p.Backup, p.Assets)
	}
	actions = append(actions, p.Unmount, p.Delete, p.Changed)
	for _, action := range actions {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := action(ctx, id); e != nil {
			return e
		}
	}
	return ctx.Err()
}
