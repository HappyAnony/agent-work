// Copyright (c) 2020-present, b3log.org
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

var ErrName = errors.New("cloud_directory_invalid_name")
var ErrProviderType = errors.New("cloud_directory_provider_unsupported")

type DirectoryRequest struct{ Kind, Name string }
type DirectoryResult struct {
	Active                    string
	Applied, Uncertain, Saved bool
}

func ValidDirectoryName(s string) bool {
	if len(s) < 1 || len(s) > 63 || !utf8.ValidString(s) || strings.ContainsAny(s, "~`!@#$%^&*()+=[]{}\\|;:'\"<,>.?/ ") {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func (s *Service) Directory(ctx context.Context, r DirectoryRequest) (DirectoryResult, error) {
	if s.life.Err() != nil {
		return DirectoryResult{}, ErrClosed
	}
	if r.Kind != "create" && r.Kind != "remove" && r.Kind != "set" {
		return DirectoryResult{}, ErrOperation
	}
	if s.o.Directory == nil {
		return DirectoryResult{}, ErrDependency
	}
	if r.Kind == "create" {
		r.Name = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(strings.TrimSpace(r.Name)))
	}
	if !(r.Kind == "remove" && r.Name == "") && !ValidDirectoryName(r.Name) {
		return DirectoryResult{}, ErrName
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return DirectoryResult{}, c.Err()
	}
	if s.life.Err() != nil {
		return DirectoryResult{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return DirectoryResult{}, e
	}
	v, e := s.o.Load(c)
	if e != nil {
		return DirectoryResult{}, e
	}
	v, e = v.Clone()
	if e != nil {
		return DirectoryResult{}, e
	}
	if v.Sync == nil {
		return DirectoryResult{}, ErrDependency
	}
	out := DirectoryResult{Active: v.Sync.CloudName}
	if r.Kind != "set" && v.Sync.Provider != conf.ProviderSiYuan && v.Sync.Provider != conf.ProviderLocal {
		return out, ErrProviderType
	}
	if r.Kind == "remove" && r.Name == "" {
		return out, nil
	}
	if e = c.Err(); e != nil {
		return out, e
	}
	out, e = s.o.Directory(c, v, r)
	// Preserve the remote outcome when cancellation or a failed local CAS follows
	// a successful remote mutation. Neither server nor agent should blindly retry.
	if c.Err() != nil {
		return out, errors.Join(e, c.Err())
	}
	return out, e
}
