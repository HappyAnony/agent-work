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
	"sort"

	"github.com/88250/lute/ast"
	"github.com/siyuan-note/logging"
	"wiki-kernel/util"
)

// EnsureSingleNotebookAndDoc guarantees that exactly one notebook and exactly
// one root document exist when the kernel runs in single-notebook mode.
//
// Called once from main.go immediately after model.InitBoxes() and before
// util.SetBooted(). Idempotent across re-runs against the same workspace
// (deterministic election: lowest Sort, lowest ID).
//
// Algorithm (see docs/feature/siyuan/Architecture/03-single-mode-bootstrap.md §3):
//
//  1. If util.SingleNotebookMode == false, return immediately (multi-mode no-op).
//  2. Elect the primary notebook from ListNotebooks():
//     0 boxes  → CreateBox(util.SingleNotebookName) + Mount
//     1 box    → use it (Mount if Closed)
//     N boxes  → keep the lowest-Sort-then-ID box, Unmount the rest
//  3. Elect the root document under primary's root path:
//     0 docs   → CreateDocByMd with util.SingleDocName
//     1 doc    → use it
//     N docs   → keep the lowest-Sort-then-ID doc, ignore the rest (data preserved)
//  4. Write util.SetSingleNotebookIDs and Conf.SingleNotebook* state.
//  5. Emit single_mode_ready log.
//
// Failure handling:
//   - CreateBox / Mount(primary) / CreateDocByMd failures → log
//     single_mode_bootstrap_failed ERROR + return without populating IDs.
//     Subsequent /api/system/getConf returns single=true with empty IDs;
//     the FE single bundle enters fallback C ("bootstrap incomplete").
//   - Unmount of an extra box: see §EA-M3-3 — existing Unmount is void and
//     logs errors internally; we record every Unmount as "extra-box" reason
//     (no "unmount-failed" reason can be observed from the call site).
func EnsureSingleNotebookAndDoc() {
	if !util.SingleNotebookMode {
		return
	}

	logging.LogInfof("event=single_mode_start workspace=%s", util.WorkspaceDir)

	primary, unmountedBoxes, ok := electAndPrepareBox()
	if !ok {
		// fatal error already logged by helper
		Conf.SingleNotebookMode = true // mark mode-on so getConf can advertise the failure state
		return
	}

	rootDocID, ignoredDocs, ok := electAndPrepareRootDoc(primary)
	if !ok {
		Conf.SingleNotebookMode = true
		Conf.SingleNotebookBoxID = primary.ID
		// rootDocID stays empty → FE fallback C
		Conf.UnmountedBoxes = unmountedBoxes
		return
	}

	// Success path: publish to util globals first (so guard middleware and
	// future readers can resolve immediately), then to Conf.
	util.SetSingleNotebookIDs(primary.ID, rootDocID)
	Conf.SingleNotebookMode = true
	Conf.SingleNotebookBoxID = primary.ID
	Conf.SingleNotebookRootDocID = rootDocID
	Conf.UnmountedBoxes = unmountedBoxes
	Conf.IgnoredDocs = ignoredDocs

	logging.LogInfof(
		"event=single_mode_ready boxID=%s rootDocID=%s unmounted=%d ignored=%d",
		primary.ID, rootDocID, len(unmountedBoxes), len(ignoredDocs),
	)
}

// electAndPrepareBox handles box-level election + Mount/Unmount.
// Returns (primary, unmountedBoxes, ok). On ok=false, a single_mode_bootstrap_failed
// log has already been written.
func electAndPrepareBox() (primary *Box, unmounted []UnmountedBoxInfo, ok bool) {
	boxes, err := ListNotebooks()
	if err != nil {
		logging.LogErrorf("event=single_mode_bootstrap_failed step=list_notebooks error=%s", err)
		return nil, nil, false
	}

	sortBoxesForElection(boxes)

	switch len(boxes) {
	case 0:
		id, err := CreateBox(util.SingleNotebookName)
		if err != nil {
			logging.LogErrorf("event=single_mode_bootstrap_failed step=create_box error=%s", err)
			return nil, nil, false
		}
		logging.LogInfof("event=single_mode_create_box boxID=%s name=%s", id, util.SingleNotebookName)
		if _, err := Mount(id); err != nil {
			logging.LogErrorf("event=single_mode_bootstrap_failed step=mount boxID=%s error=%s", id, err)
			return nil, nil, false
		}
		// Re-fetch the freshly-created+mounted box record so caller has Sort/etc.
		// (ListNotebooks always returns a fresh slice from disk, no cache issue.)
		boxes2, _ := ListNotebooks()
		for _, b := range boxes2 {
			if b.ID == id {
				return b, nil, true
			}
		}
		// Shouldn't happen — fall back to a minimal Box record.
		return &Box{ID: id, Name: util.SingleNotebookName}, nil, true

	case 1:
		primary = boxes[0]

	default:
		primary = boxes[0]
		for _, extra := range boxes[1:] {
			// Unmount is void and logs internally on failure (see §EA-M3-3).
			// We record every extra as "extra-box"; "unmount-failed" reason
			// is reserved for future use if Unmount ever surfaces errors.
			Unmount(extra.ID)
			unmounted = append(unmounted, UnmountedBoxInfo{
				ID:     extra.ID,
				Name:   extra.Name,
				Reason: "extra-box",
			})
			logging.LogInfof("event=single_mode_unmount_box boxID=%s name=%s", extra.ID, extra.Name)
		}
	}

	// Ensure primary is mounted. Mount is idempotent (returns alreadyMount=true).
	if primary.Closed {
		if _, err := Mount(primary.ID); err != nil {
			logging.LogErrorf("event=single_mode_bootstrap_failed step=mount boxID=%s error=%s", primary.ID, err)
			return nil, unmounted, false
		}
	}

	return primary, unmounted, true
}

