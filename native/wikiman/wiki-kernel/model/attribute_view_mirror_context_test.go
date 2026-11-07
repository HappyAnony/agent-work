//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func mirrorFile(t *testing.T, file string, rows map[string][]string) []byte {
	t.Helper()
	raw, e := msgpack.Marshal(rows)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return raw
}
func mirrorRows(t *testing.T, file string) map[string][]string {
	t.Helper()
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var rows map[string][]string
	if e = msgpack.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	return rows
}
func TestAVMirrorContextOriginalRegistrationAndRemovalRules(t *testing.T) {
	tx, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	reg := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	b := legacyAVBindingForTransaction(tx)
	one, two := ast.NewNodeID(), ast.NewNodeID()
	if yes, e := b.isMirror(a.ID); e != nil || yes {
		t.Fatal(yes, e)
	}
	if changed, e := b.removeMirror(tx, a.ID, one); e != nil || changed {
		t.Fatal(changed, e)
	}
	if _, e := os.Stat(reg); !os.IsNotExist(e) {
		t.Fatal("absent removal created registry", e)
	}
	for _, c := range []struct {
		id   string
		want bool
	}{{one, false}, {two, true}, {two, false}} {
		if changed, e := b.registerMirror(a.ID, c.id); e != nil || changed != c.want {
			t.Fatal(changed, e, c)
		}
	}
	// A raw registration counts even before its document is mounted/indexed.
	if yes, e := b.isMirror(a.ID); e != nil || !yes {
		t.Fatal(yes, e)
	}
	mirrorFile(t, reg, map[string][]string{a.ID: {one, one}})
	if changed, e := b.registerMirror(a.ID, one); e != nil || !changed {
		t.Fatal("original duplicate-length rule", changed, e)
	}
	row := a.GetBlockKeyValues().Values[0].BlockID
	_, tree, e := tx.attributeViewCellEnvironment().node(row)
	if e != nil {
		t.Fatal(e)
	}
	pending := ast.NewNodeID()
	tree.Root.AppendChild(&ast.Node{Type: ast.NodeParagraph, ID: pending})
	mirrorFile(t, reg, map[string][]string{a.ID: {one, row, pending, row, two}})
	if changed, e := b.removeMirror(tx, a.ID, one); e != nil || !changed {
		t.Fatal(changed, e)
	}
	if got := mirrorRows(t, reg)[a.ID]; !reflect.DeepEqual(got, []string{row, pending, row}) {
		t.Fatal("survivor order/duplicates/pending lost", got)
	}
	if _, e = b.removeMirror(tx, a.ID, row); e != nil {
		t.Fatal(e)
	}
	if _, e = b.removeMirror(tx, a.ID, pending); e != nil {
		t.Fatal(e)
	}
	rows := mirrorRows(t, reg)
	if _, ok := rows[a.ID]; !ok || rows[a.ID] != nil {
		t.Fatal("original empty key removed", rows)
	}
}
func TestAVMirrorContextBatchDedupAndConcurrentOwners(t *testing.T) {
	_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	reg := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	one, two := ast.NewNodeID(), ast.NewNodeID()
	mirrorFile(t, reg, map[string][]string{a.ID: {one, one}})
	b := newLegacyAVBinding(context.Background())
	if e := b.registerMirrors([]*ast.Node{nil, {Type: ast.NodeParagraph, ID: "ignored"}, {Type: ast.NodeAttributeView}, {Type: ast.NodeAttributeView, ID: one, AttributeViewID: a.ID}, {Type: ast.NodeAttributeView, ID: two, AttributeViewID: a.ID}}); e != nil {
		t.Fatal(e)
	}
	if got := mirrorRows(t, reg)[a.ID]; !reflect.DeepEqual(got, []string{one, two}) {
		t.Fatal(got)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		port := newLegacyAVBinding(context.Background())
		id := ast.NewNodeID()
		wg.Add(1)
		go func() { defer wg.Done(); _, e := port.registerMirror(a.ID, id); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if got := len(mirrorRows(t, reg)[a.ID]); got != 14 {
		t.Fatal("lost concurrent update", got)
	}
}
func TestAVMirrorContextCorruptionLinksAndWriteErrors(t *testing.T) {
	for _, kind := range []string{"corrupt", "null", "link", "directory", "write-denied"} {
		t.Run(kind, func(t *testing.T) {
			_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
			reg := filepath.Join(filepath.Dir(file), "blocks.msgpack")
			var before []byte
			switch kind {
			case "corrupt":
				before = []byte("bad registry")
				os.WriteFile(reg, before, 0600)
			case "null":
				before = []byte{0xc0}
				os.WriteFile(reg, before, 0600)
			case "link":
				target := filepath.Join(t.TempDir(), "external")
				before = mirrorFile(t, target, map[string][]string{})
				if e := os.Symlink(target, reg); e != nil {
					t.Fatal(e)
				}
			case "directory":
				if e := os.Mkdir(reg, 0700); e != nil {
					t.Fatal(e)
				}
			case "write-denied":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses mode bits")
				}
				before = mirrorFile(t, reg, map[string][]string{})
				dir := filepath.Dir(reg)
				if e := os.Chmod(dir, 0500); e != nil {
					t.Fatal(e)
				}
				defer os.Chmod(dir, 0700)
			}
			b := newLegacyAVBinding(context.Background())
			if _, e := b.registerMirror(a.ID, ast.NewNodeID()); e == nil {
				t.Fatal("write error hidden", kind)
			}
			if kind != "write-denied" {
				if _, e := b.isMirror(a.ID); e == nil {
					t.Fatal("read error hidden", kind)
				}
				if _, e := b.removeMirror(nil, a.ID, "missing"); e == nil {
					t.Fatal("remove error hidden", kind)
				}
				if e := b.registerMirrors(nil); e == nil {
					t.Fatal("batch error hidden", kind)
				}
			}
			if kind != "directory" {
				after, e := os.ReadFile(reg)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatal("failed operation changed registry", e)
				}
			}
		})
	}
}
func TestAVMirrorContextCanceledOldClosedAndReadonlyOwners(t *testing.T) {
	for _, kind := range []string{"cancel", "old-root", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
			reg := filepath.Join(filepath.Dir(file), "blocks.msgpack")
			before := mirrorFile(t, reg, map[string][]string{a.ID: {ast.NewNodeID()}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newLegacyAVBinding(ctx)
			switch kind {
			case "cancel":
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "closed":
				if e := Conf.CloseCloudRepo(context.Background()); e != nil {
					t.Fatal(e)
				}
			case "readonly":
				b.scope.readOnly = true
			}
			if _, e := b.registerMirror(a.ID, ast.NewNodeID()); e == nil {
				t.Fatal("write admitted")
			}
			if _, e := b.removeMirror(nil, a.ID, "missing"); e == nil {
				t.Fatal("remove admitted")
			}
			if e := b.registerMirrors(nil); e == nil {
				t.Fatal("batch admitted")
			}
			if kind != "readonly" {
				if _, e := b.isMirror(a.ID); e == nil {
					t.Fatal("read admitted")
				}
			}
			after, _ := os.ReadFile(reg)
			if !bytes.Equal(before, after) {
				t.Fatal("owner rejection changed source")
			}
		})
	}
}
func TestAVImportContextActualBindingRegistryAndLastRelation(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			tx, a, file, doc := duplicateContextFixture(t, layout)
			dest := ast.NewNodeID()
			a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "last-relation", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: dest}}})
			writeLifecycleAV(t, file, a)
			before, _ := os.ReadFile(file)
			b := legacyAVBindingForTransaction(tx)
			events := make(chan commitNotice, 4)
			b.scope.commit.publish = func(n commitNotice) { events <- n }
			mirror := ast.NewNodeID()
			tree := &parse.Tree{ID: ast.NewNodeID(), Root: &ast.Node{Type: ast.NodeDocument}}
			tree.Root.AppendChild(&ast.Node{Type: ast.NodeAttributeView, ID: mirror, AttributeViewID: a.ID})
			if e := finishImportedAttributeViewsWithEnvironment([]string{a.ID}, []*parse.Tree{tree}, importWithBinding(tx, b, context.Background())); e != nil {
				t.Fatal(e)
			}
			if got := mirrorRows(t, filepath.Join(filepath.Dir(file), "blocks.msgpack"))[a.ID]; !reflect.DeepEqual(got, []string{mirror}) {
				t.Fatal(got)
			}
			rels := mirrorRows(t, filepath.Join(filepath.Dir(file), "relations.msgpack"))
			if !reflect.DeepEqual(rels[dest], []string{a.ID}) || len(rels[a.ID]) != 0 {
				t.Fatal("last-relation projection changed", rels)
			}
			raw, _ := os.ReadFile(doc)
			if !bytes.Contains(raw, []byte(a.ID)) || bytes.Contains(raw, []byte(`"av-names"`)) {
				t.Fatal("binding projection")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("post-import upgraded original AV")
			}
			select {
			case n := <-events:
				if n.kind != "info" {
					t.Fatal(n.kind)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("import success did not publish owned info")
			}
		})
	}
}
func TestAVImportContextMissingVersusBadSourceAndOwner(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "null", "link", "cancel", "old-root", "closed"} {
		t.Run(kind, func(t *testing.T) {
			_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newLegacyAVBinding(ctx)
			e := importWithBinding(nil, b, ctx)
			switch kind {
			case "missing":
				os.Remove(file)
			case "corrupt":
				os.WriteFile(file, []byte("bad AV"), 0600)
			case "null":
				os.WriteFile(file, []byte("null"), 0600)
			case "link":
				os.Remove(file)
				os.Symlink(filepath.Join(t.TempDir(), "external"), file)
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
			}
			err := finishImportedAttributeViewsWithEnvironment([]string{a.ID}, nil, e)
			if kind == "missing" {
				if err != nil {
					t.Fatal("genuine absence no longer skipped", err)
				}
			} else if err == nil {
				t.Fatal("actual dependency failure swallowed", kind)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), "relations.msgpack")); !os.IsNotExist(err) {
				t.Fatal("failure published relations", err)
			}
		})
	}
}
func TestAVImportContextRelationFailureDoesNotAdmitSuccess(t *testing.T) {
	tx, a, file, doc := duplicateContextFixture(t, av.LayoutTypeTable)
	b := legacyAVBindingForTransaction(tx)
	rel := filepath.Join(filepath.Dir(file), "relations.msgpack")
	before := []byte("bad relations")
	os.WriteFile(rel, before, 0600)
	err := finishImportedAttributeViewsWithEnvironment([]string{a.ID}, avImportedTrees(), importWithBinding(tx, b, context.Background()))
	if err == nil {
		t.Fatal("relation failure hidden")
	}
	if Conf.repoTasks != nil {
		t.Fatal("failure admitted success tasks")
	}
	raw, _ := os.ReadFile(doc)
	if !bytes.Contains(raw, []byte(a.ID)) {
		t.Fatal("original partial binding disappeared")
	}
	after, _ := os.ReadFile(rel)
	if !bytes.Equal(before, after) {
		t.Fatal("bad relation overwritten")
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(file), "blocks.msgpack")); err != nil {
		t.Fatal("prior mirror publication unexpectedly rolled back", err)
	}
}
func TestAVImportContextUnboundAndEmptyCompletionReject(t *testing.T) {
	b := &legacyAVBinding{err: errors.New("owner unavailable")}
	e := importWithBinding(nil, b, context.Background())
	if err := finishImportedAttributeViewsWithEnvironment(nil, nil, e); err == nil {
		t.Fatal("empty finalizer ignored unavailable owner")
	}
	if err := updateBoundBlockAvsAttributeWithEnvironment(nil, e); err == nil {
		t.Fatal("empty helper ignored unavailable owner")
	}
}

