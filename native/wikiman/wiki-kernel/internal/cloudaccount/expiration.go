// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"math"
	"time"
	"wiki-kernel/conf"
)

type ExpirationNotice struct {
	Expired  bool
	Days     int
	Delay    time.Duration
	identity uploadKey
}

func (s *Service) ResetExpirationReminder(ctx context.Context) error {
	_, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	s.expirationChecked = false
	s.expirationNotice = nil
	return nil
}
func (s *Service) PlanExpirationReminder(ctx context.Context, ios bool) (*ExpirationNotice, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return nil, e
	}
	identity := uploadKey{Region: v.Region}
	if v.User != nil {
		identity = key(v)
	}
	if s.expirationChecked && s.expirationKey == identity {
		return nil, nil
	}
	s.expirationChecked = true
	s.expirationKey = identity
	s.expirationNotice = nil
	if ios || !(conf.AppData{User: v.User}).IsSubscriber() || v.User.UserSiYuanProExpireTime == -1 {
		return nil, nil
	}
	expire := v.User.UserSiYuanProExpireTime
	if math.IsNaN(expire) || math.IsInf(expire, 0) || expire >= float64(math.MaxInt64) {
		return nil, ErrResponse
	}
	expired, now := int64(expire), s.options.Now().UnixMilli()
	n := ExpirationNotice{identity: identity}
	if now >= expired {
		if now-expired > int64(48*time.Hour/time.Millisecond) {
			return nil, nil
		}
		n.Expired = true
		n.Delay = 30 * time.Second
	} else {
		remains := (expired - now) / int64(24*time.Hour/time.Millisecond)
		days := int64(15)
		if v.User.UserSiYuanSubscriptionPlan == 2 {
			days = 3
		}
		if remains <= 0 || remains >= days {
			return nil, nil
		}
		n.Days = int(remains)
		n.Delay = 7 * time.Second
	}
	s.expirationNotice = &n
	out := n
	return &out, nil
}
func (s *Service) DeliverExpirationReminder(ctx context.Context, n ExpirationNotice, notify func(context.Context, ExpirationNotice) error) error {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	if notify == nil {
		return ErrDependency
	}
	if s.expirationNotice == nil || *s.expirationNotice != n {
		return nil
	}
	s.expirationNotice = nil
	v, e := s.state(ctx)
	if e != nil {
		return e
	}
	if v.User == nil || key(v) != n.identity || !(conf.AppData{User: v.User}).IsSubscriber() {
		return nil
	}
	return notify(ctx, n)
}
func WaitReminder(ctx context.Context, delay time.Duration) error {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return ctx.Err()
	}
}

// Caller owns this callback's complete lifetime. Timers and delivery use the same
// ctx; no detached legacy queue or token captured before logout is used.
func (s *Service) RunExpirationReminder(ctx context.Context, ios bool, wait func(context.Context, time.Duration) error, notify func(context.Context, ExpirationNotice) error) error {
	if wait == nil || notify == nil {
		return ErrDependency
	}
	n, e := s.PlanExpirationReminder(ctx, ios)
	if e != nil || n == nil {
		return e
	}
	if e = wait(ctx, n.Delay); e != nil {
		return e
	}
	return s.DeliverExpirationReminder(ctx, *n, notify)
}
