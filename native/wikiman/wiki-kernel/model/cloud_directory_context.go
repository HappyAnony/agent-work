// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/cloudlock"
	"net/http"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/util"
)

// Directory and lock IO are private device/cloud effects, not wiki source writes.
func (c *AppConf) executeLegacyDirectory(ctx context.Context, v conf.AppData, r cloudrepo.DirectoryRequest, p metadataRepoPaths, opts cloudlock.Options) (out cloudrepo.DirectoryResult, err error) {
	if v.Sync == nil {
		return out, cloudrepo.ErrDependency
	}
	out.Active = v.Sync.CloudName
	c.m.Lock()
	readonly := c.persistenceReadOnly
	c.m.Unlock()
	if readonly {
		return out, ErrSettingReadOnly
	}
	if r.Kind == "set" {
		if err = c.commitRepoName(ctx, v, r.Name); err == nil {
			out.Active = r.Name
			out.Saved = true
		}
		return
	}
	if err = validateLocalDirectoryTarget(v, p); err != nil {
		return
	}
	if v.Repo == nil || v.System == nil {
		return out, cloudrepo.ErrDependency
	}
	if !cloud.IsValidCloudDirName(v.Sync.CloudName) {
		if err = c.commitRepoName(ctx, v, "main"); err != nil {
			return
		}
		v.Sync.CloudName = "main"
		out.Active = "main"
	}
	workspace, data := util.WorkspaceDir, util.DataDir
	ui := func(cctx context.Context, fn func()) {
		c.repoMu.Lock()
		defer c.repoMu.Unlock()
		if c.repoClosed || c != Conf || workspace != util.WorkspaceDir || data != util.DataDir || cctx.Err() != nil {
			return
		}
		fn()
	}
	lang := func(id int) string { return c.business.Language(v.Lang, id, true, v.CloudRegion) }
	if r.Kind == "remove" {
		msg := ""
		ui(ctx, func() { msg = util.PushMsg(lang(116), 15000) })
		defer func() {
			if msg != "" {
				ui(context.WithoutCancel(ctx), func() { util.PushClearMsg(msg) })
			}
		}()
	}
	err = dejavu.WithRepositoryContext(ctx, func() error {
		var transports []*http.Transport
		p.Transports = &transports
		repo, e := newMetadataRepository(ctx, v, p)
		if e != nil {
			return e
		}
		defer repo.CloseMetadata()
		defer func() {
			for _, t := range transports {
				t.CloseIdleConnections()
			}
		}()
		source := repo.ScopedCloud()
		token := make([]byte, 16)
		if _, e = rand.Read(token); e != nil {
			return e
		}
		opts.Owner = v.System.ID + "-" + hex.EncodeToString(token)
		opts.Event = func(cctx context.Context, acquire bool) {
			id := 187
			if acquire {
				id = 186
			}
			ui(cctx, func() { util.SetBootDetails(lang(id)); util.PushStatusBar(lang(id)) })
		}
		opts.Read = func(cctx context.Context) ([]byte, error) {
			client, done, e := cloud.OpenContext(source, cctx)
			if e != nil {
				return nil, e
			}
			defer done()
			b, e := client.DownloadObject("lock-sync")
			if errors.Is(e, cloud.ErrCloudObjectNotFound) {
				return nil, nil
			}
			return b, e
		}
		opts.Write = func(cctx context.Context, b []byte) error {
			client, done, e := cloud.OpenContext(source, cctx)
			if e != nil {
				return e
			}
			defer done()
			_, e = client.UploadBytes("lock-sync", b, true)
			return e
		}
		opts.Remove = func(cctx context.Context) error {
			client, done, e := cloud.OpenContext(source, cctx)
			if e != nil {
				return e
			}
			defer done()
			e = client.RemoveObject("lock-sync")
			if errors.Is(e, cloud.ErrCloudObjectNotFound) {
				return nil
			}
			return e
		}
		return cloudlock.WithLock(ctx, opts, func(cctx context.Context) error {
			client, done, e := cloud.OpenContext(source, cctx)
			if e != nil {
				return e
			}
			defer done()
			if e = cctx.Err(); e != nil {
				return e
			}
			switch r.Kind {
			case "create":
				e = client.CreateRepo(r.Name)
			case "remove":
				e = client.RemoveRepo(r.Name)
			default:
				return cloudrepo.ErrOperation
			}
			out.Applied = e == nil
			out.Uncertain = e != nil
			return e
		})
	})
	if err != nil {
		cause := cloudrepo.ErrProvider
		for _, known := range []error{cloudlock.ErrBusy, cloudlock.ErrLost, cloudlock.ErrUnlock, cloudlock.ErrAcquire} {
			if errors.Is(err, known) {
				cause = known
				break
			}
		}
		err = errors.Join(cause, ctx.Err())
		return
	}
	if r.Kind == "remove" && v.Sync.CloudName == r.Name {
		if err = c.commitRepoName(ctx, v, "main"); err != nil {
			return
		}
		out.Active = "main"
		out.Saved = true
		ui(ctx, func() { util.PushMsg(lang(155), 5000) })
	}
	return
}
func ChangeCloudDirectoryContext(ctx context.Context, kind, name string) (cloudrepo.DirectoryResult, error) {
	owner := Conf
	s, e := owner.CloudRepoService()
	if e != nil {
		return cloudrepo.DirectoryResult{}, e
	}
	out, e := s.Directory(ctx, cloudrepo.DirectoryRequest{Kind: kind, Name: name})
	if errors.Is(e, cloudrepo.ErrName) {
		e = directoryBusinessError{e, owner.Language(37)}
	}
	if errors.Is(e, cloudrepo.ErrProviderType) {
		e = directoryBusinessError{e, owner.Language(131)}
	}
	if errors.Is(e, cloudrepo.ErrProvider) {
		e = directoryBusinessError{e, owner.Language(30)}
	}
	return out, e
}

type directoryBusinessError struct {
	cause error
	msg   string
}

func (e directoryBusinessError) Error() string { return e.msg }
func (e directoryBusinessError) Unwrap() error { return e.cause }
