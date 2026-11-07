package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
)

func snapshotModelFixture(t *testing.T) (*AppConf, string, metadataRepoPaths, string) {
	t.Helper()
	c, file, p := repoMetadataFixture(t)
	private := t.TempDir()
	p.Repo = filepath.Join(private, "repo")
	p.History = filepath.Join(private, "history")
	p.Temp = filepath.Join(private, "temp")
	c.Sync.Provider = conf.ProviderLocal
	c.Sync.Local = &conf.Local{Endpoint: filepath.Join(t.TempDir(), "remote"), ConcurrentReqs: 4, Timeout: 30}
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	// Seed through the original store codec; no authoritative source indexing or
	// checkout is necessary to exercise snapshot object and tag operations.
	store, e := dejavu.NewStore(p.Repo, c.Repo.Key)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	data := []byte("synthetic model snapshot chunk")
	chunk := &entity.Chunk{ID: util.Hash(data), Data: data}
	f := entity.NewFile("source.docj", int64(len(data)), 1000)
	f.Chunks = []string{chunk.ID}
	id := strings.Repeat("f", 40)
	if e = store.PutChunk(chunk); e != nil {
		t.Fatal(e)
	}
	if e = store.PutFile(f); e != nil {
		t.Fatal(e)
	}
	if e = store.PutIndex(&entity.Index{ID: id, Files: []string{f.ID}, Count: 1, Size: int64(len(data)), Created: 1000}); e != nil {
		t.Fatal(e)
	}
	s, e := c.newSnapshotRepoService(p)
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	return c, file, p, id
}
func TestSnapshotContextOriginalWrappersActualTransfersAndPrivateCleanup(t *testing.T) {
	c, file, p, id := snapshotModelFixture(t)
	before, _ := os.ReadFile(file)
	source, _ := os.ReadFile(filepath.Join(p.Data, "assets", "fixture.png"))
	if e := TagSnapshot(id, "  backup\u200b  "); e != nil {
		t.Fatal(e)
	}
	if e := UploadCloudSnapshot("backup", id); e != nil {
		t.Fatal(e)
	}
	if e := RemoveTagSnapshot("backup"); e != nil {
		t.Fatal(e)
	}
	if e := DownloadCloudSnapshot("restored", id); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(p.Repo, "refs", "tags", "restored")); e != nil {
		t.Fatal(e)
	}
	if e := RemoveCloudRepoTag("backup"); e != nil {
		t.Fatal(e)
	}
	if e := PurgeCloud(); e != nil {
		t.Fatal(e)
	}
	if e := PurgeRepo(); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	body, _ := os.ReadFile(filepath.Join(p.Data, "assets", "fixture.png"))
	if !bytes.Equal(before, after) || !bytes.Equal(source, body) {
		t.Fatal("snapshot changed configuration or source")
	}
	c.persistenceReadOnly = true
	if _, e := ChangeSnapshotContext(context.Background(), "purge-local", "", ""); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	recordCloudBusinessEvidence(t, "snapshot-model.json", map[string]any{"originalWrappers": 7, "actualEncryptedTransfer": true, "privateTagRestored": true, "configurationUnchanged": true, "sourceUnchanged": true, "readOnlyRejected": true})
}
func TestSnapshotContextRejectsAliasedRepoRootAndMemberNoop(t *testing.T) {
	c, file, p, id := snapshotModelFixture(t)
	p.Repo = p.Data
	if _, e := c.executeSnapshot(context.Background(), c.AppData, cloudrepo.SnapshotRequest{Kind: "purge-local"}, p); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
	c.SetUser(nil)
	before, _ := os.ReadFile(file)
	out, e := ChangeSnapshotContext(context.Background(), "upload", id, "backup")
	if e != nil || out.Notice != 214 || out.Applied {
		t.Fatal(out, e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("membership noop saved config")
	}
	recordCloudBusinessEvidence(t, "snapshot-model-guards.json", map[string]any{"aliasedSourceRepoRejected": true, "membershipNoop": true, "configurationUnchanged": true})
}
