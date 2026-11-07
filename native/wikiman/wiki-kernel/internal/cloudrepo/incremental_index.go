// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"slices"

	"wiki-kernel/conf"
)

type IncrementalIndexRequest struct {
	Paths  []string
	Remove bool
}

// An explicitly bound compatibility capability, serialized with source repair /
// restore / sync, canceled on close and joined until the callback really returns.
func (s *Service) IncrementalIndex(ctx context.Context, r IncrementalIndexRequest) error {
	if s.o.IncrementalIndex == nil {
		return ErrDependency
	}
	r.Paths = slices.Clone(r.Paths)
	return s.maintenance(ctx, func(c context.Context, v conf.AppData) error { return s.o.IncrementalIndex(c, v, r) })
}
