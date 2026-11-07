// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
	"wiki-kernel/conf"
)

type FullSyncRequest struct {
	Mode   string
	ByHand bool
}
type FullSyncResult struct{ Started, CancellationDeferred bool }

// FullSync admits an explicitly supplied compatibility capability, serialized
// with restore/key/metadata operations. The original sync mixes source mutation
// and network IO: after admission cancellation is deferred until the complete
// merge/reindex returns. Close must join it; a deadline is never a fake join.
func (s *Service) FullSync(ctx context.Context, r FullSyncRequest) (out FullSyncResult, err error) {
	switch r.Mode {
	case "normal", "upload", "download", "boot", "exit", "boot-followup":
	default:
		return out, ErrOperation
	}
	if s.o.FullSync == nil {
		return out, ErrDependency
	}
	err = s.maintenance(ctx, func(c context.Context, v conf.AppData) error {
		out.Started = true
		e := s.o.FullSync(context.WithoutCancel(c), v, r)
		out.CancellationDeferred = c.Err() != nil
		return e
	})
	return
}

// Repair's once marker belongs to this owner, and only a successful fully
// drained run consumes it. Failed/canceled runs can retry.
func (s *Service) Repair(ctx context.Context) error {
	if s.o.Repair == nil {
		return ErrDependency
	}
	return s.maintenance(ctx, func(c context.Context, v conf.AppData) error {
		if s.repairDone {
			return nil
		}
		if e := s.o.Repair(c, v); e != nil {
			return e
		}
		if e := c.Err(); e != nil {
			return e
		}
		s.repairDone = true
		return nil
	})
}
func (s *Service) maintenance(ctx context.Context, fn func(context.Context, conf.AppData) error) error {
	if s.life.Err() != nil {
		return ErrClosed
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return c.Err()
	}
	if s.life.Err() != nil {
		return ErrClosed
	}
	if e := c.Err(); e != nil {
		return e
	}
	v, e := s.o.Load(c)
	if e != nil {
		return e
	}
	v, e = v.Clone()
	if e != nil {
		return e
	}
	if e := c.Err(); e != nil {
		return e
	}
	e = fn(c, v)
	return errors.Join(e, c.Err())
}

// Reference refresh is a separately bound compatibility derived-index operation.
// Holding the same owner gate lets switch/exit join its actual synchronous work.
func (s *Service) RefreshReferences(ctx context.Context, id string) error {
	if s.o.RefreshReferences == nil {
		return ErrDependency
	}
	if id == "" {
		return ErrOperation
	}
	return s.maintenance(ctx, func(c context.Context, v conf.AppData) error { return s.o.RefreshReferences(c, v, id) })
}
