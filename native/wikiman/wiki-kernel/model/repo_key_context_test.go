package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/siyuan-note/dejavu"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/util"
)

func repoKeyFixture(t *testing.T) (*AppConf, string, metadataRepoPaths) {
	t.Helper()
	c, file, p, _ := snapshotModelFixture(t)
	p.Data = filepath.Join(t.TempDir(), "data")
	os.Mkdir(p.Data, 0700)
	os.WriteFile(filepath.Join(p.Data, "source.docj"), []byte("synthetic key source"), 0600)
	p.Workspace = filepath.Dir(p.Data)
	workspace, data := util.WorkspaceDir, util.DataDir
	util.WorkspaceDir = p.Workspace
	util.DataDir = p.Data
	t.Cleanup(func() { util.WorkspaceDir = workspace; util.DataDir = data })
	c.repoOwner.Close(context.Background())
	s, e := c.newSnapshotRepoService(p)
	if e != nil {
		t.Fatal(e)
	}
	c.repoOwner = s
	return c, file, p
}

func TestRepoKeyOriginalWrappersActualInitAndReset(t *testing.T) {
	c, file, p := repoKeyFixture(t)
	key := bytes.Repeat([]byte{8}, 32)
	encoded := base64.StdEncoding.EncodeToString(key)
	original := c.Repo.SyncIndexTiming
	normalized, e := ImportRepoKey(" \u200b" + encoded + " ")
	if e != nil || normalized != encoded || !bytes.Equal(c.Repo.Key, key) {
		t.Fatal("import failed", e)
	}
	latest, e := os.ReadFile(filepath.Join(p.Repo, "refs", "latest"))
	if e != nil || len(latest) != 40 {
		t.Fatal("initial snapshot missing", e)
	}
	store, e := dejavu.NewStore(p.Repo, c.Repo.Key)
	if e != nil {
		t.Fatal(e)
	}
	idx, e := store.GetIndex(string(latest))
	store.Close()
	if e != nil || idx.Count != 1 {
		t.Fatal("new key cannot decrypt original codec", e)
	}
	if e = InitRepoKeyFromPassphrase(encoded); e != nil || !bytes.Equal(c.Repo.Key, key) {
		t.Fatal("base64 passphrase", e)
	}
	if e = InitRepoKey(); e != nil || len(c.Repo.Key) != 32 || bytes.Equal(c.Repo.Key, key) {
		t.Fatal("random initialization", e)
	}
	c.Sync.Enabled = true
	if e = c.SaveError(); e != nil {
		t.Fatal(e)
	}
	if e = ResetRepo(); e != nil {
		t.Fatal(e)
	}
	if len(c.Repo.Key) != 0 || c.Sync.Enabled || c.Repo.SyncIndexTiming != original {
		t.Fatal("reset mutated unrelated configuration")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var persisted conf.AppData
	if e = json.Unmarshal(raw, &persisted); e != nil || len(persisted.Repo.Key) != 0 || persisted.Sync.Enabled {
		t.Fatal("reset not persisted", e)
	}
	rows, e := os.ReadDir(p.Repo)
	if e != nil || len(rows) != 0 {
		t.Fatal("reset cache not empty", e)
	}
	if b, e := os.ReadFile(filepath.Join(p.Data, "source.docj")); e != nil || string(b) != "synthetic key source" {
		t.Fatal("source modified", e)
	}
	start := time.Now()
	if e = c.CloseCloudRepo(context.Background()); e != nil || time.Since(start) > time.Second {
		t.Fatal("delayed reload not canceled/joined", e)
	}
	recordCloudBusinessEvidence(t, "repokey-model.json", map[string]any{"originalWrappers": 4, "importNormalized": true, "base64PassphrasePreserved": true, "initialEncryptedSnapshotReadable": true, "randomKeyGenerated": true, "resetDisabledSyncAndClearedKey": true, "unrelatedPreferencesPreserved": true, "sourceUnchanged": true, "delayedReloadCanceledAndJoined": true})
}

func TestRepoKeyPersistenceFailureReadonlyAndConfigurationFence(t *testing.T) {
	c, file, p := repoKeyFixture(t)
	marker := filepath.Join(p.Repo, "keep")
	os.WriteFile(marker, []byte("synthetic private cache"), 0600)
	before, _ := os.ReadFile(file)
	key := append([]byte{}, c.Repo.Key...)
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	if _, e := ImportRepoKey("invalid key"); !errors.Is(e, cloudrepo.ErrRepoKeyInput) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, e := ImportRepoKey(encoded); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	realPath := c.confPath
	c.confPath = filepath.Join(t.TempDir(), "missing", "conf.json")
	if _, e := ImportRepoKey(encoded); e == nil {
		t.Fatal("save failure hidden")
	}
	c.confPath = realPath
	if b, e := os.ReadFile(marker); e != nil || string(b) != "synthetic private cache" {
		t.Fatal("save failure lost cache", e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) || !bytes.Equal(c.Repo.Key, key) {
		t.Fatal("save failure changed key")
	}
	var expected conf.AppData
	c.WithSettingSnapshot(context.Background(), func(v conf.AppData) error { expected = v; return nil })
	c.Repo.SyncIndexTiming++
	out, e := c.executeRepoKey(context.Background(), expected, cloudrepo.RepoKeyRequest{Kind: "reset"}, nil, p)
	if !errors.Is(e, errRepoConfigurationChanged) || out.Applied {
		t.Fatal("stale config overwrite", e)
	}
	p.Repo = p.Data
	out, e = c.executeRepoKey(context.Background(), expected, cloudrepo.RepoKeyRequest{Kind: "reset"}, nil, p)
	if e == nil || out.Applied {
		t.Fatal("source alias accepted")
	}
	recordCloudBusinessEvidence(t, "repokey-model-guards.json", map[string]any{"invalidKeyNoMutation": true, "readOnlyRejected": true, "persistenceFailureRestoredCacheAndConfig": true, "staleConfigurationRefused": true, "sourceAliasRejected": true})
}

func TestRepoKeyPostCommitCancellationAndEmptyInitialIndex(t *testing.T) {
	c, _, p := repoKeyFixture(t)
	key := bytes.Repeat([]byte{6}, 32)
	encoded := base64.StdEncoding.EncodeToString(key)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		v cloudrepo.RepoKeyResult
		e error
	}
	done := make(chan result, 1)
	go func() {
		out, e := c.repoOwner.RepoKey(ctx, cloudrepo.RepoKeyRequest{Kind: "import", Input: encoded})
		done <- result{out, e}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		committed := false
		c.WithSettingSnapshot(context.Background(), func(v conf.AppData) error { committed = bytes.Equal(v.Repo.Key, key); return nil })
		if committed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("key commit timed out")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	out := <-done
	if !errors.Is(out.e, context.Canceled) || !out.v.Applied || !out.v.InitialIndexFailed || !bytes.Equal(out.v.Key, key) || out.v.Normalized != encoded {
		t.Fatal("canceled committed key outcome lost")
	}
	if _, e := os.Stat(filepath.Join(p.Repo, "refs", "latest")); !os.IsNotExist(e) {
		t.Fatal("canceled init published index", e)
	}
	os.Remove(filepath.Join(p.Data, "source.docj"))
	out.v, out.e = c.repoOwner.RepoKey(context.Background(), cloudrepo.RepoKeyRequest{Kind: "import", Input: encoded})
	if out.e != nil || !out.v.Applied || !out.v.InitialIndexFailed {
		t.Fatal("original empty index rule changed", out.e)
	}
	recordCloudBusinessEvidence(t, "repokey-model-cancel.json", map[string]any{"postCommitCancellationPreservesAppliedAndKey": true, "unfinishedInitialIndexReported": true, "canceledInitDidNotPublishIndex": true, "emptyInitialIndexDoesNotUndoKey": true})
}

func TestRepoKeyDetachedResetStartsNoCompatibilityReload(t *testing.T) {
	c, _, p := repoKeyFixture(t)
	other, e := NewAppConfAt(filepath.Join(t.TempDir(), "conf.json"), false)
	if e != nil {
		t.Fatal(e)
	}
	c.WithSettingSnapshot(context.Background(), func(v conf.AppData) error { other.AppData = v; return nil })
	out, e := other.executeRepoKey(context.Background(), other.AppData, cloudrepo.RepoKeyRequest{Kind: "reset"}, nil, p)
	if e != nil || !out.Applied || other.repoTasks != nil {
		t.Fatal("detached reset started compatibility task", e)
	}
}

func TestRepoKeyOldLegacyRepositoryRejectedAfterRotation(t *testing.T) {
	c, _, p := repoKeyFixture(t)
	repoPath, history, temp := util.RepoDir, util.HistoryDir, util.TempDir
	util.RepoDir = p.Repo
	util.HistoryDir = p.History
	util.TempDir = p.Temp
	defer func() { util.RepoDir = repoPath; util.HistoryDir = history; util.TempDir = temp }()
	old, e := newRepository()
	if e != nil {
		t.Fatal(e)
	}
	defer old.CloseMetadata()
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	if _, e = ImportRepoKey(encoded); e != nil {
		t.Fatal(e)
	}
	if _, e = old.Index("stale", true, nil); !errors.Is(e, errRepoConfigurationChanged) {
		t.Fatal("old key index admitted", e)
	}
	if _, _, e = old.Sync(nil); !errors.Is(e, errRepoConfigurationChanged) {
		t.Fatal("old key sync admitted", e)
	}
	if _, _, e = old.SyncDownload(nil); !errors.Is(e, errRepoConfigurationChanged) {
		t.Fatal("old key download admitted", e)
	}
	if _, e = old.SyncUpload(nil); !errors.Is(e, errRepoConfigurationChanged) {
		t.Fatal("old key upload admitted", e)
	}
	if e = old.Reset(); !errors.Is(e, errRepoConfigurationChanged) {
		t.Fatal("old key reset admitted", e)
	}
	if _, e = os.Stat(filepath.Join(p.Repo, "refs", "latest")); e != nil || len(c.Repo.Key) != 32 {
		t.Fatal("new repository lost", e)
	}
	recordCloudBusinessEvidence(t, "repokey-stale-legacy.json", map[string]any{"staleIndexRefused": true, "staleSyncRefusedBeforeCloudLock": true, "staleManualTransferRefused": true, "staleResetRefused": true, "newRepositoryPreserved": true})
}
