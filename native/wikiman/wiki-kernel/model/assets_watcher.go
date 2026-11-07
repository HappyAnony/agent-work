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
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/util"
)

var assetsWatcherMu sync.Mutex
var assetsWatcher *fsnotifyOwner

func WatchAssets() {
	if err := WatchAssetsContext(context.Background()); err != nil {
		logging.LogErrorf("open assets watcher failed: %s", err)
	}
}

func WatchAssetsContext(ctx context.Context) error {
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
	assetsWatcherMu.Lock()
	defer assetsWatcherMu.Unlock()
	if assetsWatcher != nil {
		if err := assetsWatcher.Close(ctx); err != nil {
			return err
		}
		assetsWatcher = nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	assetsDir := filepath.Join(watchOwner.data, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return err
	}
	assetOwner, err := captureAssetWatchContext(ctx)
	if err != nil {
		return err
	}
	owner, err := openFSNotifyOwner(assetsDir, func(ownerCtx context.Context, watcher *fsnotify.Watcher) error {
		return runAssetsWatcher(ownerCtx, watcher, watchOwner, assetOwner)
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
		return errors.New("assets_watcher_owner_closed")
	}
	assetsWatcher = owner
	return nil
}

func runAssetsWatcher(ctx context.Context, watcher *fsnotify.Watcher, owner *workspaceWatcherOwner, assets *assetWatchContext) error {
	var timer *time.Timer
	var timerC <-chan time.Time
	var lastEvent fsnotify.Event
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			lastEvent = event
			if err := owner.guard(ctx); err != nil {
				continue
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
			if event.Op&(fsnotify.Rename|fsnotify.Write|fsnotify.Remove) != 0 {
				if err := assets.dispatch(ctx, assetflow.Event{Path: event.Name, Removed: event.Op&fsnotify.Remove != 0}); err != nil && ctx.Err() == nil {
					logging.LogErrorf("asset callback failed: %s", err)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			logging.LogErrorf("watch assets failed: %s", err)
		case <-timerC:
			timerC = nil
			if err := owner.guard(ctx); err != nil {
				return nil
			}
			if err := assets.dispatch(ctx, assetflow.Event{Path: lastEvent.Name, Removed: lastEvent.Op&fsnotify.Remove != 0, Written: lastEvent.Op&fsnotify.Write != 0}); err != nil && ctx.Err() == nil {
				logging.LogErrorf("asset callback failed: %s", err)
			}
		}
	}
}

func CloseWatchAssets() {
	if err := CloseWatchAssetsContext(context.Background()); err != nil {
		logging.LogErrorf("close assets watcher failed: %s", err)
	}
}

func CloseWatchAssetsContext(ctx context.Context) error {
	assetsWatcherMu.Lock()
	defer assetsWatcherMu.Unlock()
	if assetsWatcher != nil {
		if err := assetsWatcher.Close(ctx); err != nil {
			return err
		}
		assetsWatcher = nil
	}
	return nil
}

func StopWatchAssets() {
	assetsWatcherMu.Lock()
	defer assetsWatcherMu.Unlock()
	if assetsWatcher != nil {
		_ = assetsWatcher.Stop()
	}
}
