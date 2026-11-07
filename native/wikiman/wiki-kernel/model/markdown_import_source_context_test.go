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

	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func markdownSourceFixture(t *testing.T) (*markdownImportPreparation, string, string) {
	t.Helper()
	d, box, _, _ := markdownZIPFixture(t)
	folder := filepath.Join(t.TempDir(), "Knowledge")
	if err := os.MkdirAll(filepath.Join(folder, "Sub"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"From.md": "[[Target]]", "Target.markdown": "target", "Sub.md": "matching sibling", "Sub/Child.md": "child", ".hidden.md": "hidden"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return d.preparation, box, folder
}
func TestMarkdownSourceRootedInventoryReadAndOriginalOrder(t *testing.T) {
	m, _, folder := markdownSourceFixture(t)
	s, err := m.openLocalSource(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	var names []string
	if err = s.walk(func(current string, d fs.DirEntry, _ error) error { names = append(names, d.Name()); return nil }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "Knowledge,.hidden.md,From.md,Sub,Child.md,Sub.md,Target.markdown" {
		t.Fatal(names)
	}
	if data, err := s.read(filepath.Join(s.local, "From.md")); err != nil || string(data) != "[[Target]]" {
		t.Fatal(string(data), err)
	}
	if got, err := s.hasMarkdown(s.local); err != nil || !got {
		t.Fatal(got, err)
	}
	if got, err := s.siblingMarkdown(filepath.Join(s.local, "Sub")); err != nil || !got {
		t.Fatal(got, err)
	}
	if err := os.WriteFile(s.local+".md", []byte("outside sibling"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.siblingMarkdown(s.local); err != nil || got {
		t.Fatal("source root must not probe its outside sibling", got, err)
	}
	if _, err = s.read(filepath.Join(s.local, "../outside.md")); !errors.Is(err, indexrepair.ErrSourcePath) {
		t.Fatal(err)
	}
	if _, err = s.read(filepath.Join(s.local, "Sub")); !errors.Is(err, indexrepair.ErrSourcePath) {
		t.Fatal(err)
	}
}

func TestMarkdownSourceCheckedWalkAndManifestFailures(t *testing.T) {
	primary := errors.New("markdown source walk incomplete")
	for _, kind := range []string{"walk-incomplete", "scan-replaced"} {
		t.Run(kind, func(t *testing.T) {
			m, _, folder := markdownSourceFixture(t)
			from := filepath.Join(folder, "From.md")
			before, readErr := os.ReadFile(from)
			if readErr != nil {
				t.Fatal(readErr)
			}
			closes := 0
			m.archive.closeRoot = func(root *os.Root) error {
				closes++
				return root.Close()
			}
			op := archiveScratchWalkIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name == "Sub" {
					if kind == "walk-incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					if err := source.Data.Rename("From.md", "From.md.old"); err != nil {
						return nil, err
					}
					if err := source.Data.WriteFile("From.md", []byte("replacement"), 0600); err != nil {
						return nil, err
					}
				}
				return source.ReadDir(ctx, name)
			}}
			source, err := m.openLocalSourceWithIO(folder, op)
			if source != nil || err == nil {
				if source != nil {
					source.close()
				}
				t.Fatal("failed source scan delivered inventory", source, err)
			}
			if closes != 1 {
				t.Fatal("failed source root close count", closes)
			}
			if kind == "walk-incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete walk error lost or admitted as missing", err)
				}
				after, readErr := os.ReadFile(from)
				if readErr != nil || !bytes.Equal(before, after) {
					t.Fatal("failed walk changed source", readErr)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("scan-time replacement accepted", err)
			}
		})
	}
}

func TestMarkdownSourceRejectsLinksSpecialMissingAndReplacement(t *testing.T) {
	for _, kind := range []string{"selected-link", "nested-link", "missing", "root-replaced", "file-replaced", "modified", "file-link-after-scan"} {
		t.Run(kind, func(t *testing.T) {
			m, _, folder := markdownSourceFixture(t)
			selected := folder
			if kind == "selected-link" {
				selected = folder + "-link"
				if err := os.Symlink(folder, selected); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "nested-link" {
				if err := os.Symlink(filepath.Join(folder, "From.md"), filepath.Join(folder, "alias.md")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "missing" {
				selected = filepath.Join(folder, "missing.md")
			}
			s, err := m.openLocalSource(selected)
			if kind == "selected-link" || kind == "nested-link" || kind == "missing" {
				if err == nil {
					s.close()
					t.Fatal("unsafe source accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.close()
			file := filepath.Join(s.local, "From.md")
			switch kind {
			case "root-replaced":
				if err := os.Rename(s.local, s.local+"-old"); err != nil {
					t.Fatal(err)
				}
				os.Mkdir(s.local, 0700)
			case "file-replaced":
				os.Rename(file, file+"-old")
				os.WriteFile(file, []byte("replacement"), 0600)
			case "modified":
				os.WriteFile(file, []byte("different-size source"), 0600)
			case "file-link-after-scan":
				os.Remove(file)
				os.Symlink(filepath.Join(s.local, "Target.markdown"), file)
			}
			if _, err = s.read(file); err == nil {
				t.Fatal("changed source read succeeded")
			}
		})
	}
}
func TestMarkdownSourceCancellationOwnerAndCheckedClose(t *testing.T) {
	m, _, folder := markdownSourceFixture(t)
	s, err := m.openLocalSource(folder)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("source root close failed")
	count := 0
	s.closeRoot = func(root *os.Root) error { count++; return errors.Join(root.Close(), sentinel) }
	if !errors.Is(s.close(), sentinel) || !errors.Is(s.close(), sentinel) || count != 1 {
		t.Fatal("close not checked/idempotent", count)
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.ctx = ctx
	cancel()
	if _, err = m.openLocalSource(folder); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestMarkdownTargetCapturedRegistrySourceAndFailurePropagation(t *testing.T) {
	for _, kind := range []string{"root", "child", "missing", "invalid-box", "escape", "missing-source", "wrong-id", "query-error", "close-error", "box-link", "doc-link"} {
		t.Run(kind, func(t *testing.T) {
			m, box, _ := markdownSourceFixture(t)
			r := m.archive.binding.scope.commit.r
			docs, err := r.registry.Query(m.ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "d"})
			if err != nil || len(docs) == 0 {
				t.Fatal(docs, err)
			}
			row := docs[0]
			to := row.Path
			switch kind {
			case "root":
				to = "/"
			case "missing":
				to = "/20261101090000-absent1.docj"
			case "invalid-box":
				box = "../bad"
			case "escape":
				to = "/../escape.docj"
			case "missing-source":
				if err := os.Remove(filepath.Join(r.data, box, to)); err != nil {
					t.Fatal(err)
				}
			case "wrong-id":
				if err := os.WriteFile(filepath.Join(r.data, box, to), []byte(`{"ID":"20261101090000-wrongid","Type":"NodeDocument"}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "query-error":
				if _, err := r.registry.DB.Exec("ALTER TABLE blocktrees RENAME TO source_probe_hidden"); err != nil {
					t.Fatal(err)
				}
				defer r.registry.DB.Exec("ALTER TABLE source_probe_hidden RENAME TO blocktrees")
			case "box-link":
				boxFolder := filepath.Join(r.data, box)
				if err := os.Rename(boxFolder, boxFolder+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(boxFolder+"-original", boxFolder); err != nil {
					t.Fatal(err)
				}
			case "doc-link":
				file := filepath.Join(r.data, box, to)
				if err := os.Rename(file, file+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(file+"-original", file); err != nil {
					t.Fatal(err)
				}
			case "close-error":
				m.archive.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), errors.New("target close failure")) }
			}
			h, p, err := m.localTarget(box, to)
			if kind == "root" {
				if err != nil || h != "/" || p != "/" {
					t.Fatal(h, p, err)
				}
			} else if kind == "child" {
				if err != nil || h != row.HPath || p != strings.TrimSuffix(row.Path, ".docj") {
					t.Fatal(h, p, err)
				}
			} else if err == nil || h != "" || p != "" {
				t.Fatal("target failure became success", h, p, err)
			}
		})
	}
}
func TestMarkdownSourceActualNestedImportAndMissingTargetNoSuccess(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	owner := m.archive.binding.scope.commit.r.owner
	owner.syncRuntime.same.Store(7)
	if err := m.importFromLocalPath(box, folder, "/20261101090000-absent1.docj", "", true); err == nil || owner.syncRuntime.same.Load() != 7 {
		t.Fatal("missing target completed", err)
	}
	m2, err := captureMarkdownImportPreparation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = m2.importFromLocalPath(box, folder, "/", "", true); err != nil {
		t.Fatal(err)
	}
	titles := map[string]int{}
	err = filepath.WalkDir(filepath.Join(m.archive.binding.scope.commit.r.data, box), func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(file, ".docj") {
			b, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			for _, title := range []string{"Knowledge", "From", "Target", "Sub", "Child", ".hidden"} {
				if bytes.Contains(b, []byte(`"title":"`+title+`"`)) {
					titles[title]++
				}
			}
		}
		return nil
	})
	if err != nil || titles["Knowledge"] != 1 || titles["From"] != 1 || titles["Target"] != 1 || titles["Sub"] != 1 || titles["Child"] != 1 || titles[".hidden"] != 0 {
		t.Fatal(titles, err)
	}
}

func TestMarkdownSourceCloseFailureBlocksCompletion(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	sentinel := errors.New("selected source close failure")
	calls := 0
	m.archive.closeRoot = func(root *os.Root) error {
		err := root.Close()
		if filepath.Base(root.Name()) == "Knowledge" {
			calls++
			return errors.Join(err, sentinel)
		}
		return err
	}
	owner := m.archive.binding.scope.commit.r.owner
	owner.syncRuntime.same.Store(7)
	err := m.importFromLocalPath(box, filepath.Join(folder, "From.md"), "/", "", true)
	if !errors.Is(err, sentinel) || calls != 1 || owner.syncRuntime.same.Load() != 7 || m.trees != nil || m.links != nil {
		t.Fatal("source close failed but completed", err, calls)
	}
}

func TestMarkdownTargetDirectoryImportKeepsParentHPath(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	r := m.archive.binding.scope.commit.r
	docs, err := r.registry.Query(m.ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "d"})
	if err != nil || len(docs) == 0 {
		t.Fatal(docs, err)
	}
	parent := docs[0]
	if err := m.importFromLocalPath(box, folder, parent.Path, "", true); err != nil {
		t.Fatal(err)
	}
	rows, err := r.registry.Query(m.ctx, indexdb.BlockTreeFilter{Kind: "root-hpath", BoxID: box, Value: path.Join(parent.HPath, "Knowledge")})
	if err != nil || len(rows) != 1 || !strings.HasPrefix(rows[0].Path, strings.TrimSuffix(parent.Path, ".docj")+"/") {
		t.Fatal("directory root lost target parent", rows, err)
	}
}
