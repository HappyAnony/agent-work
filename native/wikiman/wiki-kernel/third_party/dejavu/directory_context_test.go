package dejavu

import (
	"context"
	"errors"
	"github.com/siyuan-note/dejavu/cloud"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyCloudRenewalOwnersAndCancellableRepositoryAdmission(t *testing.T) {
	makeRepo := func(parent context.Context, name string) *Repo {
		t.Helper()
		root := t.TempDir()
		cfg := &cloud.Conf{Dir: name, Local: &cloud.ConfLocal{Endpoint: filepath.Join(root, "remote")}, RequestContext: parent}
		r, e := NewRepo(root, filepath.Join(root, "repo"), filepath.Join(root, "history"), filepath.Join(root, "tmp"), name, name, "test", make([]byte, 32), nil, cloud.NewLocal(&cloud.BaseCloud{Conf: cfg}))
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	ctx, cancel := context.WithCancel(context.Background())
	a, b := makeRepo(ctx, "a"), makeRepo(context.Background(), "b")
	defer a.CloseMetadata()
	defer b.CloseMetadata()
	if e := a.tryLockCloud("a", nil); e != nil {
		t.Fatal(e)
	}
	if e := b.tryLockCloud("b", nil); e != nil {
		t.Fatal(e)
	}
	cancel()
	if a.cloud.GetConf().Context().Err() == nil {
		t.Fatal("locked provider lost owner Context")
	}
	a.unlockCloud(nil)
	if a.lockLease != nil || b.lockLease == nil || b.lockLease.Context().Err() != nil {
		t.Fatal("cross-repo stop")
	}
	b.unlockCloud(nil)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		WithRepositoryContext(context.Background(), func() error { close(entered); <-release; return nil })
	}()
	<-entered
	c, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if e := WithRepositoryContext(c, func() error { t.Error("canceled admission executed"); return nil }); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	close(release)
	<-done
}
