// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package proxy

import (
	"context"
	"net/http"
	"sync"
	"wiki-kernel/conf"
	"wiki-kernel/internal/publishsvc"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

// Compatibility slot only. Context services construct their own Service and
// auth/transport ports; no global listener or import-started worker is shared.
var legacyPublish struct {
	sync.Mutex
	service *publishsvc.Service
	owner   *model.AppConf
	closed  bool
}

// Compatibility listen address; contextual callers supply an explicit Host.
var Host = "0.0.0.0"

func ConfigurePublishService(ctx context.Context, p *conf.Publish) (uint16, error) {
	if e := model.RegisterLegacyServiceCloser("publish", ClosePublishService); e != nil {
		return 0, e
	}
	legacyPublish.Lock()
	defer legacyPublish.Unlock()
	if legacyPublish.closed {
		return 0, publishsvc.ErrClosed
	}
	owner := model.Conf
	if owner == nil || util.ServerURL == nil {
		return 0, publishsvc.ErrDependency
	}
	if legacyPublish.service != nil && legacyPublish.owner != owner {
		if e := legacyPublish.service.Close(ctx); e != nil {
			return 0, e
		}
		legacyPublish.service = nil
	}
	if legacyPublish.service == nil {
		transport := http.DefaultTransport
		if original, ok := transport.(*http.Transport); ok {
			transport = original.Clone()
		}
		svc, e := publishsvc.New(publishsvc.Options{Host: Host, Target: *util.ServerURL, Transport: transport, Auth: publishsvc.AuthPorts{
			Configure: func(p *conf.Publish) error { return model.ConfigurePublishOwnerAccounts(owner, p) },
			Close:     func() error { return model.ClearPublishOwnerAccounts(owner) },
			Account: func(name string) (publishsvc.Account, bool) {
				a := model.GetBasicAuthAccount(name)
				if a == nil {
					return publishsvc.Account{}, false
				}
				return publishsvc.Account{Username: a.Username, Password: a.Password, Token: a.Token}, true
			},
			Session: model.GetBasicAuthUsernameBySessionID, NewSession: model.GetNewSessionID, AddSession: model.AddSession, DeleteSession: model.DeleteSession,
		}})
		if e != nil {
			return 0, e
		}
		legacyPublish.service = svc
		legacyPublish.owner = owner
	}
	return legacyPublish.service.Apply(ctx, p)
}

func InitPublishService() (uint16, error) { return InitPublishServiceContext(context.Background()) }
func InitPublishServiceContext(ctx context.Context) (port uint16, err error) {
	owner := model.Conf
	if owner == nil {
		return 0, publishsvc.ErrDependency
	}
	err = owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if model.Conf != owner {
			return publishsvc.ErrDependency
		}
		var e error
		port, e = ConfigurePublishService(ctx, data.Publish)
		return e
	})
	return
}
func ClosePublishService(ctx context.Context) error {
	legacyPublish.Lock()
	defer legacyPublish.Unlock()
	legacyPublish.closed = true
	if legacyPublish.service == nil {
		return nil
	}
	return legacyPublish.service.Close(ctx)
}
