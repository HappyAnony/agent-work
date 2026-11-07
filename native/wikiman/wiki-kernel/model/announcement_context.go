// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/siyuan-note/httpclient"
	"github.com/siyuan-note/logging"
	"net/url"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/announcement"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func requestAnnouncement(ctx context.Context, region int) (announcement.Response, error) {
	return requestAnnouncementURL(ctx, util.GetCloudServerForRegion(region)+"/apis/siyuan/version?ver="+url.QueryEscape(util.Ver))
}
func requestAnnouncementURL(ctx context.Context, address string) (announcement.Response, error) {
	body := map[string]any{}
	r, e := httpclient.NewCloudRequest30s().SetContext(ctx).SetSuccessResult(&body).Get(address)
	if e != nil {
		return announcement.Response{}, e
	}
	if r == nil {
		return announcement.Response{}, announcement.ErrResponse
	}
	return announcement.Response{Status: r.StatusCode, Body: body}, nil
}
func originalAnnouncementOptions(owner *AppConf) *announcement.Options {
	home, workspace := util.HomeDir, util.WorkspaceDir
	if !filepath.IsAbs(home) {
		return nil
	} // Partial/test account owners need not bind device IO.
	store, e := announcement.NewFileStore(home)
	if e != nil {
		return nil
	}
	guard := func(ctx context.Context) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if owner != Conf || home != util.HomeDir || workspace != util.WorkspaceDir {
			return errors.New("announcement_owner_changed")
		}
		return nil
	}
	ttl := 6 * time.Hour
	if util.Container == util.ContainerDocker {
		ttl = 24 * time.Hour
	}
	return &announcement.Options{Store: store, Fetch: requestAnnouncement, Now: time.Now, CacheTTL: ttl, Region: func(ctx context.Context) (region int, err error) {
		if err = guard(ctx); err != nil {
			return
		}
		err = owner.WithSettingSnapshot(ctx, func(d conf.AppData) error { region = d.CloudRegion; return nil })
		return
	}, Notify: func(ctx context.Context, item announcement.Item) error {
		if e := guard(ctx); e != nil {
			return e
		}
		return owner.WithSettingSnapshot(ctx, func(d conf.AppData) error {
			if d.CloudRegion != item.Region {
				return announcement.ErrRegion
			}
			if e := guard(ctx); e != nil {
				return e
			}
			util.PushMsg(fmt.Sprintf(owner.business.Language(d.Lang, 11, true, d.CloudRegion), item.URL, item.Title), 0)
			return nil
		})
	}}
}
func GetAnnouncementsContext(ctx context.Context) ([]*Announcement, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return nil, e
	}
	items, e := s.AnnouncementFeed(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]*Announcement, 0, len(items))
	for _, item := range items {
		v := item
		out = append(out, &v)
	}
	return out, nil
}
func (c *AppConf) ScheduleAnnouncements() error { return c.scheduleAnnouncements(time.Minute) }
func (c *AppConf) scheduleAnnouncements(delay time.Duration) error {
	if delay < 0 {
		return announcement.ErrDependency
	}
	s, e := c.CloudAccount()
	if e != nil {
		return e
	}
	if !s.HasAnnouncements() {
		return cloudaccount.ErrDependency
	}
	c.cloudMu.Lock()
	defer c.cloudMu.Unlock()
	if c.cloudClosed {
		return cloudaccount.ErrClosed
	}
	if c.cloudAnnouncementPending {
		return nil
	}
	if c.cloudTasks == nil {
		c.cloudTasks = taskgroup.Open()
	}
	c.cloudAnnouncementPending = true
	if !c.cloudTasks.Go(func(ctx context.Context) {
		defer func() { c.cloudMu.Lock(); c.cloudAnnouncementPending = false; c.cloudMu.Unlock() }()
		if e := s.RunAnnouncements(ctx, delay); e != nil && ctx.Err() == nil {
			logging.LogErrorf("refresh announcement failed")
		}
	}) {
		c.cloudAnnouncementPending = false
		return cloudaccount.ErrClosed
	}
	return nil
}
