// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
	notebookmeta "wiki-kernel/internal/notebooks"
)

type documentTreeResult struct {
	Files []*File
	Total int
}

func runWorkspaceMetadataRead[T any](ctx context.Context, fn func(*workspaceReadOwner, context.Context) (T, error)) (out T, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return out, e
	}
	r.queryOnly = true
	if base.owner != r.owner || base.data != r.data {
		return out, conf.ErrBusinessUnbound
	}
	o := &workspaceReadOwner{base: base, r: r}
	if e = base.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error { var e error; o.settings, e = v.Clone(); return e }); e != nil {
		return out, e
	}
	err = base.run(ctx, func(c, life context.Context) error { var e error; out, e = fn(o, c); return e })
	if err != nil {
		var zero T
		return zero, err
	}
	return
}

func DocumentTreeIDsContext(ctx context.Context, box, p string) ([]*doctree.IDNode, error) {
	return runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out []*doctree.IDNode, result error) {
		io, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return nil, e
		}
		defer func() {
			result = joinRepairCloseError(result, closeIO())
			if result != nil {
				out = nil
			}
		}()
		return doctree.IDs(c, io.Source, box, p)
	})
}

func ListDocTreeContext(ctx context.Context, box, p string, sortMode int, cards, hidden bool, max int) ([]*File, int, error) {
	return listDocTreeContext(ctx, box, p, sortMode, cards, hidden, max, true)
}
func listDocTreeContext(ctx context.Context, box, p string, sortMode int, cards, hidden bool, max int, presentation bool) ([]*File, int, error) {
	q := doctree.Request{Box: box, Path: p, Sort: sortMode, Flashcard: cards, ShowHidden: hidden, Max: max}
	v, e := runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out documentTreeResult, result error) {
		if o.settings.FileTree == nil {
			return out, conf.ErrBusinessUnbound
		}
		io, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return out, e
		}
		defer func() {
			result = joinRepairCloseError(result, closeIO())
			if result != nil {
				out = documentTreeResult{}
			}
		}()
		now := time.Now()
		formatter := func(time.Time, time.Time) string { return "" }
		if presentation {
			formatter, e = o.base.owner.business.HumanizeTime(o.settings.Lang)
			if e != nil {
				return out, e
			}
		}
		notebooks, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return out, e
		}
		s := doctree.SourceServices(box, io.Source, doctree.Services{
			Settings: func(c context.Context, id string) (int, int, error) {
				for _, n := range notebooks {
					if n.ID == id {
						box, _, e := notebookmeta.Configuration(c, io.Source, id, ".siyuan")
						if e != nil {
							return 0, 0, e
						}
						return box.SortMode, o.settings.FileTree.Sort, c.Err()
					}
				}
				return 0, 0, ErrBlockNotFound
			},
			Project: func(c context.Context, f doctree.Entry, ial map[string]string) (*File, error) {
				return doctree.Metadata(c, f, ial, time.Local, func(t time.Time) (string, error) { return formatter(t, now), nil })
			},
			References: func(c context.Context) (map[string]int, error) {
				return (indexdb.References{DB: o.r.main.DB}).RootCounts(c)
			},
		}, box+"/.siyuan/sort.json")
		if cards {
			s.Cards = o.navigationCards()
		}
		out.Files, out.Total, e = doctree.List(c, q, s)
		return out, e
	})
	return v.Files, v.Total, e
}
