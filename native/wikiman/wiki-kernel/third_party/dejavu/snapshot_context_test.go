package dejavu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func snapshotFixture(t *testing.T, remote string) *Repo {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	os.MkdirAll(data, 0700)
	os.WriteFile(filepath.Join(data, "source.docj"), []byte("synthetic authoritative source"), 0600)
	cfg := &cloud.Conf{Dir: "main", AvailableSize: 1 << 30, RequestContext: context.Background(), Local: &cloud.ConfLocal{Endpoint: remote, ConcurrentReqs: 4, Timeout: 30}}
	r, e := NewRepo(data, filepath.Join(root, "repo"), filepath.Join(root, "history"), filepath.Join(root, "temp"), "fixture", "fixture", "test", bytes.Repeat([]byte{7}, 32), nil, cloud.NewLocal(&cloud.BaseCloud{Conf: cfg}))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.CloseMetadata)
	return r
}
func seedSnapshot(t *testing.T, r *Repo, id string) (*entity.File, []string) {
	t.Helper()
	ids := []string{}
	size := 0
	for _, b := range []string{"synthetic chunk one", "synthetic chunk two"} {
		data := []byte(b)
		c := &entity.Chunk{ID: util.Hash(data), Data: data}
		if e := r.store.PutChunk(c); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, c.ID)
		size += len(data)
	}
	f := entity.NewFile("source.docj", int64(size), 1000)
	f.Chunks = ids
	if e := r.store.PutFile(f); e != nil {
		t.Fatal(e)
	}
	if e := r.store.PutIndex(&entity.Index{ID: id, Files: []string{f.ID}, Count: 1, Size: int64(size), Created: 1000, Memo: "synthetic"}); e != nil {
		t.Fatal(e)
	}
	return f, ids
}
func recordSnapshotEvidence(t *testing.T, name string, v any) {
	t.Helper()
	if dir := os.Getenv("WIKI_CLOUD_BUSINESS_EVIDENCE_DIR"); dir != "" {
		os.MkdirAll(dir, 0700)
		b, _ := json.MarshalIndent(v, "", "  ")
		if e := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSnapshotActualLocalEncryptedRoundTripTagsAndPurge(t *testing.T) {
	remote := t.TempDir()
	a, b := snapshotFixture(t, remote), snapshotFixture(t, remote)
	id := strings.Repeat("a", 40)
	f, chunks := seedSnapshot(t, a, id)
	if _, e := a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "tag", ID: id, Tag: "backup"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	up, e := a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: id, Tag: "backup"}, SnapshotOptions{})
	if e != nil || !up.Applied {
		t.Fatal(up, e)
	}
	down, e := b.SnapshotContext(context.Background(), SnapshotRequest{Kind: "download", ID: id, Tag: "restored"}, SnapshotOptions{})
	if e != nil || !down.Applied {
		t.Fatal(down, e)
	}
	if got, e := b.GetTag("restored"); e != nil || got != id {
		t.Fatal(got, e)
	}
	for _, c := range chunks {
		v, e := b.store.GetChunk(c)
		if e != nil || util.Hash(v.Data) != c {
			t.Fatal("encrypted bytes did not round trip", e)
		}
	}
	if got, e := b.store.GetFile(f.ID); e != nil || got.Path != f.Path {
		t.Fatal(got, e)
	}
	source, _ := os.ReadFile(filepath.Join(b.DataPath, "source.docj"))
	if string(source) != "synthetic authoritative source" {
		t.Fatal("download checked out source")
	}
	orphan := strings.Repeat("d", 40)
	seedSnapshot(t, a, orphan)
	if _, e = a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "purge-local"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e = a.store.GetIndex(orphan); !os.IsNotExist(e) {
		t.Fatal("local orphan retained", e)
	}
	if _, e = a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "cloud-untag", Tag: "backup"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	purged, e := a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "purge-cloud"}, SnapshotOptions{})
	if e != nil || purged.Indexes != 1 || purged.Objects < 1 {
		t.Fatal(purged, e)
	}
	recordSnapshotEvidence(t, "snapshot-roundtrip.json", map[string]any{"encryptedChunks": len(chunks), "uploadFiles": up.Files, "downloadFiles": down.Files, "downloadChunks": down.Chunks, "localTagRestored": true, "cloudTagRemoved": true, "purgedIndexes": purged.Indexes, "purgedObjects": purged.Objects, "sourceUnchanged": true})
}

