// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package cloudlock preserves the lock-sync wire protocol with owned renewal.
package cloudlock

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var ErrBusy = errors.New("cloud repo is locked")
var ErrAcquire = errors.New("lock cloud repo failed")
var ErrLost = errors.New("cloud lock ownership lost")
var ErrUnlock = errors.New("cloud repo unlock failed")

type Record struct {
	Owner string `json:"deviceID"`
	Time  int64  `json:"time"`
}
type Options struct {
	Owner                             string
	Read                              func(context.Context) ([]byte, error) // nil bytes means absent, never an IO error
	Write                             func(context.Context, []byte) error
	Remove                            func(context.Context) error
	Event                             func(context.Context, bool) // true: acquiring; false: releasing
	Now                               func() time.Time
	RenewInterval, RetryInterval, TTL time.Duration // trusted Go test/host capabilities
	RenewTimeout                      time.Duration
}
type Lease struct {
	o        Options
	ctx      context.Context
	cancel   context.CancelCauseFunc
	done     chan struct{}
	closeMu  sync.Mutex
	closed   bool
	closeErr error
}

func defaults(o Options) Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.RenewInterval <= 0 {
		o.RenewInterval = 30 * time.Second
	}
	if o.RetryInterval <= 0 {
		o.RetryInterval = 5 * time.Second
	}
	if o.TTL <= 0 {
		o.TTL = 65 * time.Second
	}
	if o.RenewTimeout <= 0 {
		o.RenewTimeout = 20 * time.Second
	}
	return o
}
func record(data []byte) (Record, bool) {
	var r Record
	e := json.Unmarshal(data, &r)
	return r, e == nil && r.Owner != "" && r.Time > 0
}
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (o Options) write(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	b, _ := json.Marshal(Record{o.Owner, o.Now().UnixMilli()})
	return o.Write(ctx, b)
}
func (o Options) release(ctx context.Context) error {
	data, e := o.Read(ctx)
	if e != nil {
		return errors.Join(ErrUnlock, e)
	}
	if len(data) == 0 {
		return nil
	}
	r, ok := record(data)
	if !ok || r.Owner != o.Owner {
		return ErrLost
	}
	if o.Event != nil {
		o.Event(ctx, false)
	}
	var err error
	for i := 0; i < 3; i++ {
		if e = ctx.Err(); e != nil {
			return errors.Join(ErrUnlock, e)
		}
		err = o.Remove(ctx)
		if err == nil {
			return nil
		}
	}
	return errors.Join(ErrUnlock, err)
}
func Acquire(ctx context.Context, o Options) (*Lease, error) {
	o = defaults(o)
	if o.Owner == "" || o.Read == nil || o.Write == nil || o.Remove == nil {
		return nil, ErrAcquire
	}
	for i := 0; i < 3; i++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if o.Event != nil {
			o.Event(ctx, true)
		}
		data, e := o.Read(ctx)
		if e != nil {
			return nil, e
		}
		r, valid := record(data)
		if valid && r.Owner != o.Owner && !o.Now().After(time.UnixMilli(r.Time).Add(o.TTL)) {
			if e = wait(ctx, o.RetryInterval); e != nil {
				return nil, e
			}
			continue
		}
		if len(data) > 0 && !valid {
			if e = o.Remove(ctx); e != nil {
				return nil, e
			}
		}
		// A lost write response can follow a successful remote write. Always inspect
		// and release *our* token on this error path; never delete a foreign token.
		if e = o.write(ctx); e != nil || ctx.Err() != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			ce := o.release(cleanup)
			cancel()
			return nil, errors.Join(ErrAcquire, e, ctx.Err(), ce)
		}
		c, cancel := context.WithCancelCause(ctx)
		l := &Lease{o: o, ctx: c, cancel: cancel, done: make(chan struct{})}
		go l.renew()
		return l, nil
	}
	return nil, ErrBusy
}
func (l *Lease) Context() context.Context { return l.ctx }
func (l *Lease) renew() {
	defer close(l.done)
	defer func() {
		if recover() != nil {
			l.cancel(ErrLost)
		}
	}()
	ticker := time.NewTicker(l.o.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(l.ctx, l.o.RenewTimeout)
			data, e := l.o.Read(ctx)
			r, ok := record(data)
			if e != nil || !ok || r.Owner != l.o.Owner {
				cancel()
				l.cancel(ErrLost)
				return
			}
			if e = l.o.write(ctx); e != nil {
				cancel()
				l.cancel(ErrLost)
				return
			}
			cancel()
		}
	}
}

// Stop joins actual renewal IO, even if a faulty provider ignores cancellation.
// Its owning operation remains admitted until this join completes.
func (l *Lease) Stop() { l.cancel(context.Canceled); <-l.done }
func (l *Lease) Close(ctx context.Context) error {
	l.Stop()
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed {
		return l.closeErr
	}
	e := l.o.release(ctx)
	if e == nil || errors.Is(e, ErrLost) {
		l.closed = true
		l.closeErr = e
	}
	return e
}
func WithLock(ctx context.Context, o Options, fn func(context.Context) error) (err error) {
	if fn == nil {
		return ErrAcquire
	}
	l, e := Acquire(ctx, o)
	if e != nil {
		return e
	}
	defer func() {
		l.Stop()
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = errors.Join(err, l.Close(cleanup))
	}()
	e = fn(l.Context())
	if e == nil && l.ctx.Err() != nil {
		e = context.Cause(l.ctx)
	}
	return e
}
