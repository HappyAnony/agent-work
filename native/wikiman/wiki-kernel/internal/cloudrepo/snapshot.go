// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"strings"
	"unicode/utf8"
	"wiki-kernel/conf"
)

var ErrSnapshotID = errors.New("snapshot_invalid_id")
var ErrTag = errors.New("snapshot_invalid_tag")
var ErrKey = errors.New("repository_key_missing")

type SnapshotRequest struct{ Kind, ID, Tag string }
type SnapshotResult struct {
	Files, Chunks, Indexes, Objects int
	Bytes                           int64
	Applied, Uncertain              bool
	Notice                          int
}

func ValidSnapshotID(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func ValidSnapshotTag(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 255 || !utf8.ValidString(s) || !gulu.File.IsValidFilename(s) {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}
func (s *Service) Snapshot(ctx context.Context, r SnapshotRequest) (SnapshotResult, error) {
	if s.life.Err() != nil {
		return SnapshotResult{}, ErrClosed
	}
	switch r.Kind {
	case "tag", "untag", "cloud-untag", "upload", "download", "purge-local", "purge-cloud":
	default:
		return SnapshotResult{}, ErrOperation
	}
	if s.o.Snapshot == nil {
		return SnapshotResult{}, ErrDependency
	}
	if r.Kind == "tag" {
		r.Tag = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(strings.TrimSpace(r.Tag)))
	}
	if (r.Kind == "tag" || r.Kind == "upload" || r.Kind == "download") && !ValidSnapshotID(r.ID) {
		return SnapshotResult{}, ErrSnapshotID
	}
	if r.Kind != "purge-local" && r.Kind != "purge-cloud" && !(r.Kind == "download" && r.Tag == "") && !ValidSnapshotTag(r.Tag) {
		return SnapshotResult{}, ErrTag
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return SnapshotResult{}, c.Err()
	}
	if s.life.Err() != nil {
		return SnapshotResult{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return SnapshotResult{}, e
	}
	v, e := s.o.Load(c)
	if e != nil {
		return SnapshotResult{}, e
	}
	v, e = v.Clone()
	if e != nil {
		return SnapshotResult{}, e
	}
	if v.Sync == nil || v.Repo == nil {
		return SnapshotResult{}, ErrDependency
	}
	if r.Kind != "purge-local" && r.Kind != "purge-cloud" && len(v.Repo.Key) == 0 {
		return SnapshotResult{}, ErrKey
	}
	if r.Kind == "upload" || r.Kind == "download" || r.Kind == "cloud-untag" {
		u := v.User
		sub := u != nil && (u.UserSiYuanProExpireTime == -1 || u.UserSiYuanProExpireTime > 0) && u.UserSiYuanSubscriptionStatus == 0
		paid := sub || u != nil && u.UserSiYuanOneTimePayStatus == 1
		if v.Sync.Provider == conf.ProviderSiYuan && !sub {
			return SnapshotResult{Notice: 29}, nil
		}
		if v.Sync.Provider != conf.ProviderSiYuan && !paid {
			return SnapshotResult{Notice: 214}, nil
		}
	}
	if e = c.Err(); e != nil {
		return SnapshotResult{}, e
	}
	out, e := s.o.Snapshot(c, v, r)
	if c.Err() != nil {
		return out, errors.Join(e, c.Err())
	}
	return out, e
}
