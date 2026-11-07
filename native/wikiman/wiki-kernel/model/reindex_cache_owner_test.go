//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func openOwnedTaskFixture(t *testing.T) {
	t.Helper()
	task.OpenLegacyTasks()
	t.Cleanup(func() {
		if err := task.CloseLegacyTasks(context.Background()); err != nil {
			t.Error(err)
		}
	})
}

func runOwnedTask(t *testing.T, h *task.ContextTask, err error) {
	t.Helper()
	if err != nil || h == nil {
		t.Fatal(h, err)
	}
	task.ExecTaskJob()
	if err = h.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyReindexOwnersCommitActualHistoryAssetsAndVirtualCache(t *testing.T) {
	t.Run("history", func(t *testing.T) {
		_, _, _, _ = historyIndexFixture(t)
		openOwnedTaskFixture(t)
		h, err := startHistoryReindex(context.Background())
		runOwnedTask(t, h, err)
		queue := sql.CaptureLegacyHistoryQueue()
		var count int
		if err = queue.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); err != nil || count != 2 {
			t.Fatal(count, err)
		}
	})

	t.Run("asset-content", func(t *testing.T) {
		_, _, _, _, _ = conflictFixture(t)
		openOwnedTaskFixture(t)
		assetDir := filepath.Join(util.DataDir, "assets")
		if err := os.MkdirAll(assetDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(assetDir, "owned.txt"), []byte("owned asset projection"), 0600); err != nil {
			t.Fatal(err)
		}
		h, err := startAssetContentReindex(context.Background())
		runOwnedTask(t, h, err)
		queue := sql.CaptureLegacyAssetQueue()
		var content string
		if err = queue.DB.QueryRow("SELECT content FROM asset_contents_fts_case_insensitive WHERE path = ?", "assets/owned.txt").Scan(&content); err != nil || content != "owned asset projection" {
			t.Fatal(content, err)
		}
	})

	t.Run("virtual-reference", func(t *testing.T) {
		_, _, _, _, _ = conflictFixture(t)
		openOwnedTaskFixture(t)
		Conf.Search.VirtualRefName = false
		Conf.Search.VirtualRefAlias = false
		Conf.Search.VirtualRefAnchor = false
		Conf.Search.VirtualRefDoc = true
		h, err := startVirtualBlockRefCache(context.Background())
		runOwnedTask(t, h, err)
		value, ok := virtualBlockRefCache.Get("virtual_ref")
		if !ok {
			t.Fatal("virtual-reference cache missing")
		}
		keywords, ok := value.([]string)
		if !ok {
			t.Fatal(value)
		}
		found := false
		for _, keyword := range keywords {
			if keyword == "reference fixture" {
				found = true
			}
		}
		if !found {
			t.Fatal(keywords)
		}
	})
}

func TestLegacyReindexOwnerCancellationReplacementAndJoin(t *testing.T) {
	c, _, _, _, _ := conflictFixture(t)
	openOwnedTaskFixture(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := startHistoryReindex(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	env, err := captureLegacyVirtualCache(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	old := Conf
	Conf = NewAppConf()
	err = env.rebuild(context.Background())
	Conf = old
	if !errors.Is(err, conf.ErrBusinessUnbound) {
		t.Fatal("replacement owner accepted", err)
	}

	o, err := captureLegacyReindexOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	h, err := o.enqueue(context.Background(), "test.reindex.owner.join", func(context.Context, context.Context) error {
		close(entered)
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { task.ExecTaskJob(); close(done) }()
	<-entered
	deadline, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if err = c.CloseCloudRepo(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("owner reported false join", err)
	}
	close(release)
	if err = h.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-done
	if err = c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
}
