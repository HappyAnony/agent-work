// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"wiki-kernel/conf"
)

type NativeSettingPlan struct {
	Setting conf.SettingPlan
	Patches map[conf.Scope]conf.PreferencePatch
}

// PlanNativeSetting uses the acquired owner's actual merged configuration. It
// neither publishes runtime effects nor modifies preferences/live resources.
// Classified patches require later merge/CAS after live leases are released.
func (l *Lease) PlanNativeSetting(ctx context.Context, kind string, raw json.RawMessage) (out NativeSettingPlan, err error) {
	if len(raw) > 4<<20 {
		return out, conf.ErrSetting
	}
	raw = append(json.RawMessage(nil), raw...)
	pin, e := l.pin(true)
	if e != nil {
		return out, e
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			before := pin.workspace.resources.configuration
			plan, e := before.PrepareSettingWithServices(ctx, kind, raw, pin.workspace.resources.settingServices)
			if e != nil {
				return e
			}
			patches, e := plan.PreferencePatches(before)
			if e != nil {
				return e
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			out = NativeSettingPlan{plan, patches}
			return nil
		})
	})
	if err != nil {
		return NativeSettingPlan{}, err
	}
	return
}
