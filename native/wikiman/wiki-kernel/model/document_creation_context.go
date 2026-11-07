// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/util"
)

func captureOwnedCreationServices(ctx, life context.Context) (doclife.CreationServices, error) {
	o, e := captureLegacyDocumentLifecycle(ctx)
	if e != nil {
		return doclife.CreationServices{}, e
	}
	var settings conf.AppData
	if e = o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error { var e error; settings, e = v.Clone(); return e }); e != nil {
		return doclife.CreationServices{}, e
	}
	load := func(c context.Context, id string) (*parse.Tree, error) {
		loc, e := o.r.referenceLookup(c, id)
		if e != nil {
			return nil, e
		}
		if loc == nil {
			return nil, ErrBlockNotFound
		}
		return o.r.loadRepairTree(c, loc.BoxID, loc.Path, o.parser())
	}
	progress := ""
	services := doclife.CreationServices{Strict: true, Now: time.Now, NewID: newLifecycleID, Language: o.language, Untitled: func() string { return o.plain(16) }, BoxMissing: ErrBoxNotFound,
		Warning: func(e error) { logging.LogWarnf("document creation: %s", e) },
		Box: func(c context.Context, id string) (bool, error) {
			if e := o.guard(c); e != nil {
				return false, e
			}
			boxes, e := o.owner.business.Notebooks(c)
			if e != nil {
				return false, e
			}
			for _, box := range boxes {
				if box.ID == id && !box.Closed {
					return true, nil
				}
			}
			return false, nil
		},
		Find: func(c context.Context, mode, box, value, parent string) (doclife.Location, bool, error) {
			if e := o.guard(c); e != nil {
				return doclife.Location{}, false, e
			}
			kind := ""
			switch mode {
			case "preferred":
				kind = "preferred"
			case "path":
				kind = "root-path"
			case "hpath":
				kind = "root-hpath"
			default:
				return doclife.Location{}, false, doclife.ErrDependency
			}
			rows, e := o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: kind, BoxID: box, Value: value, ParentID: parent})
			if e != nil || len(rows) == 0 {
				return doclife.Location{}, false, e
			}
			row := rows[0]
			return doclife.Location{Box: row.BoxID, Path: row.Path, HPath: row.HPath, ID: row.RootID, Document: true}, true, nil
		},
		Create: func(c context.Context, box, p, title, dom string) (*parse.Tree, error) {
			plan, e := o.execute(c, doclife.Request{Action: "create-dom", Box: box, Path: p, Title: title, DOM: dom})
			return plan.Tree, e
		},
		Load: load, Parser: func(c context.Context) (*lute.Lute, error) {
			if e := o.guard(c); e != nil {
				return nil, e
			}
			return o.parser(), nil
		},
		Flush: func(c context.Context) error {
			if e := o.guard(c); e != nil {
				return e
			}
			if e := FlushTxQueueContext(c); e != nil {
				return e
			}
			return o.r.queue.Flush(c)
		},
		Attrs: func(c context.Context, id string, attrs map[string]string) error {
			if e := o.guard(c); e != nil {
				return e
			}
			return SetBlockAttrsContext(c, id, attrs)
		},
		Stage: func(c context.Context, tree *parse.Tree, mode string) (err error) {
			if e := o.guard(c); e != nil {
				return e
			}
			if mode == "create" {
				if e := createTreeTxContext(c, tree); e != nil {
					return e
				}
				if e := FlushTxQueueContext(c); e != nil {
					return e
				}
				return o.r.queue.Flush(c)
			}
			io, closeIO, e := o.r.openRepairIOChecked(c, true)
			if e != nil {
				return e
			}
			defer func() { err = joinRepairCloseError(err, closeIO()) }()
			if e = o.r.writeRepairTree(c, io, tree, o.parser()); e != nil {
				return e
			}
			critical := context.WithoutCancel(c)
			if e = o.r.registry.Upsert(critical, tree); e != nil {
				return e
			}
			if e = o.r.queue.Upsert(critical, tree); e != nil {
				return e
			}
			return o.r.queue.Flush(critical)
		},
		Sort: func(c context.Context, box, mode, p, id string) error {
			if e := o.guard(c); e != nil {
				return e
			}
			if mode == "after" {
				return InsertDocumentSortContext(c, box, "", id, p, nil)
			}
			return InsertDocumentSortContext(c, box, p, id, "", nil)
		},
		Sync: func(c context.Context) error {
			if e := o.guard(c); e != nil {
				return e
			}
			if settings.Sync == nil {
				return conf.ErrBusinessUnbound
			}
			o.owner.syncRuntime.same.Store(0)
			o.owner.planSyncAfter(time.Duration(settings.Sync.Interval) * time.Second)
			return nil
		},
		Daily: func(c context.Context, box string) (out doclife.DailySettings, err error) {
			io, closeIO, e := o.r.openRepairIOChecked(c, false)
			if e != nil {
				return out, e
			}
			defer func() {
				err = joinRepairCloseError(err, closeIO())
				if err != nil {
					out = doclife.DailySettings{}
				}
			}()
			configuration, _, e := notebooks.Configuration(c, io.Source, box, ".siyuan")
			if e != nil {
				return out, e
			}
			return doclife.DailySettings{SavePath: configuration.DailyNoteSavePath, TemplatePath: configuration.DailyNoteTemplatePath}, nil
		},
		RenderPath: func(c context.Context, s string) (string, error) {
			if e := o.guard(c); e != nil {
				return "", e
			}
			return RenderGoTemplateContext(c, s)
		},
		Template: func(c context.Context, p, id string) (out *parse.Tree, dom string, err error) {
			if !fs.ValidPath(p) || p == "." || strings.Contains(p, "\\") {
				return nil, "", indexrepair.ErrSourcePath
			}
			io, closeIO, e := o.r.openRepairIOChecked(c, false)
			if e != nil {
				return nil, "", e
			}
			e = io.Source.Check(path.Join("templates", p), false)
			e = joinRepairCloseError(e, closeIO())
			if indexrepair.IsSourceNotExist(e) {
				logging.LogWarnf("daily note template missing: %s", p)
				return nil, "", nil
			}
			if e != nil {
				return nil, "", e
			}
			return RenderTemplateContext(c, filepath.Join(o.data, "templates", p), id, false)
		},
		Mirror: func(c context.Context, id, block string) (err error) {
			io, closeIO, e := o.r.openRepairIOChecked(c, true)
			if e != nil {
				return e
			}
			defer func() { err = joinRepairCloseError(err, closeIO()) }()
			return o.r.bindRepairAV(c, io.Source, id, block)
		},
		Progress: func(c context.Context, start bool) error {
			if start {
				if e := o.guard(c); e != nil {
					return e
				}
				o.owner.publishSyncUI(c, func() { progress = util.PushMsg(o.language(116), 30000) })
			} else {
				o.owner.publishSyncUI(life, func() { util.PushClearMsg(progress) })
			}
			return nil
		},
	}
	return services, o.guard(ctx)
}

func runOwnedDocumentCreation(ctx context.Context, q doclife.CreationRequest) (out doclife.CreationResult, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	q.Tree = nil
	err = base.run(ctx, func(c, life context.Context) error {
		services, e := captureOwnedCreationServices(c, life)
		if e != nil {
			return e
		}
		out, e = doclife.CreatePeripheral(c, q, services)
		return e
	})
	if err != nil {
		return doclife.CreationResult{}, err
	}
	return
}
func DuplicateDocContext(ctx context.Context, id string) (*parse.Tree, error) {
	result, e := runOwnedDocumentCreation(ctx, doclife.CreationRequest{Mode: "duplicate", ID: id})
	return result.Tree, e
}
