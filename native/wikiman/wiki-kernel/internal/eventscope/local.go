package eventscope

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// LocalBus has no process relay or detached worker. Both the bus and each
// subscription can reject admission/remove references before joining callbacks.
type LocalBus[T any] struct {
	mu      sync.Mutex
	closed  bool
	next    uint64
	subs    map[uint64]*Subscription[T]
	retired map[uint64]*Subscription[T]
	closing []*Subscription[T]
}

type Subscription[T any] struct {
	bus    *LocalBus[T]
	id     uint64
	fn     func(T)
	active int
	closed bool
	done   chan struct{}
	stop   func() bool
}

func (b *LocalBus[T]) Subscribe(fn func(T)) (*Subscription[T], error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	if fn == nil {
		return nil, errors.New("invalid_event_handler")
	}
	if b.subs == nil {
		b.subs = map[uint64]*Subscription[T]{}
	}
	b.next++
	s := &Subscription[T]{bus: b, id: b.next, fn: fn, done: make(chan struct{})}
	b.subs[s.id] = s
	return s, nil
}

// SubscribeContext removes the callback when ctx is cancelled and joins any
// callback that entered before cancellation. The AfterFunc runs Close outside
// the publishing callback, so callback teardown cannot deadlock itself.
func (b *LocalBus[T]) SubscribeContext(ctx context.Context, fn func(T)) (*Subscription[T], error) {
	if ctx == nil {
		return nil, errors.New("invalid_event_context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, err := b.Subscribe(fn)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = s.Close(context.Background()) })
	b.mu.Lock()
	if s.closed {
		b.mu.Unlock()
		stop()
	} else {
		s.stop = stop
		b.mu.Unlock()
	}
	if err = ctx.Err(); err != nil {
		_ = s.Close(context.Background())
		return nil, err
	}
	return s, nil
}

func (b *LocalBus[T]) Publish(value T) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	ids := make([]uint64, 0, len(b.subs))
	for id := range b.subs {
		ids = append(ids, id)
	}
	b.mu.Unlock()
	slices.Sort(ids)
	for _, id := range ids {
		b.mu.Lock()
		s := b.subs[id]
		var fn func(T)
		if s != nil && !s.closed && !b.closed {
			s.active++
			fn = s.fn
		}
		b.mu.Unlock()
		if fn == nil {
			continue
		}
		func() {
			defer func() {
				b.mu.Lock()
				s.active--
				if s.closed && s.active == 0 {
					delete(b.retired, s.id)
					close(s.done)
				}
				b.mu.Unlock()
			}()
			fn(value)
		}()
	}
	return nil
}

// Close must be called outside this subscription's callback. A deadline leaves
// the real callback pinned; a later Close can retry the join.
func (s *Subscription[T]) Close(ctx context.Context) error {
	b := s.bus
	b.mu.Lock()
	stop := s.stop
	s.stop = nil
	if !s.closed {
		s.closed = true
		s.fn = nil
		delete(b.subs, s.id)
		if s.active == 0 {
			close(s.done)
		} else {
			if b.retired == nil {
				b.retired = map[uint64]*Subscription[T]{}
			}
			b.retired[s.id] = s
		}
	}
	b.mu.Unlock()
	if stop != nil {
		stop()
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *LocalBus[T]) Close(ctx context.Context) error {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		for _, s := range b.subs {
			b.closing = append(b.closing, s)
		}
		for _, s := range b.retired {
			b.closing = append(b.closing, s)
		}
	}
	subs := append([]*Subscription[T](nil), b.closing...)
	b.mu.Unlock()
	for _, s := range subs {
		if err := s.Close(ctx); err != nil {
			return err
		}
	}
	b.mu.Lock()
	b.closing = nil
	b.mu.Unlock()
	return nil
}
