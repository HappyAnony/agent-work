// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build darwin

package model

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/radovskyb/watcher"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/pollwatch"
	"wiki-kernel/util"
)

var emojisWatcherMu sync.Mutex
var emojisWatcher *pollwatch.Watch
var openEmojisWatcher = pollwatch.Open

func WatchEmojis() {
	if err := WatchEmojisContext(context.Background()); err != nil {
		logging.LogErrorf("open emojis watcher failed: %s", err)
	}
}

func WatchEmojisContext(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	watchOwner, err := captureWorkspaceWatcherOwner(ctx)
	if err != nil {
		return err
	}
	emojisWatcherMu.Lock()
	defer emojisWatcherMu.Unlock()
	if emojisWatcher != nil {
		if err := emojisWatcher.Close(ctx); err != nil {
			return err
		}
		emojisWatcher = nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	folder := filepath.Join(watchOwner.data, "emojis")
	owner, err := openEmojisWatcher(ctx, folder, 10*time.Second, func(ownerCtx context.Context, _ watcher.Event) {
		watchOwner.publish(ownerCtx, util.PushReloadEmojiConf)
	}, func(err error) { logging.LogErrorf("watch emojis failed: %s", err) })
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		_ = owner.Close(context.Background())
		return err
	}
	if !watchOwner.ownWorkspaceWatcher(owner) {
		_ = owner.Close(context.Background())
		return errors.New("emojis_watcher_owner_closed")
	}
	emojisWatcher = owner
	return nil
}

func StopWatchEmojis() {
	emojisWatcherMu.Lock()
	defer emojisWatcherMu.Unlock()
	if emojisWatcher != nil {
		emojisWatcher.Stop()
	}
}

func CloseWatchEmojis() {
	if err := CloseWatchEmojisContext(context.Background()); err != nil {
		logging.LogErrorf("close emojis watcher failed: %s", err)
	}
}

func CloseWatchEmojisContext(ctx context.Context) error {
	emojisWatcherMu.Lock()
	defer emojisWatcherMu.Unlock()
	if emojisWatcher != nil {
		if err := emojisWatcher.Close(ctx); err != nil {
			return err
		}
		emojisWatcher = nil
	}
	return nil
}
