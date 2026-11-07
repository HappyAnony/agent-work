//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/repoview"
)

func TestNativeRepositoryReadSameObjectIDUsesCapturedOwner(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{CloudRepoOptions: func(_, page string) (cloudrepo.Options, error) {
		return cloudrepo.Options{Load: func(context.Context) (conf.AppData, error) {
			return conf.AppData{Sync: &conf.Sync{CloudName: page}}, nil
		}, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
			return cloudrepo.Result{}, nil
		}, ReadRepository: func(ctx context.Context, v conf.AppData, q repoview.Request) (repoview.Result, error) {
			if v.Sync.CloudName != page {
				t.Error("foreign settings")
			}
			return repoview.Run(ctx, q, repoview.Ports{File: func(context.Context, string) (repoview.File, error) {
				return repoview.File{ID: q.ID, Path: "owner.txt", Data: []byte(page)}, nil
			}})
		}}, nil
	}})
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		out, e := l.NativeReadRepository(context.Background(), repoview.Request{Kind: "file", ID: strings.Repeat("a", 40)})
		if e != nil || string(out.Data) != page {
			t.Fatal(page, out, e)
		}
		out.Data[0] = 'x'
		again, e := l.NativeReadRepository(context.Background(), repoview.Request{Kind: "file", ID: strings.Repeat("a", 40)})
		if e != nil || string(again.Data) != page {
			t.Fatal(again, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("repository read changed source")
	}
}

func TestNativeRepositoryReadDrainWaitsForActualBackend(t *testing.T) {
	root := fixture(t)
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudRepoOptions: func(string, string) (cloudrepo.Options, error) {
		return cloudrepo.Options{
			Load: func(context.Context) (conf.AppData, error) { return conf.AppData{}, nil }, Execute: func(context.Context, conf.AppData, cloudrepo.Request) (cloudrepo.Result, error) {
				return cloudrepo.Result{}, nil
			}, ReadRepository: func(ctx context.Context, _ conf.AppData, _ repoview.Request) (repoview.Result, error) {
				close(entered)
				<-ctx.Done()
				close(canceled)
				<-release
				return repoview.Result{Data: []byte("partial")}, ctx.Err()
			},
		}, nil
	}})
	l := getDocLease(t, m, pageA)
	done := make(chan error, 1)
	go func() {
		out, e := l.NativeReadRepository(context.Background(), repoview.Request{Kind: "file", ID: strings.Repeat("a", 40)})
		if out.Data != nil {
			t.Error("partial result escaped")
		}
		done <- e
	}()
	<-entered
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	drained := make(chan error, 1)
	go func() { drained <- m.Drain(ctx) }()
	<-canceled
	select {
	case e := <-done:
		t.Fatal("backend detached", e)
	default:
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := <-drained; e != nil {
		t.Fatal(e)
	}
	if m.Stats().Tasks != 0 {
		t.Fatal("task leak")
	}
}
