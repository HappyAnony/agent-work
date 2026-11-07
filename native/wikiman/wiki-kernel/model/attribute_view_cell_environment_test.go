package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/av"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

const cellAV = "20261023130000-av00001"
const cellDest = "20261023130001-av00002"
const cellRelated = "20261023130002-av00003"
const cellSecond = "20261019090003-cod0001"

func cellSource(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	t.Helper()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	pages := []string{"1760000000000000002", "1760000000000000003"}
	b := filepath.Join(root, "pages", pages[1], "data")
	if e := os.MkdirAll(b, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(b, os.DirFS(filepath.Join(root, "pages", pages[0], "data"))); e != nil {
		t.Fatal(e)
	}
	for _, page := range pages {
		doc := filepath.Join(root, "pages", page, "data", settingsBox, settingsDoc+".docj")
		raw, e := os.ReadFile(doc)
		if e != nil {
			t.Fatal(e)
		}
		s := strings.Replace(string(raw), `"Properties": {"id": "`+settingsBlock+`"}`, `"Properties": {"id": "`+settingsBlock+`", "custom-avs":"`+cellAV+`", "av-names":"old", "custom-sy-av-s-text-`+cellAV+`":"old"}`, 1)
		s = strings.Replace(s, "Synthetic knowledge:", page+" Synthetic knowledge:", 1)
		if e = os.WriteFile(doc, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		for _, id := range []string{cellAV, cellDest, cellRelated} {
			a := &av.AttributeView{Spec: 4, ID: id, Name: page + " " + id, ViewID: "v", Views: []*av.View{{ID: "v", LayoutType: av.LayoutTypeTable, PageSize: 16, Table: av.NewLayoutTable(), Group: &av.ViewGroup{Field: "status", Method: av.GroupMethodValue}}}}
			for _, k := range []*av.Key{{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}, {ID: "text", Name: "Text", Type: av.KeyTypeText}, {ID: "number", Name: "Number", Type: av.KeyTypeNumber}, {ID: "date", Type: av.KeyTypeDate}, {ID: "status", Name: "Status", Type: av.KeyTypeSelect, Options: []*av.SelectOption{{Name: "Old", Color: "2"}}}, {ID: "multi", Type: av.KeyTypeMSelect}, {ID: "relation", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: cellDest, BackKeyID: "back", IsTwoWay: true}}, {ID: "back", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: cellAV, BackKeyID: "relation", IsTwoWay: true}}, {ID: "check", Type: av.KeyTypeCheckbox}, {ID: "url", Type: av.KeyTypeURL}, {ID: "asset", Type: av.KeyTypeMAsset}} {
				kv := &av.KeyValues{Key: k}
				for _, row := range []string{settingsBlock, cellSecond} {
					v := &av.Value{ID: row + k.ID, KeyID: k.ID, BlockID: row, Type: k.Type, CreatedAt: 1, UpdatedAt: 2, IsDetached: true}
					avrender.FillNilValue(v, k.Type)
					if k.Type == av.KeyTypeBlock {
						v.Block = &av.ValueBlock{}
					}
					if v.Block != nil {
						v.Block.ID = row
						v.Block.Content = page + " old"
					}
					if v.Text != nil {
						v.Text.Content = "old"
					}
					if v.MSelect != nil || k.Type == av.KeyTypeSelect {
						v.MSelect = []*av.ValueSelect{{Content: "Old", Color: "2"}}
					}
					if v.Relation != nil && (k.ID == "relation" || k.ID == "back") {
						v.Relation.BlockIDs = []string{settingsBlock}
						v.Relation.Contents = []*av.Value{{Type: av.KeyTypeText, Text: &av.ValueText{Content: "derived"}}}
					}
					kv.Values = append(kv.Values, v)
				}
				a.KeyValues = append(a.KeyValues, kv)
			}
			for _, key := range []string{"primary", "text", "number", "status"} {
				a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: key}})
			}
			raw, e = json.Marshal(a)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(dir, id+".json"), raw, 0600); e != nil {
				t.Fatal(e)
			}
		}
		raw, e = msgpack.Marshal(map[string][]string{cellAV: {cellAV, cellRelated, cellRelated}})
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, "relations.msgpack"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := workspace.NewManager(workspace.Options{})
	store, e := m.OpenStore(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := m.Drain(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return m, store, root
}

type cellHarness struct {
	t                   *testing.T
	ctx                 context.Context
	cancel              context.CancelFunc
	lease               *workspace.Lease
	root, page, failure string
	counts              map[string]int
	cache, staged       map[string]*av.AttributeView
	attrs               map[string]map[string]string
	events, related     []string
	notifications       []*Transaction
	committed           bool
	env                 *transactionEnvironment
	cell                *attributeViewCellEnvironment
	tx                  *Transaction
}

func (h *cellHarness) mark(name string) error {
	h.counts[name]++
	if h.failure == name {
		return errors.New("synthetic " + name)
	}
	if h.failure == "cancel-"+name {
		h.cancel()
	}
	return h.ctx.Err()
}
func newCellHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *cellHarness {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := &cellHarness{t: t, ctx: ctx, cancel: cancel, lease: l, root: root, page: page, failure: failure, counts: map[string]int{}, cache: map[string]*av.AttributeView{}, staged: map[string]*av.AttributeView{}, attrs: map[string]map[string]string{}}
	h.env = &transactionEnvironment{ctx: ctx, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(id string) *treenode.BlockTree {
		if id == "missing" {
			return nil
		}
		return &treenode.BlockTree{ID: id, RootID: settingsDoc, BoxID: settingsBox, Path: "/" + settingsDoc + ".docj"}
	},
		load: func(box, path string, engine *lute.Lute) (*parse.Tree, error) {
			if e := h.mark("load"); e != nil {
				return nil, e
			}
			raw, e := l.ReadDocument(ctx, box, settingsDoc)
			if e != nil {
				return nil, e
			}
			tree, _, e := dataparser.ParseJSON(raw, engine.ParseOptions)
			if e == nil {
				tree.ID, tree.Box, tree.Path = settingsDoc, box, path
			}
			return tree, e
		},
		stage: func(*parse.Tree) error { return h.mark("tree-stage") }, cacheIAL: func(id string, attrs map[string]string) { h.attrs[id] = attrs },
		finish: func(*Transaction) error {
			if e := h.mark("finish"); e != nil {
				return e
			}
			if e := flushRelatedAttributeViews(ctx, gulu.Str.RemoveDuplicatedElem(h.related), h.cell.read, h.cell.groups, h.cell.stage, func(id string) error {
				if e := h.mark("reload"); e != nil {
					return e
				}
				h.events = append(h.events, "reload:"+id)
				return nil
			}, false); e != nil {
				return e
			}
			h.committed = true
			return nil
		},
	}
	h.tx = &Transaction{environment: h.env}
	h.cell = &attributeViewCellEnvironment{ctx: ctx, now: func() time.Time { return time.UnixMilli(12345) }, newID: func() string { h.counts["id"]++; return fmt.Sprintf("generated-%s-%d", page, h.counts["id"]) }, color: func() string { h.counts["color"]++; return "7" },
		read: func(id string) (*av.AttributeView, error) {
			if e := h.mark("read:" + id); e != nil {
				return nil, e
			}
			if a := h.cache[id]; a != nil {
				return a, nil
			}
			raw, e := l.ReadNativeAttributeView(ctx, id)
			if e != nil {
				return nil, e
			}
			a := &av.AttributeView{}
			if e = json.Unmarshal(raw, a); e != nil {
				return nil, e
			}
			h.cache[id] = a
			return a, nil
		},
		stage: func(a *av.AttributeView) error {
			if e := h.mark("stage:" + a.ID); e != nil {
				return e
			}
			h.staged[a.ID] = a
			return nil
		},
		node: func(id string) (*ast.Node, *parse.Tree, error) {
			if e := h.mark("node"); e != nil {
				return nil, nil, e
			}
			tree, e := h.tx.loadTree(id)
			if e != nil {
				return nil, nil, e
			}
			return treenode.GetNodeInTree(tree, id), tree, nil
		},
		blockText: func(n *ast.Node) (string, error) {
			if e := h.mark("text"); e != nil {
				return "", e
			}
			s, e := anchor.Text(ctx, n, 1024, indexdb.ContentServices{AttributeView: func(_ context.Context, id string, _ bool) (string, error) {
				a, e := h.cell.read(id)
				if e != nil {
					return "", e
				}
				return a.Name, nil
			}})
			if s == "" && e == nil {
				s = page + " untitled"
			}
			return s, e
		},
		names: func(ids string) (string, error) {
			if e := h.mark("names"); e != nil {
				return "", e
			}
			return attributeViewNamesWithServices(ids, func(id string) (string, error) {
				a, e := h.cell.read(id)
				if e != nil {
					return "", e
				}
				return a.Name, nil
			}, func() string { return page + " untitled" }, false)
		},
		attrs: func(n *ast.Node, tree *parse.Tree, attrs map[string]string) error {
			if e := h.mark("attrs"); e != nil {
				return e
			}
			old, e := setNodeAttrs0WithServices(n, attrs, func(string) error { return errors.New("invalid attr name") }, func() error { return h.mark("tags") })
			if e != nil {
				return e
			}
			if e = h.tx.writeTree(tree); e != nil {
				return e
			}
			h.env.cacheIAL(n.ID, parse.IAL2Map(n.KramdownIAL))
			if e = h.mark("notify"); e != nil {
				return e
			}
			h.events = append(h.events, "attrs:"+n.ID)
			h.notifications = append(h.notifications, blockAttrTransactions(old, n)...)
			return nil
		},
		related: func(id string) error {
			return collectRelatedAttributeViews(ctx, id, func(id string) ([]string, error) {
				if e := h.mark("related"); e != nil {
					return nil, e
				}
				raw, e := os.ReadFile(filepath.Join(root, "pages", page, "data", "storage", "av", "relations.msgpack"))
				if e != nil {
					return nil, e
				}
				var m map[string][]string
				if e = msgpack.Unmarshal(raw, &m); e != nil {
					return nil, e
				}
				return m[id], nil
			}, func(ids []string) error {
				if e := h.mark("enqueue"); e != nil {
					return e
				}
				h.related = append(h.related, ids...)
				return nil
			})
		},
	}
	h.cell.groups = func(a *av.AttributeView) error {
		if e := h.mark("groups:" + a.ID); e != nil {
			return e
		}
		for _, v := range a.Views {
			if v.GetGroupKey(a) == nil {
				continue
			}
			if e := avrender.GenerateGroups(v, a, avrender.GroupEnvironment{Now: h.cell.now, Read: h.cell.read, NewView: avrender.NewGroupView, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
				renderEnv := cellRenderEnvironment(h)
				switch v.LayoutType {
				case av.LayoutTypeGallery:
					return avrender.Gallery(a, v, q, renderEnv)
				case av.LayoutTypeKanban:
					return avrender.Kanban(a, v, q, renderEnv)
				default:
					return avrender.Table(a, v, q, renderEnv)
				}
			}}); e != nil {
				return e
			}
		}
		return ctx.Err()
	}
	h.env.avCell = h.cell
	return h
}
func cellOp(key string, data interface{}) *Operation {
	return &Operation{Action: "updateAttrViewCell", AvID: cellAV, KeyID: key, RowID: settingsBlock, Data: data}
}
func cellDigest(t *testing.T, root, page string) map[string]string {
	t.Helper()
	ret := map[string]string{}
	for _, f := range []string{filepath.Join(settingsBox, settingsDoc+".docj"), "storage/av/" + cellAV + ".json", "storage/av/" + cellDest + ".json", "storage/av/" + cellRelated + ".json", "storage/av/relations.msgpack"} {
		b, e := os.ReadFile(filepath.Join(root, "pages", page, "data", f))
		if e != nil {
			t.Fatal(e)
		}
		ret[f] = string(b)
	}
	return ret
}

