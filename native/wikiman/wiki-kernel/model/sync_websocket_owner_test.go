// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type ownedSyncWebSocket struct {
	readErrors chan error
	closed     chan struct{}
	closeOnce  sync.Once
	closeCount atomic.Int32
}

func newOwnedSyncWebSocket() *ownedSyncWebSocket {
	return &ownedSyncWebSocket{readErrors: make(chan error, 1), closed: make(chan struct{})}
}

func (c *ownedSyncWebSocket) ReadJSON(interface{}) error {
	select {
	case err := <-c.readErrors:
		return err
	case <-c.closed:
		return errors.New("closed")
	}
}
func (*ownedSyncWebSocket) WriteJSON(interface{}) error             { return nil }
func (*ownedSyncWebSocket) SetCloseHandler(func(int, string) error) {}
func (c *ownedSyncWebSocket) Close() error {
	c.closeCount.Add(1)
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestSyncWebSocketReconnectAndCloseShareOneOwner(t *testing.T) {
	initial, replacement := newOwnedSyncWebSocket(), newOwnedSyncWebSocket()
	initial.readErrors <- errors.New("fixture disconnect")
	ctx, cancel := context.WithCancel(context.Background())
	done, dialed := make(chan struct{}), make(chan struct{})

	webSocketConnLock.Lock()
	webSocketConn, syncWebSocketCancel, syncWebSocketWorker = initial, cancel, done
	closedSyncWebSocket.Store(false)
	webSocketConnLock.Unlock()

	var waits atomic.Int32
	go runSyncWebSocket(ctx, initial, done, syncWebSocketPorts{
		wait: func(ctx context.Context, _ time.Duration) error {
			waits.Add(1)
			<-ctx.Done()
			return ctx.Err()
		},
		dial: func(context.Context) (syncWebSocketConnection, error) {
			close(dialed)
			return replacement, nil
		},
		user: func() bool { return true },
	})

	// The first wait must complete normally so the reconnect path is exercised.
	for waits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	// Replace the wait port after admission is intentionally impossible; use a
	// fresh owner below to verify reconnect publication deterministically.
	cancel()
	if err := closeSyncWebSocketContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-dialed:
		t.Fatal("canceled owner attempted reconnect")
	default:
	}
	if initial.closeCount.Load() == 0 {
		t.Fatal("owner cancellation did not close active connection")
	}

	initial, replacement = newOwnedSyncWebSocket(), newOwnedSyncWebSocket()
	initial.readErrors <- errors.New("fixture disconnect")
	ctx, cancel = context.WithCancel(context.Background())
	done, dialed = make(chan struct{}), make(chan struct{})
	webSocketConnLock.Lock()
	webSocketConn, syncWebSocketCancel, syncWebSocketWorker = initial, cancel, done
	closedSyncWebSocket.Store(false)
	webSocketConnLock.Unlock()
	go runSyncWebSocket(ctx, initial, done, syncWebSocketPorts{
		wait: func(context.Context, time.Duration) error { return nil },
		dial: func(context.Context) (syncWebSocketConnection, error) {
			close(dialed)
			return replacement, nil
		},
		user: func() bool { return true },
	})
	select {
	case <-dialed:
	case <-time.After(time.Second):
		t.Fatal("reconnect was not attempted")
	}
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := closeSyncWebSocketContext(deadline); err != nil {
		t.Fatal(err)
	}
	if initial.closeCount.Load() == 0 || replacement.closeCount.Load() == 0 {
		t.Fatalf("connections not closed: initial=%d replacement=%d", initial.closeCount.Load(), replacement.closeCount.Load())
	}
	if currentSyncWebSocket() != nil {
		t.Fatal("closed owner retained connection")
	}
}

type stubbornSyncWebSocket struct{ release chan struct{} }

func (c *stubbornSyncWebSocket) ReadJSON(interface{}) error {
	<-c.release
	return errors.New("released")
}
func (*stubbornSyncWebSocket) WriteJSON(interface{}) error             { return nil }
func (*stubbornSyncWebSocket) SetCloseHandler(func(int, string) error) {}
func (*stubbornSyncWebSocket) Close() error                            { return nil }

func TestSyncWebSocketCloseTimeoutRetainsJoinableOwner(t *testing.T) {
	conn := &stubbornSyncWebSocket{release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	webSocketConnLock.Lock()
	webSocketConn, syncWebSocketCancel, syncWebSocketWorker = conn, cancel, done
	closedSyncWebSocket.Store(false)
	webSocketConnLock.Unlock()
	go runSyncWebSocket(ctx, conn, done, syncWebSocketPorts{
		wait: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
		dial: func(context.Context) (syncWebSocketConnection, error) { return nil, errors.New("unexpected") },
		user: func() bool { return true },
	})

	deadline, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := closeSyncWebSocketContext(deadline); !errors.Is(err, context.DeadlineExceeded) {
		stop()
		t.Fatalf("expected close timeout, got %v", err)
	}
	stop()
	webSocketConnLock.Lock()
	retained := syncWebSocketWorker == done
	webSocketConnLock.Unlock()
	if !retained {
		t.Fatal("timed out close discarded active owner")
	}
	close(conn.release)
	if err := closeSyncWebSocketContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}
