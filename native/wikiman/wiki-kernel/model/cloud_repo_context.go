// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/siyuan-note/dejavu/cloud"

	"reflect"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/util"
)

func (c *AppConf) newCloudRepoService(execute func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error), directory ...func(context.Context, conf.AppData, cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error)) (*cloudrepo.Service, error) {
	o := cloudrepo.Options{Execute: execute, Load: func(ctx context.Context) (out conf.AppData, err error) {
		err = c.WithSettingSnapshot(ctx, func(v conf.AppData) error { out = v; return nil })
		return
	}}
	if len(directory) > 0 {
		o.Directory = directory[0]
	}
	return cloudrepo.New(o)
}
func (c *AppConf) CloudRepoService() (*cloudrepo.Service, error) {
	if c == nil {
		return nil, cloudrepo.ErrDependency
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed {
		return nil, cloudrepo.ErrClosed
	}
	if c.repoOwner != nil {
		return c.repoOwner, nil
	}
	// Only the compatibility runtime may capture its own process paths here.
	// Detached/native owners must supply an explicit provider capability.
	if c != Conf {
		return nil, cloudrepo.ErrDependency
	}
	p := metadataRepoPaths{Data: util.DataDir, Repo: util.RepoDir, History: util.HistoryDir, Temp: util.TempDir, Agent: util.UserAgent, Workspace: util.WorkspaceDir}
	s, e := c.newSnapshotRepoService(p)
	if e != nil {
		return nil, e
	}
	if c == Conf {
		if e = RegisterLegacyServiceCloser(fmt.Sprintf("repo-owner-%p", c), c.CloseCloudRepo); e != nil {
			_ = s.Close(context.Background())
			return nil, e
		}
	}
	c.repoOwner = s
	return s, nil
}

var errRepoConfigurationChanged = errors.New("cloud_repo_configuration_changed")

// Preserve original normalization/S3 selection without overwriting a newer
// account or provider setting published while the request was in flight.
func (c *AppConf) commitRepoName(ctx context.Context, expected conf.AppData, name string) error {
	return c.WithSettingSnapshot(ctx, func(current conf.AppData) error {
		if !reflect.DeepEqual(current.Sync, expected.Sync) || !reflect.DeepEqual(current.User, expected.User) || current.CloudRegion != expected.CloudRegion {
			return errRepoConfigurationChanged
		}
		if current.Sync == nil {
			return cloudrepo.ErrDependency
		}
		if current.Sync.CloudName == name {
			return nil
		}
		c.m.Lock()
		defer c.m.Unlock()
		if c.persistenceReadOnly {
			return ErrSettingReadOnly
		}
		current.Sync.CloudName = name
		if e := c.saveDataLocked(current); e != nil {
			return e
		}
		c.Sync = current.Sync
		return nil
	})
}
func (c *AppConf) executeLegacyMetadata(ctx context.Context, v conf.AppData, r cloudrepo.Request, p metadataRepoPaths) (cloudrepo.Result, error) {
	if v.Sync == nil {
		return cloudrepo.Result{}, cloudrepo.ErrDependency
	}
	if !cloud.IsValidCloudDirName(v.Sync.CloudName) {
		if e := c.commitRepoName(ctx, v, "main"); e != nil {
			return cloudrepo.Result{}, e
		}
		v.Sync.CloudName = "main"
	}
	if (r.Kind == "logs" || r.Kind == "tags") && (v.Repo == nil || len(v.Repo.Key) == 0) {
		return cloudrepo.Result{}, errors.New(c.business.Language(v.Lang, 26, true, v.CloudRegion))
	}
	out, e := executeMetadata(ctx, v, r, p)
	if e != nil {
		if errors.Is(e, cloud.ErrCloudAuthFailed) {
			return out, errors.New(c.business.Language(v.Lang, 31, true, v.CloudRegion))
		}
		if errors.Is(e, cloudrepo.ErrProvider) {
			return out, errors.New(c.business.Language(v.Lang, 30, true, v.CloudRegion))
		}
		return out, e
	}
	if r.Kind == "repos" && v.Sync.Provider == conf.ProviderS3 {
		var d struct {
			Checked string `json:"checkedSyncDir"`
		}
		if e = json.Unmarshal(out.Data, &d); e != nil {
			return cloudrepo.Result{}, e
		}
		if e = c.commitRepoName(ctx, v, d.Checked); e != nil {
			return cloudrepo.Result{}, e
		}
	}
	return out, nil
}
func (c *AppConf) CloseCloudRepo(ctx context.Context) (err error) {
	defer func() { err = errors.Join(err, c.CloseHTMLExports(ctx)) }()
	c.repoMu.Lock()
	c.repoClosed = true
	c.repoCheckoutClosed = true
	for _, h := range c.repoCheckouts {
		h.Cancel()
	}
	s := c.repoOwner
	tasks := c.repoTasks
	c.repoMu.Unlock()
	if tasks != nil {
		tasks.Stop()
	}
	if s != nil {
		if e := s.Close(ctx); e != nil {
			return e
		}
	}
	if tasks != nil {
		if e := tasks.Wait(ctx); e != nil {
			return e
		}
	}
	unregisterLegacyServiceCloser(fmt.Sprintf("repo-owner-%p", c))
	c.customEmojis.Close()
	return nil
}
func QueryCloudRepoContext(ctx context.Context, kind string, page int) (cloudrepo.Result, error) {
	owner := Conf
	workspace, data := util.WorkspaceDir, util.DataDir
	s, e := owner.CloudRepoService()
	if e != nil {
		return cloudrepo.Result{}, e
	}
	result, e := s.Query(ctx, cloudrepo.Request{Kind: kind, Page: page})
	if e == nil && result.Notice != 0 && workspace == util.WorkspaceDir && data == util.DataDir {
		owner.notifyRepo(ctx, owner.Language(result.Notice), 5000)
	}
	return result, e
}

// Compatibility UI publication shares the owner's close admission. Once close
// marks this owner closed, an already completed request cannot publish late.
func (c *AppConf) notifyRepo(ctx context.Context, msg string, timeout int) {
	if c == nil {
		return
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c != Conf || ctx.Err() != nil {
		return
	}
	util.PushErrMsg(msg, timeout)
}
func (c *AppConf) NotifyCloudRepoError(ctx context.Context, e error, timeout ...int) {
	if e == nil || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return
	}
	duration := 3000
	if len(timeout) > 0 {
		duration = timeout[0]
	}
	c.notifyRepo(ctx, e.Error(), duration)
}
