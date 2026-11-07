//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/util"
)

func TestHistoryIndexEmptyScannedDirectoryReplacesRowsButAbsenceDoesNot(t *testing.T) {
	_, name, _, _ := historyIndexFixture(t)
	ctx := context.Background()
	h, err := captureHistoryIndexContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.index(ctx, name, util.NewLute()); err != nil {
		t.Fatal(err)
	}
	assertHistoryRows(t, h, 2)
	dir := filepath.Join(h.root, name)
	if err = os.Rename(dir, dir+".saved"); err != nil {
		t.Fatal(err)
	}
	if err = h.index(ctx, name, util.NewLute()); err != nil {
		t.Fatal(err)
	}
	assertHistoryRows(t, h, 2)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = h.index(ctx, name, util.NewLute()); err != nil {
		t.Fatal(err)
	}
	assertHistoryRows(t, h, 0)
}
