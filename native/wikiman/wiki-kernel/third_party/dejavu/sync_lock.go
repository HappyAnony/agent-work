// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"errors"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/cloudlock"
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"strings"
	"time"
)

var ErrLockCloudFailed = cloudlock.ErrAcquire
var ErrCloudLocked = cloudlock.ErrBusy

const lockSyncKey = "lock-sync"

// Legacy callers retain their wire identity and UI events. Renewal ownership is
// per Repo, never a process-global stop channel. Their remaining sync/source
// operations still need separate Context migration.
func (repo *Repo) tryLockCloud(device string, ui map[string]interface{}) error {
	if repo.lockLease != nil {
		return ErrLockCloudFailed
	}
	source := repo.cloud
	opts := cloudlock.Options{Owner: device, Event: func(ctx context.Context, acquire bool) {
		if ctx.Err() != nil {
			return
		}
		if acquire {
			repo.emit(eventbus.EvtCloudLock, ui)
		} else {
			repo.emit(eventbus.EvtCloudUnlock, ui)
		}
	}}
	opts.Read = func(ctx context.Context) ([]byte, error) {
		c, done, e := cloud.OpenContext(source, ctx)
		if e != nil {
			return nil, e
		}
		defer done()
		data, e := c.DownloadObject(lockSyncKey)
		if errors.Is(e, cloud.ErrCloudObjectNotFound) {
			return nil, nil
		}
		return data, e
	}
	opts.Write = func(ctx context.Context, data []byte) error {
		c, done, e := cloud.OpenContext(source, ctx)
		if e != nil {
			return e
		}
		defer done()
		_, e = c.UploadBytes(lockSyncKey, data, true)
		return e
	}
	opts.Remove = func(ctx context.Context) error {
		c, done, e := cloud.OpenContext(source, ctx)
		if e != nil {
			return e
		}
		defer done()
		e = c.RemoveObject(lockSyncKey)
		if errors.Is(e, cloud.ErrCloudObjectNotFound) {
			return nil
		}
		return e
	}
	l, e := cloudlock.Acquire(repo.cloud.GetConf().Context(), opts)
	if e == nil {
		client, done, openErr := cloud.OpenContext(source, l.Context())
		if openErr != nil {
			l.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return errors.Join(openErr, l.Close(ctx))
		}
		repo.lockOriginal = source
		repo.lockClientClose = done
		repo.cloud = client
		repo.lockLease = l
	}
	return e
}
func (repo *Repo) unlockCloud(_ map[string]interface{}) {
	l := repo.lockLease
	if l == nil {
		return
	}
	l.Stop()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(repo.cloud.GetConf().Context()), 5*time.Second)
	defer cancel()
	if e := l.Close(ctx); e != nil {
		logging.LogErrorf("unlock cloud repo failed")
	}
	if repo.lockClientClose != nil {
		repo.lockClientClose()
		repo.lockClientClose = nil
	}
	if repo.lockOriginal != nil {
		repo.cloud = repo.lockOriginal
		repo.lockOriginal = nil
	}
	repo.lockLease = nil
}
func parseErr(err error) (bool, error) {
	if nil == err {
		return true, nil
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "requesttimetooskewed") || strings.Contains(msg, "request time and the current time is too large") {
		return true, cloud.ErrSystemTimeIncorrect
	} else if strings.Contains(msg, "500") || strings.Contains(msg, "internal server error") || strings.Contains(msg, "503") || strings.Contains(msg, "unavailable") {
		return true, cloud.ErrCloudServiceUnavailable
	} else if strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "403") || strings.Contains(msg, "forbidden") {
		return true, cloud.ErrCloudForbidden
	} else if strings.Contains(msg, "429") || strings.Contains(msg, "too many requests") {
		return true, cloud.ErrCloudTooManyRequests
	}
	return false, err
}
