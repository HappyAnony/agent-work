//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func archiveEntryFixture(t *testing.T) (*ArchiveImportContext, string, []byte) {
	t.Helper()
	_, _, _, doc := duplicateContextFixture(t, av.LayoutTypeTable)
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	old := util.TempDir
	util.TempDir = filepath.Join(util.WorkspaceDir, "temp")
	t.Cleanup(func() { util.TempDir = old })
	e, err := CaptureArchiveImport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return e, doc, raw
}

type archiveMember struct {
	name string
	data []byte
	mode os.FileMode
}

func makeArchive(t *testing.T, members ...archiveMember) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "source.zip")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, m := range members {
		h := &zip.FileHeader{Name: m.name, Method: zip.Store}
		if m.mode != 0 {
			h.SetMode(m.mode)
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(m.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}
func TestArchiveImportPreparationOriginalRemapAndSourceIsolation(t *testing.T) {
	e, source, raw := archiveEntryFixture(t)
	first, second := "20261031090000-doc0001", "20261031090001-doc0002"
	paragraph := "20261031090002-para001"
	doc := map[string]interface{}{"ID": first, "Type": "NodeDocument", "Properties": map[string]string{"id": first, "title": "Imported", "icon": "../bad.png"}, "Children": []interface{}{map[string]interface{}{"ID": paragraph, "Type": "NodeParagraph", "Properties": map[string]string{"id": paragraph}, "Children": []interface{}{map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "block-ref", "TextMarkBlockRefID": second, "TextMarkBlockRefSubtype": "s", "TextMarkTextContent": "Ref"}, map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "a", "TextMarkAHref": "siyuan://blocks/" + second, "TextMarkTextContent": "Link"}}}, map[string]interface{}{"ID": "20261031090004-embed01", "Type": "NodeBlockQueryEmbed", "Children": []interface{}{map[string]interface{}{"Type": "NodeBlockQueryEmbedScript", "Data": "SELECT * FROM blocks WHERE id='" + second + "'"}}}}}
	one, _ := json.Marshal(doc)
	two := []byte(`{"ID":"` + second + `","Type":"NodeDocument","Properties":{"id":"` + second + `","title":"Child"},"Children":[{"ID":"20261031090003-para002","Type":"NodeParagraph"}]}`)
	zipPath := makeArchive(t, archiveMember{"Root/" + first + ".docj", one, 0}, archiveMember{"Root/" + first + "/" + second + ".docj", two, 0})
	before, _ := os.ReadFile(zipPath)
	p, err := e.prepare(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	scratch := p.scratch
	defer p.close()
	if len(p.ids) != 5 || len(p.trees) != 2 {
		t.Fatal("original document graph", p.ids)
	}
	for old, id := range p.ids {
		if old == id || id[:14] != old[:14] || !ast.IsNodeIDPattern(id) {
			t.Fatal("timestamp/random identity rule", old, id)
		}
	}
	tree := p.trees[p.ids[first]]
	refs, links, embeds := 0, 0, 0
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.Type == ast.NodeBlockQueryEmbedScript {
			embeds++
			if !strings.Contains(string(n.Tokens), p.ids[second]) || strings.Contains(string(n.Tokens), second) {
				t.Fatal("embedded query not remapped")
			}
		}
		if enter && n.Type == ast.NodeTextMark {
			if n.IsTextMarkType("block-ref") {
				refs++
				if n.TextMarkBlockRefID != p.ids[second] {
					t.Fatal("reference not remapped")
				}
			}
			if n.IsTextMarkType("a") {
				links++
				if n.TextMarkAHref != "siyuan://blocks/"+p.ids[second] {
					t.Fatal("link not remapped")
				}
			}
		}
		return ast.WalkContinue
	})
	if refs != 1 || links != 1 || embeds != 1 || tree.ID != tree.Root.ID || tree.Root.IALAttr("id") != tree.ID || p.trees[p.ids[second]].Path != "/"+first+"/"+second+".docj" {
		t.Fatal("original graph/path semantics changed", refs, links)
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(raw, after) {
		t.Fatal("preparation changed live source")
	}
	after, _ = os.ReadFile(zipPath)
	if !bytes.Equal(before, after) {
		t.Fatal("input archive changed")
	}
	p.close()
	if _, err = os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatal("owned extraction not removed", err)
	}
}

