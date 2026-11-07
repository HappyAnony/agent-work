package cloudrepo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/repoview"
)

func TestRepositoryReadCloseWaitsForActualBackendAndDiscardsPartial(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	service, e := New(Options{Load: func(context.Context) (conf.AppData, error) { return conf.AppData{}, nil }, Execute: func(context.Context, conf.AppData, Request) (Result, error) { return Result{}, nil }, ReadRepository: func(ctx context.Context, _ conf.AppData, _ repoview.Request) (repoview.Result, error) {
		close(entered)
		<-ctx.Done()
		<-release
		return repoview.Result{Data: []byte("partial")}, ctx.Err()
	}})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		out, e := service.ReadRepository(context.Background(), repoview.Request{Kind: "file", ID: strings.Repeat("a", 40)})
		if out.Data != nil {
			t.Error("partial backend result escaped")
		}
		done <- e
	}()
	<-entered
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e = service.Close(short); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	select {
	case e := <-done:
		t.Fatal("backend was detached", e)
	default:
	}
	close(release)
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = service.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
