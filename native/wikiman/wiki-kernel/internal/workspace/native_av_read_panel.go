// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"sort"
	"strings"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/indexdb"
)

type NativeBlockAVKeys struct {
	AVID      string          `json:"avID"`
	AVName    string          `json:"avName"`
	BlockIDs  []string        `json:"blockIDs"`
	KeyValues []*av.KeyValues `json:"keyValues"`
}

func (l *Lease) NativeAttributeViewImages(ctx context.Context, id, view, query string) ([]string, error) {
	raw, e := l.nativeAttributeViewRead(ctx, id, view, query, 1, 100000, nil, true, true, nil)
	if e != nil {
		return nil, e
	}
	var table av.Table
	if e = json.Unmarshal(raw, &table); e != nil {
		return nil, e
	}
	return avrender.Images(ctx, &table)
}

// Mirrors are resolved from the actual blocktree and source AST; no registry
// repair/upsert is performed by a reader.
func (l *Lease) NativeAttributeViewMirrors(ctx context.Context, id string) (out []string, err error) {
	out = []string{}
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "av"})
		if e != nil {
			return e
		}
		for _, row := range rows {
			tree, e := r.nativeTreeByBlock(ctx, w, row.ID)
			if e != nil {
				return e
			}
			ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if enter && n.ID == row.ID && n.AttributeViewID == id {
					out = append(out, n.ID)
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return
}

func (l *Lease) NativeBlockAttributeViewKeys(ctx context.Context, nodeID string) (out []NativeBlockAVKeys, err error) {
	attrs, err := l.ReadNativeBlockAttributes(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	out = []NativeBlockAVKeys{}
	for _, id := range strings.Split(attrs[av.NodeAttrNameAvs], ",") {
		if id == "" {
			continue
		}
		mirrors, e := l.NativeAttributeViewMirrors(ctx, id)
		if e != nil {
			return nil, e
		}
		if len(mirrors) == 0 {
			continue
		}
		var value NativeBlockAVKeys
		_, e = l.nativeAttributeViewRead(ctx, id, attrs[av.NodeAttrView], "", 1, 100000, nil, true, true, func(a *av.AttributeView) error {
			if !a.ExistBoundBlock(nodeID) {
				return nil
			}
			block := a.GetBlockValueByBoundID(nodeID)
			if block == nil {
				return nil
			}
			values := avrender.BlockValues(a, block.BlockID)
			order := map[string]int{}
			for i, key := range a.KeyIDs {
				order[key] = i
			}
			for _, kv := range a.KeyValues {
				if _, ok := order[kv.Key.ID]; !ok {
					order[kv.Key.ID] = len(order)
				}
			}
			sort.SliceStable(values, func(i, j int) bool { return order[values[i].Key.ID] < order[values[j].Key.ID] })
			name := strings.TrimSpace(a.Name)
			if name == "" {
				name = l.workspace.resources.configurationBusiness.Language(l.workspace.resources.configuration.Lang, 105, false, 0)
			}
			raw, e := json.Marshal(values)
			if e != nil {
				return e
			}
			var detached []*av.KeyValues
			if e = json.Unmarshal(raw, &detached); e != nil {
				return e
			}
			value = NativeBlockAVKeys{id, name, mirrors, detached}
			return nil
		})
		if e != nil {
			return nil, e
		}
		if value.AVID != "" {
			out = append(out, value)
		}
	}
	return out, ctx.Err()
}
