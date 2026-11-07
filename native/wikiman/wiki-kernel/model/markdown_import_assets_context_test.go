//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/indexrepair"
)

func markdownAssetsFixture(t *testing.T, single bool) (*markdownImportAssets, string, string, []byte) {
	t.Helper()
	m, box, folder := markdownSourceFixture(t)
	payload := bytes.Repeat([]byte("image fixture bytes"), 14000)
	if err := os.MkdirAll(filepath.Join(folder, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "assets", "image.png")
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	selected := folder
	if single {
		selected = filepath.Join(folder, "From.md")
	}
	source, err := m.openLocalSource(selected)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.close() })
	a, err := source.openAssets()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.close() })
	return a, box, filepath.Join(source.folder, "assets", "image.png"), payload
}
func TestMarkdownAssetsDirectoryPriorityAndLinkRejection(t *testing.T) {
	a, box, _, _ := markdownAssetsFixture(t, false)
	doc := "/20261101090000-parent1/20261101090001-doc0001.docj"
	if folder, err := a.directory(box, doc); err != nil || folder != "assets" {
		t.Fatal(folder, err)
	}
	bookAssets := path.Join(box, "assets")
	if err := a.target.Data.MkdirAll(bookAssets, 0755); err != nil {
		t.Fatal(err)
	}
	if folder, err := a.directory(box, doc); err != nil || folder != bookAssets {
		t.Fatal(folder, err)
	}
	docAssets := path.Join(box, "20261101090000-parent1/assets")
	if err := a.target.Data.MkdirAll(docAssets, 0755); err != nil {
		t.Fatal(err)
	}
	if folder, err := a.directory(box, doc); err != nil || folder != docAssets {
		t.Fatal(folder, err)
	}
	absolute := filepath.Join(a.source.preparation.archive.binding.scope.commit.r.data, filepath.FromSlash(docAssets))
	if err := os.Remove(absolute); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), absolute); err != nil {
		t.Fatal(err)
	}
	if _, err := a.directory(box, doc); !errors.Is(err, indexrepair.ErrSourcePath) {
		t.Fatal(err)
	}
}
func TestMarkdownAssetsStreamingBytesAndLinkDeduplication(t *testing.T) {
	a, _, file, payload := markdownAssetsFixture(t, false)
	name, err := a.copy(file, "assets")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := a.target.Data.ReadFile("assets/" + name); err != nil || !bytes.Equal(data, payload) {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	copiedInfo, err := a.target.Data.Stat("assets/" + name)
	if err != nil || copiedInfo.Mode().Perm() != originalInfo.Mode().Perm() || !copiedInfo.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatal("original file metadata lost", copiedInfo, originalInfo, err)
	}
	if original, err := os.ReadFile(file); err != nil || !bytes.Equal(original, payload) {
		t.Fatal("source changed", err)
	}
	tree, _, _, _, err := a.source.preparation.parse([]byte("![picture](assets/image.png)\n\n[attachment](assets/image.png)\n\n[missing](assets/missing.png)\n\n[web](https://example.com/image.png)"))
	if err != nil {
		t.Fatal(err)
	}
	done := map[string]string{file: name}
	if err = a.rewrite(tree, filepath.Join(a.source.local, "From.md"), "assets", done); err != nil {
		t.Fatal(err)
	}
	copied := 0
	missing := false
	web := false
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			dest := n.TextMarkAHref
			if n.Type == ast.NodeLinkDest {
				dest = n.TokensStr()
			}
			if dest == "assets/"+name {
				copied++
			}
			missing = missing || dest == "assets/missing.png"
			web = web || dest == "https://example.com/image.png"
		}
		return ast.WalkContinue
	})
	if copied != 2 || len(done) != 1 || !missing || !web {
		t.Fatal(copied, done, missing, web)
	}
}
func TestMarkdownAssetsSingleFileRelativeSourcesAndInventoryLimits(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "single"}[single], func(t *testing.T) {
			a, _, file, _ := markdownAssetsFixture(t, single)
			if _, exists, err := a.source.assetEntry(file); err != nil || !exists {
				t.Fatal(exists, err)
			}
			if _, exists, err := a.source.assetEntry(filepath.Join(a.source.folder, "assets/missing.png")); err != nil || exists {
				t.Fatal(exists, err)
			}
			if _, _, err := a.source.assetEntry(filepath.Join(a.source.folder, "../outside.png")); !errors.Is(err, indexrepair.ErrSourcePath) {
				t.Fatal(err)
			}
			added := filepath.Join(a.source.folder, "assets/added.png")
			if err := os.WriteFile(added, []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			_, exists, err := a.source.assetEntry(added)
			if single {
				if err != nil || !exists {
					t.Fatal(exists, err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal(err)
			}
			alias := filepath.Join(a.source.folder, "assets/alias.png")
			if err := os.Symlink(file, alias); err != nil {
				t.Fatal(err)
			}
			if _, _, err := a.source.assetEntry(alias); !errors.Is(err, indexrepair.ErrSourcePath) {
				t.Fatal(err)
			}
		})
	}
}
func TestMarkdownAssetsPublishFailuresCloseAndCleanup(t *testing.T) {
	for _, kind := range []string{"read", "verify", "close", "cancel", "collision", "replacement", "cleanup", "escape"} {
		t.Run(kind, func(t *testing.T) {
			a, _, _, _ := markdownAssetsFixture(t, false)
			if err := a.target.Data.MkdirAll("assets", 0755); err != nil {
				t.Fatal(err)
			}
			primary := errors.New("asset read/verify failure")
			closing := errors.New("asset close failure")
			name := "owned.png"
			reader := io.Reader(strings.NewReader("owned bytes"))
			verify := func() error { return nil }
			closeInput := func() error { return nil }
			switch kind {
			case "read":
				reader = markdownZIPReadFunc(func([]byte) (int, error) { return 0, primary })
				closeInput = func() error { return closing }
			case "verify":
				verify = func() error { return primary }
			case "close":
				closeInput = func() error { return closing }
			case "cancel":
				ctx, cancel := context.WithCancel(a.source.preparation.ctx)
				defer cancel()
				a.source.preparation.ctx = ctx
				reader = markdownZIPReadFunc(func(p []byte) (int, error) { cancel(); return copy(p, "partial"), nil })
			case "collision":
				if err := a.target.Data.WriteFile("assets/"+name, []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				closeInput = func() error {
					if err := a.target.Data.Remove("assets/" + name); err != nil {
						return err
					}
					return a.target.Data.WriteFile("assets/"+name, []byte("foreign"), 0600)
				}
			case "cleanup":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				dir := filepath.Join(a.source.preparation.archive.binding.scope.commit.r.data, "assets")
				defer os.Chmod(dir, 0755)
				reader = markdownZIPReadFunc(func([]byte) (int, error) {
					if err := os.Chmod(dir, 0500); err != nil {
						return 0, err
					}
					return 0, primary
				})
			case "escape":
				name = "../escape.png"
			}
			err := a.publish("assets", name, reader, verify, closeInput)
			if err == nil {
				t.Fatal("failed asset published")
			}
			if kind == "read" && (!errors.Is(err, primary) || !errors.Is(err, closing)) {
				t.Fatal(err)
			}
			if kind == "verify" && !errors.Is(err, primary) || kind == "close" && !errors.Is(err, closing) || kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind == "cleanup" {
				if !errors.Is(err, primary) || !errors.Is(err, os.ErrPermission) {
					t.Fatal(err)
				}
				return
			}
			if kind == "replacement" || kind == "collision" {
				if data, err := a.target.Data.ReadFile("assets/owned.png"); err != nil || string(data) != "foreign" {
					t.Fatal("foreign output modified", string(data), err)
				}
			} else if _, err := a.target.Data.Lstat("assets/owned.png"); !os.IsNotExist(err) {
				t.Fatal("partial output remains", err)
			}
		})
	}
}
func TestMarkdownAssetsOwnerReadonlyPermissionsAndRootReplacement(t *testing.T) {
	for _, kind := range []string{"readonly", "permissions", "root"} {
		t.Run(kind, func(t *testing.T) {
			a, _, file, _ := markdownAssetsFixture(t, false)
			r := a.source.preparation.archive.binding.scope.commit.r
			switch kind {
			case "readonly":
				r.owner.m.Lock()
				r.owner.persistenceReadOnly = true
				r.owner.m.Unlock()
				defer func() { r.owner.m.Lock(); r.owner.persistenceReadOnly = false; r.owner.m.Unlock() }()
			case "permissions":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				if err := a.target.Data.MkdirAll("assets", 0755); err != nil {
					t.Fatal(err)
				}
				folder := filepath.Join(r.data, "assets")
				if err := os.Chmod(folder, 0500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(folder, 0755)
			case "root":
				if err := os.Rename(r.data, r.data+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(r.data, 0755); err != nil {
					t.Fatal(err)
				}
				defer func() { os.Remove(r.data); os.Rename(r.data+"-old", r.data) }()
			}
			if name, err := a.copy(file, "assets"); err == nil || name != "" {
				t.Fatal(name, err)
			}
		})
	}
}
func TestMarkdownAssetsActualEscapeFailureThenIndependentImport(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	owner := m.archive.binding.scope.commit.r.owner
	outside := filepath.Join(filepath.Dir(folder), "outside.png")
	payload := []byte("outside must remain private")
	if err := os.WriteFile(outside, payload, 0600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(folder, "bad.md")
	if err := os.WriteFile(bad, []byte("![outside](../outside.png)"), 0600); err != nil {
		t.Fatal(err)
	}
	owner.syncRuntime.same.Store(7)
	if err := m.importFromLocalPath(box, bad, "/", "", true); !errors.Is(err, indexrepair.ErrSourcePath) || owner.syncRuntime.same.Load() != 7 {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(outside); err != nil || !bytes.Equal(data, payload) {
		t.Fatal(err)
	}
	if m.trees != nil || m.links != nil {
		t.Fatal("failed candidates retained")
	}
	fresh, err := captureMarkdownImportPreparation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.importFromLocalPath(box, filepath.Join(folder, "From.md"), "/", "", true); err != nil {
		t.Fatal(err)
	}
}
func TestMarkdownAssetsTargetCloseFailureBlocksCompletion(t *testing.T) {
	m, box, folder := markdownSourceFixture(t)
	r := m.archive.binding.scope.commit.r
	sentinel := errors.New("asset root close failure")
	closes := 0
	m.archive.closeRoot = func(root *os.Root) error {
		err := root.Close()
		if root.Name() == r.data {
			closes++
			if closes == 3 {
				return errors.Join(err, sentinel)
			}
		}
		return err
	}
	r.owner.syncRuntime.same.Store(7)
	if err := m.importFromLocalPath(box, filepath.Join(folder, "From.md"), "/", "", true); !errors.Is(err, sentinel) || r.owner.syncRuntime.same.Load() != 7 || closes != 3 {
		t.Fatal(err, closes)
	}
}

func TestMarkdownAssetsDirectoryAttachmentsRetainNestedFilesAndEmptyFolders(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "single"}[single], func(t *testing.T) {
			a, _, file, payload := markdownAssetsFixture(t, single)
			// Single-file directory borrowing also preflights children not in the document inventory.
			if single {
				if err := os.MkdirAll(filepath.Join(a.source.folder, "assets/empty"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Dir(file)
			name, err := a.copy(dir, "assets")
			if err != nil {
				t.Fatal(err)
			}
			data, err := a.target.Data.ReadFile(path.Join("assets", name, "image.png"))
			if err != nil || !bytes.Equal(data, payload) {
				t.Fatal(err)
			}
			if single {
				info, err := a.target.Data.Stat(path.Join("assets", name, "empty"))
				if err != nil || !info.IsDir() {
					t.Fatal(err)
				}
			}
			if single {
				if err := os.Symlink(file, filepath.Join(dir, "alias.png")); err != nil {
					t.Fatal(err)
				}
				if _, err = a.copy(dir, "assets"); !errors.Is(err, indexrepair.ErrSourcePath) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestMarkdownAssetsDirectoryCheckedWalkFailuresBeforePublication(t *testing.T) {
	primary := errors.New("markdown asset directory walk incomplete")
	for _, kind := range []string{"walk-incomplete", "scan-replaced"} {
		t.Run(kind, func(t *testing.T) {
			a, _, file, payload := markdownAssetsFixture(t, true)
			dir := filepath.Dir(file)
			if err := os.Mkdir(filepath.Join(dir, "z"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "z", "late.bin"), []byte("late"), 0600); err != nil {
				t.Fatal(err)
			}
			entry, exists, err := a.source.assetEntry(dir)
			if err != nil || !exists || !entry.info.IsDir() {
				t.Fatal(exists, entry.info, err)
			}
			op := archiveScratchWalkIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name == "assets/z" {
					if kind == "walk-incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					if err := source.Data.Rename("assets/image.png", "assets/image.png.old"); err != nil {
						return nil, err
					}
					if err := source.Data.WriteFile("assets/image.png", []byte("replacement"), 0600); err != nil {
						return nil, err
					}
				}
				return source.ReadDir(ctx, name)
			}}
			name, err := a.copyDirectoryWithIO(dir, "assets", entry, op, "checked-dir")
			if err == nil || name != "" {
				t.Fatal("failed directory scan published target", name, err)
			}
			if _, statErr := a.target.Data.Lstat("assets/checked-dir"); !os.IsNotExist(statErr) {
				t.Fatal("scan failure created target directory", statErr)
			}
			if kind == "walk-incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete asset walk error lost or admitted as missing", err)
				}
				if after, readErr := os.ReadFile(file); readErr != nil || !bytes.Equal(after, payload) {
					t.Fatal("failed asset walk changed source", readErr)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("scan-time asset replacement accepted", err)
			}
		})
	}
}

func TestMarkdownAssetsDocumentLinksIgnoreSourceAncestorName(t *testing.T) {
	m, _, folder := markdownSourceFixture(t)
	container := filepath.Join(filepath.Dir(folder), "assets-container")
	if err := os.Mkdir(container, 0700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(container, "Knowledge")
	if err := os.Rename(folder, moved); err != nil {
		t.Fatal(err)
	}
	source, err := m.openLocalSource(moved)
	if err != nil {
		t.Fatal(err)
	}
	defer source.close()
	a, err := source.openAssets()
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	tree, _, _, _, err := m.parse([]byte("[target](Target.markdown)"))
	if err != nil {
		t.Fatal(err)
	}
	done := map[string]string{}
	if err = a.rewrite(tree, filepath.Join(source.local, "From.md"), "assets", done); err != nil {
		t.Fatal(err)
	}
	retained := false
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && (n.Type == ast.NodeLinkDest && n.TokensStr() == "Target.markdown" || n.TextMarkAHref == "Target.markdown") {
			retained = true
		}
		return ast.WalkContinue
	})
	if !retained || len(done) != 0 {
		t.Fatal("source ancestor contaminated document-link classification", done)
	}
}
