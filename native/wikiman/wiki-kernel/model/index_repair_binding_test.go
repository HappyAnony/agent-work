// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestIndexRepairBindingSnapshotAndStaleOwnerRejected(t *testing.T) {
	c, _, _, _ := restoreFixture(t)
	c.Search = conf.NewSearch()
	c.Search.CaseSensitive = false
	ctx := context.Background()
	r, e := captureIndexRepair(ctx)
	if e != nil {
		t.Fatal(e)
	}
	c.Search.CaseSensitive = true
	if r.caseSensitive {
		t.Fatal("preferences not captured")
	}
	before := util.DataDir
	util.DataDir = before + "-different"
	if e = r.removeDuplicateDatabaseIndex(ctx); !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal("stale paths admitted", e)
	}
	util.DataDir = before
	c.persistenceReadOnly = true
	if e = r.fixDatabaseIndexByBlockTree(ctx); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	c.persistenceReadOnly = false
	other := NewAppConf()
	Conf = other
	if e = r.resetDuplicateBlocksOnFileSys(ctx); !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal("new owner source could be repaired", e)
	}
	Conf = c
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if e = r.fixBlockTreeByFileSys(cancelCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "repair-binding.json", map[string]interface{}{"preferenceSnapshot": true, "staleOwnerRejected": true, "stalePathsRejected": true, "readonlyRejected": true, "canceledBeforeMutation": true})
}
