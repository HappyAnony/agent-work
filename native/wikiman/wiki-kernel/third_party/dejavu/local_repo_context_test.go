package dejavu

import (
	"bytes"
	"context"
	"errors"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/eventbus"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalRepoActualIndexFormatChangesAndRetention(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	large := bytes.Repeat([]byte("synthetic asset"), 100000)
	if e := os.WriteFile(filepath.Join(r.DataPath, "asset.bin"), large, 0600); e != nil {
		t.Fatal(e)
	}
	run := func() LocalRepoResult {
		out, e := r.LocalRepoContext(context.Background(), LocalRepoRequest{Kind: "index", Memo: "synthetic"}, SnapshotOptions{}, nil)
		if e != nil || !out.Applied {
			t.Fatal(out, e)
		}
		return out
	}
	first := run()
	if !first.Changed {
		t.Fatal(first)
	}
	same := run()
	if same.Changed || same.ID != first.ID {
		t.Fatal(same)
	}
	full, e := os.ReadFile(filepath.Join(r.Path, "full-latest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var idx FullIndex
	if e = msgpack.Unmarshal(full, &idx); e != nil || idx.ID != first.ID || len(idx.Files) != 2 {
		t.Fatal(idx, e)
	}
	found := false
	for _, f := range idx.Files {
		if f.Path == "/asset.bin" || f.Path == "asset.bin" {
			var got []byte
			for _, id := range f.Chunks {
				c, e := r.store.GetChunk(id)
				if e != nil {
					t.Fatal(e)
				}
				got = append(got, c.Data...)
			}
			if !bytes.Equal(got, large) {
				t.Fatal("original encrypted chunks did not round trip")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("asset missing")
	}
	os.WriteFile(filepath.Join(r.DataPath, "source.docj"), []byte("synthetic changed source"), 0600)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(filepath.Join(r.DataPath, "source.docj"), future, future)
	second := run()
	if !second.Changed || second.ID == first.ID {
		t.Fatal(second)
	}
	os.Remove(filepath.Join(r.DataPath, "asset.bin"))
	third := run()
	if !third.Changed {
		t.Fatal(third)
	}
	keep := []string{first.ID, second.ID, third.ID}
	// A fourth orphan index can be removed while every retained closure survives.
	orphan := strings.Repeat("b", 40)
	if e = r.store.PutIndex(&entity.Index{ID: orphan, Files: []string{}, Created: 1}); e != nil {
		t.Fatal(e)
	}
	out, e := r.LocalRepoContext(context.Background(), LocalRepoRequest{Kind: "auto-purge"}, SnapshotOptions{}, func(next func(int) ([]*entity.Index, int, error)) ([]string, error) {
		rows, pages, e := next(1)
		if e != nil || pages != 1 || len(rows) != 4 {
			t.Fatal(len(rows), pages, e)
		}
		return keep, nil
	})
	if e != nil || !out.Applied || out.Indexes != 1 {
		t.Fatal(out, e)
	}
	for _, id := range keep {
		if _, e = r.store.GetIndex(id); e != nil {
			t.Fatal(e)
		}
	}
	if b, e := os.ReadFile(filepath.Join(r.DataPath, "source.docj")); e != nil || string(b) != "synthetic changed source" {
		t.Fatal("source modified", e)
	}
	recordSnapshotEvidence(t, "local-repo-format.json", map[string]any{"encryptedAssetRoundTrip": true, "fullLatestOriginalMsgpack": true, "unchangedIndexReused": true, "changedAndDeletedFilesIndexed": true, "retainedClosures": 3, "orphanIndexesPurged": out.Indexes, "sourceUnchanged": true})
}

func TestLocalRepoCancellationDoesNotPublishAndGateReopens(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	out, e := r.LocalRepoContext(ctx, LocalRepoRequest{Kind: "index", Memo: "cancel"}, SnapshotOptions{Event: func(_ context.Context, topic string, _ ...interface{}) {
		if topic == eventbus.EvtIndexUpsertFiles {
			cancel()
		}
	}}, nil)
	if !errors.Is(e, context.Canceled) || out.Applied {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(r.Path, "refs", "latest")); !os.IsNotExist(e) {
		t.Fatal("published canceled index", e)
	}
	out, e = r.LocalRepoContext(context.Background(), LocalRepoRequest{Kind: "index", Memo: "retry"}, SnapshotOptions{}, nil)
	if e != nil || !out.Applied {
		t.Fatal(out, e)
	}
	skip, e := r.LocalRepoContext(context.Background(), LocalRepoRequest{Kind: "auto-purge"}, SnapshotOptions{}, func(func(int) ([]*entity.Index, int, error)) ([]string, error) { return []string{out.ID}, nil })
	if e != nil || !skip.Skipped || skip.Applied {
		t.Fatal(skip, e)
	}
	recordSnapshotEvidence(t, "local-repo-cancel.json", map[string]any{"cancelDidNotPublishLatest": true, "retrySucceeded": true, "lessThanThreeSkipsPurge": true})
}
