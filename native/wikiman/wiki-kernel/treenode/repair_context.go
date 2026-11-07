// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package treenode

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexdb"
)

// Captured only inside the compatibility owner's lifecycle barrier. Reads and
// mutations retain this DB even if the process's selected DB later changes.
type RepairRegistry struct{ indexdb.BlockTrees }

func CaptureLegacyRepairRegistry() RepairRegistry { return RepairRegistry{indexdb.BlockTrees{DB: db}} }
func (q RepairRegistry) Index(ctx context.Context, t *parse.Tree) error {
	indexBlockTreeLock.Lock()
	defer indexBlockTreeLock.Unlock()
	return q.BlockTrees.Index(ctx, t)
}
func (q RepairRegistry) Upsert(ctx context.Context, t *parse.Tree) error {
	indexBlockTreeLock.Lock()
	defer indexBlockTreeLock.Unlock()
	return q.BlockTrees.Upsert(ctx, t)
}
