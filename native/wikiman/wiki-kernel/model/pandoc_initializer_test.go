package model

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"wiki-kernel/util"
)

func TestPandocInitializerStartsOnceWithCapturedConfig(t *testing.T) {
	started := make(chan util.PandocInitConfig, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	owner := newPandocInitializer(func(ctx context.Context, config util.PandocInitConfig) (util.PandocInitResult, error) {
		calls.Add(1)
		started <- config
		select {
		case <-ctx.Done():
			return util.PandocInitResult{}, ctx.Err()
		case <-release:
			return util.PandocInitResult{BinPath: "first", Notify: true}, nil
		}
	})

	first := util.PandocInitConfig{ConfDir: "first"}
	if err := owner.Start(first, nil); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(util.PandocInitConfig{ConfDir: "second"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := <-started; got.ConfDir != first.ConfDir {
		t.Fatalf("initializer used mutable config: %#v", got)
	}
	close(release)
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("initializer calls = %d, want 1", got)
	}
}

func TestPandocInitializerCloseCancelsAndJoins(t *testing.T) {
	returned := make(chan struct{})
	applied := make(chan struct{}, 1)
	owner := newPandocInitializer(func(ctx context.Context, _ util.PandocInitConfig) (util.PandocInitResult, error) {
		<-ctx.Done()
		close(returned)
		return util.PandocInitResult{Notify: true}, nil
	})
	if err := owner.Start(util.PandocInitConfig{}, func(util.PandocInitResult) { applied <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	default:
		t.Fatal("close returned before initializer joined")
	}
	select {
	case <-applied:
		t.Fatal("canceled initializer applied a stale result")
	default:
	}
	if err := owner.Start(util.PandocInitConfig{}, nil); !errors.Is(err, errPandocInitializerClosed) {
		t.Fatalf("restart error = %v", err)
	}
}
