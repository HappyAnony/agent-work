package cloudlock

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type memory struct {
	mu              sync.Mutex
	b               []byte
	writes, removes int
}

func (m *memory) options(owner string) Options {
	return Options{Owner: owner, RenewInterval: time.Millisecond, RetryInterval: time.Millisecond, Read: func(ctx context.Context) ([]byte, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		return append([]byte(nil), m.b...), ctx.Err()
	}, Write: func(ctx context.Context, b []byte) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if e := ctx.Err(); e != nil {
			return e
		}
		m.b = append([]byte(nil), b...)
		m.writes++
		return nil
	}, Remove: func(ctx context.Context) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if e := ctx.Err(); e != nil {
			return e
		}
		m.b = nil
		m.removes++
		return nil
	}}
}
func TestRenewalCloseActuallyJoinsAndIndependentOwners(t *testing.T) {
	a, b := &memory{}, &memory{}
	oa, ob := a.options("a"), b.options("b")
	entered, release := make(chan struct{}), make(chan struct{})
	write := oa.Write
	oa.Write = func(ctx context.Context, data []byte) error {
		a.mu.Lock()
		n := a.writes
		a.mu.Unlock()
		if n == 1 {
			close(entered)
			<-release
		}
		return write(ctx, data)
	}
	la, e := Acquire(context.Background(), oa)
	if e != nil {
		t.Fatal(e)
	}
	lb, e := Acquire(context.Background(), ob)
	if e != nil {
		t.Fatal(e)
	}
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- la.Close(context.Background()) }()
	select {
	case <-closed:
		t.Fatal("returned before actual renewal IO")
	case <-time.After(10 * time.Millisecond):
	}
	b.mu.Lock()
	before := b.writes
	b.mu.Unlock()
	close(release)
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(time.Second)
	for {
		b.mu.Lock()
		n := b.writes
		b.mu.Unlock()
		if n > before {
			break
		}
		if time.Now().After(until) {
			t.Fatal("closing a stopped b")
		}
		time.Sleep(time.Millisecond)
	}
	if e = lb.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.removes != 1 || len(a.b) != 0 {
		t.Fatal(a.removes, string(a.b))
	}
}
func TestContentionCancellationExpiryRepairAndReadFailure(t *testing.T) {
	m := &memory{}
	o := m.options("ours")
	m.b, _ = json.Marshal(Record{"foreign", time.Now().UnixMilli()})
	o.RetryInterval = time.Hour
	c, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, e := Acquire(c, o); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if m.writes != 0 || m.removes != 0 {
		t.Fatal("foreign lock changed")
	}
	m.b, _ = json.Marshal(Record{"expired", time.Now().Add(-66 * time.Second).UnixMilli()})
	l, e := Acquire(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if e = l.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	m.b = []byte(`{"deviceID":123}`)
	l, e = Acquire(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if e = l.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	o.Read = func(context.Context) ([]byte, error) { return nil, errors.New("network unavailable") }
	n := m.removes
	if _, e = Acquire(context.Background(), o); e == nil || m.removes != n {
		t.Fatal("IO error treated as corrupt lock", e)
	}
}
func TestLostOwnershipAndAmbiguousAcquisitionCleanup(t *testing.T) {
	m := &memory{}
	o := m.options("ours")
	l, e := Acquire(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	m.mu.Lock()
	m.b, _ = json.Marshal(Record{"new-owner", time.Now().UnixMilli()})
	m.mu.Unlock()
	select {
	case <-l.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("renewal did not cancel on lost lock")
	}
	if e = l.Close(context.Background()); !errors.Is(e, ErrLost) {
		t.Fatal(e)
	}
	if m.removes != 0 {
		t.Fatal("foreign owner removed")
	}
	m = &memory{}
	o = m.options("ours")
	ctx, cancel := context.WithCancel(context.Background())
	write := o.Write
	o.Write = func(c context.Context, b []byte) error { e := write(c, b); cancel(); return e }
	if _, e = Acquire(ctx, o); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if len(m.b) != 0 || m.removes != 1 {
		t.Fatal("accepted write leaked after cancellation")
	}
}
func TestUnlockFailureIsReportedWithoutRenewalLeak(t *testing.T) {
	m := &memory{}
	o := m.options("ours")
	o.Remove = func(context.Context) error { return errors.New("offline") }
	e := WithLock(context.Background(), o, func(context.Context) error { return nil })
	if !errors.Is(e, ErrUnlock) {
		t.Fatal(e)
	}
	m.mu.Lock()
	n := m.writes
	m.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writes != n {
		t.Fatal("renewal after failed unlock")
	}
}
