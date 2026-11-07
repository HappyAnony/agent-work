// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"strings"
	"time"
	"wiki-kernel/conf"
)

var ErrMemo = errors.New("repository_memo_empty")
var ErrRetention = errors.New("repository_retention_invalid")

type LocalRepoRequest struct {
	Kind, Memo string
	Cron       bool
}
type LocalRepoResult struct {
	ID                                   string
	Changed, Applied, Uncertain, Skipped bool
	Indexes, Objects                     int
	Bytes                                int64
}
type RepoIndex struct {
	ID      string
	Created int64
}
type RetentionPolicy func(func(int) ([]RepoIndex, int, error)) ([]string, error)

func (s *Service) LocalRepo(ctx context.Context, r LocalRepoRequest) (LocalRepoResult, error) {
	if s.life.Err() != nil {
		return LocalRepoResult{}, ErrClosed
	}
	if r.Kind != "index" && r.Kind != "auto-purge" {
		return LocalRepoResult{}, ErrOperation
	}
	if s.o.LocalRepo == nil {
		return LocalRepoResult{}, ErrDependency
	}
	if r.Kind == "index" {
		r.Memo = gulu.Str.RemoveInvisible(strings.TrimSpace(r.Memo))
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return LocalRepoResult{}, c.Err()
	}
	if s.life.Err() != nil {
		return LocalRepoResult{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return LocalRepoResult{}, e
	}
	now := s.o.Now()
	if r.Kind == "auto-purge" {
		if r.Cron && !s.repoAfterSync {
			return LocalRepoResult{Skipped: true}, nil
		}
		if !s.lastRepoPurge.IsZero() && now.Sub(s.lastRepoPurge) < 6*time.Hour {
			return LocalRepoResult{Skipped: true}, nil
		}
		s.repoAfterSync = true
		defer func() { s.lastRepoPurge = s.o.Now() }()
	}
	v, e := s.o.Load(c)
	if e != nil {
		return LocalRepoResult{}, e
	}
	v, e = v.Clone()
	if e != nil {
		return LocalRepoResult{}, e
	}
	if v.Repo == nil {
		return LocalRepoResult{}, ErrDependency
	}
	if len(v.Repo.Key) == 0 {
		if r.Kind == "auto-purge" {
			return LocalRepoResult{Skipped: true}, nil
		}
		return LocalRepoResult{}, ErrKey
	}
	if r.Kind == "index" && r.Memo == "" {
		return LocalRepoResult{}, ErrMemo
	}
	retention := *v.Repo
	policy := func(next func(int) ([]RepoIndex, int, error)) ([]string, error) {
		return Retention(c, retention, now, next, s.o.Intn)
	}
	out, e := s.o.LocalRepo(c, v, r, policy)
	if c.Err() != nil {
		return out, errors.Join(e, c.Err())
	}
	return out, e
}

// Retention preserves the original policy: all today's indexes, daily newest
// plus bounded random picks, and no purge with fewer than three retained IDs.
func Retention(ctx context.Context, cfg conf.Repo, now time.Time, next func(int) ([]RepoIndex, int, error), intn func(int) int) ([]string, error) {
	if cfg.IndexRetentionDays < 0 || cfg.IndexRetentionDays > 365000 || cfg.RetentionIndexesDaily < 1 || cfg.RetentionIndexesDaily > 10000 || next == nil || intn == nil {
		return nil, ErrRetention
	}
	groups := map[string][]RepoIndex{}
	for page := 1; ; page++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		indexes, pages, e := next(page)
		if e != nil {
			return nil, e
		}
		if len(indexes) == 0 {
			break
		}
		old := false
		for _, idx := range indexes {
			if !ValidSnapshotID(idx.ID) {
				return nil, ErrSnapshotID
			}
			if now.UnixMilli()-idx.Created > int64(cfg.IndexRetentionDays)*24*60*60*1000 {
				old = true
				break
			}
			date := time.UnixMilli(idx.Created).In(now.Location()).Format("2006-01-02")
			groups[date] = append(groups[date], idx)
		}
		if old || page >= pages {
			break
		}
	}
	today := now.Format("2006-01-02")
	set := map[string]bool{}
	for date, indexes := range groups {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if date == today || len(indexes) <= cfg.RetentionIndexesDaily {
			for _, idx := range indexes {
				set[idx.ID] = true
			}
			continue
		}
		keep := map[int]bool{0: true}
		for i := 0; i < cfg.RetentionIndexesDaily*7; i++ {
			n := intn(len(indexes) - 1)
			if n < 0 || n >= len(indexes)-1 {
				return nil, ErrRetention
			}
			keep[n] = true
			if len(keep) >= cfg.RetentionIndexesDaily {
				break
			}
		}
		for i := range keep {
			set[indexes[i].ID] = true
		}
	}
	ids := []string{}
	for id := range set {
		ids = append(ids, id)
	}
	return ids, nil
}
