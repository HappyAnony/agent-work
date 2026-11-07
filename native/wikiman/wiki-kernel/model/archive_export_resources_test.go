//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func resourceLink(dest string) map[string]interface{} {
	return map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "a", "TextMarkAHref": dest}
}
func resourceAVNode(id string) map[string]interface{} {
	return map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeAttributeView", "AttributeViewID": id}
}
func resourceView(t *testing.T, id, related, asset string) []byte {
	t.Helper()
	keys := []interface{}{map[string]interface{}{"key": map[string]interface{}{"id": "block", "type": "block"}, "values": []interface{}{}}}
	if related != "" {
		keys = append(keys, map[string]interface{}{"key": map[string]interface{}{"id": "relation", "type": "relation", "relation": map[string]interface{}{"avID": related}}})
	}
	if asset != "" {
		keys = append(keys, map[string]interface{}{"key": map[string]interface{}{"id": "asset", "type": "mAsset"}, "values": []interface{}{map[string]interface{}{"id": "cell", "mAsset": []interface{}{map[string]interface{}{"content": asset}}}}}, map[string]interface{}{"key": map[string]interface{}{"id": "url", "type": "url"}, "values": []interface{}{map[string]interface{}{"id": "url-cell", "url": map[string]interface{}{"content": "assets/url.txt"}}}})
	}
	raw, err := json.MarshalIndent(map[string]interface{}{"id": id, "spec": 1, "keyValues": keys, "opaque": map[string]interface{}{"keep": []int{1, 2}}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}
func TestArchiveExportResourcesIndexOrderAndFiltering(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	other := ast.NewNodeID()
	e.assetBoxes = append(e.assetBoxes, other)
	archiveWrite(t, util.DataDir, box+"/assets/same.txt", []byte("first"))
	archiveWrite(t, util.DataDir, other+"/assets/same.txt", []byte("last notebook"))
	archiveWrite(t, util.DataDir, other+"/nested/assets/nested.txt", []byte("nested"))
	archiveWrite(t, util.DataDir, "assets/same.txt", []byte("global"))
	archiveWrite(t, util.DataDir, "assets/.hidden", []byte("hidden"))
	archiveWrite(t, util.DataDir, box+"/node_modules/assets/ignored", []byte("ignored"))
	io, closeIO, err := e.binding.scope.commit.r.openRepairIOChecked(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	index, err := e.assetIndex(io.Source)
	if err != nil {
		t.Fatal(err)
	}
	if index["assets/same.txt"] != "assets/same.txt" || index["assets/nested.txt"] != other+"/nested/assets/nested.txt" || index["assets/.hidden"] != "" || index["assets/ignored"] != "" {
		t.Fatal(index)
	}
	os.Remove(filepath.Join(util.DataDir, "assets/same.txt"))
	index, err = e.assetIndex(io.Source)
	if err != nil || index["assets/same.txt"] != other+"/assets/same.txt" {
		t.Fatal("notebook order lost", index, err)
	}
}

func TestArchiveExportResourcesCheckedEnumerationBeforePublication(t *testing.T) {
	primary := errors.New("archive export resource enumeration incomplete")
	for _, kind := range []string{"index-incomplete", "index-replaced", "copy-incomplete", "copy-replaced"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			id := ast.NewNodeID()
			tree := exportReferenceDoc(t, e, box, id, resourceLink("assets/bundle.rtfd/"))
			early := archiveWrite(t, util.DataDir, box+"/assets/bundle.rtfd/early.txt", []byte("early"))
			archiveWrite(t, util.DataDir, box+"/assets/bundle.rtfd/z/late.txt", []byte("late"))
			lateDir := box + "/assets/bundle.rtfd/z"
			fault := func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name == lateDir {
					if strings.HasSuffix(kind, "incomplete") {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					if err := source.Data.Rename(box+"/assets/bundle.rtfd/early.txt", box+"/assets/bundle.rtfd/early.txt.old"); err != nil {
						return nil, err
					}
					if err := source.Data.WriteFile(box+"/assets/bundle.rtfd/early.txt", []byte("replacement"), 0600); err != nil {
						return nil, err
					}
				}
				return source.ReadDir(ctx, name)
			}
			op := archiveExportResourceIO{}
			if strings.HasPrefix(kind, "index-") {
				op.indexReadDir = fault
			} else {
				op.copyReadDir = fault
			}
			folder := t.TempDir()
			err := e.exportResourcesWithIO(folder, map[string]*parse.Tree{id: tree}, op)
			if err == nil {
				t.Fatal("resource enumeration failure admitted publication")
			}
			if _, statErr := os.Lstat(filepath.Join(folder, "assets/bundle.rtfd")); !os.IsNotExist(statErr) {
				t.Fatal("resource enumeration failure created target directory", statErr)
			}
			if strings.HasSuffix(kind, "incomplete") {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete resource enumeration error lost or admitted as missing", err)
				}
				if raw, readErr := os.ReadFile(early); readErr != nil || string(raw) != "early" {
					t.Fatal("failed resource enumeration changed source", readErr)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("scan-time resource replacement accepted", err)
			}
		})
	}
}

func TestArchiveExportResourcesOriginalAttachmentsCoverDirectoryEmojiAndAnnotations(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	id := ast.NewNodeID()
	tree := exportReferenceDoc(t, e, box, id, resourceLink("assets/file.PDF?download=1"), resourceLink("assets/bundle.rtfd"), resourceLink("assets/missing.png"))
	tree.Root.SetIALAttr("title-img", "background-image: url(\"assets/cover.png\")")
	tree.Root.SetIALAttr("icon", "custom.png")
	archiveWrite(t, util.DataDir, box+"/assets/file.PDF", []byte("pdf"))
	archiveWrite(t, util.DataDir, box+"/assets/file.PDF.sya", []byte("annotations"))
	archiveWrite(t, util.DataDir, "assets/cover.png", []byte("cover"))
	archiveWrite(t, util.DataDir, "emojis/custom.png", []byte("emoji"))
	archiveWrite(t, util.DataDir, box+"/assets/bundle.rtfd/.hidden", []byte("bundle hidden"))
	os.MkdirAll(filepath.Join(util.DataDir, box, "assets/bundle.rtfd/empty"), 0700)
	stamp := time.Unix(1700000000, 0)
	if err := os.Chtimes(filepath.Join(util.DataDir, box, "assets/bundle.rtfd"), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if err := e.exportResources(folder, map[string]*parse.Tree{id: tree}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"assets/file.PDF": "pdf", "assets/file.PDF.sya": "annotations", "assets/cover.png": "cover", "emojis/custom.png": "emoji", "assets/bundle.rtfd/.hidden": "bundle hidden"} {
		got, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil || string(got) != want {
			t.Fatal(name, string(got), err)
		}
	}
	if info, err := os.Stat(filepath.Join(folder, "assets/bundle.rtfd/empty")); err != nil || !info.IsDir() {
		t.Fatal("empty directory lost", err)
	}
	if info, err := os.Stat(filepath.Join(folder, "assets/bundle.rtfd")); err != nil || !info.ModTime().Equal(stamp) {
		t.Fatal("directory mtime lost", info, err)
	}
	if _, err := os.Stat(filepath.Join(folder, "assets/missing.png")); !os.IsNotExist(err) {
		t.Fatal("dangling asset synthesized", err)
	}
}
func TestArchiveExportResourcesAVFirstReadRawRelationsAndOriginalAssetPolicy(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	a, b := ast.NewNodeID(), ast.NewNodeID()
	rawA := resourceView(t, a, b, "assets/cell.pdf")
	rawB := resourceView(t, b, a, "assets/related-only.png")
	fileA := archiveWrite(t, util.DataDir, "storage/av/"+a+".json", rawA)
	fileB := archiveWrite(t, util.DataDir, "storage/av/"+b+".json", rawB)
	if _, err := e.readExportView(a); err != nil {
		t.Fatal(err)
	}
	later := resourceView(t, a, "", "assets/later.png")
	os.WriteFile(fileA, later, 0600)
	for name, body := range map[string]string{"cell.pdf": "cell", "cell.pdf.sya": "sidecar", "url.txt": "URL", "related-only.png": "not copied", "later.png": "new version"} {
		archiveWrite(t, util.DataDir, "assets/"+name, []byte(body))
	}
	tree := exportReferenceDoc(t, e, box, ast.NewNodeID(), resourceAVNode(a))
	folder := t.TempDir()
	if err := e.exportResources(folder, map[string]*parse.Tree{tree.ID: tree}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string][]byte{a: rawA, b: rawB} {
		got, _ := os.ReadFile(filepath.Join(folder, "storage/av", id+".json"))
		if !bytes.Equal(want, got) {
			t.Fatal("AV raw snapshot/Spec/opaque fields lost", id)
		}
	}
	for _, name := range []string{"cell.pdf", "cell.pdf.sya", "url.txt"} {
		if _, err := os.Stat(filepath.Join(folder, "assets", name)); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"related-only.png", "later.png"} {
		if _, err := os.Stat(filepath.Join(folder, "assets", name)); !os.IsNotExist(err) {
			t.Fatal("related-only/new revision resource copied", name, err)
		}
	}
	gotA, _ := os.ReadFile(fileA)
	gotB, _ := os.ReadFile(fileB)
	if !bytes.Equal(later, gotA) || !bytes.Equal(rawB, gotB) {
		t.Fatal("AV source migrated or rewritten")
	}
}
func TestArchiveExportResourcesFailuresPropagateAndProtectOutside(t *testing.T) {
	for _, kind := range []string{"source-link", "global-link", "source-permission", "output-link", "output-parent-link", "output-directory", "output-permission", "sya-link", "missing-emoji", "bad-av", "linked-av", "nil-asset", "bad-asset-path", "bad-emoji-path"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			id := ast.NewNodeID()
			node := resourceLink("assets/file.pdf")
			avID := ast.NewNodeID()
			if kind == "bad-av" || kind == "linked-av" || kind == "nil-asset" {
				node = resourceAVNode(avID)
			}
			if kind == "bad-asset-path" {
				node = resourceLink("assets/../outside")
			}
			tree := exportReferenceDoc(t, e, box, id, node)
			source := archiveWrite(t, util.DataDir, "assets/file.pdf", []byte("source"))
			folder := t.TempDir()
			outside := filepath.Join(t.TempDir(), "outside")
			os.WriteFile(outside, []byte("outside"), 0600)
			switch kind {
			case "source-link":
				os.Remove(source)
				os.Symlink(outside, source)
			case "global-link":
				os.Rename(filepath.Join(util.DataDir, "assets"), filepath.Join(util.DataDir, "asset-original"))
				os.Symlink(filepath.Dir(outside), filepath.Join(util.DataDir, "assets"))
			case "source-permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(source, 0000)
				defer os.Chmod(source, 0600)
			case "output-link":
				os.MkdirAll(filepath.Join(folder, "assets"), 0700)
				os.Symlink(outside, filepath.Join(folder, "assets/file.pdf"))
			case "output-parent-link":
				os.Symlink(filepath.Dir(outside), filepath.Join(folder, "assets"))
			case "output-directory":
				os.MkdirAll(filepath.Join(folder, "assets/file.pdf"), 0700)
			case "output-permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(folder, 0500)
				defer os.Chmod(folder, 0700)
			case "sya-link":
				os.Symlink(outside, source+".sya")
			case "missing-emoji":
				tree.Root.SetIALAttr("icon", "missing.png")
			case "bad-av":
				archiveWrite(t, util.DataDir, "storage/av/"+avID+".json", []byte("bad JSON"))
			case "linked-av":
				p := archiveWrite(t, util.DataDir, "storage/av/"+avID+".json", []byte("before"))
				os.Remove(p)
				os.Symlink(outside, p)
			case "nil-asset":
				archiveWrite(t, util.DataDir, "storage/av/"+avID+".json", []byte(`{"id":"`+avID+`","keyValues":[{"key":{"type":"mAsset"},"values":[{"mAsset":[null]}]}]}`))
			case "bad-emoji-path":
				tree.Root.SetIALAttr("icon", "../../outside")
			}
			if err := e.exportResources(folder, map[string]*parse.Tree{id: tree}); err == nil {
				t.Fatal("resource failure swallowed", kind)
			}
			after, _ := os.ReadFile(outside)
			if string(after) != "outside" {
				t.Fatal("outside file modified")
			}
		})
	}
}
func TestArchiveExportResourcesCanceledChangedClosedReadonly(t *testing.T) {
	for _, kind := range []string{"cancel", "old-data", "old-temp", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			id := ast.NewNodeID()
			tree := exportReferenceDoc(t, e, box, id)
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.binding.scope.ctx = ctx
				cancel()
			case "old-data":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "old-temp":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "closed":
				Conf.CloseCloudRepo(context.Background())
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			}
			if err := e.exportResources(t.TempDir(), map[string]*parse.Tree{id: tree}); err == nil {
				t.Fatal("invalid owner copied resources", kind)
			}
			if _, err := e.readExportView(ast.NewNodeID()); err == nil {
				t.Fatal("invalid owner read AV", kind)
			}
		})
	}
}
func TestArchiveExportResourcesMissingAVAndBoundOnlyMAssetPolicy(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	id := ast.NewNodeID()
	tree := exportReferenceDoc(t, e, box, id, resourceAVNode(ast.NewNodeID()), resourceAVNode(""))
	a := ast.NewNodeID()
	archiveWrite(t, util.DataDir, "storage/av/"+a+".json", resourceView(t, a, "", "assets/bound.pdf"))
	archiveWrite(t, util.DataDir, "assets/bound.pdf", []byte("bound"))
	archiveWrite(t, util.DataDir, "assets/bound.pdf.sya", []byte("not selected"))
	archiveWrite(t, util.DataDir, "assets/url.txt", []byte("not selected URL"))
	tree.Root.SetIALAttr(av.NodeAttrNameAvs, a)
	folder := t.TempDir()
	if err := e.exportResources(folder, map[string]*parse.Tree{id: tree}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"assets/bound.pdf", "storage/av/" + a + ".json"} {
		if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"assets/bound.pdf.sya", "assets/url.txt"} {
		if _, err := os.Stat(filepath.Join(folder, name)); !os.IsNotExist(err) {
			t.Fatal("bound-only policy changed", name, err)
		}
	}

}
func TestArchiveExportResourcesControllerRejectsResourceFailure(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	id := ast.NewNodeID()
	exportReferenceDoc(t, e, box, id, resourceLink("assets/file"))
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("outside"), 0600)
	p := archiveWrite(t, util.DataDir, "assets/file", []byte("asset"))
	os.Remove(p)
	os.Symlink(outside, p)
	name, zip := ExportSY(id, context.Background())
	if name != id || zip != "" {
		t.Fatal("controller returned incomplete archive", name, zip)
	}
}

