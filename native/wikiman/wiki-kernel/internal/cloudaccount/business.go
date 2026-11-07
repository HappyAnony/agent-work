// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/conf"
)

var ErrAuthentication = errors.New("cloud_account_authentication_required")
var ErrAssetChanged = errors.New("cloud_upload_asset_changed")

type BusinessError struct{ Message string }

func (e *BusinessError) Error() string { return e.Message }

type uploadKey struct {
	Region                              int
	ID, Token                           string
	Expire, Plan, Status, Type, OneTime float64
}
type uploadCache struct {
	valid bool
	key   uploadKey
	token string
	at    time.Time
}

func key(v State) uploadKey {
	u := v.User
	return uploadKey{v.Region, u.UserId, u.UserToken, u.UserSiYuanProExpireTime, u.UserSiYuanSubscriptionPlan, u.UserSiYuanSubscriptionStatus, u.UserSiYuanSubscriptionType, u.UserSiYuanOneTimePayStatus}
}
func authenticated(v State) error {
	if v.User == nil || v.User.UserToken == "" {
		return ErrAuthentication
	}
	return nil
}
func (s *Service) authenticatedRequest(ctx context.Context, v State, route string) (Response, error) {
	if e := authenticated(v); e != nil {
		return Response{}, e
	}
	cookie := (&http.Cookie{Name: "symphony", Value: v.User.UserToken}).String()
	r, e := s.options.Request(ctx, v.Region, route, map[string]string{"Cookie": cookie}, nil)
	if ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	if e != nil {
		return Response{}, ErrNetwork
	}
	return r, nil
}
func businessReply(r Response, unauthorized bool) error {
	if r.Status == http.StatusUnauthorized && unauthorized {
		return ErrAuthentication
	}
	if r.Status != http.StatusOK {
		return ErrNetwork
	}
	n, e := code(r.Body)
	if e != nil {
		return e
	}
	if n == 0 {
		return nil
	}
	msg, ok := r.Body["msg"].(string)
	if !ok || msg == "" {
		return ErrResponse
	}
	return &BusinessError{msg}
}
func (s *Service) StartFreeTrial(ctx context.Context) error {
	return s.accountBusiness(ctx, "/apis/siyuan/user/startFreeTrial", false)
}
func (s *Service) Deactivate(ctx context.Context) error {
	return s.accountBusiness(ctx, "/apis/siyuan/user/deactivate", true)
}
func (s *Service) accountBusiness(ctx context.Context, route string, unauthorized bool) error {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return e
	}
	// Remote entitlement/account changes may succeed even when the response is
	// lost or canceled. Do not retain a credential minted before that attempt.
	s.upload = uploadCache{}
	r, e := s.authenticatedRequest(ctx, v, route)
	if e != nil {
		return e
	}
	e = businessReply(r, unauthorized)
	// Trial changes entitlements; successful deactivation must never leave an
	// upload credential usable from the previous account state. Neither original
	// endpoint logs out or rewrites local account configuration implicitly.
	if e == nil || r.Status == http.StatusUnauthorized {
		s.upload = uploadCache{}
	}
	return e
}
func (s *Service) uploadToken(ctx context.Context, v State) (string, error) {
	if e := authenticated(v); e != nil {
		s.upload = uploadCache{}
		return "", e
	}
	identity := key(v)
	now := s.options.Now()
	if s.upload.valid && s.upload.key == identity && !now.Before(s.upload.at) && now.Sub(s.upload.at) <= time.Hour {
		return s.upload.token, nil
	}
	s.upload = uploadCache{}
	r, e := s.authenticatedRequest(ctx, v, "/apis/siyuan/upload/token")
	if e != nil {
		return "", e
	}
	if e = businessReply(r, true); e != nil {
		return "", e
	}
	data, ok := r.Body["data"].(map[string]any)
	if !ok {
		return "", ErrResponse
	}
	token, ok := data["uploadToken"].(string)
	if !ok || strings.TrimSpace(token) == "" || len(token) > 16<<10 {
		return "", ErrResponse
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	s.upload = uploadCache{valid: true, key: identity, token: token, at: now}
	return token, nil
}
func (s *Service) UploadToken(ctx context.Context) (string, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return "", e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return "", e
	}
	return s.uploadToken(ctx, v)
}

const UploadAssetsBusiness = "upload-assets"
const ExportForumBusiness = "export-liandi"

