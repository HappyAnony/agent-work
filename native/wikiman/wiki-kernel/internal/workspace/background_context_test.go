package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDrainStoreCancelsOnlyOwnedBackgroundTasks(t *testing.T) {
	ctx := context.Background()
	rootA, rootB := fixture(t), fixture(t)
	otherID := "41ad10f5-79ba-4b6c-ae51-7d640b82ecb1"
	file := filepath.Join(rootB, "store.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, bytes.ReplaceAll(raw, []byte(storeID), []byte(otherID)), 0600); err != nil {
		t.Fatal(err)
	}
	m := open(t, rootA, Options{})
	if _, err = m.OpenStore(ctx, rootB); err != nil {
		t.Fatal(err)
	}
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Acquire(ctx, otherID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	first, err := a.StartTask(ctx, func(taskCtx context.Context, l *Lease) error {
		if l.StoreID() != storeID || l.PageID() != pageA {
			t.Error("wrong task identity")
		}
		close(started)
		<-taskCtx.Done()
		close(cancelled)
		<-release
		return taskCtx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	ticks := make(chan struct{}, 8)
	otherContext := make(chan context.Context, 1)
	second, err := b.StartPeriodicTask(ctx, time.Millisecond, func(taskCtx context.Context, l *Lease) error {
		select {
		case otherContext <- taskCtx:
		default:
		}
		if l.StoreID() != otherID || l.PageID() != pageA {
			return ErrIdentity
		}
		_, err := l.ReadDocument(taskCtx, boxID, docID)
		if err == nil {
			select {
			case ticks <- struct{}{}:
			default:
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var otherTaskContext context.Context
	select {
	case otherTaskContext = <-otherContext:
	case <-time.After(time.Second):
		t.Fatal("other task never started")
	}
	a.Release()
	b.Release()
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err = m.DrainStore(deadline, storeID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("store drain did not cancel its task")
	}
	if m.Stats().Tasks != 2 {
		t.Fatal("cancel released an active owner", m.Stats())
	}
	if otherTaskContext.Err() != nil {
		t.Fatal("store drain cancelled another store's context")
	}
	select {
	case <-second.Done():
		t.Fatal("other store task cancelled")
	case <-ticks:
	case <-time.After(time.Second):
		t.Fatal("other store blocked")
	}
	close(release)
	if err = first.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = m.DrainStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	second.Cancel()
	if err = second.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
