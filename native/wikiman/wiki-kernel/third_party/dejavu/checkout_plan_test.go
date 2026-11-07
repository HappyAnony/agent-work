package dejavu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckoutPlanOriginalCodecAndActualLegacyRestore(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("c", 40)
	f, _ := seedSnapshot(t, r, id)
	before, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	latest := filepath.Join(r.Path, "refs", "latest")
	plan, e := r.PrepareCheckoutContext(context.Background(), id)
	if e != nil || plan.ID != id || len(plan.Files) != 1 || plan.Files[0].Path != "source.docj" {
		t.Fatal(plan, e)
	}
	data := []byte("synthetic chunk onesynthetic chunk two")
	sum := sha256.Sum256(data)
	if plan.Bytes != int64(len(data)) || plan.Files[0].SHA256 != hex.EncodeToString(sum[:]) || plan.Files[0].ID != f.ID {
		t.Fatal("manifest differs from original decrypted bytes")
	}
	after, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if !bytes.Equal(before, after) {
		t.Fatal("candidate changed source")
	}
	if _, e = os.Stat(latest); !os.IsNotExist(e) {
		t.Fatal("candidate published latest", e)
	}
	if _, _, e = r.Checkout(id, nil); e != nil {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if !bytes.Equal(after, data) {
		t.Fatal("original checkout did not restore relative metadata")
	}
	recordSnapshotEvidence(t, "checkout-plan-codec.json", map[string]any{"originalEncryptedCodecRead": true, "sha256MatchesDecryptedBytes": true, "candidateDidNotWriteSourceOrRefs": true, "legacyRelativePathRestorePassed": true})
}

func TestCheckoutPlanBadPathsChunksAndSymlinksFailBeforeSourceWrites(t *testing.T) {
	for _, p := range []string{"../escape", "/../escape", "//server/file", "C:/drive", "a\\b", "a/../b", "a//b", "/"} {
		t.Run(p, func(t *testing.T) {
			r := snapshotFixture(t, t.TempDir())
			id := strings.Repeat("d", 40)
			f, chunks := seedSnapshot(t, r, id)
			bad := entity.NewFile(p, f.Size, f.Updated)
			bad.Chunks = chunks
			r.store.PutFile(bad)
			r.store.PutIndex(&entity.Index{ID: id, Files: []string{bad.ID}})
			before, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
			if _, e := r.PrepareCheckoutContext(context.Background(), id); e == nil {
				t.Fatal("unsafe candidate accepted")
			}
			if _, _, e := r.Checkout(id, nil); e == nil {
				t.Fatal("unsafe restore accepted")
			}
			after, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
			if !bytes.Equal(before, after) {
				t.Fatal("unsafe target changed source")
			}
		})
	}
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("e", 40)
	f, chunks := seedSnapshot(t, r, id)
	before, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if e := r.store.Remove(chunks[1]); e != nil {
		t.Fatal(e)
	}
	if _, e := r.PrepareCheckoutContext(context.Background(), id); e == nil {
		t.Fatal("missing chunk accepted")
	}
	if _, _, e := r.Checkout(id, nil); e == nil {
		t.Fatal("missing chunk restored")
	}
	after, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if !bytes.Equal(before, after) {
		t.Fatal("missing chunk modified source")
	}
	// Restore the object then place an aliased directory in the source tree.
	chunk := &entity.Chunk{ID: chunks[1], Data: []byte("synthetic chunk two")}
	r.store.PutChunk(chunk)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0600)
	os.Symlink(outside, filepath.Join(r.DataPath, "linked"))
	if _, _, e := r.Checkout(id, nil); e == nil {
		t.Fatal("source symlink accepted")
	}
	// Prefix-colliding files cannot both be materialized safely.
	child := entity.NewFile("source.docj/child", f.Size, 1000)
	child.Chunks = chunks
	r.store.PutFile(child)
	r.store.PutIndex(&entity.Index{ID: id, Files: []string{f.ID, child.ID}})
	if _, e := r.PrepareCheckoutContext(context.Background(), id); e == nil {
		t.Fatal("file/directory collision accepted")
	}
	recordSnapshotEvidence(t, "checkout-plan-guards.json", map[string]any{"unsafePathCasesRejected": 8, "missingChunkRejectedBeforeWrite": true, "sourceSymlinkRefused": true, "fileDirectoryCollisionRejected": true, "sourceUnchanged": true})
}

type checkoutCancellation struct {
	context.Context
	checks atomic.Int32
	limit  int32
	cancel context.CancelFunc
}

func (c *checkoutCancellation) Err() error {
	if c.checks.Add(1) == c.limit {
		c.cancel()
	}
	return c.Context.Err()
}
func TestCheckoutPlanLateCancellationDiscardsCandidateAndPrivateCacheIsolation(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	id := strings.Repeat("f", 40)
	f, chunks := seedSnapshot(t, r, id)
	// Poison the old process cache with another repository's same metadata ID.
	poison := *f
	poison.Chunks = []string{strings.Repeat("0", 40)}
	fileCache.Set(f.ID, &poison, 1)
	fileCache.Wait()
	defer func() { fileCache.Del(f.ID); fileCache.Wait() }()
	counter := &checkoutCancellation{Context: context.Background(), limit: -1, cancel: func() {}}
	plan, e := r.PrepareCheckoutContext(counter, id)
	if e != nil || len(plan.Files) != 1 || !bytes.Equal([]byte(plan.Files[0].Chunks[0]), []byte(chunks[0])) {
		t.Fatal("foreign global file cache used", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	late := &checkoutCancellation{Context: ctx, limit: counter.checks.Load() - 1, cancel: cancel}
	plan, e = r.PrepareCheckoutContext(late, id)
	if !errors.Is(e, context.Canceled) || plan.ID != "" || len(plan.Files) != 0 {
		t.Fatal("partial canceled plan escaped", e)
	}
	if _, e = r.PrepareCheckoutContext(context.Background(), id); e != nil {
		t.Fatal("gate or context leaked", e)
	}
	if util.Hash([]byte("synthetic chunk one")) != chunks[0] {
		t.Fatal("fixture mismatch")
	}
	recordSnapshotEvidence(t, "checkout-plan-cancel.json", map[string]any{"globalCachePoisonIgnored": true, "lateCancellationDiscardedCandidate": true, "retrySucceeded": true})
}

func TestCheckoutFileFailureCleansTemporaryFileAndKeepsSource(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	f, _ := seedSnapshot(t, r, strings.Repeat("a", 40))
	f.Chunks = append(f.Chunks, strings.Repeat("0", 40))
	before, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if e := r.checkoutFile(f, r.DataPath, 1, 1, nil); e == nil {
		t.Fatal("missing chunk accepted after temporary file opened")
	}
	after, _ := os.ReadFile(filepath.Join(r.DataPath, "source.docj"))
	if !bytes.Equal(before, after) {
		t.Fatal("partial temporary file published")
	}
	rows, e := os.ReadDir(r.DataPath)
	if e != nil || len(rows) != 1 {
		t.Fatal("failed checkout left temporary files", e)
	}
	recordSnapshotEvidence(t, "checkout-plan-temp-cleanup.json", map[string]any{"failureAfterTempOpenJoinedCleanup": true, "noTemporaryFileLeft": true, "originalSourcePreserved": true})
}
