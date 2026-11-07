//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

func TestNativeRepoKeyDrainRetainsActualOutcomeAndSourceLease(t *testing.T) {
	for _, kind := range []string{"random", "reset"} {
		t.Run(kind, func(t *testing.T) {
			root := fixture(t)
			before := digest(t, root)
			entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
				return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
					return conf.AppData{Repo: conf.NewRepo(), Sync: conf.NewSync()}, nil
				}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
					return cloudrepo.Result{}, nil
				}, RepoKey: func(ctx context.Context, _ conf.AppData, _ cloudrepo.RepoKeyRequest, _ []byte) (cloudrepo.RepoKeyResult, error) {
					close(entered)
					<-ctx.Done()
					close(canceled)
					<-release
					return cloudrepo.RepoKeyResult{Applied: true, CleanupPending: true}, ctx.Err()
				}}, nil
			}})
			l := getDocLease(t, m, pageA)
			type result struct {
				out cloudrepo.RepoKeyResult
				e   error
			}
			done := make(chan result, 1)
			go func() {
				v, e := l.NativeRepoKey(context.Background(), cloudrepo.RepoKeyRequest{Kind: kind})
				done <- result{v, e}
			}()
			<-entered
			lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
			ok, e := lock.TryLock()
			if e != nil {
				t.Fatal(e)
			}
			if ok {
				lock.Unlock()
			}
			if ok != (kind == "reset") {
				t.Fatal("incorrect source admission", kind, ok)
			}
			if e = l.WithNativeIndexes(context.Background(), func(*indexdb.Bundle) error { return nil }); e != nil {
				t.Fatal(e)
			}
			l.Release()
			drain := make(chan error, 1)
			go func() { drain <- m.Drain(context.Background()) }()
			<-canceled
			select {
			case <-done:
				t.Fatal("completed before port cleanup")
			default:
			}
			close(release)
			v := <-done
			if !v.out.Applied || !v.out.CleanupPending || !errors.Is(v.e, context.Canceled) || (kind == "random" && len(v.out.Key) != 32) {
				t.Fatal("lost partial result")
			}
			if e = <-drain; e != nil {
				t.Fatal(e)
			}
			if digest(t, root) != before || m.Stats().Tasks != 0 {
				t.Fatal("source or task leak")
			}
			recordCloudBusinessEvidence(t, "repokey-native-"+kind+".json", map[string]any{"sourceReadLeaseForInitialization": kind == "random", "resetDoesNotLockSource": kind == "reset", "resourceGateAvailable": true, "actualCleanupJoined": true, "appliedResultKept": true, "sourceUnchanged": true, "tasks": m.Stats().Tasks})
		})
	}
}

func TestNativeRepoKeyRequiresSeparateCapability(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			t.Fatal("metadata granted key change")
			return cloudrepo.Result{}, nil
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	defer l.Release()
	if _, e := l.NativeRepoKey(context.Background(), cloudrepo.RepoKeyRequest{Kind: "reset"}); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
}

func TestNativeRepoKeyTwoOwnersKeepPrivateConfiguration(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	calls := 0
	m := open(t, root, Options{CloudRepoOptions: func(_, page string) (cloudrepo.Options, error) {
		return cloudrepo.Options{
			Load: func(context.Context) (conf.AppData, error) {
				v := conf.AppData{Repo: conf.NewRepo(), Sync: conf.NewSync()}
				v.Sync.CloudName = page
				return v, nil
			},
			Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
				return cloudrepo.Result{}, nil
			},
			RepoKey: func(_ context.Context, v conf.AppData, _ cloudrepo.RepoKeyRequest, _ []byte) (cloudrepo.RepoKeyResult, error) {
				if v.Sync.CloudName != page {
					t.Error("foreign owner configuration")
				}
				calls++
				return cloudrepo.RepoKeyResult{Applied: true}, nil
			},
		}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		out, e := l.NativeRepoKey(context.Background(), cloudrepo.RepoKeyRequest{Kind: "reset"})
		if e != nil || !out.Applied {
			t.Fatal("owner reset failed", e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls != 2 || digest(t, root) != before {
		t.Fatal("owner isolation or source failure")
	}
	recordCloudBusinessEvidence(t, "repokey-native-owners.json", map[string]any{"owners": 2, "privateConfigurationBound": true, "sourceUnchanged": true})
}
