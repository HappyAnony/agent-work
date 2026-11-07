// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package announcement owns original anonymous announcements through explicit
// device ledger, region, clock, GET transport and notification capabilities.
package announcement

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"
)

var ErrDependency = errors.New("announcement_dependencies_unbound")
var ErrResponse = errors.New("announcement_invalid_response")
var ErrLedger = errors.New("announcement_invalid_ledger")
var ErrNetwork = errors.New("announcement_connection_failed")
var ErrClosed = errors.New("announcement_closed")
var ErrRegion = errors.New("announcement_region_changed")

type Item struct {
	Id     string `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Region int    `json:"region"`
}
type Response struct {
	Status int
	Body   map[string]any
}

// Update invokes merge under its device-ledger admission. It may invoke it more
// than once; merge is pure and must not send notifications or do network IO.
type Store interface {
	Update(context.Context, func([]byte, bool) ([]byte, bool, error)) error
}
type Options struct {
	Region   func(context.Context) (int, error)
	Fetch    func(context.Context, int) (Response, error)
	Store    Store
	Notify   func(context.Context, Item) error
	Now      func() time.Time
	Wait     func(context.Context, time.Duration) error
	CacheTTL time.Duration
}
type Service struct {
	o           Options
	gate        chan struct{}
	life        context.Context
	cancel      context.CancelFunc
	cache       []Item
	cached      bool
	cacheRegion int
	cacheAt     time.Time
}

func New(o Options) (*Service, error) {
	if o.Region == nil || o.Fetch == nil || o.Store == nil || o.Notify == nil || o.Now == nil || o.CacheTTL <= 0 {
		return nil, ErrDependency
	}
	if o.Wait == nil {
		o.Wait = Wait
	}
	c, cancel := context.WithCancel(context.Background())
	return &Service{o: o, gate: make(chan struct{}, 1), life: c, cancel: cancel}, nil
}
func Wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return ctx.Err()
	}
}
func (s *Service) enter(ctx context.Context) (context.Context, func(), error) {
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	cleanup := func() { stop(); cancel() }
	select {
	case s.gate <- struct{}{}:
		if s.life.Err() != nil {
			<-s.gate
			cleanup()
			return nil, nil, ErrClosed
		}
		if e := c.Err(); e != nil {
			<-s.gate
			cleanup()
			return nil, nil, e
		}
		return c, func() { <-s.gate; cleanup() }, nil
	case <-c.Done():
		cleanup()
		return nil, nil, c.Err()
	}
}
func (s *Service) Close(ctx context.Context) error {
	s.cancel()
	select {
	case s.gate <- struct{}{}:
		s.cache = nil
		s.cached = false
		<-s.gate
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func decodeFeed(r Response) ([]Item, error) {
	if r.Status != 200 {
		return nil, ErrNetwork
	}
	v, present := r.Body["announcement"]
	if !present || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok || len(items) > 10000 {
		return nil, ErrResponse
	}
	out := make([]Item, 0, len(items))
	for _, raw := range items {
		v, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrResponse
		}
		id, a := v["id"].(string)
		title, b := v["title"].(string)
		url, c := v["url"].(string)
		region, d := v["region"].(float64)
		if !a || id == "" || !b || !c || !d || math.IsNaN(region) || math.IsInf(region, 0) || region != math.Trunc(region) || region < -2147483648 || region > 2147483647 {
			return nil, ErrResponse
		}
		out = append(out, Item{id, title, url, int(region)})
	}
	return out, nil
}
func decodeLedger(raw []byte, exists bool) ([]Item, error) {
	if !exists {
		return nil, nil
	}
	var out []*Item
	if len(raw) > 16<<20 || json.Unmarshal(raw, &out) != nil || len(out) > 10000 {
		return nil, ErrLedger
	}
	items := make([]Item, 0, len(out))
	for _, v := range out {
		if v == nil || v.Id == "" {
			return nil, ErrLedger
		}
		items = append(items, *v)
	}
	return items, nil
}
func (s *Service) fetch(ctx context.Context, region int) ([]Item, error) {
	now := s.o.Now()
	if s.cached && s.cacheRegion == region && !now.Before(s.cacheAt) && now.Sub(s.cacheAt) <= s.o.CacheTTL {
		return append([]Item(nil), s.cache...), nil
	}
	s.cached = false
	s.cache = nil
	r, e := s.o.Fetch(ctx, region)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e != nil {
		return nil, ErrNetwork
	}
	items, e := decodeFeed(r)
	if e != nil {
		return nil, e
	}
	s.cache = append([]Item(nil), items...)
	s.cached = len(r.Body) > 0
	s.cacheRegion = region
	s.cacheAt = now
	return items, nil
}

// Refresh persists all new IDs (including other regions) before notifying this
// region, preserving original at-most-once ordering. Cancellation after commit
// may suppress display; it never rolls back the acknowledged device marker.
func (s *Service) Refresh(ctx context.Context) error { return s.Run(ctx, 0) }
func (s *Service) List(ctx context.Context) ([]Item, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	region, e := s.o.Region(ctx)
	if e != nil {
		return nil, e
	}
	out, e := s.fetch(ctx, region)
	if e != nil {
		return nil, e
	}
	current, e := s.o.Region(ctx)
	if e != nil {
		return nil, e
	}
	if current != region {
		return nil, ErrRegion
	}
	return append([]Item(nil), out...), ctx.Err()
}
func (s *Service) Run(ctx context.Context, delay time.Duration) error {
	if delay < 0 {
		return ErrDependency
	}
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	if delay > 0 {
		if e = s.o.Wait(ctx, delay); e != nil {
			return e
		}
	}
	region, e := s.o.Region(ctx)
	if e != nil {
		return e
	}
	items, e := s.fetch(ctx, region)
	if e != nil {
		return e
	}
	var pending []Item
	e = s.o.Store.Update(ctx, func(raw []byte, exists bool) ([]byte, bool, error) {
		pending = nil
		if e := ctx.Err(); e != nil {
			return nil, false, e
		}
		current, e := s.o.Region(ctx)
		if e != nil {
			return nil, false, e
		}
		if current != region {
			return nil, false, ErrRegion
		}
		old, e := decodeLedger(raw, exists)
		if e != nil {
			return nil, false, e
		}
		seen := map[string]bool{}
		for _, v := range old {
			seen[v.Id] = true
		}
		changed := false
		for _, v := range items {
			if seen[v.Id] {
				continue
			}
			seen[v.Id] = true
			old = append(old, v)
			changed = true
			if v.Region == region {
				pending = append(pending, v)
			}
		}
		if !changed {
			return nil, false, nil
		}
		if len(old) > 10000 {
			return nil, false, ErrLedger
		}
		data, e := json.Marshal(old)
		if len(data) > 16<<20 {
			return nil, false, ErrLedger
		}
		return data, true, e
	})
	if e != nil {
		return e
	}
	for _, v := range pending {
		if e := ctx.Err(); e != nil {
			return e
		}
		current, e := s.o.Region(ctx)
		if e != nil {
			return e
		}
		if current != region {
			return ErrRegion
		}
		if e = s.o.Notify(ctx, v); e != nil {
			return e
		}
	}
	return ctx.Err()
}
