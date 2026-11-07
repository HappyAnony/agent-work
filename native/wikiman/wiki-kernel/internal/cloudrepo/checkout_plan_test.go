package cloudrepo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func checkoutFixturePlan() CheckoutPlan {
	return CheckoutPlan{ID: strings.Repeat("a", 40), Bytes: 3, Files: []CheckoutFile{{ID: strings.Repeat("b", 40), Path: "doc.docj", SHA256: strings.Repeat("c", 64), Size: 3, Chunks: []string{strings.Repeat("d", 40)}}}}
}
func checkoutFixtureOptions() Options {
	return Options{Load: func(context.Context) (conf.AppData, error) {
		r := conf.NewRepo()
		r.Key = make([]byte, 32)
		return conf.AppData{Repo: r}, nil
	}, Execute: func(context.Context, conf.AppData, Request) (Result, error) { return Result{}, nil }}
}
func TestPrepareCheckoutCapabilityValidationAndDetachedResult(t *testing.T) {
	o := checkoutFixtureOptions()
	no, _ := New(o)
	defer no.Close(context.Background())
	id := strings.Repeat("a", 40)
	if _, e := no.PrepareCheckout(context.Background(), id); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	plan := checkoutFixturePlan()
	o.PrepareCheckout = func(_ context.Context, _ conf.AppData, _ string) (CheckoutPlan, error) { return plan, nil }
	s, _ := New(o)
	defer s.Close(context.Background())
	got, e := s.PrepareCheckout(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	got.Files[0].Chunks[0] = strings.Repeat("e", 40)
	if plan.Files[0].Chunks[0] != strings.Repeat("d", 40) {
		t.Fatal("result aliased port data")
	}
	for _, p := range []string{"../escape", "/absolute", "a/../b", "a\\b", "C:/drive"} {
		plan.Files[0].Path = p
		if _, e = s.PrepareCheckout(context.Background(), id); !errors.Is(e, ErrOperation) {
			t.Fatal("bad path accepted", p, e)
		}
	}
	plan = checkoutFixturePlan()
	plan.Files[0].Chunks = nil
	if _, e = s.PrepareCheckout(context.Background(), id); !errors.Is(e, ErrOperation) {
		t.Fatal("nonempty file without chunks accepted", e)
	}
	plan = checkoutFixturePlan()
	plan.Files = append(plan.Files, plan.Files[0])
	plan.Bytes = 6
	if _, e = s.PrepareCheckout(context.Background(), id); !errors.Is(e, ErrOperation) {
		t.Fatal("duplicates accepted", e)
	}
	if _, e = s.PrepareCheckout(context.Background(), "../id"); !errors.Is(e, ErrSnapshotID) {
		t.Fatal(e)
	}
}
func TestPrepareCheckoutCloseWaitsActualPortAndDiscardsCanceledPlan(t *testing.T) {
	o := checkoutFixtureOptions()
	entered, release := make(chan struct{}), make(chan struct{})
	o.PrepareCheckout = func(context.Context, conf.AppData, string) (CheckoutPlan, error) {
		close(entered)
		<-release
		return checkoutFixturePlan(), nil
	}
	s, _ := New(o)
	done := make(chan error, 1)
	go func() {
		p, e := s.PrepareCheckout(context.Background(), strings.Repeat("a", 40))
		if p.ID != "" {
			t.Error("canceled plan leaked")
		}
		done <- e
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	select {
	case <-done:
		t.Fatal("port returned before cleanup")
	default:
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
