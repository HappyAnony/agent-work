//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

func TestNativeCloudRepoOwnerConfigurationAndNoSourceMutation(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		return conf.AppData{Sync: &conf.Sync{CloudName: page}, User: &conf.User{UserToken: "synthetic-" + page}}, nil
	}, CloudRepoOptions: func(_, page string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
			if v.Sync.CloudName != page || v.User.UserToken != "synthetic-"+page {
				t.Error("foreign owner settings")
			}
			v.Sync.CloudName = "detached"
			return cloudrepo.Result{Data: json.RawMessage(`{"snapshots":[]}`)}, ctx.Err()
		}}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b, a} {
		if _, e := l.NativeCloudRepoQuery(context.Background(), cloudrepo.Request{Kind: "logs"}); e != nil {
			t.Fatal(e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source or task ownership changed")
	}
	recordCloudBusinessEvidence(t, "cloud-repo-native-isolation.json", map[string]any{"owners": 2, "queries": 3, "sourceUnchanged": true, "tasks": m.Stats().Tasks})
}
func TestNativeCloudRepoMissingBindingAndDrainActualIO(t *testing.T) {
	unbound := open(t, fixture(t), Options{})
	u := getDocLease(t, unbound, pageA)
	if _, e := u.NativeCloudRepoQuery(context.Background(), cloudrepo.Request{Kind: "space"}); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
	u.Release()
	failed := open(t, fixture(t), Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) { return cloudrepo.Options{}, nil }})
	if l, e := failed.Acquire(context.Background(), storeID, pageA); !errors.Is(e, cloudrepo.ErrDependency) || l != nil || failed.Stats().Workspaces != 0 {
		t.Fatal(l, e)
	}
	root := fixture(t)
	before := digest(t, root)
	entered, exited := make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(ctx context.Context, _ conf.AppData, _ cloudrepo.Request) (cloudrepo.Result, error) {
			close(entered)
			<-ctx.Done()
			close(exited)
			return cloudrepo.Result{}, ctx.Err()
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	done := make(chan error, 1)
	go func() {
		_, e := l.NativeCloudRepoQuery(context.Background(), cloudrepo.Request{Kind: "space"})
		done <- e
	}()
	<-entered
	// A network request must not retain the resource admission gate.
	if e := l.WithNativeIndexes(context.Background(), func(*indexdb.Bundle) error { return nil }); e != nil {
		t.Fatal(e)
	}
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("drain returned before actual IO")
	}
	if e := <-done; e == nil {
		t.Fatal("missing cancellation")
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source mutated or tasks leaked")
	}
	recordCloudBusinessEvidence(t, "cloud-repo-native-drain.json", map[string]any{"actualIOJoined": true, "resourceGateAvailable": true, "sourceUnchanged": true, "tasks": m.Stats().Tasks, "missingCapabilityRejected": true, "failedOwnerNotPublished": true})
}
