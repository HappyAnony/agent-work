package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/vmihailenco/msgpack/v5"
	"io/fs"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/indexdb"
)

type nativeAVSource struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ViewID    string            `json:"viewID"`
	KeyValues []json.RawMessage `json:"keyValues"`
	Views     []json.RawMessage `json:"views"`
}

func readNativeAVSource(ctx context.Context, w *WorkspaceContext, id string) (raw json.RawMessage, doc nativeAVSource, err error) {
	if !documentID.MatchString(id) {
		return nil, doc, ErrDocument
	}
	raw, err = readNativeFile(ctx, w, "storage/av/"+id+".json", 16<<20)
	if err != nil {
		return nil, doc, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, doc, ErrDocument
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, doc, err
	}
	if doc.ID != "" && doc.ID != id {
		return nil, doc, ErrDocument
	}
	return
}

func (l *Lease) ReadNativeAttributeView(ctx context.Context, id string) (ret json.RawMessage, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		ret, _, e = readNativeAVSource(ctx, l.workspace, id)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func (r *resources) nativeAVBlockExists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, ctx.Err()
	}
	if !documentID.MatchString(id) {
		return false, ErrDocument
	}
	db, e := r.native.Database(indexdb.BlockTree)
	if e != nil {
		return false, e
	}
	rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	return len(rows) > 0, e
}

type NativeAVPrimary struct {
	Name             string          `json:"attributeViewName"`
	DatabaseBlockIDs []string        `json:"databaseBlockIDs"`
	KeyValues        json.RawMessage `json:"keyValues"`
}

func (l *Lease) NativeAttributeViewPrimaryValues(ctx context.Context, id, keyword string, page, pageSize int) (ret NativeAVPrimary, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		_, doc, e := readNativeAVSource(ctx, w, id)
		if e != nil {
			return e
		}
		if !r.blockTreeReady {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		name := strings.TrimSpace(doc.Name)
		if name == "" {
			if r.configurationBusiness == nil {
				return conf.ErrBusinessUnbound
			}
			name = r.configurationBusiness.Language(r.configuration.Lang, 105, false, r.configuration.CloudRegion)
		}
		relations := map[string][]string{}
		raw, e := readNativeFile(ctx, w, "storage/av/blocks.msgpack", 16<<20)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		if e == nil {
			if e = msgpack.Unmarshal(raw, &relations); e != nil {
				return e
			}
		}
		if len(relations) > 100000 {
			return ErrBudget
		}
		for _, block := range relations[id] {
			if block != "" && !documentID.MatchString(block) {
				return ErrDocument
			}
		}
		registry, e := r.native.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		rows, e := (indexdb.BlockTrees{DB: registry}).Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: relations[id]})
		if e != nil {
			return e
		}
		present := map[string]bool{}
		for _, row := range rows {
			present[row.ID] = true
		}
		blocks, e := avquery.MirrorIDs(ctx, relations[id], func(ctx context.Context, id string) (bool, error) { return present[id], ctx.Err() })
		if e != nil {
			return e
		}
		var primary map[string]json.RawMessage
		for _, raw := range doc.KeyValues {
			var kv struct {
				Key *struct {
					Type string `json:"type"`
				} `json:"key"`
			}
			if e = json.Unmarshal(raw, &kv); e != nil {
				return e
			}
			if kv.Key == nil {
				return ErrDocument
			}
			if kv.Key.Type == "block" {
				if e = json.Unmarshal(raw, &primary); e != nil {
					return e
				}
				break
			}
		}
		if primary == nil {
			return ErrDocument
		}
		var values []json.RawMessage
		if e = json.Unmarshal(primary["values"], &values); e != nil && len(primary["values"]) > 0 {
			return e
		}
		selected, e := avquery.PrimaryValues(ctx, values, func(raw json.RawMessage) (avquery.PrimaryMeta, error) {
			var v struct {
				Type     string `json:"type"`
				Detached bool   `json:"isDetached"`
				Block    *struct {
					ID, Content string
					Updated     int64
				} `json:"block"`
			}
			if e := json.Unmarshal(raw, &v); e != nil {
				return avquery.PrimaryMeta{}, e
			}
			if v.Block == nil || v.Type != "block" {
				return avquery.PrimaryMeta{}, ErrDocument
			}
			return avquery.PrimaryMeta{BoundID: v.Block.ID, Text: avquery.BlockText(v.Block.Content), Updated: v.Block.Updated, Detached: v.Detached}, nil
		}, r.nativeAVBlockExists, keyword, page, pageSize)
		if e != nil {
			return e
		}
		// Project only known result values; retain every other original/unknown field.
		if len(selected) == 0 {
			delete(primary, "values")
		} else {
			primary["values"], e = json.Marshal(selected)
			if e != nil {
				return e
			}
		}
		projected, e := json.Marshal(primary)
		if e != nil {
			return e
		}
		ret = NativeAVPrimary{Name: name, DatabaseBlockIDs: blocks, KeyValues: projected}
		return nil
	})
	if err != nil {
		return NativeAVPrimary{}, err
	}
	return
}

