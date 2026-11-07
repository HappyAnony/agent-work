//go:build fts5

package model

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/av"
)

func TestOwnedBlockAttributesActualReadWriteBatchReset(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	id := "20261028140002-refdef1"
	ctx := context.Background()
	attrs := map[string]string{"CUSTOM-MIXED": " retained, ", "memo": "owned"}
	before := copyBlockAttributes(attrs)
	if e := SetBlockAttrsContext(ctx, id, attrs); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(attrs, before) {
		t.Fatal("caller map mutated", attrs)
	}
	out, e := ReadBlockAttributesContext(ctx, []string{id})
	if e != nil || out[id]["custom-mixed"] != "retained" || out[id]["memo"] != "owned" {
		t.Fatal(out, e)
	}
	if e = BatchSetBlockAttrsContext(ctx, []map[string]interface{}{{"id": id, "attrs": map[string]string{"memo": "batch"}}, {"id": id, "attrs": map[string]string{"custom-order": "last"}}}); e != nil {
		t.Fatal(e)
	}
	out, e = ReadBlockAttributesContext(ctx, []string{id})
	if e != nil || out[id]["memo"] != "batch" || out[id]["custom-order"] != "last" {
		t.Fatal(out, e)
	}
	if e = ResetBlockAttrsContext(ctx, id, map[string]string{"id": id, "memo": "reset"}); e != nil {
		t.Fatal(e)
	}
	out, e = ReadBlockAttributesContext(ctx, []string{id})
	if e != nil || out[id]["memo"] != "reset" || out[id]["custom-mixed"] != "" {
		t.Fatal(out, e)
	}
	c.r.owner.persistenceReadOnly = true
	if e = SetBlockAttrsContext(ctx, id, map[string]string{"memo": "forbidden"}); e == nil {
		t.Fatal("readonly mutation admitted")
	}
	if out, e = ReadBlockAttributesContext(ctx, []string{id}); e != nil || out[id]["memo"] != "reset" {
		t.Fatal(out, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = ReadBlockAttributesContext(cancelled, []string{id}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = c.r.owner.CloseCloudRepo(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestNestedLegacyAdmissionRejectsReplacementBeforeEffects(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	base, e := captureLegacyReindexOwner(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	replacement := NewAppConf()
	replacement.syncRuntime.same.Store(9)
	e = base.run(context.Background(), func(ctx, _ context.Context) error {
		Conf = replacement
		defer func() { Conf = c.r.owner }()
		if e := IncSyncContext(ctx); e == nil {
			t.Error("nested sync recaptured replacement")
		}
		if _, e := captureIndexRepair(ctx); e == nil {
			t.Error("nested DB binding recaptured replacement")
		}
		if _, e := captureLegacyDelayedUIOwner(ctx); e == nil {
			t.Error("nested UI binding recaptured replacement")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if replacement.syncRuntime.same.Load() != 9 {
		t.Fatal("replacement received effects")
	}
}