type faultCloud struct {
	cloud.Cloud
	ctx      context.Context
	download func(context.Context, string) error
	upload   func(context.Context, string) error
	traffic  func(context.Context)
}

func (c *faultCloud) DownloadObject(p string) ([]byte, error) {
	if c.download != nil {
		if e := c.download(c.ctx, p); e != nil {
			return nil, e
		}
	}
	return c.Cloud.DownloadObject(p)
}
func (c *faultCloud) UploadObject(p string, o bool) (int64, error) {
	if c.upload != nil {
		if e := c.upload(c.ctx, p); e != nil {
			return 0, e
		}
	}
	return c.Cloud.UploadObject(p, o)
}
func (c *faultCloud) AddTraffic(t *cloud.Traffic) {
	if c.traffic != nil {
		c.traffic(c.ctx)
	}
	c.Cloud.AddTraffic(t)
}

func (c *faultCloud) GetRefsFiles() ([]string, []*cloud.Ref, error) {
	return (&snapshotCloud{Cloud: c.Cloud, s: &snapshotScope{ctx: c.ctx}}).GetRefsFiles()
}
func faultOptions(download, upload func(context.Context, string) error, traffic func(context.Context)) SnapshotOptions {
	return SnapshotOptions{Open: func(source cloud.Cloud, ctx context.Context) (cloud.Cloud, func(), error) {
		if f, ok := source.(*faultCloud); ok {
			source = f.Cloud
		}
		c, done, e := cloud.OpenContext(source, ctx)
		return &faultCloud{Cloud: c, ctx: ctx, download: download, upload: upload, traffic: traffic}, done, e
	}}
}
func TestSnapshotFailedChunksNeverCommitIndexOrTagAndResumeRepairsCachedFiles(t *testing.T) {
	remote := t.TempDir()
	a, b := snapshotFixture(t, remote), snapshotFixture(t, remote)
	id := strings.Repeat("b", 40)
	f, chunks := seedSnapshot(t, a, id)
	a.AddTag(id, "backup")
	if _, e := a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: id, Tag: "backup"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	failPath := "objects/" + chunks[0][:2] + "/" + chunks[0][2:]
	opts := faultOptions(func(_ context.Context, p string) error {
		if p == failPath {
			return errors.New("synthetic chunk failure")
		}
		return nil
	}, nil, nil)
	out, e := b.SnapshotContext(context.Background(), SnapshotRequest{Kind: "download", ID: id, Tag: "restored"}, opts)
	if e == nil || out.Applied {
		t.Fatal(out, e)
	}
	if _, e = b.store.GetIndex(id); !os.IsNotExist(e) {
		t.Fatal("partial index committed", e)
	}
	if _, e = b.GetTag("restored"); e == nil {
		t.Fatal("partial tag committed")
	}
	if _, e = b.store.GetFile(f.ID); e != nil {
		t.Fatal("expected resumable private file", e)
	}
	out, e = b.SnapshotContext(context.Background(), SnapshotRequest{Kind: "download", ID: id, Tag: "restored"}, SnapshotOptions{})
	if e != nil || !out.Applied {
		t.Fatal(out, e)
	}
	for _, c := range chunks {
		if _, e = b.store.GetChunk(c); e != nil {
			t.Fatal("cached metadata caused missing chunk to be skipped", e)
		}
	}
	recordSnapshotEvidence(t, "snapshot-resume.json", map[string]any{"failedDownloadDidNotCommitIndexOrTag": true, "cachedFilesRetained": true, "resumeVerifiedAllChunks": true})
}
func TestSnapshotBatchFailureJoinsOtherWorkersAndTrafficIsJoined(t *testing.T) {
	remote := t.TempDir()
	a := snapshotFixture(t, remote)
	id := strings.Repeat("c", 40)
	_, chunks := seedSnapshot(t, a, id)
	a.AddTag(id, "backup")
	entered, exited := make(chan struct{}), make(chan struct{})
	blocked := "objects/" + chunks[0][:2] + "/" + chunks[0][2:]
	failed := "objects/" + chunks[1][:2] + "/" + chunks[1][2:]
	opts := faultOptions(nil, func(ctx context.Context, p string) error {
		if p == blocked {
			close(entered)
			<-ctx.Done()
			close(exited)
			return ctx.Err()
		}
		if p == failed {
			<-entered
			return errors.New("synthetic")
		}
		return nil
	}, nil)
	out, e := a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: id, Tag: "backup"}, opts)
	if e == nil || out.Applied {
		t.Fatal(out, e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("batch returned before canceled actual worker")
	}
	if _, e = os.Stat(filepath.Join(remote, "main", "refs", "tags", "backup")); !os.IsNotExist(e) {
		t.Fatal("failed objects published remote tag")
	}
	traffic, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	opts = faultOptions(nil, nil, func(context.Context) { close(traffic); <-release })
	go func() {
		_, e := a.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: id, Tag: "backup"}, opts)
		done <- e
	}()
	<-traffic
	select {
	case <-done:
		t.Fatal("unjoined traffic task")
	default:
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	recordSnapshotEvidence(t, "snapshot-join.json", map[string]any{"failedBatchJoinedActualWorkers": true, "failedBatchDidNotPublishTag": true, "trafficJoinedBeforeReturn": true})
}
func TestSnapshotCorruptRetainedReferenceBlocksPurgeAndInvalidNames(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("e", 40)
	seedSnapshot(t, r, id)
	r.AddTag(id, "keep")
	os.WriteFile(filepath.Join(r.Path, "indexes", id), []byte("corrupt"), 0600)
	before, _ := os.ReadDir(filepath.Join(r.Path, "objects"))
	if _, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "purge-local"}, SnapshotOptions{}); e == nil {
		t.Fatal("corrupt retained index ignored")
	}
	after, _ := os.ReadDir(filepath.Join(r.Path, "objects"))
	if len(before) != len(after) {
		t.Fatal("deleted objects before validating retention")
	}
	for _, tag := range []string{"../x", "..", ".", "a/b"} {
		if _, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "untag", Tag: tag}, SnapshotOptions{}); e == nil {
			t.Fatal(tag)
		}
	}
	recordSnapshotEvidence(t, "snapshot-purge-guard.json", map[string]any{"corruptRetainedIndexRefusedBeforeDelete": true, "invalidTagsRejected": 4})
}