// electAndPrepareRootDoc handles root-doc-level election + create.
// Returns (rootDocID, ignoredDocs, ok). On ok=false, a single_mode_bootstrap_failed
// log has already been written.
func electAndPrepareRootDoc(primary *Box) (rootDocID string, ignored []IgnoredDocInfo, ok bool) {
	// SortModeUnassigned (256) defers to box / filetree sort; we then re-sort
	// deterministically by our own rule (Sort asc, ID asc).
	files, _, err := ListDocTree(primary.ID, "/", util.SortModeUnassigned, false, false, MaxListCountForSingleMode)
	if err != nil {
		logging.LogErrorf("event=single_mode_bootstrap_failed step=list_doc_tree boxID=%s error=%s", primary.ID, err)
		return "", nil, false
	}

	sortDocsForElection(files)

	switch len(files) {
	case 0:
		// Create root doc with auto-generated ID, no markdown content.
		newID := ast.NewNodeID()
		newPath := "/" + newID + util.DocExt
		tree, err := CreateDocByMd(primary.ID, newPath, util.SingleDocName, "", nil)
		if err != nil {
			logging.LogErrorf("event=single_mode_bootstrap_failed step=create_root_doc boxID=%s error=%s", primary.ID, err)
			return "", nil, false
		}
		rootDocID = tree.Root.ID
		logging.LogInfof("event=single_mode_create_root_doc boxID=%s docID=%s title=%s",
			primary.ID, rootDocID, util.SingleDocName)
		return rootDocID, nil, true

	case 1:
		return files[0].ID, nil, true

	default:
		rootDocID = files[0].ID
		for _, extra := range files[1:] {
			title := docTitleFromFile(extra)
			ignored = append(ignored, IgnoredDocInfo{
				BoxID: primary.ID,
				ID:    extra.ID,
				Title: title,
			})
			logging.LogInfof("event=single_mode_ignore_doc boxID=%s docID=%s title=%s",
				primary.ID, extra.ID, title)
		}
		logging.LogWarnf("event=single_mode_extra_docs_found count=%d", len(ignored))
		return rootDocID, ignored, true
	}
}

// sortBoxesForElection orders boxes by (Sort ascending, ID ascending).
// The deterministic tiebreaker matches Architecture §03 §4.
func sortBoxesForElection(boxes []*Box) {
	sort.SliceStable(boxes, func(i, j int) bool {
		if boxes[i].Sort != boxes[j].Sort {
			return boxes[i].Sort < boxes[j].Sort
		}
		return boxes[i].ID < boxes[j].ID
	})
}

// sortDocsForElection orders root-level docs by (Sort ascending, ID ascending).
func sortDocsForElection(files []*File) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Sort != files[j].Sort {
			return files[i].Sort < files[j].Sort
		}
		return files[i].ID < files[j].ID
	})
}

// docTitleFromFile strips the ".docj" extension from File.Name (set by
// docFromFileInfo as `ial["title"] + util.DocExt`) so log output and the
// IgnoredDocInfo.Title field expose the human-readable title only.
func docTitleFromFile(f *File) string {
	return util.StripDocExt(f.Name)
}

// MaxListCountForSingleMode caps the ListDocTree page size during single-mode
// election. The root-doc count under a single notebook is small in practice;
// 1024 is a safe ceiling that exceeds any realistic legacy workspace.
const MaxListCountForSingleMode = 1024
