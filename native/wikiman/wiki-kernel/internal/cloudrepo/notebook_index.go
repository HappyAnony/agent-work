// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"

	"wiki-kernel/conf"
)

type NotebookIndexRequest struct{ Kind, ID string }

func (s *Service) NotebookIndex(ctx context.Context, r NotebookIndexRequest) error {
	switch r.Kind {
	case "index", "unindex", "refs", "init", "full":
	default:
		return ErrOperation
	}
	if s.o.NotebookIndex == nil {
		return ErrDependency
	}
	return s.maintenance(ctx, func(c context.Context, v conf.AppData) error { return s.o.NotebookIndex(c, v, r) })
}
