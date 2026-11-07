package job

import (
	"context"
	"testing"
	"time"

	"wiki-kernel/internal/taskgroup"
)

func TestLegacyCronCannotReplaceUndrainedOwner(t *testing.T) {
	cronOwner = taskgroup.Open()
	first := cronOwner
	entered, release := make(chan struct{}), make(chan struct{})
	cronOwner.Go(func(context.Context) { close(entered); <-release })
	<-entered
	StartCron() // must not actually start any original database/provider callback
	if cronOwner != first {
		t.Fatal("duplicate scheduler")
	}
	StopCron()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := DrainCronContext(ctx); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	StartCron()
	if cronOwner != first {
		t.Fatal("undrained owner replaced")
	}
	close(release)
	if err := DrainCronContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	StartCron()
	if cronOwner == nil || cronOwner == first {
		t.Fatal("drained scheduler did not restart with a new owner")
	}
	StopCron()
	if err := DrainCronContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	cronOwner = nil
}
