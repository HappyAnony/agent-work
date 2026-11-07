package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

// Real owner-local source reads + original performTx/receivers. All effects in
// this test are detached candidates; no claim of native R04 source commit.
func TestOriginalAVConfigurationContextTransactions(t *testing.T) {
	const src = "20261023090600-av00001"
	const dst = "20261023090601-av00002"
	const old = "20261023090602-av00003"
	const row = "20261019090002-par0001"
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	pages := []string{"1760000000000000002", "1760000000000000003"}
	for _, page := range pages {
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		for _, id := range []string{src, dst, old} {
			a := &av.AttributeView{ID: id, Name: page, Spec: 4, Views: []*av.View{{ID: "table", LayoutType: av.LayoutTypeTable, Table: av.NewLayoutTable(), Filters: []*av.ViewFilter{{Column: "rollup"}}}}, KeyValues: []*av.KeyValues{
				{Key: &av.Key{ID: "primary", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: row, KeyID: "primary", BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row, Content: page}}}},
				{Key: &av.Key{ID: "template", Type: av.KeyTypeTemplate, Template: "old"}},
				{Key: &av.Key{ID: "relation", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: old, IsTwoWay: true, BackKeyID: "back"}}, Values: []*av.Value{{ID: "relation-value", Type: av.KeyTypeRelation, KeyID: "relation", BlockID: row, Relation: &av.ValueRelation{BlockIDs: []string{row}}}}},
				{Key: &av.Key{ID: "rollup", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: "relation", KeyID: "primary"}}},
			}}
			if id == old {
				a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "back", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: src, IsTwoWay: true, BackKeyID: "relation"}}})
			}
			raw, _ := json.Marshal(a)
			if e := os.WriteFile(filepath.Join(dir, id+".json"), raw, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	m := workspace.NewManager(workspace.Options{})
	store, e := m.OpenStore(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Drain(ctx)
	for _, page := range pages {
		lease, e := m.Acquire(ctx, store, page)
		if e != nil {
			t.Fatal(e)
		}
		defer lease.Release()
		for _, action := range []string{"updateAttrViewColTemplate", "updateAttrViewColRollup", "updateAttrViewColRelation"} {
			for _, failure := range []string{"", "read", "groups", "stage", "remove", "upsert", "reload", "finish", "cancel", "bad-data"} {
				if action != "updateAttrViewColRelation" && (failure == "remove" || failure == "upsert" || failure == "reload") {
					continue
				}
				if action == "updateAttrViewColRollup" && failure == "groups" {
					continue
				}
				t.Run(page+"/"+action+"/"+failure, func(t *testing.T) {
					callCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					want := errors.New("synthetic " + failure)
					counts := map[string]int{}
					cache := map[string]*av.AttributeView{}
					staged := map[string]*av.AttributeView{}
					before := map[string]string{}
					for _, id := range []string{src, dst, old} {
						raw, e := lease.ReadNativeAttributeView(ctx, id)
						if e != nil {
							t.Fatal(e)
						}
						before[id] = string(raw)
					}
					mark := func(n string) error {
						counts[n]++
						if failure == n {
							return want
						}
						if failure == "cancel" && n == "stage" {
							cancel()
						}
						return callCtx.Err()
					}
					cfg := &attributeViewEnvironment{
						read: func(id string) (*av.AttributeView, error) {
							if e := mark("read"); e != nil {
								return nil, e
							}
							if a := cache[id]; a != nil {
								return a, nil
							}
							raw, e := lease.ReadNativeAttributeView(ctx, id)
							if e != nil {
								return nil, e
							}
							a := &av.AttributeView{}
							if e = json.Unmarshal(raw, a); e != nil {
								return nil, e
							}
							cache[id] = a
							return a, nil
						},
						stage: func(a *av.AttributeView) error {
							if e := mark("stage"); e != nil {
								return e
							}
							staged[a.ID] = a
							return nil
						}, groups: func(*av.AttributeView) error { return mark("groups") },
						removeRelation: func(a, b string) error {
							if a != src || b != old {
								t.Error(a, b)
							}
							return mark("remove")
						}, upsertRelation: func(a, b string) error {
							if !((a == src && b == dst) || (a == dst && b == src)) {
								t.Error(a, b)
							}
							return mark("upsert")
						}, reload: func(id string) error {
							if id != dst {
								t.Error(id)
							}
							return mark("reload")
						}, now: func() time.Time { return time.UnixMilli(12345) }, newID: func() string { return "context-id" },
					}
					op := &Operation{Action: action, AvID: src}
					switch action {
					case "updateAttrViewColTemplate":
						op.ID = "template"
						op.Typ = "template"
						op.Data = page + " template"
					case "updateAttrViewColRollup":
						op.ID = "rollup"
						op.ParentID = "relation"
						op.KeyID = "primary"
						op.Data = map[string]interface{}{"calc": map[string]interface{}{"operator": "Count all"}}
					case "updateAttrViewColRelation":
						op.ID = dst
						op.KeyID = "relation"
						op.IsTwoWay = true
						op.BackRelationKeyID = "new-back"
						op.Name = "back name"
						op.Format = "forward name"
					}
					if failure == "bad-data" {
						if action == "updateAttrViewColRelation" {
							cfg.read = func(string) (*av.AttributeView, error) { return nil, want }
						} else {
							op.Data = 42
						}
					}
					env := &transactionEnvironment{ctx: callCtx, avConfig: cfg, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(string) *treenode.BlockTree { return nil }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, errors.New("unexpected tree read") }, stage: func(*parse.Tree) error { return errors.New("unexpected tree stage") }, cacheIAL: func(string, map[string]string) {}, finish: func(*Transaction) error { return mark("finish") }}
					tx := &Transaction{environment: env, DoOperations: []*Operation{op}}
					result := performTx(tx)
					if failure == "" {
						if result != nil {
							t.Fatal(result)
						}
						if len(staged) == 0 || counts["finish"] != 1 {
							t.Fatal("no staged effects", counts)
						}
						a := staged[src]
						if a.Name != page {
							t.Fatal("owner leak")
						}
						switch action {
						case "updateAttrViewColTemplate":
							k, _ := a.GetKey("template")
							if k.Template != page+" template" {
								t.Fatal(k)
							}
						case "updateAttrViewColRollup":
							if len(a.Views[0].Filters) != 0 {
								t.Fatal("old filter not removed")
							}
						case "updateAttrViewColRelation":
							k, _ := a.GetKey("relation")
							back, _ := staged[dst].GetKey("new-back")
							previous, _ := staged[old].GetKey("back")
							if k.Relation.AvID != dst || back.Relation.AvID != src || previous.Relation.IsTwoWay || counts["upsert"] != 2 {
								t.Fatal("relation graph", counts)
							}
							v := staged[dst].GetValue("new-back", row)
							if v == nil || !reflect.DeepEqual(v.Relation.BlockIDs, []string{row}) {
								t.Fatal(v)
							}
						}
					} else {
						if result == nil || tx.state.Load() != 3 {
							t.Fatal("failure committed", result, tx.state.Load())
						}
						if failure != "finish" && counts["finish"] != 0 {
							t.Fatal("finished failed candidate", counts)
						}
					}
					if !tx.m.TryLock() {
						t.Fatal("transaction leaked lock")
					}
					tx.m.Unlock()
					for id, raw := range before {
						after, e := lease.ReadNativeAttributeView(ctx, id)
						if e != nil || string(after) != raw {
							t.Fatal("authority changed", id, e)
						}
					}
				})
			}
		}
	}
}