func TestAVMirrorContextLookupFailureDoesNotPrune(t *testing.T) {
	_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	b := newLegacyAVBinding(context.Background())
	reg := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	before := mirrorFile(t, reg, map[string][]string{a.ID: {ast.NewNodeID(), a.GetBlockKeyValues().Values[0].BlockID}})
	db := b.scope.commit.r.registry.DB
	if _, e := db.Exec("ALTER TABLE blocktrees RENAME TO hidden_mirror_probe"); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := db.Exec("ALTER TABLE hidden_mirror_probe RENAME TO blocktrees"); e != nil {
			t.Error(e)
		}
	}()
	if changed, e := b.removeMirror(nil, a.ID, "absent"); e == nil || changed {
		t.Fatal("lookup failure became absence", changed, e)
	}
	after, _ := os.ReadFile(reg)
	if !bytes.Equal(before, after) {
		t.Fatal("failed lookup pruned registry")
	}
}
func TestAVMirrorContextExternalVersionCannotBeOverwritten(t *testing.T) {
	_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	b := newLegacyAVBinding(context.Background())
	reg := filepath.Join(filepath.Dir(file), "blocks.msgpack")
	mirrorFile(t, reg, map[string][]string{a.ID: {ast.NewNodeID()}})
	var external []byte
	err := b.mirrorRegistry(true, func(rows map[string][]string) (bool, error) {
		external = mirrorFile(t, reg, map[string][]string{a.ID: {ast.NewNodeID()}})
		rows[a.ID] = nil
		return true, nil
	})
	if err == nil {
		t.Fatal("external registry revision overwritten")
	}
	after, _ := os.ReadFile(reg)
	if !bytes.Equal(after, external) {
		t.Fatal("external data changed")
	}
}
