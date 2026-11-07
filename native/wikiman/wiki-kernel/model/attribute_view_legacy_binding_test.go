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
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/av"
)

func TestLegacyAVOuterGroupPortsCaptureCancellationAndOwner(t *testing.T) {
	_, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
	ctx, cancel := context.WithCancel(context.Background())
	tx.environment.ctx = ctx
	ports := []func(*av.AttributeView) error{legacyAttributeViewEnvironment(ctx).groups, legacyAttributeViewOptionEnvironment(ctx).groups, legacyAttributeViewOrderEnvironment(ctx).groups, legacyAttributeViewFieldEnvironment(ctx).groups, legacyAttributeViewAppendEnvironment(ctx).groups, legacyAttributeViewManagementEnvironment(ctx).groups, tx.attributeViewCellEnvironment().groups, tx.attributeViewBoundEnvironment().groups, legacyAttributeViewUpdateEnvironment(tx).groups}
	before, _ := os.ReadFile(file)
	cancel()
	for i, fn := range ports {
		if e := fn(a); !errors.Is(e, context.Canceled) {
			t.Fatal("port did not retain cancellation", i, e)
		}
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("canceled callbacks changed source")
	}
	t.Run("canceled-UI-admission", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pending := newLegacyAVBinding(ctx)
		if e := pending.reload(a.ID); e != nil {
			t.Fatal(e)
		}
		owner := pending.scope.commit.r.owner
		owner.repoMu.Lock()
		done := make(chan error, 1)
		go func() { done <- pending.complete() }()
		select {
		case e := <-done:
			owner.repoMu.Unlock()
			t.Fatal("admission escaped owner lock", e)
		case <-time.After(10 * time.Millisecond):
		}
		cancel()
		owner.repoMu.Unlock()
		if e := <-done; !errors.Is(e, context.Canceled) {
			t.Fatal("canceled admission succeeded", e)
		}
		if owner.repoTasks != nil {
			t.Fatal("canceled UI admission created task")
		}
	})
	// A previously captured callback cannot follow a different selected owner.
	b := newLegacyAVBinding(context.Background())
	old := Conf
	Conf = &AppConf{}
	defer func() { Conf = old }()
	if _, _, e := b.text(&ast.Node{Type: ast.NodeParagraph}, a.ID); e == nil {
		t.Fatal("text followed new owner")
	}
	if _, e := b.sources(a.ID); e == nil {
		t.Fatal("relations followed new owner")
	}
	if e := b.groups(a); e == nil {
		t.Fatal("groups followed new owner")
	}
	recordCloudBusinessEvidence(t, "outer-ports.json", map[string]interface{}{"canceledGroupPorts": len(ports), "capturedOwnerRejected": true, "sourceUnchanged": true})
}
func TestLegacyAVOuterSourceTextRefreshOwnsCompletionWithoutSync(t *testing.T) {
	c, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
	for _, n := range tx.nodes {
		n.SetIALAttr(av.NodeAttrNameAvs, a.ID)
	}
	b := newLegacyAVBinding(context.Background())
	events := make(chan commitNotice, 8)
	b.scope.commit.publish = func(n commitNotice) { events <- n }
	c.r.owner.syncRuntime.same.Store(9)
	if e := updateAttributeViewBlockTextWithEnvironment(tx.nodes, legacyAVTextEnvironment(context.Background(), b)); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(file)
	saved, e := av.DecodeAttributeView(raw)
	if e != nil || saved.GetBlockKeyValues().Values[0].Block.Content != "changed definition" {
		t.Fatal("outer source text not saved", e)
	}
	if c.r.owner.syncRuntime.same.Load() != 9 {
		t.Fatal("standalone UI completion planned sync")
	}
	select {
	case n := <-events:
		if n.kind != "av" {
			t.Fatal(n.kind)
		}
	case <-time.After(time.Second):
		t.Fatal("owned reload lost")
	}
	recordCloudBusinessEvidence(t, "outer-text.json", map[string]interface{}{"actualSourceTextSave": true, "ownedReload": true, "syncPlanUnchanged": true})
}
func TestLegacyAVOuterTextFailureDoesNotAdmitReload(t *testing.T) {
	c, tx, a, file := deepAVFixture(t, av.LayoutTypeTable)
	for _, n := range tx.nodes {
		n.SetIALAttr(av.NodeAttrNameAvs, a.ID)
	}
	os.WriteFile(filepath.Join(filepath.Dir(file), "relations.msgpack"), []byte("bad relation fixture"), 0600)
	if e := updateAttributeViewBlockTextWithEnvironment(tx.nodes, legacyAttributeViewTextEnvironment()); e == nil {
		t.Fatal("outer relation failure ignored")
	}
	if c.r.owner.repoTasks != nil {
		t.Fatal("failed outer refresh scheduled UI")
	}
	// The earlier text save may have succeeded. No rollback guarantee is implied.
	raw, _ := os.ReadFile(file)
	if !bytes.Contains(raw, []byte("changed definition")) {
		t.Fatal("fixture did not reach partial source save")
	}
}
func TestLegacyAVOuterDelayedRefreshPreservesDelayAndClose(t *testing.T) {
	for _, mode := range []string{"success", "close", "cancel", "stale"} {
		t.Run(mode, func(t *testing.T) {
			c, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newLegacyAVBinding(ctx)
			events := make(chan commitNotice, 8)
			b.scope.commit.publish = func(n commitNotice) { events <- n }
			before, _ := os.ReadFile(file)
			ids := []string{a.ID}
			admitted := time.Now()
			if e := b.scheduleUpdated(ids); e != nil {
				t.Fatal(e)
			}
			ids[0] = "mutated-after-admission"
			if mode == "close" {
				if e := c.r.owner.CloseCloudRepo(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "cancel" {
				cancel()
			}
			if mode == "stale" {
				old := Conf
				Conf = &AppConf{}
				defer func() { Conf = old }()
			}
			if mode != "success" {
				if e := c.r.owner.CloseCloudRepo(context.Background()); e != nil {
					t.Fatal(e)
				}
				after, _ := os.ReadFile(file)
				if !bytes.Equal(before, after) {
					t.Fatal("canceled delayed refresh wrote source")
				}
				select {
				case <-events:
					t.Fatal("canceled delayed refresh notified")
				default:
				}
				return
			}
			after, _ := os.ReadFile(file)
			if time.Since(admitted) < 100*time.Millisecond && !bytes.Equal(before, after) {
				t.Fatal("refresh ignored original delay")
			}
			select {
			case <-events:
			case <-time.After(2 * time.Second):
				t.Fatal("owned delayed refresh never completed")
			}
			after, _ = os.ReadFile(file)
			saved, e := av.DecodeAttributeView(after)
			if e != nil || len(saved.Views[0].Groups) == 0 {
				t.Fatal("immutable delayed IDs not executed", e)
			}
		})
	}
}
func TestLegacyAVOuterLiveRenderMatchesOriginalGroupProjection(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			_, _, a, file := deepAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			old, e := av.ParseAttributeView(a.ID)
			if e != nil {
				t.Fatal(e)
			}
			want, e := renderAttributeView(old, "", old.ViewID, "", 1, -1, nil)
			if e != nil {
				t.Fatal(e)
			}
			os.WriteFile(file, before, 0600)
			got, _, e := RenderAttributeViewContext(context.Background(), "", a.ID, a.ViewID, "", 1, -1, nil)
			if e != nil {
				t.Fatal(e)
			}
			projection := func(out av.Viewable) []interface{} {
				raw, _ := json.Marshal(out)
				var obj map[string]interface{}
				json.Unmarshal(raw, &obj)
				var result []interface{}
				for _, value := range obj["groups"].([]interface{}) {
					g := value.(map[string]interface{})
					result = append(result, []interface{}{g["name"], g["groupHidden"], g["rowCount"], g["cardCount"]})
				}
				return result
			}
			wantJSON, _ := json.Marshal(projection(want))
			gotJSON, _ := json.Marshal(projection(got))
			if !bytes.Equal(wantJSON, gotJSON) {
				t.Fatal("original live group UI changed", string(wantJSON), string(gotJSON))
			}
		})
	}
	recordCloudBusinessEvidence(t, "outer-live-render.json", map[string]interface{}{"layouts": 3, "originalGroupUIParity": true, "liveContextPathExecuted": true})
}
func TestLegacyAVOuterLiveRenderPropagatesSQLAndCancel(t *testing.T) {
	_, _, a, file := deepAVFixture(t, av.LayoutTypeTable)
	a.KeyValues[1].Key.Type = av.KeyTypeTemplate
	a.KeyValues[1].Key.Template = `.action{ querySQL "SELECT * FROM missing_outer_fixture" }`
	a.KeyValues[1].Values[0].Type = av.KeyTypeTemplate
	raw, _ := json.Marshal(a)
	os.WriteFile(file, raw, 0600)
	if _, _, e := RenderAttributeViewContext(context.Background(), "", a.ID, a.ViewID, "", 1, -1, nil); e == nil {
		t.Fatal("live render swallowed actual SQL error")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(raw, after) {
		t.Fatal("failed SQL render saved success repair")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e := RenderAttributeViewContext(ctx, "", "20261029150002-notmade", "", "", 1, -1, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(file), "20261029150002-notmade.json")); !os.IsNotExist(e) {
		t.Fatal("canceled request created AV", e)
	}
}
func TestLegacyAVOuterManagementCountAndNearUseContext(t *testing.T) {
	_, _, a, _ := deepAVFixture(t, av.LayoutTypeTable)
	b := newLegacyAVBinding(context.Background())
	if e := b.groups(a); e != nil {
		t.Fatal(e)
	}
	group := a.Views[0].GetGroupByGroupValue("X")
	count, e := legacyAttributeViewManagementEnvironment().count(a, a.Views[0], group)
	if e != nil || count != 1 {
		t.Fatal("actual group count", count, e)
	}
	a.RenderedViewables = map[string]av.Viewable{}
	near, e := legacyAttributeViewDefaults().near(a, a.Views[0], group, "")
	if e != nil || near == nil {
		t.Fatal("actual near item", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defaults := legacyAttributeViewDefaults(ctx)
	cancel()
	if _, e = defaults.near(a, a.Views[0], group, ""); !errors.Is(e, context.Canceled) {
		t.Fatal("near ignored owner context", e)
	}
}