func TestAVConfigurationRejectsEveryUnboundDependencyBeforeBegin(t *testing.T) {
	for _, name := range []string{"read", "stage", "groups", "removeRelation", "upsertRelation", "reload", "now", "newID"} {
		t.Run(name, func(t *testing.T) {
			cfg := &attributeViewEnvironment{read: func(string) (*av.AttributeView, error) { t.Fatal("read before admission"); return nil, nil }, stage: func(*av.AttributeView) error { return nil }, groups: func(*av.AttributeView) error { return nil }, removeRelation: func(string, string) error { return nil }, upsertRelation: func(string, string) error { return nil }, reload: func(string) error { return nil }, now: time.Now, newID: func() string { return "" }}
			switch name {
			case "read":
				cfg.read = nil
			case "stage":
				cfg.stage = nil
			case "groups":
				cfg.groups = nil
			case "removeRelation":
				cfg.removeRelation = nil
			case "upsertRelation":
				cfg.upsertRelation = nil
			case "reload":
				cfg.reload = nil
			case "now":
				cfg.now = nil
			case "newID":
				cfg.newID = nil
			}
			env := &transactionEnvironment{ctx: context.Background(), avConfig: cfg, parser: func() *lute.Lute { return nil }, lookup: func(string) *treenode.BlockTree { return nil }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, nil }, stage: func(*parse.Tree) error { return nil }, cacheIAL: func(string, map[string]string) {}, finish: func(*Transaction) error { return nil }}
			for _, action := range []string{"updateAttrViewColTemplate", "updateAttrViewColRelation", "updateAttrViewColRollup"} {
				tx := &Transaction{environment: env, DoOperations: []*Operation{{Action: action}}}
				result := performTx(tx)
				if result == nil || !strings.Contains(result.msg, "transaction_av_dependencies_unbound") || tx.state.Load() != 0 {
					t.Fatal(fmt.Sprint(result), tx.state.Load())
				}
			}
		})
	}
}
