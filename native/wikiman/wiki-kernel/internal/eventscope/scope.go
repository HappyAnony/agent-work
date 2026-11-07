// Package eventscope owns callback lifetimes without importing the legacy model.
package eventscope

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
)

var ErrClosed = errors.New("event_scope_closed")

// Bridge installs at most one inert ingress relay per topic into an external
// bus that cannot unsubscribe. Relays retain the bridge and topic only, NEVER a
// scope, model callback or workspace. New contextual buses need no such bridge.
type Bridge struct {
	mu       sync.Mutex
	register func(string, interface{}) error
	routes   map[string]*route
	next     uint64
}

type route struct {
	typ      reflect.Type
	handlers map[uint64]handler
}
type handler struct {
	scope *Scope
	fn    reflect.Value
}
type Scope struct {
	bridge *Bridge
	closed bool
	active int
	done   chan struct{}
}

func NewBridge(register func(string, interface{}) error) *Bridge {
	return &Bridge{register: register, routes: make(map[string]*route)}
}
func (b *Bridge) Open() *Scope { return &Scope{bridge: b, done: make(chan struct{})} }

// Subscribe preserves the original synchronous, zero-return callback signatures.
func (s *Scope) Subscribe(topic string, fn interface{}) error {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() || v.Type().NumOut() != 0 || v.Type().IsVariadic() {
		return errors.New("invalid_event_handler")
	}
	b := s.bridge
	b.mu.Lock()
	defer b.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	r := b.routes[topic]
	if r == nil {
		r = &route{typ: v.Type(), handlers: make(map[uint64]handler)}
		// External registration must only register, never synchronously publish.
		relay := reflect.MakeFunc(v.Type(), func(args []reflect.Value) []reflect.Value {
			b.dispatch(topic, args)
			return nil
		})
		if b.register == nil {
			return errors.New("missing_event_ingress")
		}
		if err := b.register(topic, relay.Interface()); err != nil {
			return err
		}
		b.routes[topic] = r
	} else if r.typ != v.Type() {
		return errors.New("event_signature_mismatch")
	}
	b.next++
	r.handlers[b.next] = handler{s, v}
	return nil
}

func (b *Bridge) dispatch(topic string, args []reflect.Value) {
	b.mu.Lock()
	r := b.routes[topic]
	var ids []uint64
	if r != nil {
		for id := range r.handlers {
			ids = append(ids, id)
		}
	}
	b.mu.Unlock()
	slices.Sort(ids)
	for _, id := range ids {
		b.mu.Lock()
		h, ok := r.handlers[id]
		if ok {
			h.scope.active++
		}
		b.mu.Unlock()
		if !ok {
			continue
		}
		func() {
			defer func() {
				b.mu.Lock()
				h.scope.active--
				if h.scope.closed && h.scope.active == 0 {
					close(h.scope.done)
				}
				b.mu.Unlock()
			}()
			h.fn.Call(args)
		}()
	}
}

// Close removes callback references before joining in-flight calls. Cancellation
// is retryable and never means the callback has stopped. Call outside callbacks.
func (s *Scope) Close(ctx context.Context) error {
	b := s.bridge
	b.mu.Lock()
	if !s.closed {
		s.closed = true
		for _, r := range b.routes {
			for id, h := range r.handlers {
				if h.scope == s {
					delete(r.handlers, id)
				}
			}
		}
		if s.active == 0 {
			close(s.done)
		}
	}
	b.mu.Unlock()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
