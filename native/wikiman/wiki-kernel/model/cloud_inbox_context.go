// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

func requestCloudInbox(ctx context.Context, region int, route string, h map[string]string, body map[string]any) (cloudaccount.Response, error) {
	var payload any
	if body != nil {
		payload = body
	}
	return requestCloudAccountJSONURL(ctx, util.GetCloudServerForRegion(region)+route, h, payload)
}

func (owner *AppConf) inboxPresentation(ctx context.Context) (out cloudaccount.InboxPresentation, err error) {
	err = owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		opts, e := v.ParserOptions(owner.customEmojis.Snapshot())
		if e != nil {
			return e
		}
		out = cloudaccount.InboxPresentation{Parser: nativeparser.NewEditor(opts), Location: time.Local}
		return ctx.Err()
	})
	return
}
func GetCloudShorthandContext(ctx context.Context, id string) (map[string]any, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return nil, e
	}
	p, e := owner.inboxPresentation(ctx)
	if e != nil {
		return nil, e
	}
	v, e := s.GetCloudShorthand(ctx, id, p)
	return v, cloudBusinessError(owner, e)
}
func GetCloudShorthandsContext(ctx context.Context, page int) (map[string]any, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return nil, e
	}
	p, e := owner.inboxPresentation(ctx)
	if e != nil {
		return nil, e
	}
	v, e := s.GetCloudShorthands(ctx, page, p)
	return v, cloudBusinessError(owner, e)
}
func RemoveCloudShorthandsContext(ctx context.Context, ids []string) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	return cloudBusinessError(owner, s.RemoveCloudShorthands(ctx, ids))
}
