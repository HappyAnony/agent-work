// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"fmt"
	"github.com/siyuan-note/logging"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func (c *AppConf) ScheduleSubscriptionReminder() error {
	workspace := util.WorkspaceDir
	return c.scheduleSubscriptionReminder(util.Container == "ios", cloudaccount.WaitReminder, func(ctx context.Context, n cloudaccount.ExpirationNotice) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if c != Conf || workspace != util.WorkspaceDir {
			return nil
		}
		msg := c.Language(128)
		if !n.Expired {
			msg = fmt.Sprintf(c.Language(127), n.Days)
		}
		util.PushErrMsg(msg, 0)
		return nil
	})
}
func (c *AppConf) scheduleSubscriptionReminder(ios bool, wait func(context.Context, time.Duration) error, notify func(context.Context, cloudaccount.ExpirationNotice) error) error {
	s, e := c.CloudAccount()
	if e != nil {
		return e
	}
	if wait == nil || notify == nil {
		return cloudaccount.ErrDependency
	}
	c.cloudMu.Lock()
	defer c.cloudMu.Unlock()
	if c.cloudClosed {
		return cloudaccount.ErrClosed
	}
	if c.cloudReminderPending {
		return nil
	}
	if c.cloudTasks == nil {
		c.cloudTasks = taskgroup.Open()
	}
	c.cloudReminderPending = true
	if !c.cloudTasks.Go(func(ctx context.Context) {
		defer func() { c.cloudMu.Lock(); c.cloudReminderPending = false; c.cloudMu.Unlock() }()
		if e := s.RunExpirationReminder(ctx, ios, wait, notify); e != nil && ctx.Err() == nil {
			logging.LogErrorf("subscription reminder failed")
		}
	}) {
		c.cloudReminderPending = false
		return cloudaccount.ErrClosed
	}
	return nil
}
