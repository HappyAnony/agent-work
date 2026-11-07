//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute/ast"

	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func TestRepairSourceBindingRejectsStaleRevisionReadonlyAndOwner(t *testing.T) {
	c, r, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	p := "/" + filepath.Base(file)
	tr, e := r.loadRepairTree(ctx, box, p, util.NewLute())
	if e != nil {
		t.Fatal(e)
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		t.Fatal(e)
	}
	defer closeIO()
	before, _ := os.ReadFile(file)
	changed := append(bytes.Clone(before), ' ')
	os.WriteFile(file, changed, 0600)
	tr.Root.SetIALAttr("title", "must not overwrite concurrent edit")
	if e = r.writeRepairTree(ctx, ioctx, tr, util.NewLute()); !errors.Is(e, indexrepair.ErrSourceChanged) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(changed, after) {
		t.Fatal("concurrent source overwritten")
	}
	c.m.Lock()
	c.persistenceReadOnly = true
	c.m.Unlock()
	if _, _, e = r.openRepairIOChecked(ctx, true); e == nil {
		t.Fatal("readonly authorized source repair")
	}
	c.m.Lock()
	c.persistenceReadOnly = false
	c.m.Unlock()
	old := util.WorkspaceDir
	util.WorkspaceDir = t.TempDir()
	_, _, e = r.openRepairIOChecked(ctx, true)
	util.WorkspaceDir = old
	if e == nil {
		t.Fatal("stale archive identity accepted")
	}
	recordCloudBusinessEvidence(t, "repair-source-revision.json", map[string]interface{}{"mode": "compat-repair-source", "staleRevisionRejected": true, "readonlyRejected": true, "archiveIdentityRejected": true})
}
func TestRepairSourceStageArchiveFailureRetryAndLegacyHistory(t *testing.T) {
	_, r, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Dir(file)
	bad := filepath.Join(box, "bad.docj")
	payload := []byte("invalid filename fixture")
	os.WriteFile(bad, payload, 0600)
	outside := t.TempDir()
	archive := filepath.Join(r.workspace, "corrupted")
	if e := os.Symlink(outside, archive); e != nil {
		t.Fatal(e)
	}
	if e := r.resetDuplicateBlocksOnFileSys(ctx); e == nil {
		t.Fatal("archive failure swallowed")
	}
	if b, e := os.ReadFile(bad); e != nil || !bytes.Equal(b, payload) {
		t.Fatal("source removed after archive failure", e)
	}
	os.Remove(archive)
	history := filepath.Join(box, ".siyuan", "history")
	os.MkdirAll(history, 0700)
	os.WriteFile(filepath.Join(history, "old"), []byte("history"), 0600)
	if e := r.resetDuplicateBlocksOnFileSys(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(bad); !os.IsNotExist(e) {
		t.Fatal("invalid file not archived", e)
	}
	if _, e := os.Stat(history); !os.IsNotExist(e) {
		t.Fatal("legacy history not removed", e)
	}
	found := false
	filepath.WalkDir(archive, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Name() == "bad.docj" {
			b, e := os.ReadFile(p)
			if e == nil && bytes.Equal(b, payload) {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Fatal("archive payload missing")
	}
	recordCloudBusinessEvidence(t, "repair-source-archive.json", map[string]interface{}{"mode": "compat-repair-source", "archiveFailurePreservesSource": true, "retryArchived": true, "legacyHistoryRemoved": true})
}
func TestIndexFixCheckedEnumerationBeforeStageEffects(t *testing.T) {
	primary := errors.New("index fix enumeration incomplete")
	for _, stage := range []string{"reset", "blocktree"} {
		for _, kind := range []string{"incomplete", "replaced"} {
			t.Run(stage+"-"+kind, func(t *testing.T) {
				_, r, _, file, before := referenceModelFixture(t)
				ctx := context.Background()
				box := filepath.Base(filepath.Dir(file))
				data := filepath.Dir(filepath.Dir(file))
				badPayload := []byte("invalid filename fixture")
				bad := filepath.Join(filepath.Dir(file), "bad.docj")
				if err := os.WriteFile(bad, badPayload, 0600); err != nil {
					t.Fatal(err)
				}
				op := indexRepairWalkIO{readDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
					if name != box {
						return source.ReadDir(ctx, name)
					}
					if kind == "incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					entries, readErr := source.ReadDir(ctx, name)
					if readErr != nil {
						return nil, readErr
					}
					if err := source.Data.Rename(name, name+".old"); err != nil {
						return nil, err
					}
					if err := source.Data.Mkdir(name, 0700); err != nil {
						return nil, err
					}
					return entries, nil
				}}
				var err error
				if stage == "reset" {
					err = r.resetDuplicateBlocksOnFileSysWithIO(ctx, op)
				} else {
					err = r.fixBlockTreeByFileSysWithIO(ctx, op)
				}
				if err == nil {
					t.Fatal("failed enumeration admitted index repair stage")
				}
				root := filepath.Join(data, box)
				if kind == "incomplete" {
					if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
						t.Fatal("incomplete enumeration error lost or admitted as missing", err)
					}
				} else {
					if !errors.Is(err, indexrepair.ErrSourceChanged) {
						t.Fatal("replaced notebook accepted", err)
					}
					root += ".old"
				}
				got, readErr := os.ReadFile(filepath.Join(root, filepath.Base(file)))
				if readErr != nil || !bytes.Equal(got, before) {
					t.Fatal("source document changed after failed enumeration", readErr)
				}
				got, readErr = os.ReadFile(filepath.Join(root, "bad.docj"))
				if readErr != nil || !bytes.Equal(got, badPayload) {
					t.Fatal("invalid document archived before complete enumeration", readErr)
				}
			})
		}
	}
}
func TestRepairAVRelationCorruptionFailsWithoutOverwrite(t *testing.T) {
	_, r, _, _, _ := referenceModelFixture(t)
	ctx := context.Background()
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		t.Fatal(e)
	}
	defer closeIO()
	name := "storage/av/blocks.msgpack"
	ioctx.Source.Data.MkdirAll("storage/av", 0700)
	before := []byte{0xc1}
	ioctx.Source.Data.WriteFile(name, before, 0600)
	if e = r.bindRepairAV(ctx, ioctx.Source, "av", "block"); e == nil {
		t.Fatal("malformed AV relation accepted")
	}
	after, _ := ioctx.Source.Data.ReadFile(name)
	if !bytes.Equal(before, after) {
		t.Fatal("malformed AV overwritten")
	}
	ioctx.Source.Data.Remove(name)
	if e = r.bindRepairAV(ctx, ioctx.Source, "av", "block"); e != nil {
		t.Fatal(e)
	}
	if e = r.bindRepairAV(ctx, ioctx.Source, "av", "block"); e != nil {
		t.Fatal(e)
	}
	recordCloudBusinessEvidence(t, "repair-source-av.json", map[string]interface{}{"mode": "compat-repair-source", "malformedAVPreserved": true, "explicitAVBinding": true})
}

