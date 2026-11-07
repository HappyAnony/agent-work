// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

type documentLifecycleEnvironment struct{ services doclife.Services }

func (tx *Transaction) documentLifecycle(q doclife.Request) (doclife.Plan, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.documentLifecycle == nil {
			return doclife.Plan{}, doclife.ErrDependency
		}
		return doclife.Execute(tx.environment.ctx, q, tx.environment.documentLifecycle.services)
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return runLegacyDocumentLifecycle(ctx, q)
}

func (o *legacyDocumentLifecycleContext) apply(ctx, life context.Context, p *doclife.Plan) error {
	if e := o.guard(ctx); e != nil {
		return e
	}
	critical := ctx
	accepted := func() error {
		critical = life
		return o.guard(critical)
	}
	q := p.Request
	switch q.Action {
	case "create", "create-dom":
		box := o.owner.Box(q.Box)
		if box == nil {
			return errors.New(o.language(0))
		}
		folder := path.Dir(p.Tree.Path)
		if !box.Exist(folder) {
			if e := box.MkdirAll(folder); e != nil {
				return e
			}
		}
		if e := createTreeTxContext(ctx, p.Tree); e != nil {
			return e
		}
		if e := accepted(); e != nil {
			return e
		}
		if e := FlushTxQueueContext(critical); e != nil {
			return e
		}
		if q.Action == "create" {
			if len(q.Sorts) > 0 {
				if e := ChangeFileTreeSortContext(critical, q.Box, q.Sorts); e != nil {
					return e
				}
			} else {
				if e := InsertDocumentSortContext(critical, q.Box, folder, p.Tree.ID, "", nil); e != nil {
					return e
				}
			}
		}
	case "rename":
		t := p.Tree
		if e := renameWriteJSONQueue(t); e != nil {
			return e
		}
		if e := accepted(); e != nil {
			return e
		}
		evt := util.NewCmdResult("rename", 0, util.PushModeBroadcast)
		evt.Data = map[string]interface{}{"box": q.Box, "id": t.Root.ID, "path": q.Path, "title": t.Root.IALAttr("title"), "refText": getNodeRefText(t.Root)}
		o.owner.publishSyncUI(critical, func() { util.PushEvent(evt) })
		box := o.owner.Box(q.Box)
		if box == nil {
			return errors.New(o.language(0))
		}
		box.renameSubTrees(t)
		if e := updateRefTextRenameDocContext(critical, t); e != nil {
			return e
		}
		if e := o.r.queue.Flush(critical); e != nil {
			return e
		}
		IncSync()
	case "move":
		// All path/depth/source preflight is shared. The explicit legacy
		// executor retains the original file/AV/sort/event implementations.
		to := o.owner.Box(q.ToBox)
		if to == nil {
			return errors.New(o.language(0))
		}
		roots := lifecycleRoots(p)
		needProgress := len(p.Changes)-len(roots) > 64
		if needProgress {
			defer util.PushClearProgress()
		}
		for i, from := range roots {
			if e := critical.Err(); e != nil {
				return e
			}
			if needProgress {
				util.PushEndlessProgress(fmt.Sprintf(o.language(70), fmt.Sprintf("%d/%d", i+1, len(q.Paths))))
			}
			box := o.owner.Box(from.Box)
			if box == nil {
				return errors.New(o.language(0))
			}
			if _, e := moveDoc(box, from.Path, to, q.ToPath, o.parser(), q.Callback); e != nil {
				return e
			}
			if critical == ctx {
				if e := accepted(); e != nil {
					return e
				}
			}
		}
		if e := o.r.queue.Flush(critical); e != nil {
			return e
		}
		cache.ClearDocsIAL()
		IncSync()
	case "delete":
		util.PushEndlessProgress(o.language(116))
		defer util.PushClearProgress()
		for _, from := range lifecycleRoots(p) {
			if e := critical.Err(); e != nil {
				return e
			}
			box := o.owner.Box(from.Box)
			if box == nil {
				return errors.New(o.language(0))
			}
			tree, childrenDir, e := removeDoc(box, from.Path, o.parser())
			if e != nil {
				return e
			}
			if critical == ctx {
				if e := accepted(); e != nil {
					return e
				}
			}
			if tree != nil {
				if _, e = o.r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "root", Value: tree.ID}); e != nil {
					return e
				}
				if _, e = o.r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "prefix", Value: childrenDir}); e != nil {
					return e
				}
				if e = o.r.queue.Remove(critical, tree.ID); e != nil {
					return e
				}
				if e = o.r.queue.RemovePath(critical, tree.Box, childrenDir); e != nil {
					return e
				}
				cache.RemoveDocIAL(tree.Path)
				if e = o.owner.scheduleReferenceCounts(critical, o.r, getRefDefIDs(tree.Root), util.SQLFlushInterval); e != nil {
					return e
				}
			}
		}
		if e := o.r.queue.Flush(critical); e != nil {
			return e
		}
		if q.Path != "" {
			IncSync()
		}
	case "restore":
		return o.applyRestore(ctx, life, p)
	default:
		return doclife.ErrDependency
	}
	return errors.Join(o.guard(critical), ctx.Err())
}

