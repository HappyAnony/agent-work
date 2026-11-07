package model

import (
	"context"
	"encoding/json"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func SetSyncSettingContext(ctx context.Context, kind string, raw json.RawMessage) (value json.RawMessage, err error) {
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(op, life context.Context) error {
		services := conf.SettingServices{DisableSyncPerception: util.Container == util.ContainerDocker, ValidateSyncLocal: func(c context.Context, p string) error { return conf.ValidateSyncLocal(c, p, o.workspace) }}
		ports := SettingPorts{
			"syncInterval": func(c context.Context, p conf.SettingPlan, e conf.SettingEffect) error {
				if er := o.guard(c); er != nil {
					return er
				}
				o.owner.planSyncAfter(time.Duration(e.Int) * time.Second)
				return nil
			},
			"syncPerception": func(c context.Context, p conf.SettingPlan, e conf.SettingEffect) error {
				if er := o.guard(c); er != nil {
					return er
				}
				if e.Bool {
					return connectSyncWebSocketContext(life, o.owner)
				}
				webSocketConnLock.Lock()
				foreign := syncWebSocketOwner != nil && syncWebSocketOwner != o.owner
				webSocketConnLock.Unlock()
				if foreign {
					return conf.ErrBusinessUnbound
				}
				return closeSyncWebSocketContext(life)
			},
		}
		var e error
		value, e = o.owner.ApplySettingWithServices(op, kind, raw, ports, services)
		return e
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}
