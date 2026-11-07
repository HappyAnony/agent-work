package dejavu

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScopedIndexSameTimestampSameSizeKeepsBothImmutableVersions(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	file := filepath.Join(r.DataPath, "source.docj")
	stamp := time.Unix(1793186000, 123000000)
	write := func(v string) {
		t.Helper()
		if e := os.WriteFile(file, []byte(v), 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.Chtimes(file, stamp, stamp); e != nil {
			t.Fatal(e)
		}
	}
	index := func() string {
		v, e := r.LocalRepoContext(context.Background(), LocalRepoRequest{Kind: "index", Memo: "same-time"}, SnapshotOptions{}, nil)
		if e != nil || !v.Applied {
			t.Fatal(v, e)
		}
		return v.ID
	}
	write("old")
	a := index()
	write("new")
	b := index()
	if a == b {
		t.Fatal("same-size same-timestamp bytes were ignored")
	}
	read := func(id string) []byte {
		t.Helper()
		idx, e := r.store.GetIndex(id)
		if e != nil {
			t.Fatal(e)
		}
		f, e := r.store.GetFile(idx.Files[0])
		if e != nil {
			t.Fatal(e)
		}
		var body []byte
		for _, ch := range f.Chunks {
			c, e := r.store.GetChunk(ch)
			if e != nil {
				t.Fatal(e)
			}
			body = append(body, c.Data...)
		}
		return body
	}
	if !bytes.Equal(read(a), []byte("old")) || !bytes.Equal(read(b), []byte("new")) {
		t.Fatal("metadata identity aliased old snapshot")
	}
	if c := index(); c != b {
		t.Fatal("unchanged content did not reuse snapshot")
	}
	out, e := r.CheckoutOwned(context.Background(), a, nil)
	if e != nil || !out.Applied {
		t.Fatal(out, e)
	}
	body, _ := os.ReadFile(file)
	if string(body) != "old" {
		t.Fatal("restore skipped equal-timestamp changed bytes")
	}
	recordSnapshotEvidence(t, "restore-same-time.json", map[string]any{"sameTimestampAndSizeEditDetected": true, "oldAndNewSnapshotsRemainReadable": true, "unchangedSnapshotReused": true, "ownedRestoreRewritesAuthoritativeTargetBytes": true})
}

func TestLegacyIndexRetainsOpaqueIDsFromScopedSnapshot(t *testing.T) {
	r := snapshotFixture(t, t.TempDir())
	first, e := r.LocalRepoContext(context.Background(), LocalRepoRequest{Kind: "index", Memo: "scoped"}, SnapshotOptions{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(r.DataPath, "new.txt"), []byte("legacy addition"), 0600); e != nil {
		t.Fatal(e)
	}
	next, e := r.Index("legacy", true, nil)
	if e != nil || next.ID == first.ID || len(next.Files) != 2 {
		t.Fatal(next, e)
	}
	if e = r.validateSnapshotClosure(next); e != nil {
		t.Fatal("legacy index referenced missing opaque file metadata", e)
	}
	recordSnapshotEvidence(t, "restore-legacy-index-interop.json", map[string]any{"legacyIndexReadScopedFileIDs": true, "unchangedMetadataIDsPreserved": true, "completeReferenceClosureVerified": true})
}
