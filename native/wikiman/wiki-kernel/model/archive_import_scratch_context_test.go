//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func scratchAV(id, ref, block string) []byte {
	return []byte("{\n \"spec\":3,\"id\":\"" + id + "\",\"name\":\"raw " + id + " " + block + "\",\"keyValues\":[{\"key\":{\"id\":\"rel\",\"type\":\"relation\",\"relation\":{\"avID\":\"" + ref + "\",\"isTwoWay\":true}}}],\"views\":[]}")
}
func TestArchiveScratchAVOriginalRawRemapAndSpecPreserved(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	a, b, na, nb, oldBlock, newBlock := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	archiveWrite(t, root, "storage/av/"+a+".json", scratchAV(a, b, oldBlock))
	archiveWrite(t, root, "storage/av/nested/"+b+".json", scratchAV(b, a, oldBlock))
	extra := archiveWrite(t, root, "storage/av/readme", []byte("keep"))
	sequence := []string{na, nb}
	i := 0
	ids, present, err := e.remapScratchAVs(root, map[string]string{oldBlock: newBlock}, func() string { n := sequence[i]; i++; return n })
	if err != nil || !present || len(ids) != 2 {
		t.Fatal(ids, present, err)
	}
	for old, dir := range map[string]string{a: "storage/av", b: "storage/av/nested"} {
		file := filepath.Join(root, dir, ids[old]+".json")
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var candidate av.AttributeView
		if err = json.Unmarshal(raw, &candidate); err != nil || candidate.Spec != 3 || candidate.ID != ids[old] || !bytes.Contains(raw, []byte("{\n ")) || !bytes.Contains(raw, []byte(newBlock)) || bytes.Contains(raw, []byte(oldBlock)) {
			t.Fatal("raw/spec replacement", string(raw), err)
		}
		if _, err = os.Stat(filepath.Join(root, dir, old+".json")); !os.IsNotExist(err) {
			t.Fatal("old AV path retained", err)
		}
	}
	if _, err := os.Stat(extra); err != nil {
		t.Fatal("unrelated AV file changed", err)
	}
}
func TestArchiveScratchAVValidationAndCollisionsBeforeMutation(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "corrupt", "wrong-id", "duplicate-id", "source-link", "occupied-target", "duplicate-generated", "invalid-generated", "nil-generator"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			id, next := ast.NewNodeID(), ast.NewNodeID()
			raw := scratchAV(id, id, ast.NewNodeID())
			var file string
			gen := func() string { return next }
			if kind != "missing" {
				os.MkdirAll(filepath.Join(root, "storage/av"), 0755)
			}
			if kind != "missing" && kind != "empty" {
				file = archiveWrite(t, root, "storage/av/"+id+".json", raw)
			}
			switch kind {
			case "corrupt":
				os.WriteFile(file, []byte("bad JSON"), 0600)
			case "wrong-id":
				os.WriteFile(file, scratchAV(ast.NewNodeID(), id, ""), 0600)
			case "duplicate-id":
				archiveWrite(t, root, "storage/av/nested/"+id+".json", raw)
			case "source-link":
				os.Remove(file)
				outside := archiveWrite(t, t.TempDir(), "external", raw)
				os.Symlink(outside, file)
			case "occupied-target":
				os.Mkdir(filepath.Join(root, "storage/av", next+".json"), 0755)
			case "duplicate-generated":
				other := ast.NewNodeID()
				archiveWrite(t, root, "storage/av/"+other+".json", scratchAV(other, id, ""))
			case "invalid-generated":
				gen = func() string { return "../escape" }
			case "nil-generator":
				gen = nil
			}
			before, _ := os.ReadFile(file)
			ids, present, err := e.remapScratchAVs(root, nil, gen)
			if kind == "missing" || kind == "empty" {
				if err != nil || len(ids) != 0 || present != (kind == "empty") {
					t.Fatal(ids, present, err)
				}
				return
			}
			if err == nil {
				t.Fatal("bad AV admitted", kind)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("preflight mutated source", kind)
			}
		})
	}
}
func TestArchiveScratchDocumentWriteFormattingAndRename(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(map[bool]string{false: "formatted", true: "single"}[single], func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			e.binding.scope.commit.r.singleLine = single
			root := t.TempDir()
			old, next := ast.NewNodeID(), ast.NewNodeID()
			raw := mountDoc(old, "Keep title")
			archiveWrite(t, root, "nested/"+old+".docj", raw)
			engine := e.binding.scope.commit.parser()
			tree, _, err := dataparser.ParseJSON(raw, engine.ParseOptions)
			if err != nil {
				t.Fatal(err)
			}
			tree.ID = next
			tree.Root.ID = next
			tree.Root.SetIALAttr("id", next)
			tree.Path = "/nested/" + old + ".docj"
			tree.Root.Spec = ""
			if err = e.writeScratchDocuments(root, map[string]*parse.Tree{next: tree}, engine); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(root, "nested", next+".docj"))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(got, []byte("\n\t")) == single {
				t.Fatal("save formatting preference ignored")
			}
			copy, _, err := dataparser.ParseJSON(got, engine.ParseOptions)
			if err != nil || copy.ID != next || copy.Root.Spec != "1" || copy.Root.IALAttr("title") != "Keep title" {
				t.Fatal(copy, err)
			}
		})
	}
}
func TestArchiveScratchDocumentCollisionAndLinkedSourcePreserveTargets(t *testing.T) {
	for _, kind := range []string{"occupied", "link", "bad-tree", "source-directory"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			old, next := ast.NewNodeID(), ast.NewNodeID()
			raw := mountDoc(old, "Source")
			file := archiveWrite(t, root, old+".docj", raw)
			target := archiveWrite(t, root, next+".docj", []byte("keep target"))
			engine := e.binding.scope.commit.parser()
			tree, _, err := dataparser.ParseJSON(raw, engine.ParseOptions)
			if err != nil {
				t.Fatal(err)
			}
			tree.ID = next
			tree.Root.ID = next
			tree.Path = "/" + old + ".docj"
			switch kind {
			case "link":
				os.Remove(file)
				outside := archiveWrite(t, t.TempDir(), "outside", raw)
				os.Symlink(outside, file)
			case "bad-tree":
				tree.Path = "/../escape.docj"
			case "source-directory":
				os.Remove(file)
				os.Mkdir(file, 0755)
			}
			if err = e.writeScratchDocuments(root, map[string]*parse.Tree{next: tree}, engine); err == nil {
				t.Fatal("bad write admitted")
			}
			after, _ := os.ReadFile(target)
			if string(after) != "keep target" {
				t.Fatal("existing target overwritten")
			}
		})
	}
}
func TestArchiveScratchDirectoryRemapDeepAndPrefixSiblings(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	a, b, c, na, nb, nc := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	archiveWrite(t, root, a+"/"+b+"/"+c+"/file", []byte("deep"))
	archiveWrite(t, root, a+"-notes/"+b+"/file", []byte("sibling"))
	unknown := ast.NewNodeID()
	archiveWrite(t, root, unknown+"/file", []byte("unknown"))
	if err := e.renameScratchDirectories(root, map[string]string{a: na, b: nb, c: nc}); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{na + "/" + nb + "/" + nc + "/file": "deep", a + "-notes/" + nb + "/file": "sibling", unknown + "/file": "unknown"} {
		got, err := os.ReadFile(filepath.Join(root, file))
		if err != nil || string(got) != want {
			t.Fatal(file, string(got), err)
		}
	}
}
func TestArchiveScratchDirectoryFailureAndStorageCleanup(t *testing.T) {
	for _, kind := range []string{"collision", "link", "invalid-id", "cleanup-link", "cleanup-permission", "cleanup-success"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			a, b := ast.NewNodeID(), ast.NewNodeID()
			file := archiveWrite(t, root, a+"/file", []byte("source"))
			outside := archiveWrite(t, t.TempDir(), "outside", []byte("external"))
			switch kind {
			case "collision":
				archiveWrite(t, root, b+"/file", []byte("target"))
			case "link":
				os.Symlink(filepath.Dir(outside), filepath.Join(root, "linked"))
			case "invalid-id":
				b = "../escape"
			case "cleanup-link":
				os.MkdirAll(filepath.Join(root, "storage"), 0755)
				os.Symlink(outside, filepath.Join(root, "storage/linked"))
			case "cleanup-permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				archiveWrite(t, root, "storage/file", []byte("remove"))
				os.Chmod(filepath.Join(root, "storage"), 0500)
				defer os.Chmod(filepath.Join(root, "storage"), 0755)
			case "cleanup-success":
				archiveWrite(t, root, "storage/riff/file", []byte("remove"))
			}
			var err error
			if kind == "cleanup-link" || kind == "cleanup-permission" || kind == "cleanup-success" {
				err = e.removeScratchStorage(root)
			} else {
				err = e.renameScratchDirectories(root, map[string]string{a: b})
			}
			if kind == "cleanup-success" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(filepath.Join(root, "storage")); !os.IsNotExist(err) {
					t.Fatal("storage remained")
				}
				if err = e.removeScratchStorage(root); err != nil {
					t.Fatal("missing storage no-op", err)
				}
			} else if err == nil {
				t.Fatal("failed operation reported success", kind)
			}
			if _, err = os.Stat(file); err != nil {
				t.Fatal("unrelated source removed", err)
			}
			external, _ := os.ReadFile(outside)
			if string(external) != "external" {
				t.Fatal("outside target changed")
			}
		})
	}
}
func TestArchiveScratchContextCancellationAndChangedOwners(t *testing.T) {
	for _, kind := range []string{"cancel", "old-root", "closed", "readonly", "cancel-generator"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			id := ast.NewNodeID()
			raw := scratchAV(id, id, "")
			file := archiveWrite(t, root, "storage/av/"+id+".json", raw)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.ctx = ctx
			e.binding.scope.ctx = ctx
			gen := ast.NewNodeID
			switch kind {
			case "cancel":
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "closed":
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			case "cancel-generator":
				gen = func() string { cancel(); return ast.NewNodeID() }
			}
			if _, _, err := e.remapScratchAVs(root, nil, gen); err == nil {
				t.Fatal("invalid context remapped")
			}
			if err := e.removeScratchStorage(root); err == nil {
				t.Fatal("invalid context removed storage")
			}
			if err := e.renameScratchDirectories(root, nil); err == nil {
				t.Fatal("invalid context renamed")
			}
			if err := e.writeScratchDocuments(root, nil, e.binding.scope.commit.parser()); err == nil {
				t.Fatal("invalid context wrote docs")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(raw, after) {
				t.Fatal("invalid context changed scratch")
			}
		})
	}
}
func TestArchiveScratchWalkReportsMissingAndInvalidPaths(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	source, closeIO, err := e.openScratch(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	if err = e.walkScratch(source, "missing", func(string, os.DirEntry) error { return nil }); !os.IsNotExist(err) {
		t.Fatal("missing scan swallowed", err)
	}
	if _, _, err = e.remapScratchAVs(filepath.Join(root, "missing"), nil, ast.NewNodeID); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err = e.walkScratch(source, "../outside", func(string, os.DirEntry) error { return nil }); !errors.Is(err, indexrepair.ErrSourcePath) {
		t.Fatal(err)
	}
}

func TestArchiveScratchWalkLexicalDepthFirstAndSkipSemantics(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	for _, name := range []string{"a/hidden", "b/x", "b/y", "c"} {
		archiveWrite(t, root, name, nil)
	}
	source, closeIO, err := e.openScratch(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	var visited []string
	err = e.walkScratch(source, ".", func(name string, entry fs.DirEntry) error {
		visited = append(visited, name)
		switch name {
		case "a", "b/x":
			return fs.SkipDir
		}
		return nil
	})
	want := []string{".", "a", "b", "b/x", "c"}
	if err != nil || !reflect.DeepEqual(visited, want) {
		t.Fatal(visited, err)
	}
	visited = nil
	err = e.walkScratch(source, ".", func(name string, entry fs.DirEntry) error {
		visited = append(visited, name)
		if name == "b" {
			return fs.SkipAll
		}
		return nil
	})
	want = []string{".", "a", "a/hidden", "b"}
	if err != nil || !reflect.DeepEqual(visited, want) {
		t.Fatal(visited, err)
	}
}

func TestArchiveScratchIncompleteWalkPreventsMutations(t *testing.T) {
	primary := errors.New("scratch enumeration failed")
	fault := func(failAt int) archiveScratchWalkIO {
		calls := 0
		return archiveScratchWalkIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			calls++
			if calls == failAt {
				return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
			}
			return source.ReadDir(ctx, name)
		}}
	}
	t.Run("av", func(t *testing.T) {
		e, _, _ := archiveEntryFixture(t)
		root := t.TempDir()
		old, next := ast.NewNodeID(), ast.NewNodeID()
		raw := scratchAV(old, old, "")
		file := archiveWrite(t, root, "storage/av/"+old+".json", raw)
		archiveWrite(t, root, "storage/av/nested/keep", nil)
		ids, present, err := e.remapScratchAVsWithIO(root, nil, func() string { return next }, fault(2))
		if ids != nil || !present || !errors.Is(err, primary) || indexrepair.IsSourceNotExist(err) {
			t.Fatal(ids, present, err)
		}
		got, readErr := os.ReadFile(file)
		if readErr != nil || !bytes.Equal(got, raw) {
			t.Fatal("AV changed after incomplete walk", readErr)
		}
		if _, statErr := os.Stat(filepath.Join(root, "storage/av", next+".json")); !os.IsNotExist(statErr) {
			t.Fatal("generated AV was published", statErr)
		}
	})
	t.Run("directory-rename", func(t *testing.T) {
		e, _, _ := archiveEntryFixture(t)
		root := t.TempDir()
		old, next := ast.NewNodeID(), ast.NewNodeID()
		file := archiveWrite(t, root, old+"/file", []byte("keep"))
		err := e.renameScratchDirectoriesWithIO(root, map[string]string{old: next}, fault(2))
		if !errors.Is(err, primary) || indexrepair.IsSourceNotExist(err) {
			t.Fatal(err)
		}
		if got, readErr := os.ReadFile(file); readErr != nil || string(got) != "keep" {
			t.Fatal("directory renamed after incomplete walk", readErr)
		}
	})
	t.Run("storage-remove", func(t *testing.T) {
		e, _, _ := archiveEntryFixture(t)
		root := t.TempDir()
		file := archiveWrite(t, root, "storage/file", []byte("keep"))
		err := e.removeScratchStorageWithIO(root, fault(1))
		if !errors.Is(err, primary) || indexrepair.IsSourceNotExist(err) {
			t.Fatal(err)
		}
		if got, readErr := os.ReadFile(file); readErr != nil || string(got) != "keep" {
			t.Fatal("storage removed after incomplete walk", readErr)
		}
	})
}

func TestArchiveScratchDeniedScanIsReturned(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	denied := filepath.Join(root, "denied")
	if err := os.Mkdir(denied, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(denied, 0755)
	source, closeIO, err := e.openScratch(root)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	if err = e.walkScratch(source, ".", func(string, os.DirEntry) error { return nil }); !errors.Is(err, fs.ErrPermission) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) {
		t.Fatal("actual scan error hidden", err)
	}
}
