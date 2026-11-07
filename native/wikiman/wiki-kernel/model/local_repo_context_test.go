package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func TestLocalRepoOriginalWrapperIndexAndNoSourceMarkerWrite(t *testing.T) {
	c, file, p, _ := snapshotModelFixture(t)
	workspace, data := util.WorkspaceDir, util.DataDir
	util.WorkspaceDir = p.Workspace
	util.DataDir = p.Data
	defer func() { util.WorkspaceDir = workspace; util.DataDir = data }()
	before, _ := os.ReadFile(file)
	// Remove seeded orphan metadata by choosing a fresh private repository.
	p.Repo = filepath.Join(t.TempDir(), "repo")
	s, e := c.newSnapshotRepoService(p)
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner.Close(context.Background())
	c.repoOwner = s
	if e = IndexRepo(" synthetic "); e != nil {
		t.Fatal(e)
	}
	first, e := s.LocalRepo(context.Background(), cloudrepo.LocalRepoRequest{Kind: "index", Memo: "same"})
	if e != nil || !first.Applied || first.Changed {
		t.Fatal(first, e)
	}
	if _, e = os.Stat(filepath.Join(p.Data, ".siyuan", "syncignore")); !os.IsNotExist(e) {
		t.Fatal("index created source marker", e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("index changed configuration")
	}
	if e = IndexRepo("  "); !errors.Is(e, cloudrepo.ErrMemo) {
		t.Fatal(e)
	}
	c.Repo.Key = nil
	if e = IndexRepo("x"); !errors.Is(e, cloudrepo.ErrKey) {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "local-repo-model.json", map[string]any{"originalIndexWrapper": true, "unchangedIndexReused": true, "sourceMarkerNotCreated": true, "configurationUnchanged": true, "memoAndKeyRejected": true})
}

func TestLocalRepoOwnedSchedulerDeduplicatesAndCloseJoins(t *testing.T) {
	c, _, _ := repoMetadataFixture(t)
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s, e := cloudrepo.New(cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) { return conf.AppData{Repo: c.Repo}, nil }, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
		return cloudrepo.Result{}, nil
	}, LocalRepo: func(ctx context.Context, _ conf.AppData, _ cloudrepo.LocalRepoRequest, _ cloudrepo.RetentionPolicy) (cloudrepo.LocalRepoResult, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return cloudrepo.LocalRepoResult{}, ctx.Err()
	}})
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	c.repoTasks = taskgroup.Open()
	c.scheduleLocalRepo(context.Background(), false, false)
	<-entered
	c.scheduleLocalRepo(context.Background(), false, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e = c.CloseCloudRepo(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	<-canceled
	close(release)
	if e = c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	c.repoMu.Lock()
	pending := c.repoPurgePending
	c.repoMu.Unlock()
	if pending {
		t.Fatal("task not joined")
	}
	recordCloudBusinessEvidence(t, "local-repo-task.json", map[string]any{"schedulerDeduplicated": true, "ownerCloseCanceled": true, "actualCleanupJoined": true})
}
