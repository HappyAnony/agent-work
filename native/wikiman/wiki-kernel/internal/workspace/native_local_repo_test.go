//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

func TestNativeLocalRepoSourceReadLeaseAndActualDrain(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
			v := conf.AppData{Repo: conf.NewRepo()}
			v.Repo.Key = make([]byte, 32)
			return v, nil
		}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, nil
		}, LocalRepo: func(ctx context.Context, _ conf.AppData, _ cloudrepo.LocalRepoRequest, _ cloudrepo.RetentionPolicy) (cloudrepo.LocalRepoResult, error) {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
			return cloudrepo.LocalRepoResult{Applied: true, Uncertain: true}, ctx.Err()
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	type answer struct {
		v cloudrepo.LocalRepoResult
		e error
	}
	done := make(chan answer, 1)
	go func() {
		v, e := l.NativeLocalRepo(context.Background(), cloudrepo.LocalRepoRequest{Kind: "index", Memo: "fixture"})
		done <- answer{v, e}
	}()
	<-entered
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || ok {
		if ok {
			lock.Unlock()
		}
		t.Fatal("source read lease absent", ok, e)
	}
	if e = l.WithNativeIndexes(context.Background(), func(*indexdb.Bundle) error { return nil }); e != nil {
		t.Fatal(e)
	}
	l.Release()
	drained := make(chan error, 1)
	go func() { drained <- m.Drain(context.Background()) }()
	<-canceled
	select {
	case <-done:
		t.Fatal("returned before cleanup")
	default:
	}
	close(release)
	out := <-done
	if !out.v.Applied || !out.v.Uncertain || !errors.Is(out.e, context.Canceled) {
		t.Fatal(out)
	}
	if e = <-drained; e != nil {
		t.Fatal(e)
	}
	ok, e = lock.TryLock()
	if e != nil || !ok {
		t.Fatal("source lock leaked", e)
	}
	lock.Unlock()
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source or task leak")
	}
	recordCloudBusinessEvidence(t, "local-repo-native.json", map[string]any{"sourceReadLeaseHeldDuringIndex": true, "resourceGateAvailable": true, "drainCanceledAndJoined": true, "partialOutcomeKept": true, "sourceUnchanged": true, "tasks": m.Stats().Tasks})
}

func TestNativeLocalRepoSeparateCapability(t *testing.T) {
	m := open(t, fixture(t), Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			t.Fatal("metadata granted index")
			return cloudrepo.Result{}, nil
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	defer l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := l.NativeLocalRepo(ctx, cloudrepo.LocalRepoRequest{Kind: "index", Memo: "x"}); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
}
