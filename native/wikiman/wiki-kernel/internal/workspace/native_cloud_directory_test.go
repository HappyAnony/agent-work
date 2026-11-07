//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

func TestNativeCloudDirectoryExplicitDeviceOwnersAndReadOnlyCapability(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{CloudRepoOptions: func(_, page string) (cloudrepo.Options, error) {
		state := conf.AppData{Sync: &conf.Sync{CloudName: page}}
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) { return state, nil }, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{Data: json.RawMessage(`{}`)}, nil
		}, Directory: func(ctx context.Context, v conf.AppData, r cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error) {
			if v.Sync.CloudName != state.Sync.CloudName {
				t.Error("foreign device owner")
			}
			state.Sync.CloudName = r.Name
			return cloudrepo.DirectoryResult{Active: r.Name, Saved: true}, ctx.Err()
		}}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		out, e := l.NativeCloudDirectory(context.Background(), cloudrepo.DirectoryRequest{Kind: "set", Name: "selected"})
		if e != nil || !out.Saved || out.Active != "selected" {
			t.Fatal(out, e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if before != digest(t, root) {
		t.Fatal("private cloud setting wrote shared source")
	}
	read := open(t, fixture(t), Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			t.Fatal("metadata port used for mutation")
			return cloudrepo.Result{}, nil
		}}, nil
	}})
	l := getDocLease(t, read, pageA)
	if _, e := l.NativeCloudDirectory(context.Background(), cloudrepo.DirectoryRequest{Kind: "remove", Name: "main"}); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
	l.Release()
	recordCloudBusinessEvidence(t, "cloud-directory-native-owners.json", map[string]any{"deviceOwners": 2, "explicitWriteCapabilityRequired": true, "sourceUnchanged": true})
}
func TestNativeCloudDirectoryDrainJoinsCleanupAndPreservesAppliedOutcome(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) { return conf.AppData{Sync: conf.NewSync()}, nil }, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, cloudrepo.ErrDependency
		}, Directory: func(ctx context.Context, _ conf.AppData, _ cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error) {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
			return cloudrepo.DirectoryResult{Active: "main", Applied: true}, ctx.Err()
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	type answer struct {
		v cloudrepo.DirectoryResult
		e error
	}
	done := make(chan answer, 1)
	go func() {
		v, e := l.NativeCloudDirectory(context.Background(), cloudrepo.DirectoryRequest{Kind: "remove", Name: "main"})
		done <- answer{v, e}
	}()
	<-entered
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		t.Fatal("source admission held during network", e)
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
	select {
	case <-drained:
		t.Fatal("false drain join")
	default:
	}
	close(release)
	got := <-done
	if !got.v.Applied || got.v.Saved || !errors.Is(got.e, context.Canceled) {
		t.Fatal(got)
	}
	if e = <-drained; e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source or task lifetime changed")
	}
	recordCloudBusinessEvidence(t, "cloud-directory-native-drain.json", map[string]any{"sourceLockAvailable": true, "resourceGateAvailable": true, "actualCleanupJoined": true, "remoteAppliedPreserved": true, "tasks": m.Stats().Tasks, "sourceUnchanged": true})
}
