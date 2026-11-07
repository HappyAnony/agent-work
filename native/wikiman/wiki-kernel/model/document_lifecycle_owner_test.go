//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/util"
)

func TestLegacyDocumentLifecycleOwnerAdmissionReplacementAndJoin(t *testing.T) {
	t.Run("canceled-and-replaced", func(t *testing.T) {
		_, _, file, before, _ := conflictFixture(t)
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := captureLegacyDocumentLifecycle(canceled); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		o, err := captureLegacyDocumentLifecycle(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		oldConf, oldData := Conf, util.DataDir
		Conf, util.DataDir = NewAppConf(), t.TempDir()
		_, err = o.execute(context.Background(), doclife.Request{Action: "rename", Box: filepath.Base(filepath.Dir(file)), Path: "/" + filepath.Base(file), Title: "must not write"})
		Conf, util.DataDir = oldConf, oldData
		if !errors.Is(err, conf.ErrBusinessUnbound) {
			t.Fatal("replacement owner accepted", err)
		}
		after, readErr := os.ReadFile(file)
		if readErr != nil || !bytes.Equal(after, before) {
			t.Fatal("stale owner changed source", readErr)
		}
	})

	t.Run("close-joins-entered-operation", func(t *testing.T) {
		c, _, _, _, _ := conflictFixture(t)
		o, err := captureLegacyDocumentLifecycle(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		entered, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, runErr := o.run(context.Background(), func(context.Context, context.Context) (doclife.Plan, error) {
				close(entered)
				<-release
				return doclife.Plan{}, nil
			})
			done <- runErr
		}()
		<-entered
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if err = c.CloseCloudRepo(deadline); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("owner reported false join", err)
		}
		close(release)
		if err = <-done; err != nil {
			t.Fatal(err)
		}
		if err = c.CloseCloudRepo(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func assertLifecycleProjection(t *testing.T, r *indexRepairContext, id, wantContent, wantPath string) {
	t.Helper()
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var content, path string
		if err := r.main.DB.QueryRowContext(context.Background(), "SELECT content, path FROM "+table+" WHERE id = ?", id).Scan(&content, &path); err != nil || content != wantContent || path != wantPath {
			t.Fatalf("%s projection mismatch: content=%q path=%q err=%v", table, content, path, err)
		}
	}
}

func assertLifecycleProjectionRemoved(t *testing.T, r *indexRepairContext, rootID string) {
	t.Helper()
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var count int
		if err := r.main.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE root_id = ?", rootID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s projection remains: count=%d err=%v", table, count, err)
		}
	}
}

func TestLegacyDocumentLifecycleRenameAndRestoreCommitCapturedIndexes(t *testing.T) {
	t.Run("create-move-delete", func(t *testing.T) {
		_, r, file, _, _ := conflictFixture(t)
		isolatedConflictQueue(t, flushTx)
		ctx := context.Background()
		box := filepath.Base(filepath.Dir(file))
		id := "20261103200000-life001"
		createdPath := "/" + id + ".docj"
		if _, err := CreateDocByMdContext(ctx, box, createdPath, "Owned lifecycle create", "created body", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(file), id+".docj")); err != nil {
			t.Fatal("created source missing", err)
		}
		var content, indexedPath string
		if err := r.main.DB.QueryRowContext(ctx, "SELECT content, path FROM blocks WHERE id = ?", id).Scan(&content, &indexedPath); err != nil || content != "Owned lifecycle create" || indexedPath != createdPath {
			t.Fatal("created projection missing", content, indexedPath, err)
		}
		assertLifecycleProjection(t, r, id, "Owned lifecycle create", createdPath)
		parent := "/20261028140001-refdoc1.docj"
		if err := MoveDocsContext(ctx, []string{createdPath}, box, parent, nil); err != nil {
			t.Fatal(err)
		}
		movedPath := "/20261028140001-refdoc1/" + id + ".docj"
		movedFile := filepath.Join(filepath.Dir(file), "20261028140001-refdoc1", id+".docj")
		if _, err := os.Stat(movedFile); err != nil {
			t.Fatal("moved source missing", err)
		}
		if err := r.main.DB.QueryRowContext(ctx, "SELECT path FROM blocks WHERE id = ?", id).Scan(&indexedPath); err != nil || indexedPath != movedPath {
			t.Fatal("moved projection missing", indexedPath, err)
		}
		assertLifecycleProjection(t, r, id, "Owned lifecycle create", movedPath)
		if err := RemoveDocContext(ctx, box, movedPath); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(movedFile); !os.IsNotExist(err) {
			t.Fatal("deleted source remains", err)
		}
		assertLifecycleProjectionRemoved(t, r, id)
	})

	t.Run("rename", func(t *testing.T) {
		_, r, file, _, _ := conflictFixture(t)
		box, root := filepath.Base(filepath.Dir(file)), filepath.Base(file[:len(file)-len(filepath.Ext(file))])
		if err := RenameDocContext(context.Background(), box, "/"+filepath.Base(file), "Owned lifecycle rename"); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(file)
		if err != nil || !bytes.Contains(raw, []byte("Owned lifecycle rename")) {
			t.Fatal("renamed source missing", err)
		}
		var content string
		if err = r.main.DB.QueryRowContext(context.Background(), "SELECT content FROM blocks WHERE id = ?", root).Scan(&content); err != nil || content != "Owned lifecycle rename" {
			t.Fatal("renamed derived index missing", content, err)
		}
		assertLifecycleProjection(t, r, root, "Owned lifecycle rename", "/"+filepath.Base(file))
	})

	t.Run("restore", func(t *testing.T) {
		c, r, file, before, _ := conflictFixture(t)
		box, root := filepath.Base(filepath.Dir(file)), filepath.Base(file[:len(file)-len(filepath.Ext(file))])
		var doc map[string]interface{}
		if err := json.Unmarshal(before, &doc); err != nil {
			t.Fatal(err)
		}
		doc["Properties"].(map[string]interface{})["title"] = "Owned lifecycle restore"
		historyRaw, _ := json.Marshal(doc)
		historyFile := filepath.Join(util.HistoryDir, "2026-11-03-190000-restore", box, filepath.Base(file))
		if err := os.MkdirAll(filepath.Dir(historyFile), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(historyFile, historyRaw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := RollbackDocHistoryContext(context.Background(), box, historyFile); err != nil {
			t.Fatal(err)
		}
		if err := c.CloseCloudRepo(context.Background()); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(file)
		if err != nil || !bytes.Contains(raw, []byte("Owned lifecycle restore")) {
			t.Fatal("restored source missing", err)
		}
		var content string
		if err = r.main.DB.QueryRowContext(context.Background(), "SELECT content FROM blocks WHERE id = ?", root).Scan(&content); err != nil || content != "Owned lifecycle restore" {
			t.Fatal("restored derived index missing", content, err)
		}
		assertLifecycleProjection(t, r, root, "Owned lifecycle restore", "/"+filepath.Base(file))
		recordCloudBusinessEvidence(t, "document-lifecycle-owner.json", map[string]interface{}{
			"canceledCaptureRejected": true, "replacementOwnerRejected": true, "ownerCloseJoinsEnteredWork": true,
			"renameSourceAndIndexCommitted": true, "restoreSourceAndIndexCommitted": true, "restoreFollowupOwned": true,
		})
	})
}
