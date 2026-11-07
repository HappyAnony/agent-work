// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package cloudrepo owns cloud repository metadata and separately authorized
// directory operations. Provider IO and private persistence are explicit ports.
package cloudrepo

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/repoview"
)

var ErrDependency = errors.New("cloud_repo_unbound")
var ErrClosed = errors.New("cloud_repo_closed")
var ErrOperation = errors.New("cloud_repo_invalid_operation")
var ErrProvider = errors.New("cloud_repo_provider_failed")

type Request struct {
	Kind string
	Page int
}
type Result struct {
	Data json.RawMessage
	// Original membership failures show a notification and return an empty list.
	Notice int
}
type Options struct {
	NotebookIndex     func(context.Context, conf.AppData, NotebookIndexRequest) error
	IncrementalIndex  func(context.Context, conf.AppData, IncrementalIndexRequest) error
	RefreshReferences func(context.Context, conf.AppData, string) error
	FullSync          func(context.Context, conf.AppData, FullSyncRequest) error
	Repair            func(context.Context, conf.AppData) error
	Load              func(context.Context) (conf.AppData, error)
	Execute           func(context.Context, conf.AppData, Request) (Result, error)
	Directory         func(context.Context, conf.AppData, DirectoryRequest) (DirectoryResult, error)
	Snapshot          func(context.Context, conf.AppData, SnapshotRequest) (SnapshotResult, error)
	ReadRepository    func(context.Context, conf.AppData, repoview.Request) (repoview.Result, error)
	LocalRepo         func(context.Context, conf.AppData, LocalRepoRequest, RetentionPolicy) (LocalRepoResult, error)
	RepoKey           func(context.Context, conf.AppData, RepoKeyRequest, []byte) (RepoKeyResult, error)
	PrepareCheckout   func(context.Context, conf.AppData, string) (CheckoutPlan, error)
	Restore           func(context.Context, conf.AppData, string) (RestoreResult, error)
	Now               func() time.Time
	Intn              func(int) int
}
type Service struct {
	o             Options
	gate          chan struct{}
	life          context.Context
	cancel        context.CancelFunc
	repairDone    bool
	repoAfterSync bool
	lastRepoPurge time.Time
}

func New(o Options) (*Service, error) {
	if o.Load == nil || o.Execute == nil {
		return nil, ErrDependency
	}
	c, cancel := context.WithCancel(context.Background())
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Intn == nil {
		rng := rand.New(rand.NewSource(o.Now().UnixNano()))
		o.Intn = rng.Intn
	}
	return &Service{o: o, gate: make(chan struct{}, 1), life: c, cancel: cancel}, nil
}
func (s *Service) Query(ctx context.Context, r Request) (Result, error) {
	if s.life.Err() != nil {
		return Result{}, ErrClosed
	}
	switch r.Kind {
	case "space", "repos", "logs", "tags":
	default:
		return Result{}, ErrOperation
	}
	if r.Page > 1000000 {
		return Result{}, ErrOperation
	}
	if r.Page < 1 {
		r.Page = 1
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return Result{}, c.Err()
	}
	if s.life.Err() != nil {
		return Result{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return Result{}, e
	}
	cfg, e := s.o.Load(c)
	if e != nil {
		return Result{}, e
	}
	cfg, e = cfg.Clone()
	if e != nil {
		return Result{}, e
	}
	if e = c.Err(); e != nil {
		return Result{}, e
	}
	out, e := s.o.Execute(c, cfg, r)
	if c.Err() != nil {
		return Result{}, c.Err()
	}
	if e != nil {
		return Result{}, e
	}
	if e = c.Err(); e != nil {
		return Result{}, e
	}
	if !json.Valid(out.Data) {
		return Result{}, ErrOperation
	}
	out.Data = append(json.RawMessage(nil), out.Data...)
	return out, nil
}
func (s *Service) Close(ctx context.Context) error {
	s.cancel()
	select {
	case s.gate <- struct{}{}:
		<-s.gate
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
