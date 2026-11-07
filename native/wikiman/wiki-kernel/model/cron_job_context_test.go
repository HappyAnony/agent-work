//go:build fts5

package model

import (
	"context"
	"errors"
	"testing"
	"time"
	"wiki-kernel/av"
)

func TestCapturedPeriodicJobNeverRecapturesReplacementAndJoins(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	started, release := make(chan struct{}), make(chan struct{})
	calls := 0
	job, e := CaptureWorkspaceJob(context.Background(), func(ctx context.Context) error { calls++; close(started); <-release; return ctx.Err() })
	if e != nil {
		t.Fatal(e)
	}
	old := Conf
	Conf = NewAppConf()
	if e = job(context.Background()); e == nil || calls != 0 {
		t.Fatal("job recaptured replacement", e, calls)
	}
	Conf = old
	done := make(chan error, 1)
	go func() { done <- job(context.Background()) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if e = c.r.owner.CloseCloudRepo(ctx); !errors.Is(e, context.DeadlineExceeded) {
		close(release)
		t.Fatal("periodic worker detached", e)
	}
	close(release)
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = c.r.owner.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
}