type NativeAVView struct {
	View    json.RawMessage   `json:"view"`
	Filters []json.RawMessage `json:"filters"`
	Sorts   []json.RawMessage `json:"sorts"`
}

// Select the block override/current/first view, but never run rendering/fixing or
// view calculations that could save the AV, change items or schedule work.
func (l *Lease) NativeAttributeViewView(ctx context.Context, id, blockID string) (ret NativeAVView, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		_, doc, e := readNativeAVSource(ctx, w, id)
		if e != nil {
			return e
		}
		requested := ""
		if blockID != "" {
			if !documentID.MatchString(blockID) {
				return ErrDocument
			}
			if !r.blockTreeReady {
				if e = r.rebuildBlockTree(ctx, w); e != nil {
					return e
				}
			}
			exists, e := r.nativeAVBlockExists(ctx, blockID)
			if e != nil {
				return e
			}
			if exists {
				tree, e := r.nativeTreeByBlock(ctx, w, blockID)
				if e != nil {
					return e
				}
				found := false
				ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
					if enter && n.ID == blockID {
						found = true
						requested = n.IALAttr("custom-sy-av-view")
						return ast.WalkStop
					}
					return ast.WalkContinue
				})
				if !found {
					return ErrDocument
				}
			}
		}
		type view struct {
			Raw json.RawMessage
			ID  string
		}
		var views []view
		for _, raw := range doc.Views {
			var v *struct {
				ID string `json:"id"`
			}
			if e = json.Unmarshal(raw, &v); e != nil {
				return e
			}
			if v == nil {
				return ErrDocument
			}
			views = append(views, view{Raw: raw, ID: v.ID})
		}
		selected, e := avquery.CurrentView(ctx, views, requested, doc.ViewID, func(v view) string { return v.ID })
		if e != nil {
			return e
		}
		var contents struct {
			Filters []json.RawMessage `json:"filters"`
			Sorts   []json.RawMessage `json:"sorts"`
		}
		if e = json.Unmarshal(selected.Raw, &contents); e != nil {
			return e
		}
		for _, rules := range [][]json.RawMessage{contents.Filters, contents.Sorts} {
			for _, rule := range rules {
				trimmed := bytes.TrimSpace(rule)
				if len(trimmed) == 0 || trimmed[0] != '{' {
					return ErrDocument
				}
			}
		}
		if len(contents.Filters) == 0 {
			contents.Filters = []json.RawMessage{}
		}
		if len(contents.Sorts) == 0 {
			contents.Sorts = []json.RawMessage{}
		}
		ret = NativeAVView{View: append(json.RawMessage(nil), selected.Raw...), Filters: contents.Filters, Sorts: contents.Sorts}
		return nil
	})
	if err != nil {
		return NativeAVView{}, err
	}
	return
}
