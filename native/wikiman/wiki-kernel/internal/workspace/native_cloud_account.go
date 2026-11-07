// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
)

// Credential persistence is an explicit private-device capability. These calls
// never publish source receipts or insert credentials into page preferences.
func (l *Lease) NativeCloudLogin(ctx context.Context, name, password, captcha string, region int) (out cloudaccount.Result, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.cloudAccount == nil {
			return cloudaccount.ErrDependency
		}
		var e error
		out, e = r.cloudAccount.Login(ctx, name, password, captcha, region)
		return errors.Join(e, r.syncCloudRuntime(context.WithoutCancel(ctx)))
	})
	return
}
func (l *Lease) NativeCloudLogin2FA(ctx context.Context, token, code string) (out map[string]any, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.cloudAccount == nil {
			return cloudaccount.ErrDependency
		}
		var e error
		out, e = r.cloudAccount.Login2FA(ctx, token, code)
		return e
	})
	return
}
func (l *Lease) NativeCloudRefresh(ctx context.Context, token string) (out *conf.User, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.cloudAccount == nil {
			return cloudaccount.ErrDependency
		}
		var e error
		out, e = r.cloudAccount.Refresh(ctx, token)
		return errors.Join(e, r.syncCloudRuntime(context.WithoutCancel(ctx)))
	})
	return
}
func (l *Lease) NativeCloudLogout(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		if r.cloudAccount == nil {
			return cloudaccount.ErrDependency
		}
		if e := r.cloudAccount.Logout(ctx); e != nil {
			return e
		}
		r.configuration.User = nil
		r.configuration.UserData = ""
		return nil
	})
}

// The existing task owner holds the creation lease, cancels on drain and joins
// the actual callback. Read the current account after the delay, not an old token.
func (l *Lease) StartNativeCloudRefresh(ctx context.Context, delay time.Duration) (*Task, error) {
	if delay < 0 {
		return nil, ErrIdentity
	}
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		if l.workspace.resources.cloudAccount == nil {
			return cloudaccount.ErrDependency
		}
		return nil
	}); e != nil {
		return nil, e
	}
	return l.StartTask(ctx, func(ctx context.Context, lease *Lease) error {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		return lease.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
			r := lease.workspace.resources
			if r.cloudAccount == nil {
				return cloudaccount.ErrDependency
			}
			_, e := r.cloudAccount.RefreshCurrent(ctx)
			return errors.Join(e, r.syncCloudRuntime(context.WithoutCancel(ctx)))
		})
	})
}

func (r *resources) syncCloudRuntime(ctx context.Context) error {
	v, e := r.cloudAccount.Snapshot(ctx)
	if e != nil {
		return e
	}
	r.configuration.User = v.User
	r.configuration.UserData = v.Cipher
	r.configuration.CloudRegion = v.Region
	return nil
}

// Device account requests have an owned task lifetime without holding source
// read locks during the explicitly configured network request.
func (l *Lease) NativeDeviceCloudAccount(ctx context.Context, action, token string) (*conf.User, error) {
	result := make(chan *conf.User, 1)
	err := l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error {
		var user *conf.User
		var err error
		switch action {
		case "status":
			var v cloudaccount.State
			v, err = s.Snapshot(c)
			user = v.User
		case "connect":
			user, err = s.Refresh(c, token)
		case "refresh":
			user, err = s.RefreshCurrent(c)
		case "logout":
			err = s.Logout(c)
		default:
			return ErrIdentity
		}
		result <- user
		return err
	})
	if err != nil {
		return nil, err
	}
	return <-result, nil
}

func (l *Lease) NativeDeviceCloudSignIn(ctx context.Context, region int, name, password, captcha string) (cloudaccount.Result, error) {
	result := make(chan cloudaccount.Result, 1)
	err := l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, service *cloudaccount.Service) error {
		reply, err := service.Login(c, name, password, captcha, region)
		if err == nil && reply.Code == 0 {
			token, _ := reply.Data["token"].(string)
			err = finishDeviceCloudSignIn(c, service, token)
		}
		result <- reply
		return err
	})
	if err != nil {
		return cloudaccount.Result{}, err
	}
	return <-result, nil
}

func (l *Lease) NativeDeviceCloudVerify(ctx context.Context, token, code string) error {
	return l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, service *cloudaccount.Service) error {
		reply, err := service.Login2FA(c, token, code)
		if err != nil {
			return err
		}
		if reply["code"] != float64(0) {
			return cloudaccount.ErrSession
		}
		verified, _ := reply["token"].(string)
		if data, ok := reply["data"].(map[string]any); ok && verified == "" {
			verified, _ = data["token"].(string)
		}
		return finishDeviceCloudSignIn(c, service, verified)
	})
}

func finishDeviceCloudSignIn(ctx context.Context, service *cloudaccount.Service, token string) error {
	if token == "" || len(token) > 2048 {
		return cloudaccount.ErrResponse
	}
	_, err := service.Refresh(ctx, token)
	return err
}