func TestArchiveImportPreparationCheckedWalkAndManifestFailures(t *testing.T) {
	primary := errors.New("archive preparation source incomplete")
	for _, kind := range []string{"walk-incomplete", "scan-replaced", "post-read-replaced", "partial-read"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			first, second := ast.NewNodeID(), ast.NewNodeID()
			firstRaw, secondRaw := mountDoc(first, "First"), mountDoc(second, "Second")
			file := makeArchive(t,
				archiveMember{"Root/" + first + ".docj", firstRaw, 0},
				archiveMember{"Root/" + second + ".docj", secondRaw, 0},
				archiveMember{"Root/z/keep", []byte("unrelated"), 0},
			)
			before, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatal(readErr)
			}
			op := archivePrepareIO{}
			if kind == "walk-incomplete" || kind == "scan-replaced" {
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
					if name == "z" {
						if kind == "walk-incomplete" {
							return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
						}
						original := first + ".docj"
						if err := source.Data.Rename(original, original+".old"); err != nil {
							return nil, err
						}
						if err := source.Data.WriteFile(original, mountDoc(ast.NewNodeID(), "Replacement"), 0600); err != nil {
							return nil, err
						}
					}
					return source.ReadDir(ctx, name)
				}
			} else {
				reads := 0
				op.read = func(ctx context.Context, source indexrepair.Source, name string) ([]byte, error) {
					reads++
					if kind == "partial-read" && reads == 2 {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, primary)
					}
					raw, err := source.Read(ctx, name)
					if err == nil && kind == "post-read-replaced" && reads == 1 {
						if err = source.Data.Rename(name, name+".old"); err == nil {
							err = source.Data.WriteFile(name, mountDoc(ast.NewNodeID(), "Replacement"), 0600)
						}
					}
					return raw, err
				}
			}
			prepared, err := e.prepareWithIO(file, op)
			if prepared != nil || err == nil {
				if prepared != nil {
					prepared.close()
				}
				t.Fatal("failed preparation delivered candidates", prepared, err)
			}
			if kind == "walk-incomplete" || kind == "partial-read" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete source error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("candidate replacement accepted", err)
			}
			assertNoImportScratch(t, e)
			after, readErr := os.ReadFile(file)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("input ZIP changed", readErr)
			}
		})
	}
}

func TestArchiveExtractionRootAdmissionFailures(t *testing.T) {
	primary := errors.New("archive extraction root incomplete")
	for _, kind := range []string{"read-incomplete", "selected-replaced"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			file := makeArchive(t, archiveMember{"Root/file", []byte("payload"), 0})
			before, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatal(readErr)
			}
			op := archiveExtractIO{}
			if kind == "read-incomplete" {
				op.readDir = func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error) {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
			} else {
				op.afterCapture = func(source indexrepair.Source, name string) error {
					if err := source.Data.RemoveAll(name); err != nil {
						return err
					}
					if err := source.Data.Mkdir(name, 0755); err != nil {
						return err
					}
					return source.Data.WriteFile(filepath.Join(name, "replacement"), []byte("replacement"), 0600)
				}
			}
			scratch, root, err := e.extractArchiveWithIO(file, false, op)
			if err == nil || root != "" {
				t.Fatal("failed extraction root admitted", scratch, root, err)
			}
			if kind == "read-incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete root read error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("selected root replacement accepted", err)
			}
			assertNoImportScratch(t, e)
			after, readErr := os.ReadFile(file)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("input ZIP changed", readErr)
			}
		})
	}
}

