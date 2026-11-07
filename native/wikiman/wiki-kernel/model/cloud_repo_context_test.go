package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/klauspost/compress/zstd"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
)

func repoMetadataFixture(t *testing.T) (*AppConf, string, metadataRepoPaths) {
	t.Helper()
	c, file, dir := cloudBusinessFixture(t)
	c.Sync = conf.NewSync()
	c.Repo = conf.NewRepo()
	c.Repo.Key = bytes.Repeat([]byte{1}, 32)
	if c.System == nil {
		c.System = conf.NewSystem()
	}
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	p := metadataRepoPaths{Data: dir, Repo: filepath.Join(dir, "repo"), History: filepath.Join(dir, "history"), Temp: filepath.Join(dir, "temp"), Agent: "synthetic"}
	t.Cleanup(func() { c.CloseCloudRepo(context.Background()) })
	return c, file, p
}
func TestCloudRepoMetadataActualHTTPProjectionAndLegacyWrappers(t *testing.T) {
	c, file, p := repoMetadataFixture(t)
	before, _ := os.ReadFile(file)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var b map[string]any
		if json.NewDecoder(r.Body).Decode(&b) != nil || b["token"] != "fixture-user-token" || r.URL.Query().Get("uid") != "fixture-user" {
			t.Error("wrong owner protocol")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/siyuan/dejavu/getRepoStat":
			w.Write([]byte(`{"code":0,"data":{"sync":{"size":10,"updated":"s"},"backup":{"size":20,"updated":"b"},"assetSize":30}}`))
		case "/apis/siyuan/dejavu/getRepos":
			w.Write([]byte(`{"code":0,"data":{"repos":[{"name":"main","size":10,"updated":"u"}],"size":10}}`))
		case "/apis/siyuan/dejavu/getRepoIndexes":
			if b["page"] != float64(1) {
				t.Error("page clamp")
			}
			w.Write([]byte(`{"code":0,"data":{"indexes":[{"id":"synthetic-index","memo":"fixture","created":1000,"files":[],"count":2,"size":10}],"pageCount":1,"totalCount":1}}`))
		case "/apis/siyuan/dejavu/getRepoTags":
			w.Write([]byte(`{"code":0,"data":{"tags":[]}}`))
		default:
			t.Error("unexpected cloud route")
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	p.Server = server.URL
	s, e := c.newCloudRepoService(func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
		return c.executeLegacyMetadata(ctx, v, r, p)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	sy, ba, sz, _, _, _, _, _, _, _, e := GetCloudSpace()
	if e != nil || sy.Size != 10 || ba.Size != 20 || sz == "-" {
		t.Fatal(sy, ba, sz, e)
	}
	dirs, h, e := ListCloudSyncDir()
	if e != nil || len(dirs) != 1 || dirs[0].CloudName != "main" || h == "-" {
		t.Fatal(dirs, h, e)
	}
	logs, pc, tc, e := GetCloudRepoSnapshots(-4)
	if e != nil || len(logs) != 1 || pc != 1 || tc != 1 || logs[0].Memo != "fixture" {
		t.Fatal(logs, pc, tc, e)
	}
	tags, e := GetCloudRepoTagSnapshots()
	if e != nil || tags == nil || len(tags) != 0 {
		t.Fatal(tags, e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) || calls.Load() != 4 {
		t.Fatal("configuration mutated or protocol retried")
	}
	recordCloudBusinessEvidence(t, "cloud-repo-http.json", map[string]any{"requests": calls.Load(), "configurationUnchanged": true, "originalProjection": true, "legacyWrappers": 4})
}
func TestCloudRepoMetadataActualCancelCloseAndBadReply(t *testing.T) {
	c, _, p := repoMetadataFixture(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	p.Server = server.URL
	s, e := c.newCloudRepoService(func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
		return executeMetadata(ctx, v, r, p)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	done := make(chan error, 1)
	go func() { _, e := s.Query(context.Background(), cloudrepo.Request{Kind: "space"}); done <- e }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e == nil {
			t.Fatal("lost cancellation")
		}
	case <-ctx.Done():
		t.Fatal("close did not join IO")
	}
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("HTTP not canceled")
	}
	if _, e = s.Query(context.Background(), cloudrepo.Request{Kind: "space"}); !errors.Is(e, cloudrepo.ErrClosed) {
		t.Fatal(e)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"data":null}`))
	}))
	defer bad.Close()
	p.Server = bad.URL
	for _, kind := range []string{"space", "repos", "logs", "tags"} {
		if _, e = executeMetadata(context.Background(), c.AppData, cloudrepo.Request{Kind: kind, Page: 1}, p); e == nil {
			t.Fatal("malformed reply accepted", kind)
		}
	}
	recordCloudBusinessEvidence(t, "cloud-repo-close.json", map[string]any{"actualHTTPCanceled": true, "closeJoined": true, "malformedRepliesRejected": 4})
}
func TestCloudRepoMetadataSameIDLocalStoresAndMembership(t *testing.T) {
	c, file, p := repoMetadataFixture(t)
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: filepath.Join(t.TempDir(), "remote")}
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(file)
	id := strings.Repeat("a", 40)
	// Prime the *legacy* global cache with a foreign copy, then prove it is live
	// even after its backing fixture disappears. Scoped queries must bypass it.
	seedDir := t.TempDir()
	seed, e := dejavu.NewStore(seedDir, c.Repo.Key)
	if e != nil {
		t.Fatal(e)
	}
	defer seed.Close()
	if e = seed.PutIndex(&entity.Index{ID: id, Memo: "foreign-global", Created: 1000}); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(seedDir, "indexes", id)); e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(time.Second)
	for {
		v, e := seed.GetIndex(id)
		if e == nil && v.Memo == "foreign-global" {
			break
		}
		if time.Now().After(until) {
			t.Fatal("legacy cache was not primed")
		}
		time.Sleep(time.Millisecond)
	}
	ref := filepath.Join(c.Sync.Local.Endpoint, "main", "refs", "tags")
	os.MkdirAll(ref, 0700)
	os.WriteFile(filepath.Join(ref, "tag"), []byte(id), 0600)
	enc, _ := zstd.NewWriter(nil)
	defer enc.Close()
	for _, name := range []string{"one", "two"} {
		q := p
		q.Repo = filepath.Join(t.TempDir(), "repo")
		os.MkdirAll(filepath.Join(q.Repo, "indexes"), 0700)
		raw, _ := json.Marshal(entity.Index{ID: id, Memo: name, Created: 1000})
		os.WriteFile(filepath.Join(q.Repo, "indexes", id), enc.EncodeAll(raw, nil), 0600)
		out, e := executeMetadata(context.Background(), c.AppData, cloudrepo.Request{Kind: "tags"}, q)
		if e != nil {
			t.Fatal(e)
		}
		var v struct {
			Snapshots []struct {
				Memo string `json:"memo"`
			}
		}
		json.Unmarshal(out.Data, &v)
		if len(v.Snapshots) != 1 || v.Snapshots[0].Memo != name {
			t.Fatal("cross-store cached ID", string(out.Data))
		}
	}
	if e := os.WriteFile(filepath.Join(ref, "tag"), []byte("../outside"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := executeMetadata(context.Background(), c.AppData, cloudrepo.Request{Kind: "tags"}, p); e == nil {
		t.Fatal("remote reference became a local path")
	}
	cfg, _ := c.AppData.Clone()
	cfg.User = nil
	out, e := executeMetadata(context.Background(), cfg, cloudrepo.Request{Kind: "logs", Page: 1}, p)
	if e != nil || out.Notice != 214 || !bytes.Contains(out.Data, []byte(`"snapshots":[]`)) {
		t.Fatal(out, e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("read changed configuration")
	}
	old, _ := c.AppData.Clone()
	if e = c.commitRepoName(context.Background(), old, "changed"); e != nil {
		t.Fatal(e)
	}
	if e = c.commitRepoName(context.Background(), old, "stale"); !errors.Is(e, errRepoConfigurationChanged) {
		t.Fatal("stale configuration overwritten", e)
	}
	recordCloudBusinessEvidence(t, "cloud-repo-stores.json", map[string]any{"sameIDStoresIsolated": 2, "foreignGlobalCacheBypassed": true, "malformedSnapshotRefsRejected": 1, "membershipNoOp": true, "staleCommitRejected": true, "readsDoNotSave": true})
}