func TestSnapshotCloudPurgePreservesRecursiveTagRefsAndRejectsCorruptRetainedIndex(t *testing.T) {
	remote := t.TempDir()
	r := snapshotFixture(t, remote)
	keep, orphan := strings.Repeat("a", 40), strings.Repeat("b", 40)
	_, chunks := seedSnapshot(t, r, keep)
	r.AddTag(keep, "keep")
	if _, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: keep, Tag: "keep"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	seedSnapshot(t, r, orphan)
	r.AddTag(orphan, "orphan")
	if _, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "upload", ID: orphan, Tag: "orphan"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "cloud-untag", Tag: "orphan"}, SnapshotOptions{}); e != nil {
		t.Fatal(e)
	}
	out, e := r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "purge-cloud"}, SnapshotOptions{})
	if e != nil || out.Indexes != 1 || out.Objects != 0 {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(remote, "main", "indexes", keep)); e != nil {
		t.Fatal("referenced index deleted", e)
	}
	os.WriteFile(filepath.Join(remote, "main", "indexes", keep), []byte("corrupt retained index"), 0600)
	if _, e = r.SnapshotContext(context.Background(), SnapshotRequest{Kind: "purge-cloud"}, SnapshotOptions{}); e == nil {
		t.Fatal("corrupt retained cloud index ignored")
	}
	for _, c := range chunks {
		if _, e = os.Stat(filepath.Join(remote, "main", "objects", c[:2], c[2:])); e != nil {
			t.Fatal("deleted retained chunk", e)
		}
	}
	recordSnapshotEvidence(t, "snapshot-cloud-purge.json", map[string]any{"orphanIndexDeleted": 1, "referencedObjectsDeleted": 0, "recursiveTagReferencesPreserved": true, "corruptRetainedCloudIndexRefusedBeforeDelete": true})
}