func TestArchiveImportExtractionRejectsMalformedMembersBeforePublication(t *testing.T) {
	for _, kind := range []string{"traversal", "absolute", "backslash", "link", "duplicate", "alias", "two-roots", "backup-root", "bad-document", "bad-id", "duplicate-id", "crc"} {
		t.Run(kind, func(t *testing.T) {
			e, source, raw := archiveEntryFixture(t)
			id := "20261031091000-doc0001"
			member := archiveMember{"Root/" + id + ".docj", []byte(`{"ID":"` + id + `","Type":"NodeDocument"}`), 0}
			members := []archiveMember{member}
			switch kind {
			case "traversal":
				members = append(members, archiveMember{"../escape", nil, 0})
			case "absolute":
				members = append(members, archiveMember{"/escape", nil, 0})
			case "backslash":
				members = append(members, archiveMember{`Root\escape`, nil, 0})
			case "link":
				members = append(members, archiveMember{"Root/linked", []byte("outside"), os.ModeSymlink | 0777})
			case "duplicate":
				members = append(members, member)
			case "alias":
				members = append(members, archiveMember{"Root/../alias", nil, 0})
			case "two-roots":
				members = append(members, archiveMember{"Other/file", nil, 0})
			case "backup-root":
				members[0].name = "data-20230321175442/" + id + ".docj"
			case "bad-document":
				members[0].data = []byte("bad JSON")
			case "bad-id":
				members[0].data = []byte(`{"ID":"invalid","Type":"NodeDocument"}`)
			case "duplicate-id":
				members[0].data = []byte(`{"ID":"` + id + `","Type":"NodeDocument","Children":[{"ID":"` + id + `","Type":"NodeParagraph"}]}`)
			}
			file := makeArchive(t, members...)
			if kind == "crc" {
				data, _ := os.ReadFile(file)
				at := bytes.Index(data, member.data)
				if at < 0 {
					t.Fatal("fixture payload missing")
				}
				data[at] ^= 1
				os.WriteFile(file, data, 0600)
			}
			if p, err := e.prepare(file); err == nil {
				p.close()
				t.Fatal("invalid archive accepted", kind)
			}
			leftovers, _ := filepath.Glob(filepath.Join(e.temp, "wiki-extract-*"))
			if len(leftovers) != 0 {
				t.Fatal("failed extraction leaked", leftovers)
			}
			after, _ := os.ReadFile(source)
			if !bytes.Equal(raw, after) {
				t.Fatal("failed preparation changed live data")
			}
		})
	}
}
func TestArchiveImportPreparationRejectsCanceledOldClosedAndReadonly(t *testing.T) {
	for _, kind := range []string{"cancel", "old-root", "old-temp", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			e, _, raw := archiveEntryFixture(t)
			file := makeArchive(t, archiveMember{"Root/doc.docj", raw, 0})
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.ctx = ctx
				e.binding.scope.ctx = ctx
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "old-temp":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "closed":
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			}
			if p, err := e.prepare(file); err == nil {
				p.close()
				t.Fatal("invalid owner prepared archive")
			}
			leftovers, _ := filepath.Glob(filepath.Join(e.temp, "wiki-extract-*"))
			if len(leftovers) > 0 {
				t.Fatal("rejection wrote scratch")
			}
		})
	}
}

type archiveUploadFailure struct {
	ctx         context.Context
	cancel      context.CancelFunc
	switchOwner func()
}

func (r archiveUploadFailure) Read(p []byte) (int, error) {
	if r.cancel != nil {
		r.cancel()
	}
	if r.switchOwner != nil {
		r.switchOwner()
	}
	copy(p, "partial")
	return 7, io.ErrUnexpectedEOF
}
func TestArchiveImportUploadCleanupAndUniqueRequestDirectories(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	box := "20261031092000-box0001"
	external := filepath.Join(e.temp, "same-name.zip")
	os.MkdirAll(e.temp, 0755)
	os.WriteFile(external, []byte("untouched"), 0600)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- e.ImportReader(archiveUploadFailure{}, box, "/") }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal(err)
		}
	}
	files, _ := os.ReadDir(e.temp)
	if len(files) != 1 || files[0].Name() != "same-name.zip" {
		t.Fatal("owned upload leaked or touched other request", files)
	}
	raw, _ := os.ReadFile(external)
	if string(raw) != "untouched" {
		t.Fatal("external upload touched")
	}
}
func TestArchiveImportUploadAdmissionAndChangedOwner(t *testing.T) {
	for _, kind := range []string{"bad-box", "bad-path", "cancel-during-read", "owner-during-read", "temp-link"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			box, to := "20261031092000-box0001", "/"
			var input io.Reader = strings.NewReader("bad archive")
			switch kind {
			case "bad-box":
				box = "../outside"
			case "bad-path":
				to = "/../outside"
			case "cancel-during-read":
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				e.ctx = ctx
				e.binding.scope.ctx = ctx
				input = archiveUploadFailure{cancel: cancel}
			case "owner-during-read":
				old := util.DataDir
				defer func() { util.DataDir = old }()
				target := t.TempDir()
				input = archiveUploadFailure{switchOwner: func() { util.DataDir = target }}
			case "temp-link":
				outside := t.TempDir()
				if err := os.Symlink(outside, e.temp); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.ImportReader(input, box, to); err == nil {
				t.Fatal("upload admitted")
			}
			scratch, _ := filepath.Glob(filepath.Join(e.temp, "wiki-import-*"))
			if len(scratch) > 0 {
				t.Fatal("upload not cleaned")
			}
		})
	}
}
func TestArchiveImportCanceledWaitUnlocksCapturedSyncRuntime(t *testing.T) {
	e, _, raw := archiveEntryFixture(t)
	file := makeArchive(t, archiveMember{"Root/doc.docj", raw, 0})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx = ctx
	e.binding.scope.ctx = ctx
	runtime := &Conf.syncRuntime
	runtime.lock.Lock()
	done := make(chan error, 1)
	go func() { done <- e.importFile(file, "20261031092000-box0001", "/") }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	runtime.lock.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("captured lock not released")
	}
	if runtime.running.Load() {
		t.Fatal("running state not cleared")
	}
	if !runtime.lock.TryLock() {
		t.Fatal("runtime remained locked")
	}
	runtime.lock.Unlock()
}
