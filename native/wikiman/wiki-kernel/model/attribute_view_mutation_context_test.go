//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func TestAVMutationPortsVersionedAndRaw(t *testing.T) {
	constructors := map[string]func(context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error){
		"configuration": func(c context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error) {
			e := legacyAttributeViewEnvironment(c)
			return e.read, e.stage
		},
		"field": func(c context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error) {
			e := legacyAttributeViewFieldEnvironment(c)
			return e.read, e.stage
		},
		"option": func(c context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error) {
			e := legacyAttributeViewOptionEnvironment(c)
			return e.read, e.stage
		},
		"append": func(c context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error) {
			e := legacyAttributeViewAppendEnvironment(c)
			return e.read, e.stage
		},
		"settings": func(c context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error) {
			e := legacyAttributeViewSettingsEnvironment(c)
			return e.read, e.stage
		},
		"order": func(c context.Context) (func(string) (*av.AttributeView, error), func(*av.AttributeView) error) {
			e := legacyAttributeViewOrderEnvironment(c)
			return e.read, e.stage
		},
	}
	for name, makePorts := range constructors {
		t.Run(name, func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
			a.Spec = 3
			before := writeLifecycleAV(t, file, a)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			read, stage := makePorts(ctx)
			candidate, err := read(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(file)
			if !bytes.Equal(got, before) || candidate.Spec != 3 {
				t.Fatal("read performed eager upgrade")
			}
			candidate.Name = "saved"
			if err = stage(candidate); err != nil {
				t.Fatal(err)
			}
			var saved av.AttributeView
			got, _ = os.ReadFile(file)
			if err = json.Unmarshal(got, &saved); err != nil || saved.Name != "saved" || saved.Spec != 4 {
				t.Fatal("upgrade/save", err)
			}
			external := append(append([]byte(nil), got...), '\n')
			if err = os.WriteFile(file, external, 0600); err != nil {
				t.Fatal(err)
			}
			candidate.Name = "conflict"
			if err = stage(candidate); err == nil {
				t.Fatal("stale write accepted")
			}
			got, _ = os.ReadFile(file)
			if !bytes.Equal(got, external) {
				t.Fatal("external source overwritten")
			}
			cancel()
			if _, err = read(a.ID); !errors.Is(err, context.Canceled) {
				t.Fatal("canceled read", err)
			}
		})
	}
}

func TestAVMutationRelationRootAndErrors(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	b := newLegacyAVBinding(context.Background())
	const src = "20261030000001-source1"
	const other = "20261030000002-source2"
	relationFile := filepath.Join(filepath.Dir(file), "relations.msgpack")
	if err := b.removeRelation(src, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(relationFile); !os.IsNotExist(err) {
		t.Fatal("missing remove created index")
	}
	original := map[string][]string{a.ID: {other, src, other}, other: {src}}
	raw, _ := msgpack.Marshal(original)
	if err := os.WriteFile(relationFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.upsertRelation(src, a.ID); err != nil {
		t.Fatal(err)
	}
	var out map[string][]string
	got, _ := os.ReadFile(relationFile)
	if err := msgpack.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out[a.ID], []string{other, src}) || !reflect.DeepEqual(out[other], []string{src}) {
		t.Fatal("relation order/dedupe changed", out)
	}
	if err := b.removeRelation(src, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.removeRelation(other, a.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(relationFile)
	msgpack.Unmarshal(got, &out)
	if v, ok := out[a.ID]; !ok || len(v) != 0 {
		t.Fatal("empty destination lost")
	}
	broken := []byte("corrupt index")
	os.WriteFile(relationFile, broken, 0600)
	if err := b.upsertRelation(src, a.ID); err == nil {
		t.Fatal("corrupt index accepted")
	}
	got, _ = os.ReadFile(relationFile)
	if !bytes.Equal(got, broken) {
		t.Fatal("corrupt source replaced")
	}
	old := util.DataDir
	wrong := t.TempDir()
	util.DataDir = wrong
	defer func() { util.DataDir = old }()
	if err := b.upsertRelation(src, a.ID); err == nil {
		t.Fatal("changed owner accepted")
	}
	if _, err := os.Stat(filepath.Join(wrong, "storage")); !os.IsNotExist(err) {
		t.Fatal("wrote wrong workspace")
	}
}

func mutationRelationFixture(t *testing.T) (*Operation, *attributeViewEnvironment, string, string) {
	t.Helper()
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.Views[0].Group = nil
	dest := &av.AttributeView{}
	// Clone the complete original layout without sharing source values.
	raw, _ := json.Marshal(a)
	if err := json.Unmarshal(raw, &dest); err != nil {
		t.Fatal(err)
	}
	dest.ID = "20261030000004-destav1"
	dest.Name = "Destination"
	destFile := filepath.Join(filepath.Dir(file), dest.ID+".json")
	writeLifecycleAV(t, destFile, dest)
	a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "rel", Name: "Links", Type: av.KeyTypeRelation}})
	a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: "rel"}})
	writeLifecycleAV(t, file, a)
	return &Operation{Action: "updateAttrViewColRelation", AvID: a.ID, ID: dest.ID, KeyID: "rel", BackRelationKeyID: "back", IsTwoWay: true, Format: "Links", Name: "Back"}, legacyAttributeViewEnvironment(context.Background()), file, destFile
}

