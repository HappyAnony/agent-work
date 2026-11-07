package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func restoreFixture(t *testing.T) (*AppConf, string, metadataRepoPaths, string) {
	t.Helper()
	c, file, p := repoKeyFixture(t)
	repo, history, temp := util.RepoDir, util.HistoryDir, util.TempDir
	util.RepoDir = p.Repo
	util.HistoryDir = p.History
	util.TempDir = p.Temp
	t.Cleanup(func() { util.RepoDir = repo; util.HistoryDir = history; util.TempDir = temp })
	c.Sync.Enabled = true
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	return c, file, p, strings.Repeat("f", 40)
}
func bindRestore(t *testing.T, c *AppConf, fn func(context.Context, conf.AppData, string) (cloudrepo.RestoreResult, error)) {
	t.Helper()
	if c.repoOwner != nil {
		c.repoOwner.Close(context.Background())
	}
	s, e := cloudrepo.New(cloudrepo.Options{Load: func(ctx context.Context) (out conf.AppData, e error) {
		e = c.WithSettingSnapshot(ctx, func(v conf.AppData) error { out = v; return nil })
		return
	}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
		return cloudrepo.Result{}, cloudrepo.ErrDependency
	}, Restore: fn})
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
}
func TestRestoreActualBackupWriteReindexAndDeferredCancellation(t *testing.T) {
	c, file, p, id := restoreFixture(t)
	extra := filepath.Join(p.Data, "extra.txt")
	os.WriteFile(extra, []byte("after snapshot"), 0600)
	suspended, resumed, reindexed := 0, 0, 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ports := restorePorts{Suspend: func(context.Context) (func(context.Context) error, error) {
		suspended++
		return func(ctx context.Context) error {
			if ctx.Err() != nil {
				t.Error("cleanup context canceled")
			}
			resumed++
			return nil
		}, nil
	}, Reindex: func(critical context.Context) error {
		cancel()
		if critical.Err() != nil {
			t.Error("critical rebuild interrupted")
		}
		reindexed++
		return nil
	}}
	bindRestore(t, c, func(ctx context.Context, v conf.AppData, id string) (cloudrepo.RestoreResult, error) {
		return c.executeLegacyRestore(ctx, v, id, p, ports)
	})
	out, e := c.repoOwner.Restore(ctx, id)
	if !errors.Is(e, context.Canceled) || !out.Applied || !out.SourceTouched || !out.Reindexed || !out.CancellationDeferred || !out.SyncPaused || out.BackupID == "" || out.Uncertain {
		t.Fatal(out, e)
	}
	b, e := os.ReadFile(filepath.Join(p.Data, "source.docj"))
	if e != nil || string(b) != "synthetic model snapshot chunk" {
		t.Fatal("source not fully restored", e)
	}
	if _, e = os.Stat(extra); !os.IsNotExist(e) {
		t.Fatal("post-snapshot file survived", e)
	}
	if _, e = os.Stat(filepath.Join(p.Repo, "indexes", out.BackupID)); e != nil {
		t.Fatal("backup missing", e)
	}
	if suspended != 1 || resumed != 1 || reindexed != 1 || c.Sync.Enabled {
		t.Fatal("phase order or sync policy")
	}
	raw, _ := os.ReadFile(file)
	if !bytes.Contains(raw, []byte(`"enabled": false`)) {
		t.Fatal("sync pause not persisted")
	}
	recordCloudBusinessEvidence(t, "restore-model-critical.json", map[string]any{"actualEncryptedBackupAndRestore": true, "postSnapshotFileDeleted": true, "syncPausePersisted": true, "rebuildJoinedBeforeReturn": true, "cancelAfterCommitKeptActualResult": true, "watcherCleanupCalled": true})
}
func TestRestoreGuardsDoNotTouchSourceOrStartEffects(t *testing.T) {
	c, _, p, id := restoreFixture(t)
	before := checkoutTreeHash(t, p.Data)
	called := 0
	ports := restorePorts{Suspend: func(context.Context) (func(context.Context) error, error) {
		called++
		return func(context.Context) error { return nil }, nil
	}, Reindex: func(context.Context) error { called++; return nil }}
	var v conf.AppData
	c.WithSettingSnapshot(context.Background(), func(x conf.AppData) error { v = x; return nil })
	c.persistenceReadOnly = true
	if _, e := c.executeLegacyRestore(context.Background(), v, id, p, ports); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	if _, e := c.executeLegacyRestore(context.Background(), v, strings.Repeat("0", 40), p, ports); e == nil {
		t.Fatal("missing target accepted")
	}
	if _, e := c.executeLegacyRestore(context.Background(), v, id, p, restorePorts{}); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
	if called != 0 || !bytes.Equal(before, checkoutTreeHash(t, p.Data)) || !c.Sync.Enabled {
		t.Fatal("failed preflight had effects")
	}
	recordCloudBusinessEvidence(t, "restore-model-guards.json", map[string]any{"readOnlyAndMissingPortsRefused": true, "badTargetBeforeSuspendAndSyncPause": true, "sourceUnchanged": true})
}
func TestRestoreQueuedTaskDeduplicatesAndDoesNotFollowNewOwner(t *testing.T) {
	c, _, _, id := restoreFixture(t)
	task.OpenLegacyTasks()
	defer task.CloseLegacyTasks(context.Background())
	calls := 0
	bindRestore(t, c, func(context.Context, conf.AppData, string) (cloudrepo.RestoreResult, error) {
		calls++
		return cloudrepo.RestoreResult{}, nil
	})
	req, cancel := context.WithCancel(context.Background())
	if e := EnqueueRepoCheckoutContext(req, id); e != nil {
		t.Fatal(e)
	}
	cancel()
	if e := EnqueueRepoCheckoutContext(context.Background(), id); e != nil {
		t.Fatal(e)
	}
	c.repoMu.Lock()
	n := len(c.repoCheckouts)
	c.repoMu.Unlock()
	if n != 1 {
		t.Fatal("duplicate admitted", n)
	}
	task.ExecTaskJob()
	if calls != 1 {
		t.Fatal("accepted task was canceled with HTTP request")
	}
	if e := EnqueueRepoCheckoutContext(context.Background(), strings.Repeat("b", 40)); e != nil {
		t.Fatal(e)
	}
	previous := Conf
	Conf = NewAppConf()
	task.ExecTaskJob()
	Conf = previous
	if e := c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("queued restore followed new owner")
	}
	recordCloudBusinessEvidence(t, "restore-queue-identity.json", map[string]any{"sameTargetDeduplicated": true, "acceptedJobOutlivesHTTPContext": true, "foreignOwnerRejectedBeforeCallback": true})
}

