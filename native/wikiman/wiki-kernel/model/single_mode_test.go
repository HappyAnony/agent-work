// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/util"
)

// setupSingleModeTestEnv prepares a temp workspace and returns a teardown.
// Caller is responsible for setting util.SingleNotebookMode / Name / DocName
// as appropriate.
func setupSingleModeTestEnv(t *testing.T) (dataDir string, teardown func()) {
	t.Helper()
	tmp := t.TempDir()
	dataDir = filepath.Join(tmp, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	// Save and restore globals + Conf.
	prevDataDir := util.DataDir
	prevMode := util.SingleNotebookMode
	prevName := util.SingleNotebookName
	prevDoc := util.SingleDocName
	prevBox := util.SingleNotebookBoxID
	prevRoot := util.SingleNotebookRootDocID
	prevConf := Conf

	util.DataDir = dataDir
	util.SingleNotebookMode = true
	util.SingleNotebookName = "Notebook"
	util.SingleDocName = "Untitled"
	util.SingleNotebookBoxID = ""
	util.SingleNotebookRootDocID = ""
	Conf = NewAppConfWithBusiness(legacyConfigurationBusiness())

	teardown = func() {
		util.DataDir = prevDataDir
		util.SingleNotebookMode = prevMode
		util.SingleNotebookName = prevName
		util.SingleDocName = prevDoc
		util.SingleNotebookBoxID = prevBox
		util.SingleNotebookRootDocID = prevRoot
		Conf = prevConf
	}
	return dataDir, teardown
}

func TestEnsureSingleNotebookAndDoc_MultiModeNoOp(t *testing.T) {
	_, teardown := setupSingleModeTestEnv(t)
	defer teardown()
	util.SingleNotebookMode = false

	EnsureSingleNotebookAndDoc()

	if Conf.SingleNotebookMode {
		t.Error("expected Conf.SingleNotebookMode to stay false")
	}
	if util.SingleNotebookBoxID != "" || util.SingleNotebookRootDocID != "" {
		t.Errorf("expected empty IDs, got box=%q root=%q",
			util.SingleNotebookBoxID, util.SingleNotebookRootDocID)
	}
}

func TestSortBoxesForElection(t *testing.T) {
	boxes := []*Box{
		{ID: "20240101000000-xxxx", Sort: 5},
		{ID: "20240101000001-aaaa", Sort: 2},
		{ID: "20240101000002-bbbb", Sort: 2}, // ties with previous; ID asc breaks tie
		{ID: "20240101000003-cccc", Sort: 1},
	}
	sortBoxesForElection(boxes)

	if boxes[0].Sort != 1 {
		t.Errorf("expected lowest Sort first, got %d", boxes[0].Sort)
	}
	// Ties: Sort=2 → IDs 0001-aaaa < 0002-bbbb
	if boxes[1].ID != "20240101000001-aaaa" || boxes[2].ID != "20240101000002-bbbb" {
		t.Errorf("tiebreaker by ID failed: got [%s, %s]", boxes[1].ID, boxes[2].ID)
	}
}

func TestSortDocsForElection(t *testing.T) {
	files := []*File{
		{ID: "20240101000000-x", Sort: 5},
		{ID: "20240101000001-a", Sort: 1},
		{ID: "20240101000002-b", Sort: 1}, // tie
	}
	sortDocsForElection(files)

	if files[0].ID != "20240101000001-a" {
		t.Errorf("expected lowest (Sort, ID) first, got %s", files[0].ID)
	}
}

// Higher-level scenario tests (empty workspace / legacy multi-box / etc.)
// require a live wiki-kernel test harness because CreateBox / Mount /
// CreateDocByMd touch global state (boxLock, sync state, Lute engine).
// Those scenarios are covered by Stage 8 integration smoke tests
// (test-tech-integration-smoke). See §EA-M3-4.
