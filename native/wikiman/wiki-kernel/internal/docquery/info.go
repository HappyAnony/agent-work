// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/blockinfo.go document metadata and backlink promotion.
package docquery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"slices"
	"strings"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

type Info struct {
	ID           string            `json:"id"`
	RootID       string            `json:"rootID"`
	Name         string            `json:"name"`
	RefCount     int               `json:"refCount"`
	SubFileCount int               `json:"subFileCount"`
	RefIDs       []string          `json:"refIDs"`
	IAL          map[string]string `json:"ial"`
	Icon         string            `json:"icon"`
	AttrViews    []*InfoAV         `json:"attrViews"`
}
type InfoAV struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type RefDefs struct {
	RefID  string   `json:"refID"`
	DefIDs []string `json:"defIDs"`
}
type InfoRequest struct {
	IDs                   []string
	Batch, References, AV bool
}
type InfoServices struct {
	Wait                                       func(context.Context) error
	Trees                                      func(context.Context, []string) (map[string]*parse.Tree, error)
	Exists                                     func(context.Context, string) (bool, error)
	References                                 func(context.Context, string, string) ([]string, error)
	AVName                                     func(context.Context, string) (string, error)
	DefaultAVName                              func(context.Context) (string, error)
	Subfiles                                   func(context.Context, *parse.Tree) (int, error)
	IgnoreAVErrors, IgnoreSingleDirectoryError bool
	Warn                                       func(string, string)
}

var ErrInfoDependency = errors.New("document_info_dependencies_unbound")

func Infos(ctx context.Context, q InfoRequest, s InfoServices) (ret []*Info, err error) {
	if ctx == nil || s.Wait == nil || s.Trees == nil || s.Exists == nil || s.Subfiles == nil || ((!q.Batch || q.References) && s.References == nil) || ((!q.Batch || q.AV) && (s.AVName == nil || s.DefaultAVName == nil)) {
		return nil, ErrInfoDependency
	}
	q.IDs = slices.Clone(q.IDs)
	guard := func(fn func() error) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := fn(); e != nil {
			return e
		}
		return ctx.Err()
	}
	if err = guard(func() error { return s.Wait(ctx) }); err != nil {
		return nil, err
	}
	var trees map[string]*parse.Tree
	if err = guard(func() (e error) { trees, e = s.Trees(ctx, slices.Clone(q.IDs)); return }); err != nil {
		return nil, err
	}
	for _, id := range q.IDs {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		t := trees[id]
		if t == nil {
			continue
		}
		if t.Root == nil {
			return nil, errors.New("document_info_invalid_tree")
		}
		info := &Info{ID: id, RootID: t.Root.ID, Name: t.Root.IALAttr("title"), IAL: parse.IAL2Map(t.Root.KramdownIAL), Icon: t.Root.IALAttr("icon")}
		if err = cleanScroll(ctx, info.IAL, s); err != nil {
			return nil, err
		}
		if !q.Batch || q.References {
			if err = guard(func() (e error) { info.RefIDs, e = s.References(ctx, id, t.Root.ID); return }); err != nil {
				return nil, err
			}
			info.RefIDs = slices.Clone(info.RefIDs)
			if len(info.RefIDs) == 0 {
				info.RefIDs = []string{}
			}
			info.RefCount = len(info.RefIDs)
		}
		if !q.Batch || q.AV {
			for _, avID := range strings.Split(info.IAL["custom-avs"], ",") {
				// Original empty-ID lookup fails and is skipped; no default AV is added.
				if avID == "" {
					continue
				}
				var name string
				e := guard(func() (e error) { name, e = s.AVName(ctx, avID); return })
				if e != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					if s.IgnoreAVErrors {
						continue
					}
					return nil, e
				}
				if name == "" {
					if err = guard(func() (e error) { name, e = s.DefaultAVName(ctx); return }); err != nil {
						return nil, err
					}
				}
				info.AttrViews = append(info.AttrViews, &InfoAV{ID: avID, Name: name})
			}
		}
		e := guard(func() (e error) { info.SubFileCount, e = s.Subfiles(ctx, t); return })
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if q.Batch || !s.IgnoreSingleDirectoryError {
				return nil, e
			}
			info.SubFileCount = 0
		}
		ret = append(ret, info)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return
}

func cleanScroll(ctx context.Context, ial map[string]string, s InfoServices) error {
	raw := ial["scroll"]
	if raw == "" {
		return nil
	}
	var data map[string]interface{}
	invalid := json.Unmarshal([]byte(raw), &data) != nil
	var ids []interface{}
	if !invalid {
		if v := data["zoomInId"]; v != nil {
			ids = append(ids, v)
		} else {
			if v := data["startId"]; v != nil {
				ids = append(ids, v)
			}
			if v := data["endId"]; v != nil {
				ids = append(ids, v)
			}
		}
	}
	for _, v := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		id, ok := v.(string)
		if !ok {
			invalid = true
			continue
		}
		found, e := s.Exists(ctx, id)
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if !found {
			invalid = true
		}
	}
	if invalid {
		delete(ial, "scroll")
		if s.Warn != nil {
			s.Warn("scroll", raw)
		}
	}
	return ctx.Err()
}

type RefPromotionServices struct {
	Blocks        func(context.Context, []string) ([]indexdb.BlockRecord, error)
	Parser        func(context.Context) (*lute.Lute, error)
	CaseSensitive bool
}

// Preserve original SQL row order, last paragraph per parent and duplicate
// promoted IDs. There is deliberately no new dedup/sort of the final count.
func PromoteRefDefs(ctx context.Context, defs []*RefDefs, s RefPromotionServices) (map[string]string, error) {
	if ctx == nil || s.Blocks == nil || s.Parser == nil {
		return nil, ErrInfoDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	var ids []string
	for _, d := range defs {
		ids = append(ids, d.RefID)
	}
	rows, e := s.Blocks(ctx, ids)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	paragraphs := map[string]*blockview.Block{}
	var parents []string
	for i := range rows {
		b := blockview.FromRow(&rows[i], "", 12, s.CaseSensitive)
		if b.Type == "NodeParagraph" {
			paragraphs[b.ParentID] = b
			parents = append(parents, b.ParentID)
		}
	}
	rows, e = s.Blocks(ctx, parents)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	parser, e := s.Parser(ctx)
	if e != nil {
		return nil, e
	}
	if parser == nil || parser.ParseOptions == nil {
		return nil, ErrInfoDependency
	}
	ret := map[string]string{}
	for i := range rows {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		p := blockview.FromRow(&rows[i], "", 12, s.CaseSensitive)
		if p.Type != "NodeListItem" && p.Type != "NodeBlockquote" && p.Type != "NodeSuperBlock" {
			continue
		}
		ref := paragraphs[p.ID]
		if ref == nil {
			continue
		}
		promote := true
		if p.Type == "NodeListItem" && p.FContent != ref.Content {
			inline := parse.Inline("", []byte(ref.Markdown), parser.ParseOptions)
			if inline != nil {
				if inline.Root == nil || inline.Root.FirstChild == nil {
					return nil, errors.New("document_info_invalid_inline")
				}
				for c := inline.Root.FirstChild.FirstChild; c != nil; c = c.Next {
					if docview.IsBlockRef(c) {
						continue
					}
					if strings.TrimSpace(c.Text()) != "" {
						promote = false
						break
					}
				}
			}
		}
		if promote {
			for _, d := range defs {
				if d.RefID == ref.ID {
					d.RefID = p.ID
					break
				}
			}
		}
		ret[p.ID] = ref.ID
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return ret, nil
}
