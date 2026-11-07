package cloudrepo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestLocalRepoRetentionPaginationNewestTodayAndCancellation(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	cfg := *conf.NewRepo()
	cfg.IndexRetentionDays = 7
	cfg.RetentionIndexesDaily = 2
	rows := []RepoIndex{}
	for i := 0; i < 8; i++ {
		stamp := now.Add(-24 * time.Hour)
		if i < 3 {
			stamp = now
		}
		if i == 7 {
			stamp = now.Add(-8 * 24 * time.Hour)
		}
		rows = append(rows, RepoIndex{ID: fmt.Sprintf("%040x", i+1), Created: stamp.UnixMilli()})
	}
	pages := 0
	ids, e := Retention(context.Background(), cfg, now, func(page int) ([]RepoIndex, int, error) {
		pages++
		if page == 1 {
			return rows[:4], 2, nil
		}
		return rows[4:], 2, nil
	}, func(n int) int { return n - 1 })
	if e != nil || pages != 2 || len(ids) != 5 {
		t.Fatal(ids, pages, e)
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, i := range []int{0, 1, 2, 3, 5} {
		if !set[rows[i].ID] {
			t.Fatal("missing required retained index", i)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Retention(ctx, cfg, now, nil, nil); !errors.Is(e, ErrRetention) {
		t.Fatal(e)
	}
	if _, e = Retention(ctx, cfg, now, func(int) ([]RepoIndex, int, error) { t.Fatal("canceled read"); return nil, 0, nil }, func(int) int { return 0 }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	cfg.RetentionIndexesDaily = 0
	if _, e = Retention(context.Background(), cfg, now, func(int) ([]RepoIndex, int, error) { return nil, 0, nil }, func(int) int { return 0 }); !errors.Is(e, ErrRetention) {
		t.Fatal(e)
	}
}

func TestLocalRepoOwnerThrottleCapabilitiesAndTrueJoin(t *testing.T) {
	now := time.Now()
	calls := 0
	cfg := conf.AppData{Repo: conf.NewRepo()}
	cfg.Repo.Key = make([]byte, 32)
	options := func() Options {
		return Options{Now: func() time.Time { return now }, Load: func(context.Context) (conf.AppData, error) { return cfg, nil }, Execute: func(context.Context, conf.AppData, Request) (Result, error) { return Result{}, nil }, LocalRepo: func(_ context.Context, v conf.AppData, _ LocalRepoRequest, _ RetentionPolicy) (LocalRepoResult, error) {
			calls++
			v.Repo.Key[0] = 99
			return LocalRepoResult{Applied: true}, nil
		}}
	}
	a, _ := New(options())
	b, _ := New(options())
	defer a.Close(context.Background())
	defer b.Close(context.Background())
	run := func(s *Service, cron bool) LocalRepoResult {
		v, e := s.LocalRepo(context.Background(), LocalRepoRequest{Kind: "auto-purge", Cron: cron})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	if !run(a, true).Skipped || !run(a, false).Applied || !run(a, false).Skipped || !run(b, true).Skipped || !run(b, false).Applied {
		t.Fatal("owner throttle")
	}
	now = now.Add(6 * time.Hour)
	if !run(a, true).Applied || calls != 3 || cfg.Repo.Key[0] != 0 {
		t.Fatal("throttle or alias", calls)
	}
	if _, e := a.LocalRepo(context.Background(), LocalRepoRequest{Kind: "index", Memo: " \u200b "}); !errors.Is(e, ErrMemo) {
		t.Fatal(e)
	}
	cfg.Repo.Key = nil
	if _, e := a.LocalRepo(context.Background(), LocalRepoRequest{Kind: "index", Memo: ""}); !errors.Is(e, ErrKey) {
		t.Fatal("original missing-key precedence", e)
	}
	cfg.Repo.Key = make([]byte, 32)
	no := options()
	no.LocalRepo = nil
	s, _ := New(no)
	defer s.Close(context.Background())
	if _, e := s.LocalRepo(context.Background(), LocalRepoRequest{Kind: "index", Memo: "x"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	o := options()
	o.LocalRepo = func(ctx context.Context, _ conf.AppData, _ LocalRepoRequest, _ RetentionPolicy) (LocalRepoResult, error) {
		close(entered)
		<-release
		return LocalRepoResult{Applied: true, Uncertain: true}, ctx.Err()
	}
	owner, _ := New(o)
	done := make(chan error, 1)
	go func() {
		out, e := owner.LocalRepo(context.Background(), LocalRepoRequest{Kind: "index", Memo: "x"})
		if !out.Applied || !out.Uncertain {
			t.Error(out)
		}
		done <- e
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := owner.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := owner.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