func TestAVMutationTwoWayRelationAndDestinationFailure(t *testing.T) {
	t.Run("persist-original-two-way", func(t *testing.T) {
		op, e, src, dest := mutationRelationFixture(t)
		if err := runAttributeViewConfiguration(context.Background(), op, e, updateAttributeViewColRelationWithEnvironment); err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct{ file, key, target string }{{src, "rel", op.ID}, {dest, "back", op.AvID}} {
			var a av.AttributeView
			raw, _ := os.ReadFile(item.file)
			if err := json.Unmarshal(raw, &a); err != nil {
				t.Fatal(err)
			}
			k, _ := a.GetKey(item.key)
			if k == nil || k.Relation == nil || !k.Relation.IsTwoWay || k.Relation.AvID != item.target {
				t.Fatal("two-way source missing")
			}
		}
		var rel map[string][]string
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(src), "relations.msgpack"))
		if err != nil {
			t.Fatal(err)
		}
		if err = msgpack.Unmarshal(raw, &rel); err != nil || !reflect.DeepEqual(rel[op.ID], []string{op.AvID}) || !reflect.DeepEqual(rel[op.AvID], []string{op.ID}) {
			t.Fatal("two-way index", err, rel)
		}
	})
	t.Run("destination-save-failure-is-not-overwritten", func(t *testing.T) {
		op, e, _, dest := mutationRelationFixture(t)
		originalStage := e.stage
		reloaded, finished := false, false
		e.stage = func(a *av.AttributeView) error {
			if a.ID == op.ID {
				if err := os.Remove(dest); err != nil {
					return err
				}
				if err := os.Mkdir(dest, 0700); err != nil {
					return err
				}
			}
			return originalStage(a)
		}
		e.reload = func(string) error { reloaded = true; return nil }
		e.complete = func() error { finished = true; return nil }
		if err := runAttributeViewConfiguration(context.Background(), op, e, updateAttributeViewColRelationWithEnvironment); err == nil {
			t.Fatal("destination IO failure swallowed")
		}
		if reloaded || finished {
			t.Fatal("failed operation published success")
		}
	})

	t.Run("late-index-failure-keeps-partial-source-without-success-notices", func(t *testing.T) {
		op, e, src, dest := mutationRelationFixture(t)
		before, _ := os.ReadFile(src)
		beforeDest, _ := os.ReadFile(dest)
		relation := filepath.Join(filepath.Dir(src), "relations.msgpack")
		bad := []byte("corrupt index")
		if err := os.WriteFile(relation, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if err := runAttributeViewConfiguration(context.Background(), op, e, updateAttributeViewColRelationWithEnvironment); err == nil {
			t.Fatal("late relation failure swallowed")
		}
		after, _ := os.ReadFile(src)
		afterDest, _ := os.ReadFile(dest)
		index, _ := os.ReadFile(relation)
		if bytes.Equal(before, after) || bytes.Equal(beforeDest, afterDest) {
			t.Fatal("expected original sequential partial saves")
		}
		if !bytes.Equal(bad, index) {
			t.Fatal("corrupt index overwritten")
		}
		Conf.repoMu.Lock()
		scheduled := Conf.repoTasks != nil
		Conf.repoMu.Unlock()
		if scheduled {
			t.Fatal("failed relation operation scheduled success notices")
		}
	})
	t.Run("missing-old-target-fails-before-mutation", func(t *testing.T) {
		op, e, src, _ := mutationRelationFixture(t)
		var a av.AttributeView
		raw, _ := os.ReadFile(src)
		json.Unmarshal(raw, &a)
		key, _ := a.GetKey("rel")
		key.Relation = &av.Relation{AvID: "20261030000005-missing", IsTwoWay: true, BackKeyID: "old"}
		before := writeLifecycleAV(t, src, &a)
		if err := runAttributeViewConfiguration(context.Background(), op, e, updateAttributeViewColRelationWithEnvironment); err == nil {
			t.Fatal("old target error swallowed")
		}
		after, _ := os.ReadFile(src)
		if !bytes.Equal(before, after) {
			t.Fatal("failed dependency mutated source")
		}
	})
}

