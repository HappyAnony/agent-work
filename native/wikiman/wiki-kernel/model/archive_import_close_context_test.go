//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func TestArchiveBorrowedRootCloseOnceAndOwnerSurvives(t *testing.T) {
	e, _ := ownedImportFixture(t)
	owned, err := e.newImportScratch("wiki-import-")
	if err != nil {
		t.Fatal(err)
	}
	defer owned.close()
	sentinel := errors.New("borrow close failed")
	calls := 0
	e.closeRoot = func(root *os.Root) error { calls++; return errors.Join(root.Close(), sentinel) }
	source, closeIO, err := e.openScratch(owned.folder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Data.Stat("."); err != nil {
		t.Fatal(err)
	}
	// The close capability is captured, not looked up again after a workspace switch.
	e.closeRoot = func(*os.Root) error { t.Fatal("closer changed after borrow"); return nil }
	for i := 0; i < 2; i++ {
		if err = closeIO(); !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("descriptor closed more than once", calls)
	}
	if _, err = source.Data.Stat("."); err == nil {
		t.Fatal("borrowed descriptor still open")
	}
	if _, err = owned.root.Stat("."); err != nil {
		t.Fatal("borrow close closed the scratch owner", err)
	}
}
func TestArchiveBorrowedChildOpenFailureClosesAllDescriptors(t *testing.T) {
	e, _ := ownedImportFixture(t)
	owned, err := e.newImportScratch("wiki-import-")
	if err != nil {
		t.Fatal(err)
	}
	defer owned.close()
	owned.root.Mkdir("child", 0700)
	sentinel := errors.New("parent close failed")
	var closed []*os.Root
	e.closeRoot = func(root *os.Root) error { closed = append(closed, root); return errors.Join(root.Close(), sentinel) }
	source, closeIO, err := e.openScratch(filepath.Join(owned.folder, "child"))
	if !errors.Is(err, sentinel) || source.Data != nil || closeIO != nil || len(closed) != 2 {
		t.Fatal("failed child handoff leaked root", err, len(closed))
	}
	for _, root := range closed {
		if _, err = root.Stat("."); err == nil {
			t.Fatal("borrowed root still open")
		}
	}
	if _, err = owned.root.Stat("."); err != nil {
		t.Fatal("owner was closed", err)
	}
	// Check failure still closes the parent borrow too.
	e.closeRoot = func(root *os.Root) error { closed = append(closed, root); return root.Close() }
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(owned.folder, "link"))
	if _, _, err = e.openScratch(filepath.Join(owned.folder, "link")); err == nil || len(closed) != 3 {
		t.Fatal("invalid descendant leaked parent", err, len(closed))
	}
}
func TestArchiveCheckedRepairRootsJoinAndIdempotentClose(t *testing.T) {
	e, _ := ownedImportFixture(t)
	r := e.binding.scope.commit.r
	a, b := errors.New("workspace close"), errors.New("data close")
	var closed []*os.Root
	io, closeIO, err := r.openRepairIOCheckedWithCloser(e.ctx, true, func(root *os.Root) error {
		closed = append(closed, root)
		extra := b
		if len(closed) == 1 {
			extra = a
		}
		return errors.Join(root.Close(), extra)
	})
	if err != nil {
		t.Fatal(err)
	}
	if io.Source.Data == nil || io.Source.ArchiveRoot == nil {
		t.Fatal("missing rooted IO")
	}
	for i := 0; i < 2; i++ {
		err = closeIO()
		if !errors.Is(err, a) || !errors.Is(err, b) {
			t.Fatal("lost close causes", err)
		}
	}
	if len(closed) != 2 {
		t.Fatal("closed root count", len(closed))
	}
	for _, root := range closed {
		if _, err = root.Stat("."); err == nil {
			t.Fatal("root still open")
		}
	}
	if _, _, err = r.openRepairIOCheckedWithCloser(e.ctx, false, nil); err == nil {
		t.Fatal("unbound backend accepted")
	}
}
func TestArchiveScratchCloseErrorsAbortResultsAndJoinPrimary(t *testing.T) {
	for _, kind := range []string{"av-absent", "av-corrupt", "rename-collision", "write", "rename", "remove"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := ownedImportFixture(t)
			root := t.TempDir()
			sentinel := errors.New("scratch close failed")
			e.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), sentinel) }
			var err error
			switch kind {
			case "av-absent":
				var result map[string]string
				result, _, err = e.remapScratchAVs(root, nil, ast.NewNodeID)
				if result != nil {
					t.Fatal("success-looking result escaped close failure")
				}
			case "av-corrupt":
				id := ast.NewNodeID()
				archiveWrite(t, root, "storage/av/"+id+".json", []byte("broken"))
				_, _, err = e.remapScratchAVs(root, nil, ast.NewNodeID)
				if !strings.Contains(err.Error(), "invalid character") {
					t.Fatal("primary decode error lost", err)
				}
			case "rename-collision":
				old, next := ast.NewNodeID(), ast.NewNodeID()
				os.Mkdir(filepath.Join(root, old), 0700)
				os.Mkdir(filepath.Join(root, next), 0700)
				err = e.renameScratchDirectories(root, map[string]string{old: next})
				if !errors.Is(err, indexrepair.ErrSourceChanged) {
					t.Fatal("primary collision lost", err)
				}
			case "write":
				err = e.writeScratchDocuments(root, nil, e.binding.scope.commit.parser())
			case "rename":
				err = e.renameScratchDirectories(root, nil)
			case "remove":
				err = e.removeScratchStorage(root)
			}
			if !errors.Is(err, sentinel) {
				t.Fatal("close error swallowed", err)
			}
		})
	}
}
func TestArchivePreparationAndUploadCloseFailureCleanup(t *testing.T) {
	for _, upload := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepare", true: "upload"}[upload], func(t *testing.T) {
			e, box := ownedImportFixture(t)
			id := ast.NewNodeID()
			file := makeArchive(t, archiveMember{"Root/" + id + ".docj", mountDoc(id, "Original import"), 0})
			before, _ := os.ReadFile(file)
			sentinel := errors.New("borrow close failed")
			e.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), sentinel) }
			if upload {
				if err := e.ImportReader(bytes.NewReader(before), box, "/"); !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			} else {
				prepared, err := e.prepare(file)
				if !errors.Is(err, sentinel) || prepared != nil {
					t.Fatal("prepared result survived close error", err)
				}
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("caller archive changed")
			}
			assertNoImportScratch(t, e)
		})
	}
}
func TestArchivePublicationCloseFailureStopsCompletionWithoutRollbackClaim(t *testing.T) {
	for _, kind := range []string{"source", "target", "sort", "resources", "mount"} {
		t.Run(kind, func(t *testing.T) {
			e, box := ownedImportFixture(t)
			root := t.TempDir()
			sentinel := errors.New("publication close failed")
			calls := 0
			e.closeRoot = func(r *os.Root) error {
				calls++
				closeErr := r.Close()
				if kind != "target" || filepath.Clean(r.Name()) == filepath.Clean(e.binding.scope.commit.r.data) {
					return errors.Join(closeErr, sentinel)
				}
				return closeErr
			}
			var err error
			switch kind {
			case "source", "target":
				archiveWrite(t, root, "file.txt", []byte("published bytes"))
				err = e.mergeArchiveDirectory(root, "assets")
			case "sort":
				err = e.mergeArchiveSort(root, box, nil)
			case "resources":
				archiveWrite(t, root, "assets/file.txt", []byte("published bytes"))
				err = e.publishArchiveResources(root, "assets")
			case "mount":
				id := ast.NewNodeID()
				archiveWrite(t, root, id+".docj", mountDoc(id, "Mounted"))
				err = e.finishArchive(root, box, "/", e.binding.scope.commit.parser(), map[string]string{id: id})
			}
			if !errors.Is(err, sentinel) || calls == 0 {
				t.Fatal("publication close failure hidden", kind, err, calls)
			}
			// Sequential writes can already exist. A close failure is not rollback.
			if kind == "source" || kind == "target" {
				raw, e2 := os.ReadFile(filepath.Join(e.binding.scope.commit.r.data, "assets", "file.txt"))
				if e2 != nil || string(raw) != "published bytes" {
					t.Fatal("sequential merge changed", e2)
				}
			}
		})
	}
}
func TestArchiveCheckedReadOnlyAndCanceledOpen(t *testing.T) {
	e, _ := ownedImportFixture(t)
	r := e.binding.scope.commit.r
	io, closeIO, err := r.openRepairIOChecked(e.ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	if err = io.Source.Rewrite(e.ctx, "forbidden", nil, []byte("no")); !errors.Is(err, indexrepair.ErrSourceWrite) {
		t.Fatal("read-only root write rejection lost", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, closeCanceled, err := r.openRepairIOChecked(ctx, true); !errors.Is(err, context.Canceled) || closeCanceled != nil {
		t.Fatal("canceled open admitted resources", err)
	}
}

func TestArchiveCheckedRepairSecondOpenFailureClosesFirstRoot(t *testing.T) {
	e, _ := ownedImportFixture(t)
	r := e.binding.scope.commit.r
	workspace := r.workspace
	previous := util.WorkspaceDir
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	r.workspace = file
	util.WorkspaceDir = file
	defer func() { r.workspace = workspace; util.WorkspaceDir = previous }()
	sentinel := errors.New("data root close failed")
	var closed *os.Root
	_, closer, err := r.openRepairIOCheckedWithCloser(e.ctx, true, func(root *os.Root) error { closed = root; return errors.Join(root.Close(), sentinel) })
	var pathErr *os.PathError
	if !errors.Is(err, sentinel) || !errors.As(err, &pathErr) || closer != nil || closed == nil {
		t.Fatal("open/close errors not both retained", err)
	}
	if _, err = closed.Stat("."); err == nil {
		t.Fatal("first root leaked after second open failure")
	}
}
func TestArchiveMountLateCloseFailureJoinsIndexTail(t *testing.T) {
	e, box := ownedImportFixture(t)
	folder := t.TempDir()
	id := ast.NewNodeID()
	archiveWrite(t, folder, id+".docj", mountDoc(id, "Close after indexing"))
	sentinel := errors.New("mount root close failed")
	sourceCloses := 0
	e.closeRoot = func(root *os.Root) error {
		err := root.Close()
		if filepath.Clean(root.Name()) == filepath.Clean(folder) {
			sourceCloses++
			if sourceCloses == 2 {
				return errors.Join(err, sentinel)
			}
		}
		return err
	}
	err := e.finishArchive(folder, box, "/", e.binding.scope.commit.parser(), map[string]string{id: id})
	if !errors.Is(err, sentinel) || sourceCloses != 2 {
		t.Fatal("late mount close failure hidden", err, sourceCloses)
	}
	var count int
	if err = e.binding.scope.commit.r.main.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", id).Scan(&count); err != nil || count != 1 {
		t.Fatal("returned without joining admitted index work", err, count)
	}
}
