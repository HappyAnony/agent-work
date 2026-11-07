// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/reminder"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func requestCloudReminder(ctx context.Context, region int, route string, h map[string]string, body map[string]any) (cloudaccount.Response, error) {
	return requestCloudAccountJSONURL(ctx, util.GetCloudServerForRegion(region)+route, h, body)
}
func SetCloudBlockReminderContext(ctx context.Context, id, data string, timed int64) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	return cloudBusinessError(owner, s.SetCloudBlockReminder(ctx, reminder.Payload{ID: id, Content: data, Milliseconds: timed}))
}

type blockReminderEnvironment struct {
	gate     *attributeMutationGate
	readOnly bool
	guard    func(context.Context) error
	wait     func(context.Context) error
	load     func(context.Context, string) (*parse.Tree, map[string]string, error)
	stage    func(context.Context, *parse.Tree) error
	cache    func(context.Context, string, map[string]string) error
	sync     func(context.Context) error
	notify   func(context.Context, string, int) error
	services reminder.Services
}

func (owner *AppConf) blockReminderEnvironment() *blockReminderEnvironment {
	workspace := util.WorkspaceDir
	dataDir := util.DataDir
	guard := func(ctx context.Context) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if owner != Conf || workspace != util.WorkspaceDir || dataDir != util.DataDir {
			return errors.New("reminder_owner_changed")
		}
		if util.ReadOnly || owner.persistenceReadOnly {
			return ErrSettingReadOnly
		}
		return nil
	}
	return &blockReminderEnvironment{gate: legacyAttributeMutationGate, readOnly: owner.persistenceReadOnly || util.ReadOnly, guard: guard,
		wait: func(ctx context.Context) error {
			if e := guard(ctx); e != nil {
				return e
			}
			if e := FlushTxQueueContext(ctx); e != nil {
				return e
			}
			return guard(ctx)
		},
		load: func(ctx context.Context, id string) (*parse.Tree, map[string]string, error) {
			if e := guard(ctx); e != nil {
				return nil, nil, e
			}
			tree, e := LoadTreeByBlockID(id)
			if e != nil {
				return nil, nil, e
			}
			attrs := cache.GetBlockIAL(id)
			if attrs == nil {
				attrs = map[string]string{}
				if tree != nil && tree.Root != nil {
					ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
						if enter && n.ID == id {
							for _, v := range n.KramdownIAL {
								if len(v) == 2 {
									attrs[v[0]] = html.UnescapeAttrVal(v[1])
								}
							}
							return ast.WalkStop
						}
						return ast.WalkContinue
					})
				}
			}
			return tree, attrs, guard(ctx)
		},
		stage: func(ctx context.Context, t *parse.Tree) error {
			if e := guard(ctx); e != nil {
				return e
			}
			return indexWriteTreeUpsertQueue(t)
		},
		cache: func(ctx context.Context, id string, a map[string]string) error {
			if e := guard(ctx); e != nil {
				return e
			}
			cache.PutBlockIAL(id, a)
			return nil
		},
		sync: func(ctx context.Context) error {
			if e := guard(ctx); e != nil {
				return e
			}
			IncSync()
			return nil
		},
		notify: func(ctx context.Context, msg string, ms int) error {
			if e := guard(ctx); e != nil {
				return e
			}
			util.PushMsg(msg, ms)
			return nil
		},
		services: reminder.Services{Location: time.Local, Language: owner.Language, Content: func(ctx context.Context, n *ast.Node) (string, error) {
			if e := guard(ctx); e != nil {
				return "", e
			}
			v, e := sql.NodeStaticContentContext(ctx, n, nil, false, false, false)
			if e != nil {
				return "", e
			}
			return v, guard(ctx)
		}},
	}
}
func SetBlockReminderContext(ctx context.Context, id, timed string) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	e = setBlockReminderWithEnvironment(ctx, s, id, timed, owner.blockReminderEnvironment())
	if errors.Is(e, cloudaccount.ErrSubscription) {
		n := 29
		if util.Container == "ios" {
			n = 122
		}
		return errors.New(owner.Language(n))
	}
	if errors.Is(e, reminder.ErrMissing) {
		return fmt.Errorf(owner.Language(15), id)
	}
	return cloudBusinessError(owner, e)
}
func setBlockReminderWithEnvironment(ctx context.Context, s *cloudaccount.Service, id, timed string, e *blockReminderEnvironment) error {
	if e == nil || s == nil || e.gate == nil || e.guard == nil || e.wait == nil || e.load == nil || e.stage == nil || e.cache == nil || e.sync == nil || e.notify == nil || e.services.Content == nil || e.services.Language == nil || e.services.Location == nil {
		return reminder.ErrDependency
	}
	if e.readOnly {
		return ErrSettingReadOnly
	}
	return e.gate.with(ctx, func() error {
		return s.ExecuteBlockReminder(ctx, func(c context.Context, send func(reminder.Payload) error) error {
			if err := e.guard(c); err != nil {
				return err
			}
			if err := e.wait(c); err != nil {
				return err
			}
			t, attrs, err := e.load(c, id)
			if err != nil {
				return err
			}
			p, err := reminder.Prepare(c, id, timed, t, attrs, e.services)
			if err != nil {
				return err
			}
			if err = e.guard(c); err != nil {
				return err
			}
			if err = send(p.Payload); err != nil {
				return err
			}
			if err = e.guard(c); err != nil {
				return fmt.Errorf("reminder_remote_applied_local_not_staged: %w", err)
			}
			old := p.Node.KramdownIAL
			copyIAL := make([][]string, len(old))
			for i, v := range old {
				copyIAL[i] = append([]string(nil), v...)
			}
			if timed == "0" {
				p.Node.RemoveIALAttr(reminder.Attribute)
			} else {
				p.Node.SetIALAttr(reminder.Attribute, timed)
			}
			if err = e.stage(c, p.Tree); err != nil {
				p.Node.KramdownIAL = copyIAL
				return fmt.Errorf("reminder_remote_applied_local_not_staged: %w", err)
			}
			// Once the original queue accepts the tree, finish its accepted effects.
			finish := context.WithoutCancel(c)
			if err = e.sync(finish); err == nil {
				err = e.cache(finish, id, p.Attrs)
			}
			if err == nil && p.Notice != "" {
				err = e.notify(finish, p.Notice, p.NoticeMilliseconds)
			}
			if err != nil {
				return fmt.Errorf("reminder_local_staged_effect_failed: %w", err)
			}
			return nil
		})
	})
}