func lifecycleRoots(p *doclife.Plan) []doclife.Location {
	var ret []doclife.Location
	for _, c := range p.Changes {
		child := false
		for _, r := range ret {
			if c.From.Box == r.Box && strings.HasPrefix(c.From.Path, util.StripDocExt(r.Path)+"/") {
				child = true
				break
			}
		}
		if !child {
			ret = append(ret, c.From)
		}
	}
	return ret
}

func (o *legacyDocumentLifecycleContext) applyRestore(ctx, life context.Context, p *doclife.Plan) (err error) {
	if err = o.guard(ctx); err != nil {
		return err
	}
	q, t := p.Request, p.Tree
	rootID := p.IDs[0]
	if p.Working != nil && p.Working.Document {
		if e := filelock.Remove(filepath.Join(o.data, q.Box, p.Working.Path)); e != nil {
			return e
		}
	}
	if e := os.MkdirAll(filepath.Join(o.data, q.Box, path.Dir(t.Path)), 0755); e != nil {
		return e
	}
	historyDir := strings.TrimPrefix(q.HistoryPath, o.history+string(os.PathSeparator))
	if i := strings.Index(historyDir, string(os.PathSeparator)); i >= 0 {
		historyDir = historyDir[:i]
	}
	historyDir = filepath.Join(o.history, historyDir)
	for _, id := range p.AVIDs {
		if e := ctx.Err(); e != nil {
			return e
		}
		src, dest := filepath.Join(historyDir, "storage", "av", id+".json"), filepath.Join(o.data, "storage", "av", id+".json")
		if filelock.IsExist(dest) {
			if e := filelock.CopyNewtimes(src, dest); e != nil {
				// Original restoration continues when an AV preimage is absent.
				logging.LogErrorf("copy av [%s] failed: %s", src, e)
			}
		}
	}
	// The first source mutation has been accepted. Finish the captured source,
	// registry and derived queue handoff under the owner lifetime.
	if err = o.guard(life); err != nil {
		return err
	}
	if p.Working != nil {
		if _, err = o.r.registry.Remove(life, indexdb.BlockTreeFilter{Kind: "root", Value: rootID}); err != nil {
			return err
		}
	}
	if err = o.r.queue.Remove(life, rootID); err != nil {
		return err
	}
	ioctx, closeIO, err := o.r.openRepairIOChecked(life, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	if err = ioctx.Write(life, t, nil, o.parser()); err != nil {
		return err
	}
	if err = o.r.queue.Index(life, t); err != nil {
		return err
	}
	if err = o.r.queue.Flush(life); err != nil {
		return err
	}
	if err = closeIO(); err != nil {
		return err
	}
	closeIO = func() error { return nil }
	events := o.sourceEvents(life)
	if err = reloadProtyleWithEnvironment(rootID, events); err != nil {
		return err
	}
	if err = o.schedule(life, 200*time.Millisecond, func() error { util.PushReloadFiletree(); return nil }); err != nil {
		return err
	}
	o.owner.publishSyncUI(life, func() { util.PushMsg(o.language(102), 3000) })
	IncSync()
	for _, id := range p.AVIDs {
		id := id
		if err = o.schedule(life, 200*time.Millisecond, func() error { pushReloadAttrView(id); return nil }); err != nil {
			return err
		}
	}
	if err = o.scheduleRestoreFollowup(life, q.Box, rootID); err != nil {
		return err
	}
	return ctx.Err()
}

// Keep legacy ID generation explicit; owned callers inject their own source.
func newLifecycleID() string { return ast.NewNodeID() }
