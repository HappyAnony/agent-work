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

//go:build !darwin

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/siyuan-note/logging"
	"wiki-kernel/util"
)

var emojisWatcherMu sync.Mutex
var emojisWatcher *fsnotifyOwner

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
	if util.ContainerAndroid == util.Container || util.ContainerIOS == util.Container || util.ContainerHarmony == util.Container {
		return nil
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
	emojisDir := filepath.Join(watchOwner.data, "emojis")
	if err := os.MkdirAll(emojisDir, 0755); err != nil {
		return err
	}
	owner, err := openFSNotifyOwner(emojisDir, func(ownerCtx context.Context, watcher *fsnotify.Watcher) error {
		return runEmojisWatcher(ownerCtx, watcher, watchOwner)
	})
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

func runEmojisWatcher(ctx context.Context, watcher *fsnotify.Watcher, owner *workspaceWatcherOwner) error {
	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if timer == nil {
				timer = time.NewTimer(100 * time.Millisecond)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(100 * time.Millisecond)
				timerC = timer.C
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			logging.LogErrorf("watch emojis failed: %s", err)
		case <-timerC:
			timerC = nil
			if err := owner.guard(ctx); err != nil {
				return nil
			}
			owner.publish(ctx, util.PushReloadEmojiConf)
		}
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

func StopWatchEmojis() {
	emojisWatcherMu.Lock()
	defer emojisWatcherMu.Unlock()
	if emojisWatcher != nil {
		_ = emojisWatcher.Stop()
	}
}
