// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"fmt"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func requestCloudAI(ctx context.Context, region int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
	return requestCloudAccountJSONURL(ctx, util.GetCloudServerForRegion(region)+route, h, b)
}
func (c *AppConf) newAIService(open func(conf.OpenAI) (aiflow.Client, error), event func(context.Context, aiflow.Event)) (*aiflow.Service, error) {
	return aiflow.New(aiflow.Options{Open: open, Event: event, Load: func(ctx context.Context) (out aiflow.Settings, err error) {
		err = c.WithSettingSnapshot(ctx, func(v conf.AppData) error {
			if v.AI == nil || v.AI.OpenAI == nil {
				return aiflow.ErrConfig
			}
			out.API = *v.AI.OpenAI
			if v.Editor != nil && v.Editor.Markdown != nil {
				m := *v.Editor.Markdown
				out.Markdown = &m
			}
			return nil
		})
		return
	}})
}
func (c *AppConf) AIService() (*aiflow.Service, error) {
	if c == nil {
		return nil, aiflow.ErrDependency
	}
	c.aiMu.Lock()
	defer c.aiMu.Unlock()
	if c.aiClosed {
		return nil, aiflow.ErrClosed
	}
	if c.aiOwner != nil {
		return c.aiOwner, nil
	}
	workspace, data := util.WorkspaceDir, util.DataDir
	service, e := c.newAIService(aiflow.OpenAI, func(ctx context.Context, event aiflow.Event) {
		if ctx.Err() != nil || c != Conf || workspace != util.WorkspaceDir || data != util.DataDir {
			return
		}
		switch event {
		case aiflow.MissingKey:
			_ = c.WithSettingSnapshot(ctx, func(v conf.AppData) error {
				util.PushMsg(c.business.Language(v.Lang, 193, true, v.CloudRegion), 5000)
				return nil
			})
		case aiflow.Requesting:
			util.PushEndlessProgress("Requesting...")
		case aiflow.Continuing:
			util.PushEndlessProgress("Continue requesting...")
		case aiflow.Clear:
			util.ClearPushProgress(100)
		case aiflow.Failed:
			util.PushErrMsg("Requesting failed, please check kernel log for more details", 3000)
			logging.LogErrorf("create chat completion failed")
		}
	})
	if e != nil {
		return nil, e
	}
	if c == Conf {
		if e = RegisterLegacyServiceCloser(fmt.Sprintf("ai-owner-%p", c), c.CloseAI); e != nil {
			_ = service.Close(context.Background())
			return nil, e
		}
	}
	c.aiOwner = service
	return service, nil
}
func (c *AppConf) CloseAI(ctx context.Context) error {
	c.aiMu.Lock()
	c.aiClosed = true
	s := c.aiOwner
	c.aiMu.Unlock()
	if s != nil {
		if e := s.Close(ctx); e != nil {
			return e
		}
	}
	unregisterLegacyServiceCloser(fmt.Sprintf("ai-owner-%p", c))
	return nil
}
func ChatGPTContext(ctx context.Context, msg string) (string, error) {
	s, e := Conf.AIService()
	if e != nil {
		return "", e
	}
	return s.Chat(ctx, msg)
}
func ChatGPTWithActionContext(ctx context.Context, ids []string, action string) (string, error) {
	owner := Conf
	s, e := owner.AIService()
	if e != nil {
		return "", e
	}
	workspace, data := util.WorkspaceDir, util.DataDir
	return s.Action(ctx, ids, action, func(c context.Context, ids []string, v aiflow.Settings) (string, error) {
		if owner != Conf || workspace != util.WorkspaceDir || data != util.DataDir {
			return "", aiflow.ErrClosed
		}
		if v.Markdown == nil {
			return "", aiflow.ErrConfig
		}
		trees := map[string]*parse.Tree{}
		roots := map[string]*parse.Tree{}
		for _, id := range ids {
			if e := c.Err(); e != nil {
				return "", e
			}
			bt := treenode.GetBlockTree(id)
			if bt == nil {
				continue
			}
			tree := roots[bt.RootID]
			if tree == nil {
				tree, _ = LoadTreeByBlockID(bt.RootID)
				if tree == nil {
					continue
				}
				roots[bt.RootID] = tree
			}
			trees[id] = tree
		}
		if owner != Conf || workspace != util.WorkspaceDir || data != util.DataDir {
			return "", aiflow.ErrClosed
		}
		return aiflow.Blocks(c, ids, trees, *v.Markdown)
	})
}
func CloudChatGPTContext(ctx context.Context, msg string, history []string) (string, bool, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return "", true, e
	}
	p, e := s.CloudChatGPT(ctx, msg, history)
	return p.Text, p.Stop, cloudBusinessError(owner, e)
}
