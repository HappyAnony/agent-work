//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

func TestNativeSnapshotSeparateCapabilityAndContextOwners(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	count := 0
	m := open(t, root, Options{CloudRepoOptions: func(_, page string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
			v := conf.AppData{Sync: &conf.Sync{CloudName: page}, Repo: conf.NewRepo()}
			v.Repo.Key = make([]byte, 32)
			return v, nil
		}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{Data: json.RawMessage(`{}`)}, nil
		}, Snapshot: func(ctx context.Context, v conf.AppData, r cloudrepo.SnapshotRequest) (cloudrepo.SnapshotResult, error) {
			if v.Sync.CloudName != page {
				t.Error("foreign context configuration")
			}
			count++
			return cloudrepo.SnapshotResult{Applied: true}, ctx.Err()
		}}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		out, e := l.NativeSnapshot(context.Background(), cloudrepo.SnapshotRequest{Kind: "tag", ID: strings.Repeat("a", 40), Tag: "backup"})
		if e != nil || !out.Applied {
			t.Fatal(out, e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if count != 2 || digest(t, root) != before {
		t.Fatal("context or source changed")
	}
	read := open(t, fixture(t), Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			t.Fatal("metadata used to mutate snapshot")
			return cloudrepo.Result{}, nil
		}}, nil
	}})
	l := getDocLease(t, read, pageA)
	if _, e := l.NativeSnapshot(context.Background(), cloudrepo.SnapshotRequest{Kind: "purge-cloud"}); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
	l.Release()
	recordCloudBusinessEvidence(t, "snapshot-native-owners.json", map[string]any{"owners": 2, "separateSnapshotCapabilityRequired": true, "sourceUnchanged": true})
}
func TestNativeSnapshotDrainWaitsActualCleanupAndKeepsPartialOutcome(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
			return conf.AppData{Sync: conf.NewSync(), Repo: conf.NewRepo()}, nil
		}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, cloudrepo.ErrDependency
		}, Snapshot: func(ctx context.Context, _ conf.AppData, _ cloudrepo.SnapshotRequest) (cloudrepo.SnapshotResult, error) {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
			return cloudrepo.SnapshotResult{Applied: true, Uncertain: true, Objects: 1}, ctx.Err()
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	type answer struct {
		out cloudrepo.SnapshotResult
		e   error
	}
	done := make(chan answer, 1)
	go func() {
		out, e := l.NativeSnapshot(context.Background(), cloudrepo.SnapshotRequest{Kind: "purge-cloud"})
		done <- answer{out, e}
	}()
	<-entered
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		t.Fatal(e)
	}
	lock.Unlock()
	if e = l.WithNativeIndexes(context.Background(), func(*indexdb.Bundle) error { return nil }); e != nil {
		t.Fatal(e)
	}
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	drained := make(chan error, 1)
	go func() { drained <- m.Drain(ctx) }()
	<-canceled
	select {
	case <-done:
		t.Fatal("returned before actual cleanup")
	default:
	}
	close(release)
	v := <-done
	if !v.out.Applied || !v.out.Uncertain || v.out.Objects != 1 || !errors.Is(v.e, context.Canceled) {
		t.Fatal(v)
	}
	if e = <-drained; e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source or task leak")
	}
	recordCloudBusinessEvidence(t, "snapshot-native-drain.json", map[string]any{"partialOutcomeKept": true, "actualCleanupJoined": true, "sourceLockAvailable": true, "resourceGateAvailable": true, "sourceUnchanged": true, "tasks": m.Stats().Tasks})
}
