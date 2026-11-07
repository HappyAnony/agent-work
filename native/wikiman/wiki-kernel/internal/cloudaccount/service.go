// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package cloudaccount preserves the original account protocol using explicit
// device authority, clock, codec and network ports. Construction starts no IO.
package cloudaccount

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/announcement"
)

var (
	ErrNetwork     = errors.New("cloud_account_connection_failed")
	ErrDependency  = errors.New("cloud_account_unbound")
	ErrClosed      = errors.New("cloud_account_closed")
	ErrResponse    = errors.New("cloud_account_invalid_response")
	ErrSession     = errors.New("cloud_account_refresh_failed")
	ErrInvalidUser = errors.New("invalid user")
)

type State struct {
	Region int
	Cipher string
	User   *conf.User
}

// Change is a field patch, never a replacement of an entire configuration.
type Change struct {
	Region      *int
	SetUser     bool
	RuntimeOnly bool
	Refresh     bool // Original read-only refresh may update memory while saving is skipped.
	Cipher      string
	User        *conf.User
}
type Response struct {
	Status int
	Body   map[string]any
}
type Result struct {
	Code int
	Msg  string
	Data map[string]any
}
type Options struct {
	Load   func(context.Context) (State, error)
	Commit func(context.Context, Change) error
	// Request must honor ctx and never automatically retry useActivationcode:
	// a lost response may follow successful remote redemption.
	Request func(context.Context, int, string, map[string]string, map[string]string) (Response, error)
	Decode  func(string, bool) (*conf.User, error) // bool identifies persisted (hex-wrapped) data.
	Encode  func(*conf.User) (string, error)
	Now     func() time.Time
	// Optional owned multipart transport; required only for an upload batch.
	Upload UploadTransport
	// Optional JSON-array transport, required only by the Inbox handlers.
	InboxRequest    InboxTransport
	ReminderRequest ReminderTransport
	Announcements   *announcement.Options
	AIRequest       AITransport
}
type Service struct {
	options           Options
	gate              chan struct{}
	lifetime          context.Context
	cancel            context.CancelFunc
	upload            uploadCache
	expirationChecked bool
	expirationKey     uploadKey
	expirationNotice  *ExpirationNotice
	announcements     *announcement.Service
}