func TestAVMutationActualSettingsPreserveOtherViewData(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	op := &Operation{Action: "setAttrViewPageSize", AvID: a.ID, Data: float64(7)}
	if err := setAttributeViewSettings(context.Background(), op, legacyAttributeViewSettingsEnvironment()); err != nil {
		t.Fatal(err)
	}
	var saved av.AttributeView
	raw, _ := os.ReadFile(file)
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Views[0].PageSize != 7 || saved.Views[0].Group.Field != "status" || len(saved.KeyValues) != 2 {
		t.Fatal("settings changed unrelated source")
	}
}

func TestAVMutationOptionOrderAndAppendActualIO(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, layout)
			option := &Operation{Action: "updateAttrViewColOptions", AvID: a.ID, ID: "status", Data: []*av.SelectOption{{Name: "X", Desc: "updated"}, {Name: "Y"}, {Name: "Empty"}, {Name: "New"}}}
			if err := runAttributeViewOption(context.Background(), option, legacyAttributeViewOptionEnvironment()); err != nil {
				t.Fatal(err)
			}
			rows := [][]*av.Value{{{KeyID: "block", Type: av.KeyTypeBlock, Block: &av.ValueBlock{Content: "detached"}}, {KeyID: "status", Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: "X"}}}}}
			before, _ := json.Marshal(rows)
			if err := appendAttributeViewDetachedBlocksWithEnvironment(a.ID, rows, legacyAttributeViewAppendEnvironment()); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(rows)
			if !bytes.Equal(before, after) {
				t.Fatal("append mutated caller input")
			}
			var saved av.AttributeView
			raw, _ := os.ReadFile(file)
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			key, _ := saved.GetKey("status")
			if len(key.Options) != 4 || key.Options[0].Desc != "updated" {
				t.Fatal("option update missing")
			}
			block := saved.GetBlockKeyValues()
			if len(block.Values) != 2 || !block.Values[1].IsDetached {
				t.Fatal("append missing detached row")
			}
			id := block.Values[1].BlockID
			op := &Operation{Action: "sortAttrViewRow", AvID: a.ID, ID: id}
			if err := runAttributeViewOrder(context.Background(), op, legacyAttributeViewOrderEnvironment()); err != nil {
				t.Fatal(err)
			}
			raw, _ = os.ReadFile(file)
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Views[0].ItemIDs) == 0 || saved.Views[0].ItemIDs[0] != id {
				t.Fatal("row ordering not persisted")
			}
		})
	}
}

func TestAVMutationRelationPublicationAndSymlinkFailure(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	dir := filepath.Dir(file)
	relation := filepath.Join(dir, "relations.msgpack")
	b := newLegacyAVBinding(context.Background())
	const src = "20261030000001-source1"
	raw, _ := msgpack.Marshal(map[string][]string{a.ID: {}})
	os.WriteFile(relation, raw, 0600)
	if os.Geteuid() != 0 {
		if err := os.Chmod(dir, 0500); err != nil {
			t.Fatal(err)
		}
		err := b.upsertRelation(src, a.ID)
		os.Chmod(dir, 0700)
		if err == nil {
			t.Fatal("relation publication error swallowed")
		}
		after, _ := os.ReadFile(relation)
		if !bytes.Equal(raw, after) {
			t.Fatal("failed publication changed source")
		}
	}
	external := filepath.Join(t.TempDir(), "external.msgpack")
	os.WriteFile(external, raw, 0600)
	os.Remove(relation)
	if err := os.Symlink(external, relation); err != nil {
		t.Fatal(err)
	}
	if err := b.upsertRelation(src, a.ID); err == nil {
		t.Fatal("symlink source accepted")
	}
	after, _ := os.ReadFile(external)
	if !bytes.Equal(raw, after) {
		t.Fatal("symlink target overwritten")
	}
}

func TestAVMutationStandaloneRowViewUsesTransactionOwner(t *testing.T) {
	_, tx, a, _ := deepAVFixture(t, av.LayoutTypeTable)
	e := tx.attributeViewRowEnvironment()
	v, err := e.view(a, "")
	if err != nil || v != a.Views[0] {
		t.Fatal("bound standalone row view", v, err)
	}
	old := util.DataDir
	util.DataDir = t.TempDir()
	defer func() { util.DataDir = old }()
	if _, err = e.view(a, ""); err == nil {
		t.Fatal("row view switched owners")
	}
}
