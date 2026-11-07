// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/eventbus"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func (c *AppConf) executeLocalRepo(ctx context.Context, v conf.AppData, r cloudrepo.LocalRepoRequest, p metadataRepoPaths, policy cloudrepo.RetentionPolicy) (out cloudrepo.LocalRepoResult, err error) {
	c.m.Lock()
	readonly := c.persistenceReadOnly
	c.m.Unlock()
	if readonly {
		return out, ErrSettingReadOnly
	}
	if v.Repo == nil || v.Sync == nil || v.System == nil {
		return out, cloudrepo.ErrDependency
	}
	if e := validateSnapshotPaths(p); e != nil {
		return out, e
	}
	if r.Kind == "index" {
		// Native owners provide their own source-read lease. Compatibility only may
		// flush the original transaction queue; detached configurations never do so.
		if c == Conf {
			if p.Data != util.DataDir || p.Workspace != util.WorkspaceDir {
				return out, cloudrepo.ErrDependency
			}
			if e := FlushTxQueueContext(ctx); e != nil {
				return out, e
			}
		}
		b, e := os.ReadFile(filepath.Join(p.Data, ".siyuan", "syncignore"))
		if e != nil && !os.IsNotExist(e) {
			return out, cloudrepo.ErrProvider
		}
		p.Ignore = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
		p.Ignore = append(p.Ignore, "20210808180117-6v0mkxr/**/*", "20210808180117-czj9bvb/**/*", "20211226090932-5lcq56f/**/*", "20240530133126-axarxgx/**/*")
	}
	ui := func(cctx context.Context, fn func()) {
		c.repoMu.Lock()
		defer c.repoMu.Unlock()
		if c.repoClosed || c != Conf || p.Workspace != util.WorkspaceDir || p.Data != util.DataDir || cctx.Err() != nil {
			return
		}
		fn()
	}
	lang := func(id int) string { return c.business.Language(v.Lang, id, true, v.CloudRegion) }
	if r.Kind == "index" {
		ui(ctx, func() { util.PushEndlessProgress(lang(143)) })
		defer ui(context.WithoutCancel(ctx), util.PushClearProgress)
	}
	var transports []*http.Transport
	p.Transports = &transports
	defer func() {
		for _, t := range transports {
			t.CloseIdleConnections()
		}
	}()
	repo, e := newMetadataRepository(ctx, v, p)
	if e != nil {
		return out, cloudrepo.ErrProvider
	}
	defer repo.CloseMetadata()
	start := time.Now()
	result, e := repo.LocalRepoContext(ctx, dejavu.LocalRepoRequest{Kind: r.Kind, Memo: r.Memo}, dejavu.SnapshotOptions{Event: func(cctx context.Context, topic string, args ...interface{}) {
		id := 0
		vals := []interface{}{}
		if len(args) > 0 {
			vals = append(vals, args[1:]...)
		}
		switch topic {
		case eventbus.EvtIndexBeforeWalkData, eventbus.EvtIndexWalkData:
			id = 158
			if len(vals) > 0 {
				if path, ok := vals[0].(string); ok {
					vals[0] = filepath.Base(path)
				}
			}
		case eventbus.EvtIndexBeforeGetLatestFiles:
			id = 159
			vals = append([]interface{}{0}, vals...)
		case eventbus.EvtIndexGetLatestFile:
			id = 159
		case eventbus.EvtIndexUpsertFiles:
			id = 160
			vals = append([]interface{}{0}, vals...)
		case eventbus.EvtIndexUpsertFile:
			id = 160
		}
		if id != 0 {
			msg := fmt.Sprintf(lang(id), vals...)
			ui(cctx, func() { util.SetBootDetails(msg); util.PushStatusBar(msg); util.PushEndlessProgress(msg) })
		}
	}}, func(next func(int) ([]*entity.Index, int, error)) ([]string, error) {
		return policy(func(page int) ([]cloudrepo.RepoIndex, int, error) {
			rows, pages, e := next(page)
			ret := []cloudrepo.RepoIndex{}
			for _, row := range rows {
				if row == nil {
					return nil, 0, cloudrepo.ErrRetention
				}
				ret = append(ret, cloudrepo.RepoIndex{ID: row.ID, Created: row.Created})
			}
			return ret, pages, e
		})
	})
	out = cloudrepo.LocalRepoResult{ID: result.ID, Changed: result.Changed, Applied: result.Applied, Uncertain: result.Uncertain, Skipped: result.Skipped, Indexes: result.Indexes, Objects: result.Objects, Bytes: result.Bytes}
	if e != nil {
		return out, errors.Join(cloudrepo.ErrProvider, ctx.Err())
	}
	if r.Kind == "index" {
		id := 148
		if out.Changed {
			id = 147
		}
		msg := fmt.Sprintf(lang(id), time.Since(start).Seconds())
		ui(ctx, func() { util.PushStatusBar(msg); util.PushMsg(msg, 5000) })
	}
	return out, nil
}

func IndexRepoContext(ctx context.Context, memo string) error {
	owner := Conf
	s, e := owner.CloudRepoService()
	if e != nil {
		return e
	}
	_, e = s.LocalRepo(ctx, cloudrepo.LocalRepoRequest{Kind: "index", Memo: memo})
	for _, x := range []struct {
		cause error
		id    int
	}{{cloudrepo.ErrKey, 26}, {cloudrepo.ErrMemo, 142}} {
		if errors.Is(e, x.cause) {
			return directoryBusinessError{e, owner.Language(x.id)}
		}
	}
	return e
}

func AutoPurgeRepoJobContext(ctx context.Context) {
	if Conf != nil {
		Conf.scheduleLocalRepo(ctx, true, false)
	}
}

// Registration, cancellation and actual completion belong to the captured
// configuration. Index repair and purge share the owner repository lease.
func (c *AppConf) scheduleLocalRepo(parent context.Context, cron, check bool) {
	s, e := c.CloudRepoService()
	if e != nil {
		return
	}
	workspace, data := util.WorkspaceDir, util.DataDir
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c.repoPurgePending || parent.Err() != nil {
		return
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	c.repoPurgePending = true
	if !c.repoTasks.Go(func(life context.Context) {
		defer func() { c.repoMu.Lock(); c.repoPurgePending = false; c.repoMu.Unlock() }()
		ctx, cancel := context.WithTimeout(life, 12*time.Hour)
		stop := context.AfterFunc(parent, cancel)
		defer func() { stop(); cancel() }()
		if ctx.Err() != nil || c != Conf || workspace != util.WorkspaceDir || data != util.DataDir {
			return
		}
		if check {
			if e := s.Repair(ctx); e != nil {
				return
			}
		}
		if ctx.Err() != nil || c != Conf || workspace != util.WorkspaceDir || data != util.DataDir {
			return
		}
		_, _ = s.LocalRepo(ctx, cloudrepo.LocalRepoRequest{Kind: "auto-purge", Cron: cron})
	}) {
		c.repoPurgePending = false
	}
}