func TestOriginalAVCellContextFieldsBindingsRelationsAndOwners(t *testing.T) {
	m, store, root := cellSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, e := m.Acquire(context.Background(), store, page)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(l.Release)
			for _, scenario := range []string{"text", "number", "empty-number", "empty-date", "status", "empty-status", "multi", "check", "url", "asset", "create", "bind", "unbind", "rebind", "static", "dynamic", "relation-add", "relation-remove", "relation-order", "relation-replace", "self-relation"} {
				t.Run(scenario, func(t *testing.T) {
					before := cellDigest(t, root, page)
					h := newCellHarness(t, l, root, page, "")
					a, e := h.cell.read(cellAV)
					if e != nil {
						t.Fatal(e)
					}
					key, data := "text", interface{}(map[string]interface{}{"text": map[string]interface{}{"content": page}})
					primary := a.GetValue("primary", settingsBlock)
					switch scenario {
					case "number":
						key = "number"
						data = map[string]interface{}{"number": map[string]interface{}{"content": 123.5, "isNotEmpty": true}}
					case "empty-number":
						key = "number"
						data = map[string]interface{}{"number": map[string]interface{}{"content": 123.5, "isNotEmpty": false, "formattedContent": "stale"}}
					case "empty-date":
						key = "date"
						data = map[string]interface{}{"date": map[string]interface{}{"content": 123, "isNotEmpty": false, "formattedContent": "stale"}}
					case "status", "multi", "empty-status":
						key = "status"
						if scenario == "multi" {
							key = "multi"
						}
						data = map[string]interface{}{"mSelect": []interface{}{map[string]interface{}{"content": ""}, map[string]interface{}{"content": "Old", "color": "99"}, map[string]interface{}{"content": page}}}
						if scenario == "empty-status" {
							data = map[string]interface{}{"mSelect": []interface{}{map[string]interface{}{"content": ""}}}
						}
					case "check":
						key = "check"
						data = map[string]interface{}{"checkbox": map[string]interface{}{"checked": true}}
					case "url":
						key = "url"
						data = map[string]interface{}{"url": map[string]interface{}{"content": "https://example.invalid/" + page}}
					case "asset":
						key = "asset"
						data = map[string]interface{}{"mAsset": []interface{}{map[string]interface{}{"name": "local", "content": "assets/local.png"}}}
					case "create":
						kv, _ := a.GetKeyValues("text")
						kv.Values = kv.Values[1:]
					case "bind", "unbind", "rebind", "static", "dynamic":
						key = "primary"
						primary.IsDetached = scenario == "bind"
						target, content, detached := settingsBlock, "literal &amp; text", false
						if scenario == "unbind" {
							detached = true
						}
						if scenario == "rebind" {
							target = cellSecond
						}
						if scenario == "dynamic" {
							content = ""
						}
						data = map[string]interface{}{"isDetached": detached, "block": map[string]interface{}{"id": target, "content": content}}
					case "relation-add", "relation-remove", "relation-order", "relation-replace", "self-relation":
						key = "relation"
						ids := []string{settingsBlock, cellSecond, cellSecond}
						if scenario == "relation-remove" {
							ids = nil
						}
						if scenario == "relation-order" {
							ids = []string{cellSecond, settingsBlock}
							a.GetValue("relation", settingsBlock).Relation.BlockIDs = []string{settingsBlock, cellSecond}
						}
						if scenario == "relation-replace" {
							ids = []string{cellSecond}
						}
						if scenario == "self-relation" {
							k, _ := a.GetKey(key)
							k.Relation.AvID = cellAV
						}
						if scenario == "relation-add" || scenario == "relation-replace" || scenario == "self-relation" {
							dest := a
							if scenario != "self-relation" {
								dest, e = h.cell.read(cellDest)
								if e != nil {
									t.Fatal(e)
								}
							}
							back, _ := dest.GetKeyValues("back")
							back.Values = back.Values[:1]
						}
						data = map[string]interface{}{"relation": map[string]interface{}{"blockIDs": ids, "contents": []interface{}{map[string]interface{}{"type": "text", "text": map[string]interface{}{"content": "must clear"}}}}}
					}
					h.tx.DoOperations = []*Operation{cellOp(key, data)}
					ret := performTx(h.tx)
					if ret != nil || !h.committed {
						t.Fatal(ret, h.counts)
					}
					v := a.GetValue(key, settingsBlock)
					updated := int64(12345)
					if scenario == "create" {
						updated = 13345
					}
					if v == nil || (scenario != "empty-status" && (v.UpdatedAt != updated || primary.Block.Updated != 12345)) || a.Name != page+" "+cellAV {
						t.Fatal("timestamp/owner", v, primary)
					}
					switch scenario {
					case "text", "create":
						if v.Text.Content != page {
							t.Fatal(v)
						}
						if scenario == "create" && (v.CreatedAt != 12345 || !strings.Contains(v.ID, page)) {
							t.Fatal("creation identity", v)
						}
					case "check":
						if v.Checkbox == nil || !v.Checkbox.Checked {
							t.Fatal(v)
						}
					case "url":
						if v.URL == nil || v.URL.Content != "https://example.invalid/"+page {
							t.Fatal(v)
						}
					case "asset":
						if len(v.MAsset) != 1 || v.MAsset[0].Content != "assets/local.png" {
							t.Fatal(v)
						}
					case "number":
						if v.Number.Content != 123.5 || v.Number.FormattedContent == "" {
							t.Fatal(v)
						}
					case "empty-number":
						if v.Number.Content != 0 || v.Number.FormattedContent != "" {
							t.Fatal(v)
						}
					case "empty-date":
						if v.Date.Content != 0 || v.Date.FormattedContent != "" {
							t.Fatal(v)
						}
					case "status", "multi":
						k, _ := a.GetKey(key)
						if len(v.MSelect) != 2 || k.GetOption(page) == nil || k.GetOption(page).Color != "7" {
							t.Fatal("options", v, k)
						}
						if scenario == "status" && v.MSelect[0].Color != "2" {
							t.Fatal("original option color replaced")
						}
						if len(a.Views[0].Groups) == 0 {
							t.Fatal("groups not regenerated")
						}
					case "empty-status":
						if len(v.MSelect) != 0 || h.staged[cellAV] != nil || primary.Block.Updated != 0 {
							t.Fatal("original empty-option early return changed")
						}
					case "bind":
						if !strings.Contains(h.attrs[settingsBlock][av.NodeAttrNameAvs], cellAV) || !strings.Contains(h.attrs[settingsBlock][av.NodeAttrViewNames], page) {
							t.Fatal("binding/name projection", h.attrs)
						}
					case "unbind":
						if h.attrs[settingsBlock][av.NodeAttrNameAvs] != "" || v.Block.ID != "" || !v.IsDetached {
							t.Fatal("unbind", v, h.attrs)
						}
					case "rebind":
						if h.attrs[settingsBlock][av.NodeAttrNameAvs] != "" || !strings.Contains(h.attrs[cellSecond][av.NodeAttrNameAvs], cellAV) || v.Block.Content != "literal & text" {
							t.Fatal("rebind", v, h.attrs)
						}
					case "static":
						if h.attrs[settingsBlock][av.NodeAttrViewStaticText+"-"+cellAV] != "literal &amp; text" {
							t.Fatal(h.attrs)
						}
					case "dynamic":
						if !strings.Contains(v.Block.Content, page) || h.attrs[settingsBlock][av.NodeAttrViewStaticText+"-"+cellAV] != "" {
							t.Fatal("dynamic anchor", v, h.attrs)
						}
					case "relation-add", "relation-remove", "relation-order", "relation-replace", "self-relation":
						if v.Relation.Contents != nil {
							t.Fatal("derived relation contents saved")
						}
						dest := h.cache[cellDest]
						if scenario == "self-relation" {
							dest = a
						}
						if dest == nil {
							t.Fatal("target not read")
						}
						back := dest.GetValue("back", cellSecond)
						if (scenario == "relation-add" || scenario == "relation-replace" || scenario == "self-relation") && !reflect.DeepEqual(back.Relation.BlockIDs, []string{settingsBlock}) {
							t.Fatal(back)
						}
						if scenario == "relation-add" || scenario == "relation-replace" || scenario == "self-relation" {
							if back.CreatedAt != 12345 || back.UpdatedAt != 13345 || !strings.Contains(back.ID, page) {
								t.Fatal("reverse creation identity/time", back)
							}
						}
						if scenario == "relation-order" && !reflect.DeepEqual(v.Relation.BlockIDs, []string{cellSecond, settingsBlock}) {
							t.Fatal("relation order changed", v)
						}
						if scenario == "relation-replace" && !reflect.DeepEqual(dest.GetValue("back", settingsBlock).Relation.BlockIDs, []string{settingsBlock}) {
							t.Fatal("original same-count mode changed")
						}
						if scenario == "relation-remove" && len(dest.GetValue("back", settingsBlock).Relation.BlockIDs) != 0 {
							t.Fatal("reverse relation not removed")
						}
					}
					if scenario != "empty-status" && (h.staged[cellAV] == nil || h.staged[cellRelated] == nil || h.counts["reload"] != 1) {
						t.Fatal("staging/related refresh", h.counts)
					}
					if len(h.notifications) != h.counts["notify"] {
						t.Fatal("attribute notifications not staged")
					}
					for _, notification := range h.notifications {
						if len(notification.DoOperations) != 1 || notification.DoOperations[0].Action != "updateAttrs" {
							t.Fatal("wrong original notification", notification)
						}
						payload := notification.DoOperations[0].Data.(map[string]interface{})
						if payload["old"] == nil || payload["new"] == nil {
							t.Fatal("attribute delta lost", payload)
						}
					}
					if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
						t.Fatal("authoritative source changed")
					}
				})
			}
		})
	}
}

