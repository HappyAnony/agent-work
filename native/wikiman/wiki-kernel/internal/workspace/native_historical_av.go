// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/repoview"
)

type NativeHistoricalAVRequest struct {
	Mode, IndexID, Created, AVID, ViewID, Query string
	Page, PageSize                              int
	Paging                                      map[string]NativeAVPaging
	DefaultLabels                               map[string]string
}

// Historical candidates share the original AV renderer and owner-local related
// data. Missing snapshots can create detached defaults only with explicit labels.
func (l *Lease) NativeHistoricalAttributeView(ctx context.Context, q NativeHistoricalAVRequest) (json.RawMessage, error) {
	if !documentID.MatchString(q.AVID) {
		return nil, ErrDocument
	}
	q.Paging = maps.Clone(q.Paging)
	q.DefaultLabels = maps.Clone(q.DefaultLabels)
	var selected avrender.HistorySource
	switch q.Mode {
	case "repo":
		if !documentID.MatchString(q.AVID) {
			return nil, ErrDocument
		}
		v, e := l.NativeReadRepository(ctx, repoview.Request{Kind: "snapshot-file", ID: q.IndexID, Path: "storage/av/" + q.AVID + ".json"})
		if e != nil {
			return nil, e
		}
		selected = avrender.HistorySource{Selected: true, Missing: v.Path == "", Raw: append([]byte(nil), v.Data...)}
	case "history":
		if _, e := strconv.ParseInt(q.Created, 10, 64); e != nil {
			return nil, e
		}
		e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) (result error) {
			w := l.workspace
			name := filepath.Join(w.paths.RuntimeDir, "history")
			info, e := os.Lstat(name)
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				return e
			}
			if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
				return ErrDocument
			}
			root, e := os.OpenRoot(name)
			if e != nil {
				return e
			}
			defer func() { result = errors.Join(result, root.Close()) }()
			actual, e := root.Stat(".")
			if e != nil {
				return e
			}
			if !os.SameFile(info, actual) {
				return ErrDocument
			}
			source := indexrepair.Source{Data: root}
			selected, e = avrender.HistoryAttributeViewSource(ctx, q.Created, q.AVID, avrender.HistorySourcePorts{
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
				}, History: source.Read,
				Current: func(c context.Context, id string) ([]byte, error) {
					raw, _, e := readNativeAVSource(c, w, id)
					return raw, e
				}, Missing: indexrepair.IsSourceNotExist, Location: time.Local,
			})
			return e
		})
		if e != nil {
			return nil, e
		}
	default:
		return nil, ErrDocument
	}
	if !selected.Selected {
		return nil, nil
	}
	raw := selected.Raw
	if selected.Missing {
		candidate, e := avmodel.DefaultAttributeView(q.AVID, q.DefaultLabels, ast.NewNodeID)
		if e != nil {
			return nil, e
		}
		raw, e = json.Marshal(candidate)
		if e != nil {
			return nil, e
		}
	}
	return l.nativeAttributeViewRender(ctx, q.AVID, q.ViewID, q.Query, q.Page, q.PageSize, q.Paging, false, json.RawMessage(raw))
}