type UploadTransport func(context.Context, int, string, UploadFile, string, string) (Response, error)
type UploadFile struct {
	Name string
	Data []byte
}
type UploadEvent struct {
	Kind, Asset string
	Total       int
	Limit       int64
}

// Callbacks are bounded, honor ctx and never reenter the account owner. Resolve
// returns an opaque file key (empty means missing). Open never grants write IO.
type UploadPorts struct {
	Resolve func(context.Context, string) (string, error)
	Open    func(context.Context, string) (io.ReadCloser, int64, error)
	Event   func(context.Context, UploadEvent) error
}
type uploadReader struct {
	ctx context.Context
	io.Reader
}

func (r uploadReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.Reader.Read(p)
}

// The same serialized owner covers token acquisition, immutable per-file bytes
// and each upload. Logout/account switch cannot interleave mid-batch. Partial
// remote uploads are counted and never claimed to have been rolled back.
func (s *Service) UploadAssets(ctx context.Context, paths []string, biz string, ports UploadPorts) (count int, err error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return 0, e
	}
	defer done()
	if biz != UploadAssetsBusiness && biz != ExportForumBusiness {
		return 0, ErrResponse
	}
	if len(paths) == 0 {
		return 0, nil
	}
	if len(paths) > 10000 {
		return 0, ErrResponse
	}
	if s.options.Upload == nil || ports.Resolve == nil || ports.Open == nil {
		return 0, ErrDependency
	}
	v, e := s.state(ctx)
	if e != nil {
		return 0, e
	}
	subscriber := (conf.AppData{User: v.User}).IsSubscriber()
	if biz == UploadAssetsBusiness && !subscriber {
		return 0, nil
	}
	var files []string
	seen := map[string]bool{}
	for _, p := range paths {
		if e = ctx.Err(); e != nil {
			return 0, e
		}
		name, e := ports.Resolve(ctx, p)
		if e != nil {
			return 0, e
		}
		if name != "" && !seen[name] {
			seen[name] = true
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return 0, nil
	}
	emit := func(event UploadEvent) error {
		if ports.Event == nil {
			return nil
		}
		return ports.Event(ctx, event)
	}
	defer func() {
		if ports.Event != nil {
			err = errors.Join(err, ports.Event(context.WithoutCancel(ctx), UploadEvent{Kind: "end", Total: count}))
		}
	}()
	if e = emit(UploadEvent{Kind: "begin", Total: len(files)}); e != nil {
		return 0, e
	}
	token, e := s.uploadToken(ctx, v)
	if e != nil {
		return 0, e
	}
	limit := int64(3 << 20)
	if subscriber {
		limit = 10 << 20
	}
	meta := "5"
	if biz == ExportForumBusiness {
		meta = "4"
	}
	oversize := 0
	for _, name := range files {
		if e = ctx.Err(); e != nil {
			return count, e
		}
		f, size, e := ports.Open(ctx, name)
		if e != nil {
			return count, e
		}
		if f == nil {
			return count, ErrDependency
		}
		if size < 0 {
			return count, errors.Join(ErrAssetChanged, f.Close(), ctx.Err())
		}
		if size > limit {
			e = errors.Join(f.Close(), ctx.Err())
			if e != nil {
				return count, e
			}
			oversize++
			if oversize <= 3 {
				if e = emit(UploadEvent{Kind: "oversize", Asset: name, Limit: limit}); e != nil {
					return count, e
				}
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(uploadReader{ctx, f}, limit+1))
		e = errors.Join(readErr, f.Close(), ctx.Err())
		if e != nil {
			return count, e
		}
		if len(data) != int(size) || int64(len(data)) > limit {
			return count, ErrAssetChanged
		}
		if e = emit(UploadEvent{Kind: "upload", Asset: name, Limit: limit}); e != nil {
			return count, e
		}
		r, e := s.options.Upload(ctx, v.Region, token, UploadFile{Name: filepath.Base(name), Data: bytes.Clone(data)}, biz, meta)
		if ctx.Err() != nil {
			return count, ctx.Err()
		}
		if e != nil {
			return count, ErrNetwork
		}
		if e = businessReply(r, true); e != nil {
			if errors.Is(e, ErrAuthentication) {
				s.upload = uploadCache{}
			}
			return count, e
		}
		count++
		if e = emit(UploadEvent{Kind: "uploaded", Asset: name}); e != nil {
			return count, e
		}
	}
	return count, nil
}
