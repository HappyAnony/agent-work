// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"errors"
	"net/http"
	"wiki-kernel/conf"
	"wiki-kernel/internal/reminder"
)

var ErrSubscription = errors.New("cloud_subscription_required")

// ReminderTransport carries the original numeric timed JSON field. A write must
// honor ctx, be sent once, and never log block content or account credentials.
type ReminderTransport func(context.Context, int, string, map[string]string, map[string]any) (Response, error)

func (s *Service) sendReminder(ctx context.Context, v State, p reminder.Payload) error {
	if s.options.ReminderRequest == nil {
		return ErrDependency
	}
	if e := authenticated(v); e != nil {
		return e
	}
	cookie := (&http.Cookie{Name: "symphony", Value: v.User.UserToken}).String()
	r, e := s.options.ReminderRequest(ctx, v.Region, "/apis/siyuan/calendar/setBlockReminder", map[string]string{"Cookie": cookie}, map[string]any{"dataId": p.ID, "data": p.Content, "timed": p.Milliseconds})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e != nil {
		return ErrNetwork
	}
	if r.Status == 401 {
		s.upload = uploadCache{}
	}
	return businessReply(r, true)
}

// Low-level cloud compatibility port; subscription checks belong to the block
// workflow, exactly as before. This call grants no local source-write authority.
func (s *Service) SetCloudBlockReminder(ctx context.Context, p reminder.Payload) error {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return e
	}
	return s.sendReminder(ctx, v, p)
}

// One account admission spans preparation, network IO and the caller's explicit
// local continuation. Login/logout cannot replace the account mid-operation.
func (s *Service) ExecuteBlockReminder(ctx context.Context, run func(context.Context, func(reminder.Payload) error) error) error {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	if run == nil || s.options.ReminderRequest == nil {
		return ErrDependency
	}
	v, e := s.state(ctx)
	if e != nil {
		return e
	}
	if !(conf.AppData{User: v.User}).IsSubscriber() {
		return ErrSubscription
	}
	if e = authenticated(v); e != nil {
		return e
	}
	return run(ctx, func(p reminder.Payload) error { return s.sendReminder(ctx, v, p) })
}
