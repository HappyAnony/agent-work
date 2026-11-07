// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/siyuan-note/dejavu"
	"os"
	"path/filepath"
	"reflect"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

// Atomic private configuration replacement: persistence failure cannot publish a
// new in-memory key. c.m is held only during commit, never while awaiting repo IO.
func (c *AppConf) saveRepoKeyDataLocked(v conf.AppData) error {
	if c.persistenceReadOnly {
		return ErrSettingReadOnly
	}
	if !filepath.IsAbs(c.confPath) {
		return ErrUnboundAppConf
	}
	b, e := gulu.JSON.MarshalIndentJSON(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(c.confPath), ".wiki-repo-key-conf-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, c.confPath)
}

func (c *AppConf) executeRepoKey(ctx context.Context, expected conf.AppData, r cloudrepo.RepoKeyRequest, key []byte, p metadataRepoPaths) (out cloudrepo.RepoKeyResult, err error) {
	if expected.Repo == nil || expected.Sync == nil {
		return out, cloudrepo.ErrDependency
	}
	if e := validateSnapshotPaths(p); e != nil {
		return out, e
	}
	// Never replace a cache containing this owner's only key configuration.
	repo, e := canonicalDirectoryPath(p.Repo)
	if e != nil {
		return out, cloudrepo.ErrDependency
	}
	confPath, e := canonicalDirectoryPath(c.confPath)
	if e != nil || directoryContains(repo, confPath) {
		return out, cloudrepo.ErrDependency
	}
	c.m.Lock()
	readonly := c.persistenceReadOnly
	c.m.Unlock()
	if readonly {
		return out, ErrSettingReadOnly
	}
	ui := func(cctx context.Context, fn func()) {
		c.repoMu.Lock()
		defer c.repoMu.Unlock()
		if c.repoClosed || c != Conf || p.Workspace != util.WorkspaceDir || p.Data != util.DataDir || cctx.Err() != nil {
			return
		}
		fn()
	}
	lang := func(id int) string { return c.business.Language(expected.Lang, id, true, expected.CloudRegion) }
	msgID := ""
	defer func() {
		if err != nil && msgID != "" {
			ui(context.WithoutCancel(ctx), func() { util.PushClearMsg(msgID) })
		}
	}()
	ui(ctx, func() {
		if r.Kind == "reset" {
			msgID = util.PushMsg(lang(144), 60000)
		} else {
			util.PushMsg(lang(136), 3000)
		}
	})
	var updated conf.AppData
	err = c.WithSettingSnapshot(ctx, func(current conf.AppData) error {
		if !reflect.DeepEqual(current.Repo, expected.Repo) || !reflect.DeepEqual(current.Sync, expected.Sync) {
			return errRepoConfigurationChanged
		}
		result, e := dejavu.ReplaceRepositoryContext(ctx, p.Repo, func() error {
			c.m.Lock()
			defer c.m.Unlock()
			latest, e := c.AppData.Clone()
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(latest.Repo, expected.Repo) || !reflect.DeepEqual(latest.Sync, expected.Sync) {
				return errRepoConfigurationChanged
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			latest.Repo.Key = append([]byte(nil), key...)
			if r.Kind == "reset" {
				latest.Sync.Enabled = false
			}
			if e = c.saveRepoKeyDataLocked(latest); e != nil {
				return e
			}
			c.Repo = latest.Repo
			c.Sync = latest.Sync
			updated = latest
			return nil
		})
		out.Applied, out.Uncertain, out.CleanupPending = result.Applied, result.Uncertain, result.CleanupPending
		return e
	})
	if err != nil {
		if out.Applied && r.Kind != "reset" {
			out.InitialIndexFailed = true
		}
		return out, errors.Join(err, ctx.Err())
	}
	if r.Kind == "reset" {
		ui(ctx, func() { util.PushUpdateMsg(msgID, lang(145), 3000) })
		c.scheduleRepoReload(p)
		return out, nil
	}
	// Preserve the original two-stage initialization notices, with cancellable
	// timers. Index failure is still a UI error after successful key publication.
	for i := 0; i < 2; i++ {
		if e = waitRepoDelay(ctx, time.Second); e != nil {
			out.InitialIndexFailed = true
			return out, e
		}
		if i == 0 {
			ui(ctx, func() { util.PushMsg(lang(138), 3000) })
		}
	}
	_, e = c.executeLocalRepo(ctx, updated, cloudrepo.LocalRepoRequest{Kind: "index", Memo: "[Init] Init local data repo"}, p, nil)
	if e != nil {
		out.InitialIndexFailed = true
		ui(ctx, func() { util.PushErrMsg(fmt.Sprintf(lang(140), e), 0) })
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
	}
	return out, nil
}

func waitRepoDelay(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func (c *AppConf) scheduleRepoReload(p metadataRepoPaths) {
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c != Conf || p.Workspace != util.WorkspaceDir || p.Data != util.DataDir {
		return
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	c.repoTasks.Go(func(ctx context.Context) {
		if waitRepoDelay(ctx, 2*time.Second) != nil {
			return
		}
		_ = c.WithSettingSnapshot(ctx, func(v conf.AppData) error {
			if v.Repo == nil || len(v.Repo.Key) != 0 || v.Sync == nil || v.Sync.Enabled {
				return nil
			}
			c.repoMu.Lock()
			defer c.repoMu.Unlock()
			if !c.repoClosed && c == Conf && p.Workspace == util.WorkspaceDir && p.Data == util.DataDir && ctx.Err() == nil {
				util.ReloadUI()
			}
			return nil
		})
	})
}

func ChangeRepoKeyContext(ctx context.Context, kind, input string) (cloudrepo.RepoKeyResult, error) {
	owner := Conf
	s, e := owner.CloudRepoService()
	if e != nil {
		return cloudrepo.RepoKeyResult{}, e
	}
	out, e := s.RepoKey(ctx, cloudrepo.RepoKeyRequest{Kind: kind, Input: input})
	for _, v := range []struct {
		cause error
		id    int
	}{{cloudrepo.ErrMemo, 142}, {cloudrepo.ErrRepoKeyInput, 157}} {
		if errors.Is(e, v.cause) {
			e = directoryBusinessError{e, owner.Language(v.id)}
			break
		}
	}
	return out, e
}
