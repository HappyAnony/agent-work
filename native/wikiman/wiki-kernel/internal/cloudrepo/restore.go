// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
)

type RestoreResult struct {
	BackupID                                                                       string
	Applied, SourceTouched, Reindexed, Uncertain, CancellationDeferred, SyncPaused bool
}

var ErrRestoreBackup = errors.New("repository_restore_backup_failed")

// Restore is a separate trusted compatibility write capability. Native source
// mutation is not exposed here: its CAS/journal assembly remains required.
func (s *Service) Restore(ctx context.Context, id string) (RestoreResult, error) {
	if s.life.Err() != nil {
		return RestoreResult{}, ErrClosed
	}
	if !ValidSnapshotID(id) {
		return RestoreResult{}, ErrSnapshotID
	}
	if s.o.Restore == nil {
		return RestoreResult{}, ErrDependency
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return RestoreResult{}, c.Err()
	}
	if s.life.Err() != nil {
		return RestoreResult{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return RestoreResult{}, e
	}
	v, e := s.o.Load(c)
	if e != nil {
		return RestoreResult{}, e
	}
	v, e = v.Clone()
	if e != nil {
		return RestoreResult{}, e
	}
	if v.Repo == nil {
		return RestoreResult{}, ErrDependency
	}
	if len(v.Repo.Key) == 0 {
		return RestoreResult{}, ErrKey
	}
	out, e := s.o.Restore(c, v, id)
	if c.Err() != nil {
		return out, errors.Join(e, c.Err())
	}
	return out, e
}
