package dejavu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplaceRepositoryCommitRollbackAndCancellation(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0700)
	os.WriteFile(filepath.Join(repo, "old"), []byte("private synthetic cache"), 0600)
	failed := errors.New("synthetic persistence failure")
	out, e := ReplaceRepositoryContext(context.Background(), repo, func() error { return failed })
	if !errors.Is(e, failed) || out.Applied || out.Uncertain {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(repo, "old")); e != nil {
		t.Fatal("rollback lost old cache", e)
	}
	out, e = ReplaceRepositoryContext(context.Background(), repo, func() error { os.WriteFile(filepath.Join(repo, "new"), []byte("new private cache"), 0600); return nil })
	if e != nil || !out.Applied || out.CleanupPending {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(repo, "old")); !os.IsNotExist(e) {
		t.Fatal("old cache survived successful replacement", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out, e = ReplaceRepositoryContext(ctx, repo, func() error { cancel(); return nil })
	if !errors.Is(e, context.Canceled) || !out.Applied || !out.CleanupPending {
		t.Fatal(out, e)
	}
	rows, _ := os.ReadDir(root)
	kept := 0
	for _, d := range rows {
		if d.Name() != "repo" {
			kept++
		}
	}
	if kept != 1 {
		t.Fatal("canceled cleanup not retained", kept)
	}
	if rows, _ := os.ReadDir(repo); len(rows) != 0 {
		t.Fatal("committed new cache not kept")
	}
	recordSnapshotEvidence(t, "repokey-replace.json", map[string]any{"saveFailureRestoredOldCache": true, "successfulReplaceRemovedOldCache": true, "postCommitCancellationKeptApplied": true, "cleanupQuarantineRetained": true})
}

func TestReplaceRepositoryGateCancellationAndSymlinkRefusal(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	outside := filepath.Join(root, "outside")
	os.Mkdir(outside, 0700)
	os.Symlink(outside, repo)
	if _, e := ReplaceRepositoryContext(context.Background(), repo, func() error { t.Fatal("symlink commit"); return nil }); e == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(repo)
	lock.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, e := ReplaceRepositoryContext(ctx, repo, func() error { t.Fatal("canceled queued mutation"); return nil })
	lock.Unlock()
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}
