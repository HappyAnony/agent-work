// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
)

// Metadata IO retains creation identity without holding source/resource locks.

// Read-only restore candidate. No source read/write lock is needed to decrypt
// private cache; applying this manifest must go through future source CAS/journal
// and exclusive workspace mutation, never through the compatibility Checkout.
func (l *Lease) NativePrepareCheckout(ctx context.Context, id string) (cloudrepo.CheckoutPlan, error) {
	var s *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s = l.workspace.resources.cloudRepo
		if s == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return cloudrepo.CheckoutPlan{}, e
	}
	out := make(chan cloudrepo.CheckoutPlan, 1)
	task, e := l.StartTask(ctx, func(c context.Context, _ *Lease) error {
		v, e := s.PrepareCheckout(c, id)
		if e == nil {
			out <- v
		}
		return e
	})
	if e != nil {
		return cloudrepo.CheckoutPlan{}, e
	}
	if e = task.Wait(context.WithoutCancel(ctx)); e != nil {
		return cloudrepo.CheckoutPlan{}, e
	}
	return <-out, nil
}

// Repo key changes require their own port, not index or snapshot authority.
// Initialization includes a source scan; reset only replaces private cache.
func (l *Lease) NativeRepoKey(ctx context.Context, r cloudrepo.RepoKeyRequest) (cloudrepo.RepoKeyResult, error) {
	var s *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s = l.workspace.resources.cloudRepo
		if s == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return cloudrepo.RepoKeyResult{}, e
	}
	out := make(chan cloudrepo.RepoKeyResult, 1)
	task, e := l.StartTask(ctx, func(c context.Context, pin *Lease) error {
		var v cloudrepo.RepoKeyResult
		run := func() error { var e error; v, e = s.RepoKey(c, r); return e }
		var e error
		if r.Kind == "reset" {
			e = run()
		} else {
			e = pin.sourceRead(c, run)
		}
		out <- v
		return e
	})
	if e != nil {
		return cloudrepo.RepoKeyResult{}, e
	}
	e = task.Wait(context.WithoutCancel(ctx))
	select {
	case v := <-out:
		return v, e
	default:
		return cloudrepo.RepoKeyResult{Uncertain: true}, e
	}
}

// Indexing reads authoritative files under the pinned source lease. Automatic
// retention only mutates the private repository. Neither holds resource serial
// locks across IO, and metadata authority cannot implicitly grant this port.
func (l *Lease) NativeLocalRepo(ctx context.Context, r cloudrepo.LocalRepoRequest) (cloudrepo.LocalRepoResult, error) {
	var s *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s = l.workspace.resources.cloudRepo
		if s == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return cloudrepo.LocalRepoResult{}, e
	}
	out := make(chan cloudrepo.LocalRepoResult, 1)
	task, e := l.StartTask(ctx, func(c context.Context, pin *Lease) error {
		var v cloudrepo.LocalRepoResult
		run := func() error { var e error; v, e = s.LocalRepo(c, r); return e }
		var e error
		if r.Kind == "index" {
			e = pin.sourceRead(c, run)
		} else {
			e = run()
		}
		out <- v
		return e
	})
	if e != nil {
		return cloudrepo.LocalRepoResult{}, e
	}
	e = task.Wait(context.WithoutCancel(ctx))
	select {
	case v := <-out:
		return v, e
	default:
		return cloudrepo.LocalRepoResult{Uncertain: true}, e
	}
}

// Directory mutations and snapshot transfer are deliberately separate capabilities.
func (l *Lease) NativeCloudRepoQuery(ctx context.Context, r cloudrepo.Request) (cloudrepo.Result, error) {
	var s *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s = l.workspace.resources.cloudRepo
		if s == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return cloudrepo.Result{}, e
	}
	out := make(chan cloudrepo.Result, 1)
	task, e := l.StartTask(ctx, func(c context.Context, _ *Lease) error {
		v, e := s.Query(c, r)
		if e == nil {
			out <- v
		}
		return e
	})
	if e != nil {
		return cloudrepo.Result{}, e
	}
	if e = task.Wait(ctx); e != nil {
		return cloudrepo.Result{}, e
	}
	return <-out, nil
}

// Explicit private-cloud/device mutation authority; never an implicit extension
// of metadata Execute or a wiki source transaction. Wait for the actual outcome
// and unlock cleanup so an agent does not retry an already applied mutation.
func (l *Lease) NativeCloudDirectory(ctx context.Context, r cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error) {
	var s *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s = l.workspace.resources.cloudRepo
		if s == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return cloudrepo.DirectoryResult{}, e
	}
	out := make(chan cloudrepo.DirectoryResult, 1)
	task, e := l.StartTask(ctx, func(c context.Context, _ *Lease) error { v, e := s.Directory(c, r); out <- v; return e })
	if e != nil {
		return cloudrepo.DirectoryResult{}, e
	}
	e = task.Wait(context.WithoutCancel(ctx))
	select {
	case v := <-out:
		return v, e
	default:
		return cloudrepo.DirectoryResult{Uncertain: true}, e
	}
}

// Snapshot capability is separate from both metadata and directory authority.
// Preserve partial results and retain the lease through traffic/unlock cleanup.
func (l *Lease) NativeSnapshot(ctx context.Context, r cloudrepo.SnapshotRequest) (cloudrepo.SnapshotResult, error) {
	var s *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s = l.workspace.resources.cloudRepo
		if s == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return cloudrepo.SnapshotResult{}, e
	}
	out := make(chan cloudrepo.SnapshotResult, 1)
	task, e := l.StartTask(ctx, func(c context.Context, _ *Lease) error { v, e := s.Snapshot(c, r); out <- v; return e })
	if e != nil {
		return cloudrepo.SnapshotResult{}, e
	}
	e = task.Wait(context.WithoutCancel(ctx))
	select {
	case v := <-out:
		return v, e
	default:
		return cloudrepo.SnapshotResult{Uncertain: true}, e
	}
}