// Cancel on the first visible staged bytes, so the test exercises failure
// after streaming has started without racing a timer against file IO.
type resourceStreamCancelContext struct {
	context.Context
	cancel    context.CancelFunc
	folder    string
	triggered bool
}

func (c *resourceStreamCancelContext) Err() error {
	entries, _ := os.ReadDir(c.folder)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wiki-export-") {
			if info, err := entry.Info(); err == nil && info.Size() > 0 {
				c.triggered = true
				c.cancel()
			}
		}
	}
	return c.Context.Err()
}
func TestArchiveExportResourcesStreamingLargeFileAndCanceledPublication(t *testing.T) {
	e, _, _ := exportContextFixture(t)
	file := archiveWrite(t, util.DataDir, "assets/large.bin", []byte("header"))
	f, err := os.OpenFile(file, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(8 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	stamp := time.Unix(1700000000, 0)
	if err = os.Chmod(file, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	source, closeIO, err := e.binding.scope.commit.r.openRepairIOChecked(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	folder := t.TempDir()
	root, err := openArchivePublicationSource(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	target := indexrepair.Source{Data: root, Authorize: func(context.Context) error { return e.guard() }}
	if err = e.copyExportFile(source.Source, target, "assets/large.bin", "assets/large.bin"); err != nil {
		t.Fatal(err)
	}
	digest := func(name string) [32]byte {
		t.Helper()
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			t.Fatal(err)
		}
		var result [32]byte
		copy(result[:], h.Sum(nil))
		return result
	}
	output := filepath.Join(folder, "assets/large.bin")
	if info, err := os.Stat(output); err != nil || info.Mode().Perm() != 0750 || !info.ModTime().Equal(stamp) {
		t.Fatal("source mode/mtime lost", info, err)
	}
	if digest(file) != digest(output) {
		t.Fatal("streamed bytes differ")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controlled := &resourceStreamCancelContext{Context: ctx, cancel: cancel, folder: filepath.Join(folder, "assets")}
	e.binding.scope.ctx = controlled
	if err = e.copyExportFile(source.Source, target, "assets/large.bin", "assets/large.bin"); err == nil {
		t.Fatal("canceled stream published")
	}
	if !controlled.triggered {
		t.Fatal("fixture did not cancel after streaming began")
	}
	if digest(file) != digest(output) {
		t.Fatal("failed stream replaced prior output")
	}
	entries, _ := os.ReadDir(filepath.Join(folder, "assets"))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wiki-export-") {
			t.Fatal("stream temporary file leaked")
		}
	}
}
