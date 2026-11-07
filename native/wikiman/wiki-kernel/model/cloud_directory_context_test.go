package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/cloudlock"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
)

func directoryFixture(t *testing.T) (*AppConf, string, metadataRepoPaths) {
	t.Helper()
	c, file, p := repoMetadataFixture(t)
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: filepath.Join(t.TempDir(), "remote")}
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	s, e := c.newCloudRepoService(func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
		return c.executeLegacyMetadata(ctx, v, r, p)
	}, func(ctx context.Context, v conf.AppData, r cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error) {
		return c.executeLegacyDirectory(ctx, v, r, p, cloudlock.Options{})
	})
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	return c, file, p
}

func TestCloudDirectoryRejectsRelativeOverlappingAndSymlinkedLocalRoots(t *testing.T) {
	c, _, p := directoryFixture(t)
	original := c.Sync.Local.Endpoint
	for _, endpoint := range []string{"", ".", p.Data, filepath.Dir(p.Data), filepath.Join(p.Data, "nested")} {
		c.Sync.Local.Endpoint = endpoint
		if e := validateLocalDirectoryTarget(c.AppData, p); e == nil {
			t.Fatal("unsafe endpoint accepted", endpoint)
		}
	}
	alias := filepath.Join(t.TempDir(), "link")
	if e := os.Symlink(p.Data, alias); e != nil {
		t.Fatal(e)
	}
	c.Sync.Local.Endpoint = filepath.Join(alias, "not-created")
	if e := validateLocalDirectoryTarget(c.AppData, p); e == nil {
		t.Fatal("symlink ancestor escaped source guard")
	}
	c.Sync.Local.Endpoint = original
	if e := validateLocalDirectoryTarget(c.AppData, p); e != nil {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "cloud-directory-paths.json", map[string]any{"relativeAndOverlappingRejected": 5, "symlinkAncestorRejected": true, "independentPrivateRootAllowed": true})
}
func TestCloudDirectoryOfficialCreateRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/siyuan/dejavu/createRepo" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()
	client := cloud.NewSiYuan(&cloud.BaseCloud{Conf: &cloud.Conf{Server: server.URL, RequestContext: context.Background()}})
	if e := client.CreateRepo("fixture"); !errors.Is(e, cloud.ErrCloudCheckFailed) {
		t.Fatal(e)
	}
}
func TestCloudDirectoryActualLocalCreateSwitchRemoveAndPreflight(t *testing.T) {
	c, file, p := directoryFixture(t)
	source, _ := os.ReadFile(filepath.Join(p.Data, "assets", "fixture.png"))
	before, _ := os.ReadFile(file)
	if e := CreateCloudSyncDir("  New\u200bDir  "); e != nil {
		t.Fatal(e)
	}
	if st, e := os.Stat(filepath.Join(c.Sync.Local.Endpoint, "NewDir")); e != nil || !st.IsDir() {
		t.Fatal(e)
	}
	now, _ := os.ReadFile(file)
	if !bytes.Equal(before, now) {
		t.Fatal("create changed selection")
	}
	SetCloudSyncDir("NewDir")
	if c.Sync.CloudName != "NewDir" {
		t.Fatal("switch not published")
	}
	var saved conf.AppData
	raw, _ := os.ReadFile(file)
	json.Unmarshal(raw, &saved)
	if saved.Sync.CloudName != "NewDir" {
		t.Fatal("switch not saved")
	}
	if e := RemoveCloudSyncDir("NewDir"); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(c.Sync.Local.Endpoint, "NewDir")); !os.IsNotExist(e) {
		t.Fatal("directory not removed", e)
	}
	if c.Sync.CloudName != "main" {
		t.Fatal("active removal not reset")
	}
	if _, e := os.Stat(filepath.Join(c.Sync.Local.Endpoint, "main", "lock-sync")); !os.IsNotExist(e) {
		t.Fatal("cloud lock leaked", e)
	}
	for _, name := range []string{"../outside", "main/x", "/", "a b"} {
		if _, e := ChangeCloudDirectoryContext(context.Background(), "remove", name); !errors.Is(e, cloudrepo.ErrName) {
			t.Fatal(name, e)
		}
	}
	if e := RemoveCloudSyncDir(""); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, e := ChangeCloudDirectoryContext(context.Background(), "create", "rejected"); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	after, _ := os.ReadFile(filepath.Join(p.Data, "assets", "fixture.png"))
	if !bytes.Equal(source, after) {
		t.Fatal("source asset changed")
	}
	recordCloudBusinessEvidence(t, "cloud-directory-local.json", map[string]any{"normalizedCreate": true, "switchSaved": true, "activeRemovalReset": true, "lockRemoved": true, "invalidNamesRejected": 4, "readOnlyRejected": true, "sourceUnchanged": true})
}
func TestCloudDirectoryAppliedRemoteRemovalDoesNotOverwriteNewAccount(t *testing.T) {
	c, file, p := directoryFixture(t)
	os.MkdirAll(filepath.Join(c.Sync.Local.Endpoint, "main"), 0700)
	before, _ := os.ReadFile(file)
	var once sync.Once
	opts := cloudlock.Options{Now: func() time.Time {
		once.Do(func() { c.SetUser(&conf.User{UserId: "new-account", UserToken: "synthetic-new"}) })
		return time.Now()
	}}
	out, e := c.executeLegacyDirectory(context.Background(), c.AppData, cloudrepo.DirectoryRequest{Kind: "remove", Name: "main"}, p, opts)
	if !out.Applied || out.Saved || !errors.Is(e, errRepoConfigurationChanged) || c.GetUser().UserId != "new-account" {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(c.Sync.Local.Endpoint, "main")); !os.IsNotExist(e) {
		t.Fatal("remote mutation did not actually happen")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("stale account saved")
	}
	recordCloudBusinessEvidence(t, "cloud-directory-partial.json", map[string]any{"remoteApplied": true, "localSelectionSaved": false, "staleAccountCommitRejected": true, "requiresRefreshBeforeRetry": true})
}
func TestCloudDirectoryActualHTTPAndRepositoryAdmissionCancellation(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		c, file, p := directoryFixture(t)
		c.Sync.Provider = conf.ProviderSiYuan
		before, _ := os.ReadFile(file)
		started, gone := make(chan struct{}), make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Error("mutation before lock acquisition")
			}
			close(started)
			<-r.Context().Done()
			close(gone)
		}))
		defer server.Close()
		p.Endpoint = server.URL + "/"
		s, e := c.newCloudRepoService(func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, cloudrepo.ErrDependency
		}, func(ctx context.Context, v conf.AppData, r cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error) {
			return c.executeLegacyDirectory(ctx, v, r, p, cloudlock.Options{})
		})
		if e != nil {
			t.Fatal(e)
		}
		c.repoOwner = s
		done := make(chan error, 1)
		go func() {
			_, e := s.Directory(context.Background(), cloudrepo.DirectoryRequest{Kind: "create", Name: "new"})
			done <- e
		}()
		<-started
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if e = c.CloseCloudRepo(ctx); e != nil {
			t.Fatal(e)
		}
		if e = <-done; !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		select {
		case <-gone:
		case <-ctx.Done():
			t.Fatal("actual HTTP not canceled")
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(before, after) {
			t.Fatal("cancel saved config")
		}
		recordCloudBusinessEvidence(t, "cloud-directory-http-cancel.json", map[string]any{"actualOfficialLockReadCanceled": true, "closeJoined": true, "remoteMutationStarted": false, "configurationUnchanged": true})
	})
	t.Run("admission", func(t *testing.T) {
		c, _, _ := directoryFixture(t)
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			defer close(done)
			dejavu.WithRepositoryContext(context.Background(), func() error { close(entered); <-release; return nil })
		}()
		<-entered
		defer func() { close(release); <-done }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if _, e := ChangeCloudDirectoryContext(ctx, "create", "new"); !errors.Is(e, context.DeadlineExceeded) {
			t.Fatal(e)
		}
		if _, e := os.Stat(c.Sync.Local.Endpoint); !os.IsNotExist(e) {
			t.Fatal("queued operation touched remote")
		}
	})
}
