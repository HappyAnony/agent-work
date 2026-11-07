package util

import (
	"context"
	"errors"
	"testing"
)

func TestHookUILoadedContextRejectsCancellation(t *testing.T) {
	wasLoaded := IsUILoaded.Load()
	IsUILoaded.Store(false)
	t.Cleanup(func() { IsUILoaded.Store(wasLoaded) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := HookUILoadedContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("HookUILoadedContext cancellation = %v", err)
	}
}
