// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/siyuan-note/httpclient"
	"github.com/siyuan-note/logging"
	"net/url"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/announcement"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func decodeCloudUser(value string, persisted bool) (u *conf.User, err error) {
	// The inherited AES decoder can panic on corrupt lengths/padding. Keep the
	// wire/storage codec, but reject corrupt cloud or device input safely.
	defer func() {
		if recover() != nil {
			u = nil
			err = cloudaccount.ErrResponse
		}
	}()
	raw := util.AESDecrypt(value)
	if persisted {
		raw, err = hex.DecodeString(string(raw))
		if err != nil {
			return nil, cloudaccount.ErrResponse
		}
	}
	if err = json.Unmarshal(raw, &u); err != nil || u == nil {
		return nil, cloudaccount.ErrResponse
	}
	return u, nil
}
func requestCloudAccount(ctx context.Context, region int, route string, headers, body map[string]string) (cloudaccount.Response, error) {
	return requestCloudAccountURL(ctx, util.GetCloudServerForRegion(region)+route, headers, body)
}
func requestCloudAccountURL(ctx context.Context, address string, headers, body map[string]string) (cloudaccount.Response, error) {
	var payload any
	if body != nil {
		payload = body
	}
	return requestCloudAccountJSONURL(ctx, address, headers, payload)
}
func requestCloudAccountJSONURL(ctx context.Context, address string, headers map[string]string, body any) (cloudaccount.Response, error) {
	result := map[string]any{}
	request := httpclient.NewCloudRequest30s().SetContext(ctx).SetSuccessResult(&result)
	target, e := url.Parse(address)
	if e != nil {
		return cloudaccount.Response{}, cloudaccount.ErrResponse
	}
	if target.Path == "/apis/siyuan/useActivationcode" || target.Path == "/apis/siyuan/inbox/removeCloudShorthands" || target.Path == "/apis/siyuan/calendar/setBlockReminder" || target.Path == "/apis/siyuan/ai/chatGPT" {
		// The shared client retries errors/503 once. Redemption can already have
		// succeeded remotely (as may Inbox removal); do not replay these writes.
		request.SetRetryCount(0)
	}
	if body != nil {
		request.SetBody(body)
	}
	for k, v := range headers {
		request.SetHeader(k, v)
	}
	resp, e := request.Post(address)
	if e != nil {
		return cloudaccount.Response{}, e
	}
	if resp == nil {
		return cloudaccount.Response{}, cloudaccount.ErrResponse
	}
	return cloudaccount.Response{Status: resp.StatusCode, Body: result}, nil
}
func (c *AppConf) newCloudAccount(request func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error), uploads ...cloudaccount.UploadTransport) (*cloudaccount.Service, error) {
	return c.newCloudAccountWithInbox(request, requestCloudInbox, uploads...)
}
func (c *AppConf) newCloudAccountWithInbox(request func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error), inbox cloudaccount.InboxTransport, uploads ...cloudaccount.UploadTransport) (*cloudaccount.Service, error) {
	return c.newCloudAccountWithReminder(request, inbox, requestCloudReminder, uploads...)
}
func (c *AppConf) newCloudAccountWithReminder(request func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error), inbox cloudaccount.InboxTransport, reminder cloudaccount.ReminderTransport, uploads ...cloudaccount.UploadTransport) (*cloudaccount.Service, error) {
	return c.newCloudAccountWithAnnouncements(request, inbox, reminder, originalAnnouncementOptions(c), uploads...)
}
func (c *AppConf) newCloudAccountWithAnnouncements(request func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error), inbox cloudaccount.InboxTransport, reminder cloudaccount.ReminderTransport, ann *announcement.Options, uploads ...cloudaccount.UploadTransport) (*cloudaccount.Service, error) {
	upload := cloudaccount.UploadTransport(requestCloudUpload)
	if len(uploads) > 0 {
		upload = uploads[0]
	}
	return cloudaccount.New(cloudaccount.Options{Upload: upload, InboxRequest: inbox, ReminderRequest: reminder, Announcements: ann, AIRequest: requestCloudAI,
		Load: func(ctx context.Context) (out cloudaccount.State, err error) {
			err = c.WithSettingSnapshot(ctx, func(v conf.AppData) error {
				out = cloudaccount.State{Region: v.CloudRegion, Cipher: v.UserData, User: v.User}
				return nil
			})
			return
		},
		Commit: func(ctx context.Context, patch cloudaccount.Change) error {
			return c.WithSettingSnapshot(ctx, func(data conf.AppData) error {
				c.m.Lock()
				defer c.m.Unlock()
				if c.persistenceReadOnly && !patch.RuntimeOnly && !patch.Refresh {
					return ErrSettingReadOnly
				}
				if c.confPath == "" {
					return ErrUnboundAppConf
				}
				if e := ctx.Err(); e != nil {
					return e
				}
				if patch.Region != nil {
					data.CloudRegion = *patch.Region
				}
				if patch.SetUser {
					data.UserData = patch.Cipher
					data.User = patch.User
				}
				copy, e := data.Clone()
				if e != nil {
					return e
				}
				if !patch.RuntimeOnly {
					if e = c.saveDataLocked(copy); e != nil {
						return e
					}
				}
				c.AppData = copy
				if c == Conf && patch.Region != nil {
					util.CurrentCloudRegion = *patch.Region
				}
				return nil
			})
		}, Request: request, Decode: decodeCloudUser,
		Encode: func(u *conf.User) (string, error) {
			raw, e := json.Marshal(u)
			if e != nil {
				return "", e
			}
			value := util.AESEncrypt(string(raw))
			if value == "" {
				return "", cloudaccount.ErrResponse
			}
			return value, nil
		}, Now: time.Now,
	})
}
func (c *AppConf) CloudAccount() (*cloudaccount.Service, error) {
	if c == nil {
		return nil, cloudaccount.ErrDependency
	}
	c.cloudMu.Lock()
	defer c.cloudMu.Unlock()
	if c.cloudClosed {
		return nil, cloudaccount.ErrClosed
	}
	if c.cloudAccount == nil {
		v, e := c.newCloudAccount(requestCloudAccount)
		if e != nil {
			return nil, e
		}
		// Registration is explicit, not an import-time subscription. The selected
		// owner closes its active requests and delayed jobs before DB teardown.
		if c == Conf {
			if e = RegisterLegacyServiceCloser(fmt.Sprintf("cloud-account-%p", c), c.CloseCloudAccount); e != nil {
				_ = v.Close(context.Background())
				return nil, e
			}
		}
		c.cloudAccount = v
		c.cloudTasks = taskgroup.Open()
	}
	return c.cloudAccount, nil
}
func (c *AppConf) CloseCloudAccount(ctx context.Context) error {
	c.cloudMu.Lock()
	c.cloudClosed = true
	service, tasks := c.cloudAccount, c.cloudTasks
	c.cloudMu.Unlock()
	if tasks != nil {
		tasks.Stop()
	}
	var e error
	if service != nil {
		e = service.Close(ctx)
	}
	if tasks != nil {
		e = errors.Join(e, tasks.Wait(ctx))
	}
	if e == nil {
		unregisterLegacyServiceCloser(fmt.Sprintf("cloud-account-%p", c))
	}
	return e
}
func (c *AppConf) ScheduleCloudRefresh(delay time.Duration) error {
	service, e := c.CloudAccount()
	if e != nil {
		return e
	}
	state, e := service.Snapshot(context.Background())
	if e != nil {
		return e
	}
	if state.User == nil {
		return nil
	}
	c.cloudMu.Lock()
	defer c.cloudMu.Unlock()
	if c.cloudClosed {
		return cloudaccount.ErrClosed
	}
	if c.cloudRefreshPending {
		return nil
	}
	c.cloudRefreshPending = true
	if !c.cloudTasks.Go(func(ctx context.Context) {
		defer func() { c.cloudMu.Lock(); c.cloudRefreshPending = false; c.cloudMu.Unlock() }()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		// Read the current owner, never a token captured before logout.
		_, e := service.RefreshCurrent(ctx)
		if e != nil && ctx.Err() == nil {
			logging.LogErrorf("cloud account refresh failed")
		}
		_ = service.ResetExpirationReminder(ctx)
	}) {
		c.cloudRefreshPending = false
		return cloudaccount.ErrClosed
	}
	return nil
}
func LoginContext(ctx context.Context, name, password, captcha string, region int) (cloudaccount.Result, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return cloudaccount.Result{}, e
	}
	return s.Login(ctx, name, password, captcha, region)
}
func Login2faContext(ctx context.Context, token, code string) (map[string]any, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return nil, e
	}
	return s.Login2FA(ctx, token, code)
}
func RefreshUserContext(ctx context.Context, token string) (*conf.User, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return nil, e
	}
	u, e := s.Refresh(ctx, token)
	if errors.Is(e, cloudaccount.ErrSession) {
		util.PushErrMsg(owner.Language(19), 5000)
	}
	return u, e
}
func LogoutUserContext(ctx context.Context) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	return s.Logout(ctx)
}
