package sourcetx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadContextReusesOnlyLiveExactStoreAdmission(t *testing.T) {
	root := fixture(t)
	s, _ := open(t, root)
	other, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	var saved context.Context
	if err := s.ReadContext(context.Background(), func(ctx context.Context) error {
		saved = ctx
		admission := ctx.Value(readAdmissionKey{}).(*readAdmission)
		if err := s.ReadContext(ctx, func(child context.Context) error {
			if child.Value(readAdmissionKey{}) != admission {
				t.Fatal("nested read created another admission")
			}
			_, err := s.SourceFile(child, docPath)
			return err
		}); err != nil {
			return err
		}
		if err := other.Read(ctx, func() error { t.Fatal("cross-store callback ran"); return nil }); !errors.Is(err, ErrIdentity) {
			t.Fatalf("cross-store: %v", err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if err := s.Read(cancelled, func() error { t.Fatal("cancelled callback ran"); return nil }); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(saved, func() error { t.Fatal("expired callback ran"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("expired: %v", err)
	}
	// New requests must perform recovery admission again, never reuse a global
	// validated flag. This orphan simulates a newly interrupted publication.
	if err := os.WriteFile(filepath.Join(root, ".local/journal/orphan.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(context.Background(), func() error { t.Fatal("pending journal admitted"); return nil }); !errors.Is(err, ErrRecovery) {
		t.Fatalf("recovery: %v", err)
	}
}

func TestReadContextDrainsBorrowersBeforeUnlockAndRevokesLateReads(t *testing.T) {
	s, w := open(t, fixture(t))
	entered := make(chan context.Context, 1)
	leaveRoot := make(chan struct{})
	leaveChild := make(chan struct{})
	childDone := make(chan error, 1)
	rootDone := make(chan error, 1)
	t.Cleanup(func() {
		select {
		case <-leaveRoot:
		default:
			close(leaveRoot)
		}
		select {
		case <-leaveChild:
		default:
			close(leaveChild)
		}
	})
	go func() {
		rootDone <- s.ReadContext(context.Background(), func(ctx context.Context) error {
			go func() {
				childDone <- s.ReadContext(ctx, func(context.Context) error { entered <- ctx; <-leaveChild; return nil })
			}()
			<-leaveRoot
			return nil
		})
	}()
	var ctx context.Context
	select {
	case ctx = <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("child did not enter")
	}
	close(leaveRoot)
	a := ctx.Value(readAdmissionKey{}).(*readAdmission)
	deadline := time.After(2 * time.Second)
	for {
		a.mu.Lock()
		active := a.active
		a.mu.Unlock()
		if !active {
			break
		}
		select {
		case <-deadline:
			t.Fatal("root did not revoke admission")
		case <-time.After(time.Millisecond):
		}
	}
	if err := s.Read(ctx, func() error { t.Fatal("late borrower ran"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("late: %v", err)
	}
	select {
	case err := <-rootDone:
		t.Fatalf("unlocked before child finished: %v", err)
	default:
	}
	blocked, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	err := w.exclusive(blocked, false, func(*os.Root) error { t.Error("writer entered during child read"); return nil })
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("writer not blocked: %v", err)
	}
	close(leaveChild)
	if err := <-childDone; err != nil {
		t.Fatal(err)
	}
	if err := <-rootDone; err != nil {
		t.Fatal(err)
	}
	if err := w.exclusive(context.Background(), false, func(*os.Root) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestReadContextPanicRevokesAdmissionAndUnlocks(t *testing.T) {
	s, w := open(t, fixture(t))
	var saved context.Context
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing panic")
			}
		}()
		_ = s.ReadContext(context.Background(), func(ctx context.Context) error { saved = ctx; panic("test") })
	}()
	if err := s.Read(saved, func() error { t.Fatal("panic retained admission"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := w.exclusive(context.Background(), false, func(*os.Root) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestInspectionReportsPublishingSourceWithoutWaiting(t *testing.T) {
	ctx := context.Background()
	s, writer := open(t, fixture(t))
	store, err := s.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- writer.exclusive(ctx, true, func(*os.Root) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	status, err := s.Inspect(deadline, store.StoreID)
	if !errors.Is(err, ErrBusy) || status.SourceLock != "busy" || status.Recovery != "unknown" {
		t.Fatal(status, err)
	}
	if _, err := s.PreviewRecovery(deadline, store.StoreID); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
}