func New(o Options) (*Service, error) {
	if o.Load == nil || o.Commit == nil || o.Request == nil || o.Decode == nil || o.Encode == nil || o.Now == nil {
		return nil, ErrDependency
	}
	c, cancel := context.WithCancel(context.Background())
	s := &Service{options: o, gate: make(chan struct{}, 1), lifetime: c, cancel: cancel}
	if o.Announcements != nil {
		opts := *o.Announcements
		if opts.Region == nil {
			opts.Region = func(ctx context.Context) (int, error) { v, e := s.Snapshot(ctx); return v.Region, e }
		}
		if opts.Now == nil {
			opts.Now = o.Now
		}
		var e error
		s.announcements, e = announcement.New(opts)
		if e != nil {
			cancel()
			return nil, e
		}
	}
	return s, nil
}
func clone(u *conf.User) (*conf.User, error) {
	if u == nil {
		return nil, nil
	}
	raw, e := json.Marshal(u)
	if e != nil {
		return nil, e
	}
	var out *conf.User
	e = json.Unmarshal(raw, &out)
	return out, e
}
func (s *Service) enter(c context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(c)
	stop := context.AfterFunc(s.lifetime, cancel)
	cleanup := func() { stop(); cancel() }
	select {
	case s.gate <- struct{}{}:
		if s.lifetime.Err() != nil {
			<-s.gate
			cleanup()
			return nil, nil, ErrClosed
		}
		if e := ctx.Err(); e != nil {
			<-s.gate
			cleanup()
			return nil, nil, e
		}
		return ctx, func() { <-s.gate; cleanup() }, nil
	case <-ctx.Done():
		cleanup()
		return nil, nil, ctx.Err()
	}
}
func (s *Service) Close(ctx context.Context) error {
	s.cancel()
	var annErr error
	if s.announcements != nil {
		annErr = s.announcements.Close(ctx)
	}
	select {
	case s.gate <- struct{}{}:
		s.upload = uploadCache{}
		s.expirationNotice = nil
		<-s.gate
		return annErr
	case <-ctx.Done():
		return errors.Join(annErr, ctx.Err())
	}
}
func (s *Service) state(ctx context.Context) (State, error) {
	v, e := s.options.Load(ctx)
	if e != nil {
		return State{}, e
	}
	v.User, e = clone(v.User)
	return v, e
}
func (s *Service) Snapshot(ctx context.Context) (State, error) {
	c, done, e := s.enter(ctx)
	if e != nil {
		return State{}, e
	}
	defer done()
	return s.state(c)
}
func (s *Service) commit(ctx context.Context, v Change) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return s.options.Commit(ctx, v)
}
func code(body map[string]any) (int, error) {
	n, ok := body["code"].(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < -2147483648 || n > 2147483647 {
		return 0, ErrResponse
	}
	return int(n), nil
}
func (s *Service) Login(ctx context.Context, name, password, captcha string, region int) (Result, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return Result{}, e
	}
	defer done()
	// Original behavior persists region even when authentication fails.
	if e = s.commit(ctx, Change{Region: &region}); e != nil {
		return Result{}, e
	}
	s.upload = uploadCache{}
	r, e := s.options.Request(ctx, region, "/apis/siyuan/login", nil, map[string]string{"userName": name, "userPassword": password, "captcha": captcha})
	if e != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, ErrNetwork
	}
	if e = ctx.Err(); e != nil {
		return Result{}, e
	}
	if r.Status != 200 {
		return Result{}, ErrResponse
	}
	n, e := code(r.Body)
	if e != nil {
		return Result{}, e
	}
	msg, ok := r.Body["msg"].(string)
	if !ok {
		return Result{}, ErrResponse
	}
	if n == -1 {
		n = 1
	}
	return Result{Code: n, Msg: msg, Data: map[string]any{"userName": r.Body["userName"], "token": r.Body["token"], "needCaptcha": r.Body["needCaptcha"]}}, nil
}
func (s *Service) Login2FA(ctx context.Context, token, value string) (map[string]any, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return nil, e
	}
	r, e := s.options.Request(ctx, v.Region, "/apis/siyuan/login/2fa", map[string]string{"token": token}, map[string]string{"twofactorAuthCode": value})
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrNetwork
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if r.Status != 200 || r.Body == nil {
		return nil, ErrResponse
	}
	if _, e = code(r.Body); e != nil {
		return nil, e
	}
	return r.Body, nil
}
func expires(u *conf.User) (time.Time, error) {
	if u == nil {
		return time.Time{}, ErrSession
	}
	n, e := strconv.ParseInt(u.UserTokenExpireTime, 10, 64)
	if e != nil {
		return time.Time{}, ErrSession
	}
	return time.Unix(n, 0), nil
}
func (s *Service) Refresh(ctx context.Context, token string) (*conf.User, error) {
	return s.refresh(ctx, token, false)
}
func (s *Service) RefreshCurrent(ctx context.Context) (*conf.User, error) {
	return s.refresh(ctx, "", true)
}
func (s *Service) refresh(ctx context.Context, token string, force bool) (*conf.User, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	state, e := s.state(ctx)
	if e != nil {
		return nil, e
	}
	u := state.User
	if force {
		if u == nil {
			return nil, nil
		}
		token = u.UserToken
	}
	threshold := s.options.Now().Add(72 * time.Hour)
	if token == "" {
		if state.Cipher != "" {
			u, e = s.options.Decode(state.Cipher, true)
			if e != nil {
				return state.User, ErrSession
			}
			u, e = clone(u)
			if e != nil {
				return nil, e
			}
		}
		if u == nil {
			return nil, nil
		}
		if state.Cipher != "" {
			if e = s.commit(ctx, Change{SetUser: true, RuntimeOnly: true, Cipher: state.Cipher, User: u}); e != nil {
				return nil, e
			}
		}
		expiry, e := expires(u)
		if e != nil {
			return u, e
		}
		if !threshold.After(expiry) { // Preserve local hydration without rewriting credentials.
			return u, nil
		}
		token = u.UserToken
	}
	r, e := s.options.Request(ctx, state.Region, "/apis/siyuan/user", nil, map[string]string{"token": token})
	if ce := ctx.Err(); ce != nil {
		return nil, ce
	}
	var fresh *conf.User
	if e == nil {
		if r.Status != 200 {
			e = ErrResponse
		} else {
			var n int
			n, e = code(r.Body)
			if e == nil && n != 0 {
				if n == 255 {
					e = ErrInvalidUser
				} else {
					e = ErrResponse
				}
			}
			if e == nil {
				cipher, ok := r.Body["data"].(string)
				if !ok {
					e = ErrResponse
				} else {
					fresh, e = s.options.Decode(cipher, false)
					if fresh == nil && e == nil {
						e = ErrResponse
					}
				}
			}
		}
	}
	if e != nil {
		if errors.Is(e, ErrInvalidUser) || u == nil {
			return u, ErrSession
		}
		expiry, err := expires(u)
		if err != nil || threshold.After(expiry) {
			return u, ErrSession
		}
		return u, nil
	}
	fresh, e = clone(fresh)
	if e != nil {
		return nil, e
	}
	cipher, e := s.options.Encode(fresh)
	if e != nil {
		return nil, e
	}
	if cipher == "" {
		return nil, ErrResponse
	}
	if e = s.commit(ctx, Change{SetUser: true, Refresh: true, Cipher: cipher, User: fresh}); e != nil {
		return nil, e
	}
	return clone(fresh)
}
func (s *Service) Logout(ctx context.Context) error {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	if e = s.commit(ctx, Change{SetUser: true}); e != nil {
		return e
	}
	s.upload = uploadCache{}
	return nil
}