func TestRepairResetRetainsInternalReferencesAndPropagatesBindingFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			old := "20261029150000-oldroot"
			tree := treenode.NewTree("20261029150001-boxaaaa", "/"+old+".docj", "/Original", "Original")
			paragraph := tree.Root.FirstChild
			if paragraph == nil {
				paragraph = treenode.NewParagraph("")
				tree.Root.AppendChild(paragraph)
			}
			ref := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: paragraph.ID, TextMarkBlockRefSubtype: "d", TextMarkTextContent: "Original"}
			paragraph.AppendChild(ref)
			view := &ast.Node{Type: ast.NodeAttributeView, ID: "20261029150002-viewaaa", AttributeViewID: "fixture-av"}
			tree.Root.AppendChild(view)
			tree.Root.SetIALAttr("custom-avs", "fixture-av")
			tree.Root.SetIALAttr("scroll", "offset")
			sentinel := errors.New("binding failed")
			counter := 0
			bound := false
			err := resetTreeWithServices(context.Background(), tree, "Copy", true, resetTreeServices{NewID: func() string { counter++; return fmt.Sprintf("202610291501%02d-newnode", counter) }, Now: func() time.Time { return time.Unix(0, 0) }, Untitled: func() string { return "Untitled" }, Bind: func(_ context.Context, a, b string) error {
				bound = a == "fixture-av" && b == view.ID
				if fail {
					return sentinel
				}
				return nil
			}})
			if fail && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
			if !bound || ref.TextMarkBlockRefID != paragraph.ID || tree.ID == old || !strings.HasSuffix(tree.Path, tree.ID+".docj") || tree.Root.IALAttr("scroll") != "" {
				t.Fatal("original reset behavior lost")
			}
			if !fail && (tree.Root.IALAttr("custom-avs") != "" || !strings.HasPrefix(tree.Root.IALAttr("title"), "Original (Copy")) {
				t.Fatal("original title/binding rules lost")
			}
		})
	}
}
