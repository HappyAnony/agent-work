// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func SearchAttributeViewContext(ctx context.Context, keyword string, exclude []string) ([]*AvSearchResult, error) {
	exclude = append([]string(nil), exclude...)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out []*AvSearchResult, err error) {
		if e := waitForSyncingStoragesOwnerContext(c, o.base.owner); e != nil {
			return nil, e
		}
		io, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return nil, e
		}
		defer func() {
			err = joinRepairCloseError(err, closeIO())
			if err != nil {
				out = nil
			}
		}()
		catalog, e := avquery.ReadSourceCatalog(c, io.Source)
		if e != nil {
			return nil, e
		}
		boxes, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return nil, e
		}
		names := map[string]string{}
		for _, box := range boxes {
			names[box.ID] = box.Name
		}
		results, e := avquery.Search(c, keyword, exclude, avquery.Services{Strict: true, Entries: catalog.Entries, Relations: catalog.Relations,
			Views: func(c context.Context, id string) ([]avquery.View, error) {
				if e := o.r.guard(c); e != nil {
					return nil, e
				}
				raw, exists := catalog.Documents[id]
				if !exists {
					return nil, indexrepair.ErrSourcePath
				}
				var view *av.AttributeView
				if e := json.Unmarshal(raw, &view); e != nil {
					return nil, e
				}
				if view == nil {
					return nil, indexrepair.ErrSourcePath
				}
				if e := avmodel.UpgradeInMemory(view, avmodel.UpgradeEnvironment{Now: time.Now, NewID: ast.NewNodeID}); e != nil {
					return nil, e
				}
				result := []avquery.View{}
				for _, v := range view.Views {
					if v == nil {
						return nil, indexrepair.ErrSourcePath
					}
					result = append(result, avquery.View{ID: v.ID, Name: v.Name, Layout: string(v.LayoutType)})
				}
				return result, nil
			},
			Resolve: func(c context.Context, ids []string) (avquery.Location, bool, error) {
				for _, id := range ids {
					loc, e := o.r.referenceLookup(c, id)
					if e != nil {
						return avquery.Location{}, false, e
					}
					if loc == nil {
						continue
					}
					tree, e := o.r.loadRepairTree(c, loc.BoxID, loc.Path, o.parser)
					if e != nil {
						return avquery.Location{}, false, e
					}
					node := docview.GetNodeInTree(tree, id)
					if node == nil || node.Type != ast.NodeAttributeView || node.AttributeViewID == "" {
						continue
					}
					rows, e := o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: tree.Box, Value: tree.Path})
					if e != nil {
						return avquery.Location{}, false, e
					}
					hpath := ""
					if len(rows) > 0 {
						hpath = rows[0].HPath
					}
					hpath = names[tree.Box] + hpath
					return avquery.Location{BlockID: id, HPath: hpath}, true, nil
				}
				return avquery.Location{}, false, nil
			},
		})
		if e != nil {
			return nil, e
		}
		if e = catalog.Confirm(c, io.Source); e != nil {
			return nil, e
		}
		out = []*AvSearchResult{}
		for _, result := range results {
			parent := &AvSearchResult{AvID: result.AvID, AvName: result.AvName, BlockID: result.BlockID, HPath: result.HPath}
			for _, child := range result.Children {
				parent.Children = append(parent.Children, &AvSearchResult{AvID: child.AvID, AvName: child.AvName, ViewName: child.ViewName, ViewID: child.ViewID, ViewLayout: av.LayoutType(child.ViewLayout), BlockID: child.BlockID, HPath: child.HPath})
			}
			out = append(out, parent)
		}
		return out, nil
	})
}
