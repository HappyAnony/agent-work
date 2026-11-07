//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiki-kernel/av"
)

func failRepairClose(r *indexRepairContext, cause error) {
	r.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), cause) }
}

func TestRepairCloseCapturedBackendAndJoinedErrors(t *testing.T) {
	e, _ := ownedImportFixture(t)
	r := e.binding.scope.commit.r
	sentinel := errors.New("captured close failed")
	calls := 0
	r.closeRoot = func(root *os.Root) error { calls++; return errors.Join(root.Close(), sentinel) }
	io, closeIO, err := r.openRepairIOChecked(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	r.closeRoot = func(*os.Root) error { t.Fatal("borrow followed changed context backend"); return nil }
	first := closeIO()
	if !errors.Is(first, sentinel) || closeIO() != first || calls != 2 {
		t.Fatal(first, calls)
	}
	if _, err = io.Source.Data.Stat("."); err == nil {
		t.Fatal("data root not closed")
	}
	if _, err = io.Source.ArchiveRoot.Stat("."); err == nil {
		t.Fatal("workspace root not closed")
	}
	primary := errors.New("primary source failure")
	joined := joinRepairCloseError(primary, first)
	if !errors.Is(joined, primary) || !errors.Is(joined, sentinel) || joinRepairCloseError(joined, first) != joined {
		t.Fatal("close causes lost or duplicated", joined)
	}
}

func TestRepairCloseExportReadsDiscardResults(t *testing.T) {
	e, box, file := exportContextFixture(t)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("export root close failed")
	failRepairClose(e.binding.scope.commit.r, sentinel)
	if paths, err := e.documentPaths(box, "/", false); !errors.Is(err, sentinel) || paths != nil {
		t.Fatal(paths, err)
	}
	if trees, err := e.selectedTrees(box, []string{"/" + filepath.Base(file)}); !errors.Is(err, sentinel) || trees != nil {
		t.Fatal(trees, err)
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(before, after) {
		t.Fatal("read changed source")
	}
}

func TestRepairCloseAVReadAndSaveInvalidatePublishedRevision(t *testing.T) {
	c, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	scope := c.avContext(context.Background())
	loaded, err := scope.read(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("AV root close failed")
	failRepairClose(c.r, sentinel)
	if raw, err := scope.source("storage/av/" + a.ID + ".json"); !errors.Is(err, sentinel) || raw != nil {
		t.Fatal(raw, err)
	}
	loaded.Name = "written but close failed"
	if err = scope.save(loaded); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if scope.cached[a.ID] != nil || scope.versions[a.ID] != nil {
		t.Fatal("failed completion retained success revision/cache")
	}
	raw, err := os.ReadFile(file)
	if err != nil || !bytes.Contains(raw, []byte(loaded.Name)) {
		t.Fatal("close failure incorrectly implied write rollback", err)
	}
	c.r.closeRoot = nil
	reloaded, err := scope.read(a.ID)
	if err != nil || reloaded.Name != loaded.Name {
		t.Fatal("failed close poisoned retry", err)
	}
}

func TestRepairCloseCommitFailureSuppressesCompletion(t *testing.T) {
	c, env, tx, _, _ := commitFixture(t)
	// Empty source batch isolates the final root close, after the queue flush.
	clear(tx.trees)
	clear(tx.nodes)
	sentinel := errors.New("commit root close failed")
	failRepairClose(env.commitContext.r, sentinel)
	completed := false
	env.commitContext.onComplete = func(context.Context, []commitNotice) error { completed = true; return nil }
	c.syncRuntime.same.Store(7)
	if err := env.commitContext.finish(context.Background(), tx); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if completed || c.syncRuntime.same.Load() != 7 {
		t.Fatal("completion/sync escaped failed close")
	}
}

func TestRepairCloseIndexDrainsAndNotebookDoesNotMarkHistory(t *testing.T) {
	for _, kind := range []string{"incremental", "notebook", "paths"} {
		t.Run(kind, func(t *testing.T) {
			c, r, _, file, _ := referenceModelFixture(t)
			box := filepath.Base(filepath.Dir(file))
			sentinel := errors.New("index root close failed")
			closed := 0
			r.closeRoot = func(root *os.Root) error {
				closed++
				if kind != "paths" {
					var count int
					if err := r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", strings.TrimSuffix(filepath.Base(file), ".docj")).Scan(&count); err != nil || count != 1 {
						t.Errorf("root closed before projection drained: %d %v", count, err)
					}
				}
				return errors.Join(root.Close(), sentinel)
			}
			var err error
			switch kind {
			case "incremental":
				result, e := r.incrementalIndex(context.Background(), []string{box + "/" + filepath.Base(file)}, nil)
				err = e
				if len(result.UpsertRootIDs) != 1 {
					t.Fatal("completed derived work lost from diagnostics", result)
				}
			case "notebook":
				err = r.indexNotebook(context.Background(), box)
				if _, ok := c.historyGenerated.Load(box); ok {
					t.Fatal("failed close marked history complete")
				}
			case "paths":
				paths, e := r.expandIndexPaths(context.Background(), []string{box})
				err = e
				if paths != nil {
					t.Fatal("scan returned success-looking result", paths)
				}
			}
			if !errors.Is(err, sentinel) || closed == 0 {
				t.Fatal(err, closed)
			}
		})
	}
}
