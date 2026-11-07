//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexdb"

	"wiki-kernel/treenode"
)

func markdownWriterFixture(t *testing.T) (*markdownImportWriter, string, *parse.Tree) {
	t.Helper()
	m, box, _ := markdownSourceFixture(t)
	w, err := m.openDocumentWriter(box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.close() })
	id := ast.NewNodeID()
	tree := treenode.NewTree(box, "/"+id+".docj", "/Writer fixture", "Writer fixture")
	return w, box, tree
}
func assertMarkdownWriterIndexCount(t *testing.T, w *markdownImportWriter, id string, want int) {
	t.Helper()
	rows, err := w.preparation.archive.binding.scope.commit.r.registry.Query(context.Background(), indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if err != nil || len(rows) != want {
		t.Fatal("blocktree count", rows, err)
	}
}
func TestMarkdownWriterOriginalJSONEmptyNormalizationAndSynchronousIndexes(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "paragraph", true: "empty"}[empty], func(t *testing.T) {
			w, box, tree := markdownWriterFixture(t)
			if empty {
				for tree.Root.FirstChild != nil {
					tree.Root.FirstChild.Unlink()
				}
				tree.Root.Spec = ""
			}
			if err := w.write(context.Background(), box, tree); err != nil {
				t.Fatal(err)
			}
			if err := w.close(); err != nil {
				t.Fatal(err)
			}
			raw, err := w.io.Source.Data.ReadFile(box + tree.Path)
			if err == nil {
				t.Fatal("closed root still usable")
			}
			raw, err = os.ReadFile(filepath.Join(w.preparation.archive.binding.scope.commit.r.data, box, tree.Path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte(`"Spec": "1"`)) && !bytes.Contains(raw, []byte(`"Spec":"1"`)) {
				t.Fatal("Spec normalization lost")
			}
			if tree.Root.FirstChild == nil || tree.Root.IALAttr("type") != "doc" {
				t.Fatal("empty normalization lost")
			}
			assertMarkdownWriterIndexCount(t, w, tree.ID, 1)
			assertMarkdownWriterIndexCount(t, w, tree.Root.FirstChild.ID, 1)
			var count int
			if err := w.queue.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", tree.ID).Scan(&count); err != nil || count != 1 {
				t.Fatal("main index not drained", count, err)
			}
		})
	}
}
func TestMarkdownWriterRejectsCollisionsPathsAndSourceLinksWithoutPreindex(t *testing.T) {
	for _, kind := range []string{"existing", "index-collision", "escape", "wrong-id", "wrong-box", "parent-link", "target-link"} {
		t.Run(kind, func(t *testing.T) {
			w, box, tree := markdownWriterFixture(t)
			dataRoot := w.preparation.archive.binding.scope.commit.r.data
			original := []byte("foreign document")
			switch kind {
			case "existing":
				if err := os.WriteFile(filepath.Join(dataRoot, box, tree.Path), original, 0600); err != nil {
					t.Fatal(err)
				}
			case "index-collision":
				if err := w.preparation.archive.binding.scope.commit.r.registry.Index(context.Background(), tree); err != nil {
					t.Fatal(err)
				}
			case "escape":
				tree.Path = "/../" + tree.ID + ".docj"
			case "wrong-id":
				tree.Root.ID = ast.NewNodeID()
			case "wrong-box":
				tree.Box = ast.NewNodeID()
			case "parent-link":
				id := ast.NewNodeID()
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(dataRoot, box, id)); err != nil {
					t.Fatal(err)
				}
				tree.Path = "/" + id + "/" + tree.ID + ".docj"
			case "target-link":
				outside := filepath.Join(t.TempDir(), "outside.docj")
				os.WriteFile(outside, original, 0600)
				if err := os.Symlink(outside, filepath.Join(dataRoot, box, tree.Path)); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.write(context.Background(), box, tree); err == nil {
				t.Fatal("unsafe write accepted")
			}
			if kind != "index-collision" {
				assertMarkdownWriterIndexCount(t, w, tree.ID, 0)
			}
			if kind == "existing" || kind == "target-link" {
				raw, err := os.ReadFile(filepath.Join(dataRoot, box, tree.Path))
				if err != nil || !bytes.Equal(raw, original) {
					t.Fatal("foreign file changed", err)
				}
			}
		})
	}
}
func TestMarkdownWriterFilesystemFailureDoesNotCreateGhostRegistry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	w, box, tree := markdownWriterFixture(t)
	folder := filepath.Join(w.preparation.archive.binding.scope.commit.r.data, box)
	if err := os.Chmod(folder, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(folder, 0755)
	if err := w.write(context.Background(), box, tree); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	assertMarkdownWriterIndexCount(t, w, tree.ID, 0)
	if _, err := os.Stat(filepath.Join(folder, tree.Path)); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestMarkdownWriterRegistryAndSQLFailuresRemainErrorsAfterSourcePublication(t *testing.T) {
	for _, kind := range []string{"registry", "queue"} {
		t.Run(kind, func(t *testing.T) {
			w, box, tree := markdownWriterFixture(t)
			r := w.preparation.archive.binding.scope.commit.r
			db := r.registry.DB
			statement := "CREATE TRIGGER fail_writer BEFORE INSERT ON blocktrees BEGIN SELECT RAISE(ABORT,'writer registry failed'); END"
			if kind == "queue" {
				db = w.queue.DB
				statement = "CREATE TRIGGER fail_writer BEFORE INSERT ON blocks BEGIN SELECT RAISE(ABORT,'writer queue failed'); END"
			}
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			defer func() {
				db.Exec("DROP TRIGGER fail_writer")
				if kind == "queue" {
					w.queue.Flush(context.Background())
				}
			}()
			err := w.write(context.Background(), box, tree)
			if kind == "registry" {
				if err == nil || !strings.Contains(err.Error(), "writer registry failed") {
					t.Fatal(err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err = w.close(); err == nil || !strings.Contains(err.Error(), "writer queue failed") {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(filepath.Join(r.data, box, tree.Path)); err != nil {
				t.Fatal("source publication unexpectedly rolled back", err)
			}
		})
	}
}
func TestMarkdownWriterCancellationDrainsAdmittedPrefixAndCloseIsIdempotent(t *testing.T) {
	w, box, tree := markdownWriterFixture(t)
	ctx, cancel := context.WithCancel(w.preparation.ctx)
	w.preparation.ctx = ctx
	if err := w.write(ctx, box, tree); err != nil {
		t.Fatal(err)
	}
	cancel()
	later := treenode.NewTree(box, "/"+ast.NewNodeID()+".docj", "/Later", "Later")
	if err := w.write(ctx, box, later); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := w.close(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := w.queue.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE id=?", tree.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	assertMarkdownWriterIndexCount(t, w, later.ID, 0)
	if err := w.close(); err != nil {
		t.Fatal(err)
	}
}
func TestMarkdownWriterCloseAndOwnerErrorsArePreserved(t *testing.T) {
	w, box, tree := markdownWriterFixture(t)
	sentinel := errors.New("writer close failed")
	closeOriginal := w.closeIO
	calls := 0
	w.closeIO = func() error { calls++; return errors.Join(closeOriginal(), sentinel) }
	if err := w.write(context.Background(), box, tree); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(w.close(), sentinel) || !errors.Is(w.close(), sentinel) || calls != 1 {
		t.Fatal("close result lost", calls)
	}
	if err := w.write(context.Background(), box, tree); err == nil {
		t.Fatal("closed root reused")
	}
}
func TestMarkdownWriterActualImportFailureNoCompletionThenIndependentSuccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	m, box, folder := markdownSourceFixture(t)
	r := m.archive.binding.scope.commit.r
	before := map[string]bool{}
	if err := filepath.WalkDir(filepath.Join(r.data, box), func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			before[p] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(r.data, box)
	if err := os.Chmod(target, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0755)
	r.owner.syncRuntime.same.Store(7)
	if err := m.importFromLocalPath(box, filepath.Join(folder, "From.md"), "/", "", true); !errors.Is(err, os.ErrPermission) || r.owner.syncRuntime.same.Load() != 7 {
		t.Fatal(err)
	}
	if m.trees != nil || m.links != nil {
		t.Fatal("failed candidates retained")
	}
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatal(err)
	}
	fresh, err := captureMarkdownImportPreparation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.importFromLocalPath(box, filepath.Join(folder, "From.md"), "/", "", true); err != nil {
		t.Fatal(err)
	}
	var found int
	err = filepath.WalkDir(target, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && !before[p] && path.Ext(p) == ".docj" {
			found++
		}
		return nil
	})
	if err != nil || found != 1 {
		t.Fatal(found, err)
	}
}

func TestMarkdownWriterGuardPinsOwnerDataAndNotebookRoot(t *testing.T) {
	for _, kind := range []string{"owner", "readonly", "data-root", "notebook-root"} {
		t.Run(kind, func(t *testing.T) {
			w, box, tree := markdownWriterFixture(t)
			r := w.preparation.archive.binding.scope.commit.r
			switch kind {
			case "owner":
				old := Conf
				Conf = NewAppConf()
				defer func() { Conf = old }()
			case "readonly":
				r.owner.m.Lock()
				r.owner.persistenceReadOnly = true
				r.owner.m.Unlock()
				defer func() { r.owner.m.Lock(); r.owner.persistenceReadOnly = false; r.owner.m.Unlock() }()
			case "data-root", "notebook-root":
				folder := r.data
				if kind == "notebook-root" {
					folder = filepath.Join(folder, box)
				}
				if err := os.Rename(folder, folder+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(folder, 0755); err != nil {
					t.Fatal(err)
				}
				defer func() { os.Remove(folder); os.Rename(folder+"-old", folder) }()
			}
			if err := w.write(context.Background(), box, tree); err == nil {
				t.Fatal("stale owner/root write succeeded")
			}
		})
	}
}