func cellRenderEnvironment(h *cellHarness) avrender.Environment {
	ctx := h.ctx
	return avrender.Environment{Trees: func(ids []string) (map[string]*parse.Tree, error) {
		trees := map[string]*parse.Tree{}
		for _, id := range ids {
			_, tree, err := h.cell.node(id)
			if err != nil {
				return nil, err
			}
			trees[id] = tree
		}
		return trees, nil
	}, Cover: avrender.CoverServices{Parser: h.env.parser()}, RemoveMissingField: func(*av.AttributeView, *av.View, string) error { return errors.New("unexpected missing field") }, FilterItems: func(items map[string][]*av.KeyValues) error {
		for row, values := range items {
			for _, kv := range values {
				if kv.Key.Type == av.KeyTypeBlock && !kv.Values[0].IsDetached {
					if _, _, e := h.cell.node(kv.Values[0].Block.ID); e != nil {
						delete(items, row)
					}
				}
			}
		}
		return ctx.Err()
	}, BlockAttrs: func(ids []string) (map[string]map[string]string, error) {
		ret := map[string]map[string]string{}
		for _, id := range ids {
			n, _, e := h.cell.node(id)
			if e != nil {
				return nil, e
			}
			if n == nil {
				return nil, errors.New("missing block")
			}
			ret[id] = parse.IAL2Map(n.KramdownIAL)
		}
		return ret, nil
	}, AutoValues: func(a *av.AttributeView, c av.Collection, ials map[string]map[string]string) error {
		avrender.FillBasicAutoValues(a, c, ials, h.cell.now)
		return ctx.Err()
	}, Templates: func(_ *av.AttributeView, _ *av.View, c av.Collection, _ map[string]map[string]string) error {
		for _, item := range c.GetItems() {
			for _, v := range item.GetValues() {
				if v.Type == av.KeyTypeTemplate {
					return errors.New("unexpected template fixture")
				}
			}
		}
		return ctx.Err()
	}}
}
