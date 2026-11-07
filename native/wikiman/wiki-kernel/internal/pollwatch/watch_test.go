package pollwatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/radovskyb/watcher"
)

func TestCloseWaitsForCallbackAndIsRetryable(t *testing.T) {
	folder := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	callbackCanceled := make(chan struct{})
	type identityKey struct{}
	identity := make(chan string, 1)
	parent := context.WithValue(context.Background(), identityKey{}, "workspace-a")
	w, err := Open(parent, folder, 5*time.Millisecond, func(ctx context.Context, _ watcher.Event) {
		identity <- ctx.Value(identityKey{}).(string)
		close(entered)
		<-ctx.Done()
		close(callbackCanceled)
		<-release
	}, func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "synthetic.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
	if got := <-identity; got != "workspace-a" {
		t.Fatalf("callback identity = %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := w.Close(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	select {
	case <-callbackCanceled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel active callback context")
	}
	close(release)
	deadline, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err := w.Close(deadline); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(deadline); err != nil {
		t.Fatal(err)
	}
}

func TestStopJoinsInternalPollersAndInterruptsLongInterval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	folder := t.TempDir()
	for i := 0; i < 100; i++ {
		w, err := Open(context.Background(), folder, time.Hour, func(context.Context, watcher.Event) {}, func(err error) { t.Error(err) })
		if err != nil {
			t.Fatal(err)
		}
		if err = w.Close(ctx); err != nil {
			t.Fatal("long polling sleep ignored Stop", err)
		}
	}
	stack := make([]byte, 1<<20)
	n := runtime.Stack(stack, true)
	if strings.Contains(string(stack[:n]), "wiki-kernel/internal/pollwatch/backend.(*Watcher).Start") {
		t.Fatal("backend or its poll helper survived joined close")
	}
}

func TestOriginalNonrecursiveEventSemantics(t *testing.T) {
	folder := t.TempDir()
	events := make(chan watcher.Event, 32)
	w, err := Open(context.Background(), folder, 2*time.Millisecond, func(_ context.Context, e watcher.Event) { events <- e }, func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close(context.Background())
	file := filepath.Join(folder, "synthetic.txt")
	wait := func(op watcher.Op, name string) {
		t.Helper()
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		for {
			select {
			case e := <-events:
				if e.Op == op && e.Path == name {
					return
				}
			case <-timer.C:
				t.Fatalf("missing event %v", op)
			}
		}
	}
	if err = os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	wait(watcher.Create, file)
	if err = os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	wait(watcher.Write, file)
	renamed := filepath.Join(folder, "renamed.txt")
	if err = os.Rename(file, renamed); err != nil {
		t.Fatal(err)
	}
	wait(watcher.Rename, renamed)
	if err = os.Remove(renamed); err != nil {
		t.Fatal(err)
	}
	wait(watcher.Remove, renamed)
}

func TestOpenFailureAndImmediateClose(t *testing.T) {
	if _, err := Open(nil, t.TempDir(), time.Second, func(context.Context, watcher.Event) {}, func(error) {}); err == nil {
		t.Fatal("nil context accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(canceled, t.TempDir(), time.Second, func(context.Context, watcher.Event) {}, func(error) {}); err != context.Canceled {
		t.Fatalf("canceled context error = %v", err)
	}
	if _, err := Open(context.Background(), "relative", time.Second, func(context.Context, watcher.Event) {}, func(error) {}); err == nil {
		t.Fatal("relative accepted")
	}
	if _, err := Open(context.Background(), filepath.Join(t.TempDir(), "missing"), time.Second, func(context.Context, watcher.Event) {}, func(error) {}); err == nil {
		t.Fatal("missing accepted")
	}
	w, err := Open(context.Background(), t.TempDir(), time.Millisecond, func(context.Context, watcher.Event) { t.Error("late callback") }, func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
