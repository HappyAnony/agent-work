//go:build fts5

package model

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The migrated callback belongs to AppConf, not the old process task queue.
func TestBlockAttributeLegacyOwnerJoinsAndRejectsAfterClose(t *testing.T) {
	c, _, _, _, _ := conflictFixture(t)
	e := legacySourceEventEnvironment()
	entered, release := make(chan struct{}), make(chan struct{})
	if err := e.launch(func(context.Context) error { close(entered); <-release; return nil }); err != nil {
		t.Fatal(err)
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := c.CloseCloudRepo(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Error("did not join callback", err)
	}
	close(release)
	if err := c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.launch(func(context.Context) error { t.Error("launched on closed owner"); return nil }); err == nil {
		t.Fatal("closed owner accepted")
	}
}
