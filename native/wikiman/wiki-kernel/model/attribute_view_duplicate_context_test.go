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
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func duplicateContextFixture(t *testing.T, layout av.LayoutType) (*Transaction, *av.AttributeView, string, string) {
	t.Helper()
	c, tx, a, file := deepAVFixture(t, layout)
	a.Spec = 3
	a.Name = "Original " + a.ID
	a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "self", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: a.ID, IsTwoWay: true, BackKeyID: "back"}}}, &av.KeyValues{Key: &av.Key{ID: "text", Type: av.KeyTypeText}, Values: []*av.Value{{ID: "text-value", KeyID: "text", BlockID: a.GetBlockKeyValues().Values[0].BlockID, Type: av.KeyTypeText, Text: &av.ValueText{Content: "source id " + a.ID}}}})
	writeLifecycleAV(t, file, a)
	id := a.GetBlockKeyValues().Values[0].BlockID
	n, tree, err := tx.attributeViewCellEnvironment().node(id)
	if err != nil {
		t.Fatal(err)
	}
	n.SetIALAttr(av.NodeAttrNameAvs, a.ID)
	io, closeIO, err := c.r.openRepairIOChecked(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	if err = c.r.writeRepairTree(context.Background(), io, tree, c.parser()); err != nil {
		t.Fatal(err)
	}
	return tx, a, file, filepath.Join(util.DataDir, tree.Box, tree.Path)
}
func TestAVDuplicateContextOriginalCopyRulesAndRealBindingWrites(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			_, a, file, doc := duplicateContextFixture(t, layout)
			before, _ := os.ReadFile(file)
			id, block, err := DuplicateDatabaseBlock(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			target := loadCellRowAV(t, filepath.Join(filepath.Dir(file), id+".json"))
			if id == a.ID || block == id || target.Spec != 3 || !strings.HasPrefix(target.Name, a.Name+" (Duplicated ") || target.Views[0].LayoutType != layout {
				t.Fatal("original copy identity/name/spec/layout rules changed")
			}
			k, _ := target.GetKey("self")
			if k.Relation.AvID != id || k.Relation.IsTwoWay || k.Relation.BackKeyID != "" {
				t.Fatal("self relation/detach rules changed")
			}
			if !strings.Contains(target.KeyValues[len(target.KeyValues)-1].Values[0].Text.Content, id) {
				t.Fatal("whole JSON replacement changed")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("copy changed original AV")
			}
			raw, _ := os.ReadFile(doc)
			if !bytes.Contains(raw, []byte(id)) || bytes.Contains(raw, []byte(`"av-names"`)) {
				t.Fatal("binding not written/derived names leaked")
			}
			row := a.GetBlockKeyValues().Values[0].BlockID
			if !strings.Contains(cache.GetBlockIAL(row)[av.NodeAttrViewNames], target.Name) {
				t.Fatal("cache did not retain refreshed name")
			}
			db := sql.CaptureLegacyRepairQueries().DB
			var ial string
			if err = db.QueryRow("SELECT ial FROM blocks WHERE id=?", row).Scan(&ial); err != nil || !strings.Contains(ial, id) {
				t.Fatal("joined primary index not updated", err, ial)
			}
			var registry map[string][]string
			data, _ := os.ReadFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"))
			msgpack.Unmarshal(data, &registry)
			if !reflect.DeepEqual(registry[id], []string{block}) {
				t.Fatal("new mirror not reserved", registry)
			}
		})
	}
}
func TestAVDuplicateContextSourceSnapshotIsSingleVersion(t *testing.T) {
	_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	e := legacyAttributeViewDuplicateEnvironment()
	original, raw, err := e.source(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := *a
	changed.Name = "Later external version"
	external := writeLifecycleAV(t, file, &changed)
	e.source = func(string) (*av.AttributeView, []byte, error) { return original, raw, nil }
	id, _, err := duplicateDatabaseBlockWithEnvironment(a.ID, e)
	if err != nil {
		t.Fatal(err)
	}
	copy := loadCellRowAV(t, filepath.Join(filepath.Dir(file), id+".json"))
	if !strings.HasPrefix(copy.Name, a.Name+" (Duplicated ") {
		t.Fatal("copy mixed source versions")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(external, after) {
		t.Fatal("snapshot copy overwrote later original")
	}
}
func TestAVDuplicateContextUsedIDsAndRegistryFailures(t *testing.T) {
	for _, mode := range []string{"file", "directory", "block", "registry-key", "registry-value", "corrupt-registry", "null-registry", "linked-target"} {
		t.Run(mode, func(t *testing.T) {
			_, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
			const candidate = "20261030230000-copied1"
			e := legacyAttributeViewDuplicateEnvironment()
			target := filepath.Join(filepath.Dir(file), candidate+".json")
			registry := filepath.Join(filepath.Dir(file), "blocks.msgpack")
			id := candidate
			switch mode {
			case "file":
				os.WriteFile(target, []byte("occupied"), 0600)
			case "directory":
				os.Mkdir(target, 0700)
			case "block":
				id = a.GetBlockKeyValues().Values[0].Block.ID
			case "registry-key", "registry-value":
				rows := map[string][]string{candidate: {"reserved"}}
				if mode == "registry-value" {
					rows = map[string][]string{"other": {candidate}}
				}
				raw, _ := msgpack.Marshal(rows)
				os.WriteFile(registry, raw, 0600)
			case "corrupt-registry":
				os.WriteFile(registry, []byte("corrupt registry"), 0600)
			case "null-registry":
				os.WriteFile(registry, []byte{0xc0}, 0600)
			case "linked-target":
				external := filepath.Join(t.TempDir(), "external")
				os.WriteFile(external, []byte("external"), 0600)
				os.Symlink(external, target)
			}
			used, err := e.usedID(id)
			if strings.Contains(mode, "corrupt") || mode == "null-registry" || mode == "linked-target" {
				if err == nil {
					t.Fatal("invalid occupancy source treated as free")
				}
			} else if err != nil || !used {
				t.Fatal("occupied ID missed", mode, used, err)
			}
		})
	}
}
func TestAVDuplicateContextConcurrentReservationsDoNotMergeCollisions(t *testing.T) {
	_, _, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	a := newLegacyAVBinding(context.Background())
	b := newLegacyAVBinding(context.Background())
	const id = "20261030230001-copied1"
	blocks := []string{"20261030230002-mirror1", "20261030230003-mirror2"}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i, binding := range []*legacyAVBinding{a, b} {
		go func(i int, b *legacyAVBinding) { defer wg.Done(); errs[i] = b.reserveDuplicateIDs(nil, id, blocks[i]) }(i, binding)
	}
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("reservation collision", errs)
	}
	var rows map[string][]string
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"))
	if err := msgpack.Unmarshal(raw, &rows); err != nil || len(rows[id]) != 1 {
		t.Fatal("conflicting reservations merged", rows, err)
	}
}
func TestAVDuplicateContextCreateRacePreservesTargetAndPartialReservation(t *testing.T) {
	_, a, file, doc := duplicateContextFixture(t, av.LayoutTypeTable)
	e := legacyAttributeViewDuplicateEnvironment()
	beforeDoc, _ := os.ReadFile(doc)
	before, _ := os.ReadFile(file)
	stage := e.stage
	e.stage = func(id string, raw []byte) error {
		os.WriteFile(filepath.Join(filepath.Dir(file), id+".json"), []byte("other writer"), 0600)
		return stage(id, raw)
	}
	id, block, err := duplicateDatabaseBlockWithEnvironment(a.ID, e)
	if !errors.Is(err, indexrepair.ErrSourceChanged) {
		t.Fatal("create race not rejected", err)
	}
	target, _ := os.ReadFile(filepath.Join(filepath.Dir(file), id+".json"))
	afterDoc, _ := os.ReadFile(doc)
	after, _ := os.ReadFile(file)
	if string(target) != "other writer" || !bytes.Equal(beforeDoc, afterDoc) || !bytes.Equal(before, after) {
		t.Fatal("failed copy overwrote existing source")
	}
	var rows map[string][]string
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(file), "blocks.msgpack"))
	msgpack.Unmarshal(raw, &rows)
	if !reflect.DeepEqual(rows[id], []string{block}) {
		t.Fatal("original reservation-before-stage order changed")
	}
	Conf.repoMu.Lock()
	scheduled := Conf.repoTasks != nil
	Conf.repoMu.Unlock()
	if scheduled {
		t.Fatal("failed copy scheduled success notification")
	}
}
func TestAVDuplicateContextBindingConflictAndSQLFailurePropagate(t *testing.T) {
	for _, mode := range []string{"document-conflict", "sql-failure"} {
		t.Run(mode, func(t *testing.T) {
			_, a, file, doc := duplicateContextFixture(t, av.LayoutTypeTable)
			e := legacyAttributeViewDuplicateEnvironment()
			db := sql.CaptureLegacyRepairQueries().DB
			var external []byte
			if mode == "document-conflict" {
				load := e.bindings.node
				e.bindings.node = func(id string) (*ast.Node, *parse.Tree, error) {
					n, tree, err := load(id)
					raw, _ := os.ReadFile(doc)
					external = append(raw, '\n')
					os.WriteFile(doc, external, 0600)
					return n, tree, err
				}
			}
			if mode == "sql-failure" {
				_, err := db.Exec("CREATE TRIGGER fail_duplicate BEFORE INSERT ON blocks BEGIN SELECT RAISE(ABORT,'duplicate fixture SQL failure'); END")
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					db.Exec("DROP TRIGGER fail_duplicate")
					if err := (sql.LegacyMainQueue{DB: db}).Flush(context.Background()); err != nil {
						t.Error("retry retained queue", err)
					}
				}()
			}
			id, _, err := duplicateDatabaseBlockWithEnvironment(a.ID, e)
			if err == nil {
				t.Fatal("binding publication failure swallowed")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), id+".json")); err != nil {
				t.Fatal("expected prior AV partial save", err)
			}
			if mode == "document-conflict" {
				raw, _ := os.ReadFile(doc)
				if !bytes.Equal(raw, external) {
					t.Fatal("stale binding overwrote external source")
				}
			}
		})
	}
}
func TestAVDuplicateContextCancellationOwnerAndReadonlyRejectWrites(t *testing.T) {
	for _, mode := range []string{"cancel", "changed-owner", "closed-owner", "read-only"} {
		t.Run(mode, func(t *testing.T) {
			tx, a, file, _ := duplicateContextFixture(t, av.LayoutTypeTable)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var b *legacyAVBinding
			if mode == "read-only" {
				b = readonlyAVBinding(tx, ctx)
			} else {
				b = newLegacyAVBinding(ctx)
			}
			e := duplicateWithBinding(nil, b, ctx)
			before, _ := os.ReadFile(file)
			switch mode {
			case "cancel":
				cancel()
			case "changed-owner":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "closed-owner":
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := duplicateDatabaseBlockWithEnvironment(a.ID, e); err == nil {
				t.Fatal("invalid scope copied data")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("failed scope changed source")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), "blocks.msgpack")); !os.IsNotExist(err) {
				t.Fatal("failed scope reserved IDs")
			}
		})
	}
}
func TestAVDuplicateContextDocInfoIsOwnedAndFrozen(t *testing.T) {
	for _, closeOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "published", true: "closed"}[closeOwner], func(t *testing.T) {
			_, a, _, _ := duplicateContextFixture(t, av.LayoutTypeTable)
			b := newLegacyAVBinding(context.Background())
			events := make(chan commitNotice, 4)
			b.scope.commit.publish = func(n commitNotice) { events <- n }
			if _, _, err := duplicateDatabaseBlockWithEnvironment(a.ID, duplicateWithBinding(nil, b, context.Background())); err != nil {
				t.Fatal(err)
			}
			if closeOwner {
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
				select {
				case <-events:
					t.Fatal("closed owner published")
				case <-time.After(600 * time.Millisecond):
				}
				return
			}
			select {
			case n := <-events:
				var body map[string]interface{}
				if n.kind != "info" || json.Unmarshal(n.payload, &body) != nil || body["rootID"] == nil {
					t.Fatal("original document info projection missing", n.kind)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("owned 500ms info not published")
			}
		})
	}
}