func TestRestoreStopPendingBeforeWorkspaceTeardown(t *testing.T) {
	c, _, _, id := restoreFixture(t)
	task.OpenLegacyTasks()
	defer task.CloseLegacyTasks(context.Background())
	calls := 0
	bindRestore(t, c, func(context.Context, conf.AppData, string) (cloudrepo.RestoreResult, error) {
		calls++
		return cloudrepo.RestoreResult{}, nil
	})
	if e := EnqueueRepoCheckoutContext(context.Background(), id); e != nil {
		t.Fatal(e)
	}
	if e := c.StopRepoCheckouts(context.Background()); e != nil {
		t.Fatal(e)
	}
	task.ExecTaskJob()
	if calls != 0 {
		t.Fatal("pending task entered after teardown cancellation")
	}
	if e := EnqueueRepoCheckoutContext(context.Background(), strings.Repeat("a", 40)); !errors.Is(e, cloudrepo.ErrClosed) {
		t.Fatal(e)
	}
	if e := c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "restore-switch-drain.json", map[string]any{"queuedRestoreCanceledBeforeTeardown": true, "outgoingOwnerRejectsNewRestores": true, "callbackNotRun": true})
}
func TestRestoreOwnerCloseWaitsActualCriticalRebuild(t *testing.T) {
	c, _, p, id := restoreFixture(t)
	task.OpenLegacyTasks()
	defer task.CloseLegacyTasks(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	results := make(chan cloudrepo.RestoreResult, 1)
	ports := restorePorts{Suspend: func(context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}, Reindex: func(ctx context.Context) error { close(entered); <-release; return ctx.Err() }}
	bindRestore(t, c, func(ctx context.Context, v conf.AppData, id string) (cloudrepo.RestoreResult, error) {
		out, e := c.executeLegacyRestore(ctx, v, id, p, ports)
		results <- out
		return out, e
	})
	if e := EnqueueRepoCheckoutContext(context.Background(), id); e != nil {
		t.Fatal(e)
	}
	ran := make(chan struct{})
	go func() { task.ExecTaskJob(); close(ran) }()
	<-entered
	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := c.CloseCloudRepo(timeout); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	close(release)
	<-ran
	if e := c.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	out := <-results
	if !out.Applied || !out.Reindexed || !out.CancellationDeferred {
		t.Fatal(out)
	}
	c.repoMu.Lock()
	pending := len(c.repoCheckouts)
	c.repoMu.Unlock()
	if pending != 0 {
		t.Fatal("pending task leak")
	}
	recordCloudBusinessEvidence(t, "restore-owner-close.json", map[string]any{"ownerCloseCanceledAdmission": true, "criticalSourceAndRebuildActuallyJoined": true, "appliedAndReindexedResultRetained": true, "pendingTasks": pending})
}

func TestRestoreWatcherSuspendResumeRollsBackPartialOwners(t *testing.T) {
	failure := errors.New("watcher failure")
	for _, scenario := range []string{"close-assets", "close-emojis", "open-assets", "open-emojis", "success"} {
		t.Run(scenario, func(t *testing.T) {
			var calls []string
			call := func(name string) func(context.Context) error {
				return func(ctx context.Context) error {
					calls = append(calls, name)
					if scenario == name {
						return failure
					}
					return ctx.Err()
				}
			}
			resume, err := suspendRestoreWatchers(context.Background(), restoreWatcherPorts{
				closeAssets: call("close-assets"), closeEmojis: call("close-emojis"),
				openAssets: call("open-assets"), openEmojis: call("open-emojis"),
			})
			switch scenario {
			case "close-assets":
				if !errors.Is(err, failure) || resume != nil || !reflect.DeepEqual(calls, []string{"close-assets"}) {
					t.Fatal(err, calls)
				}
			case "close-emojis":
				if !errors.Is(err, failure) || resume != nil || !reflect.DeepEqual(calls, []string{"close-assets", "close-emojis", "open-assets"}) {
					t.Fatal(err, calls)
				}
			default:
				if err != nil || resume == nil {
					t.Fatal(err)
				}
				err = resume(context.Background())
				if scenario == "open-assets" && (!errors.Is(err, failure) || !reflect.DeepEqual(calls, []string{"close-assets", "close-emojis", "open-assets"})) {
					t.Fatal(err, calls)
				}
				if scenario == "open-emojis" && (!errors.Is(err, failure) || !reflect.DeepEqual(calls, []string{"close-assets", "close-emojis", "open-assets", "open-emojis", "close-assets"})) {
					t.Fatal(err, calls)
				}
				if scenario == "success" && (err != nil || !reflect.DeepEqual(calls, []string{"close-assets", "close-emojis", "open-assets", "open-emojis"})) {
					t.Fatal(err, calls)
				}
			}
		})
	}
	if resume, err := suspendRestoreWatchers(context.Background(), restoreWatcherPorts{}); resume != nil || !errors.Is(err, cloudrepo.ErrDependency) {
		t.Fatal(resume != nil, err)
	}
}

func TestRestoreRebuildFailureGatesRequestsAndRetryClearsState(t *testing.T) {
	c, _, p, id := restoreFixture(t)
	failure := errors.New("synthetic rebuild failure")
	ports := restorePorts{Suspend: func(context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}, Reindex: func(context.Context) error { return failure }}
	bindRestore(t, c, func(ctx context.Context, v conf.AppData, id string) (cloudrepo.RestoreResult, error) {
		return c.executeLegacyRestore(ctx, v, id, p, ports)
	})
	out, e := c.repoOwner.Restore(context.Background(), id)
	if e == nil || !out.Applied || out.Reindexed || !out.Uncertain || !c.RestoreDegraded() {
		t.Fatal(out, e)
	}
	r := gin.New()
	r.Use(ControlConcurrency)
	r.POST("/api/block/updateBlock", func(ctx *gin.Context) { ctx.Status(204) })
	r.GET("/api/system/getWorkspaceStatus", func(ctx *gin.Context) { ctx.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/block/updateBlock", nil))
	if w.Code != 503 {
		t.Fatal("degraded workspace admitted edit", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/system/getWorkspaceStatus", nil))
	if w.Code != 204 {
		t.Fatal("diagnostic unavailable", w.Code)
	}
	ports.Reindex = func(context.Context) error { return nil }
	out, e = c.repoOwner.Restore(context.Background(), id)
	if e != nil || !out.Reindexed || c.RestoreDegraded() {
		t.Fatal("successful retry did not clear degraded state", e)
	}
	ports.Reindex = func(context.Context) error { panic("synthetic rebuild panic") }
	out, e = c.repoOwner.Restore(context.Background(), id)
	if !errors.Is(e, task.ErrHandler) || !out.Uncertain || !c.RestoreDegraded() {
		t.Fatal("unexpected rebuild failure escaped recovery gate", e)
	}
	ports.Reindex = func(context.Context) error { return nil }
	if _, e = c.repoOwner.Restore(context.Background(), id); e != nil || c.RestoreDegraded() {
		t.Fatal("retry after panic failed", e)
	}
	resumeFailure := errors.New("synthetic watcher resume failure")
	ports.Suspend = func(context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return resumeFailure }, nil
	}
	out, e = c.repoOwner.Restore(context.Background(), id)
	if !errors.Is(e, resumeFailure) || !out.Applied || !out.Reindexed || !out.Uncertain || !c.RestoreDegraded() {
		t.Fatal("watcher cleanup failure reported as complete", e)
	}
	recordCloudBusinessEvidence(t, "restore-rebuild-failure.json", map[string]any{"appliedSourceRetainedOnRebuildFailure": true, "editingGatedWhileDegraded": true, "diagnosticsAvailable": true, "successfulRetryClearedState": true, "rebuildPanicAlsoGated": true, "watcherResumeFailureReported": true})
}
