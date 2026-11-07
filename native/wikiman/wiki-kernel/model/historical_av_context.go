// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"io/fs"
	"os"
	"strconv"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/repoview"
)

type historicalAVRequest struct {
	Mode, IndexID, Created, BlockID, AVID, ViewID, Query string
	Page, Size                                           int
	Paging                                               map[string]interface{}
}
type historicalAVResult struct {
	View av.Viewable
	AV   *av.AttributeView
}

func renderHistoricalAVContext(ctx context.Context, q historicalAVRequest) (av.Viewable, *av.AttributeView, error) {
	if q.Paging != nil {
		raw, e := json.Marshal(q.Paging)
		if e != nil {
			return nil, nil, e
		}
		q.Paging = nil
		if e = json.Unmarshal(raw, &q.Paging); e != nil {
			return nil, nil, e
		}
	}
	v, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out historicalAVResult, err error) {
		if !ast.IsNodeIDPattern(q.AVID) {
			return out, avrender.ErrShape
		}
		selected := avrender.HistorySource{}
		switch q.Mode {
		case "repo":
			service, e := o.base.owner.CloudRepoService()
			if e != nil {
				return out, e
			}
			result, e := service.ReadRepository(c, repoview.Request{Kind: "snapshot-file", ID: q.IndexID, Path: "storage/av/" + q.AVID + ".json"})
			if e != nil {
				return out, e
			}
			selected = avrender.HistorySource{Selected: true, Missing: result.Path == "", Raw: bytes.Clone(result.Data)}
		case "history":
			if _, e := strconv.ParseInt(q.Created, 10, 64); e != nil {
				return out, e
			}
			info, e := os.Lstat(o.base.history)
			if os.IsNotExist(e) {
				return out, nil
			}
			if e != nil {
				return out, e
			}
			if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
				return out, indexrepair.ErrSourcePath
			}
			root, e := os.OpenRoot(o.base.history)
			if e != nil {
				return out, e
			}
			defer func() {
				err = errors.Join(err, root.Close())
				if err != nil {
					out = historicalAVResult{}
				}
			}()
			actual, e := root.Stat(".")
			if e != nil {
				return out, e
			}
			if !os.SameFile(info, actual) {
				return out, indexrepair.ErrSourceChanged
			}
			source := indexrepair.Source{Data: root}
			selected, e = avrender.HistoryAttributeViewSource(c, q.Created, q.AVID, avrender.HistorySourcePorts{
				Directories: func(c context.Context) ([]string, error) {
					entries, e := source.ReadDir(c, ".")
					if e != nil {
						return nil, e
					}
					names := []string{}
					for _, entry := range entries {
						names = append(names, entry.Name())
					}
					return names, nil
				},
				History: source.Read, Current: func(c context.Context, id string) ([]byte, error) { return o.raw(c, "storage/av/"+id+".json") }, Missing: indexrepair.IsSourceNotExist, Location: time.Local,
			})
			if e != nil {
				return out, e
			}
		default:
			return out, avrender.ErrShape
		}
		if !selected.Selected {
			return out, nil
		}
		env := legacyTransactionEnvironmentContext(c)
		env.ctx = c
		env.parser = func() *lute.Lute { return o.parser }
		if e := bindLegacyCommit(c, env, o.r); e != nil {
			return out, e
		}
		binding := bindAVOperation(env.commitContext, c)
		binding.scope.readOnly = true
		binding.detachedSave = true
		var candidate *av.AttributeView
		var e error
		if selected.Missing {
			candidate, e = avmodel.DefaultAttributeView(q.AVID, env.commitContext.avLabels, ast.NewNodeID)
		} else {
			candidate = &av.AttributeView{}
			e = json.Unmarshal(selected.Raw, candidate)
		}
		if e != nil {
			return out, e
		}
		if candidate == nil || candidate.ID != q.AVID {
			return out, avrender.ErrShape
		}
		binding.scope.cached[q.AVID] = candidate
		binding.scope.versions[q.AVID] = bytes.Clone(selected.Raw)
		binding.scope.bind(candidate)
		out.View, e = renderAttributeView(candidate, q.BlockID, q.ViewID, q.Query, q.Page, q.Size, q.Paging, binding)
		if e != nil {
			return historicalAVResult{}, e
		}
		out.AV = candidate
		return out, c.Err()
	})
	return v.View, v.AV, e
}
func RenderRepoSnapshotAttributeViewContext(ctx context.Context, index, id string) (av.Viewable, *av.AttributeView, error) {
	return renderHistoricalAVContext(ctx, historicalAVRequest{Mode: "repo", IndexID: index, AVID: id, Page: 1, Size: -1})
}
func RenderHistoryAttributeViewContext(ctx context.Context, block, id, view, query string, page, size int, paging map[string]interface{}, created string) (av.Viewable, *av.AttributeView, error) {
	return renderHistoricalAVContext(ctx, historicalAVRequest{Mode: "history", Created: created, BlockID: block, AVID: id, ViewID: view, Query: query, Page: page, Size: size, Paging: paging})
}
