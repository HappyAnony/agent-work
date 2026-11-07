//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

func nativeCheckoutPlan(id string) cloudrepo.CheckoutPlan {
	return cloudrepo.CheckoutPlan{ID: id, Bytes: 3, Files: []cloudrepo.CheckoutFile{{ID: strings.Repeat("b", 40), Path: "doc.docj", Size: 3, SHA256: strings.Repeat("c", 64), Chunks: []string{strings.Repeat("d", 40)}}}}
}
func TestNativePrepareCheckoutTwoOwnersReadOnlyAndSeparateCapability(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	calls := 0
	m := open(t, root, Options{CloudRepoOptions: func(_, page string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
			r := conf.NewRepo()
			r.Key = make([]byte, 32)
			return conf.AppData{Repo: r, Sync: &conf.Sync{CloudName: page}}, nil
		}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, nil
		}, PrepareCheckout: func(ctx context.Context, v conf.AppData, id string) (cloudrepo.CheckoutPlan, error) {
			if v.Sync.CloudName != page {
				t.Error("foreign owner")
			}
			calls++
			return nativeCheckoutPlan(id), ctx.Err()
		}}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		p, e := l.NativePrepareCheckout(context.Background(), strings.Repeat("a", 40))
		if e != nil || p.Bytes != 3 {
			t.Fatal(p, e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls != 2 || digest(t, root) != before {
		t.Fatal("owner or source leak")
	}
	no := open(t, fixture(t), Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			t.Fatal("metadata granted restore candidate")
			return cloudrepo.Result{}, nil
		}}, nil
	}})
	l := getDocLease(t, no, pageA)
	defer l.Release()
	if _, e := l.NativePrepareCheckout(context.Background(), strings.Repeat("a", 40)); !errors.Is(e, cloudrepo.ErrDependency) {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "checkout-plan-native-owners.json", map[string]any{"owners": 2, "separateCapabilityRequired": true, "sourceUnchanged": true, "noSourceApplyCapabilityAdded": true})
}

func TestNativePrepareCheckoutDrainJoinsAndDoesNotLockSource(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
			r := conf.NewRepo()
			r.Key = make([]byte, 32)
			return conf.AppData{Repo: r}, nil
		}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, nil
		}, PrepareCheckout: func(ctx context.Context, _ conf.AppData, id string) (cloudrepo.CheckoutPlan, error) {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
			return nativeCheckoutPlan(id), nil
		}}, nil
	}})
	l := getDocLease(t, m, pageA)
	done := make(chan error, 1)
	go func() {
		p, e := l.NativePrepareCheckout(context.Background(), strings.Repeat("a", 40))
		if p.ID != "" {
			t.Error("canceled candidate escaped")
		}
		done <- e
	}()
	<-entered
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		t.Fatal("source lock held during cache read", e)
	}
	lock.Unlock()
	if e = l.WithNativeIndexes(context.Background(), func(*indexdb.Bundle) error { return nil }); e != nil {
		t.Fatal(e)
	}
	l.Release()
	drain := make(chan error, 1)
	go func() { drain <- m.Drain(context.Background()) }()
	<-canceled
	select {
	case <-done:
		t.Fatal("early join")
	default:
	}
	close(release)
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = <-drain; e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source or task leak")
	}
	recordCloudBusinessEvidence(t, "checkout-plan-native-drain.json", map[string]any{"actualPortJoined": true, "canceledCandidateDiscarded": true, "sourceAndResourceLocksAvailable": true, "sourceUnchanged": true, "tasks": m.Stats().Tasks})
}
