// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

// legacyDocumentLifecycleContext pins the compatibility workspace selected at
// admission. Its source mutations remain sequential compatibility operations;
// R04 owns the authoritative journal/receipt transaction.
type legacyDocumentLifecycleContext struct {
	r           *indexRepairContext
	owner       *AppConf
	data        string
	history     string
	markdown    nativeparser.Markdown
	allowDeeper bool
	language    func(int) string
	plain       func(int) string
}

func captureLegacyDocumentLifecycle(ctx context.Context) (*legacyDocumentLifecycleContext, error) {
	if ctx == nil {
		return nil, doclife.ErrDependency
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := captureIndexRepair(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = r.owner.CloudRepoService(); err != nil {
		return nil, err
	}
	o := &legacyDocumentLifecycleContext{r: r, owner: r.owner, data: r.data, history: util.HistoryDir}
	err = r.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.FileTree == nil || v.Editor == nil || v.Editor.Markdown == nil {
			return conf.ErrBusinessUnbound
		}
		o.allowDeeper = v.FileTree.AllowCreateDeeper
		o.markdown = *v.Editor.Markdown
		o.language = func(id int) string { return r.owner.business.Language(v.Lang, id, true, v.CloudRegion) }
		o.plain = func(id int) string { return r.owner.business.Language(v.Lang, id, false, v.CloudRegion) }
		return nil
	})
	if err != nil {
		return nil, err
	}
	return o, o.guard(ctx)
}

func (o *legacyDocumentLifecycleContext) guard(ctx context.Context) error {
	if o == nil || o.r == nil || o.owner == nil || o.language == nil || o.plain == nil {
		return doclife.ErrDependency
	}
	if err := o.r.guard(ctx); err != nil {
		return err
	}
	if o.owner != Conf || o.data != util.DataDir || o.history != util.HistoryDir {
		return conf.ErrBusinessUnbound
	}
	return nil
}

type legacyDocumentLifecycleResult struct {
	plan doclife.Plan
	err  error
}

// run registers before launching. CloseCloudRepo cancels and joins this exact
// operation before the compatibility workspace globals may be replaced.
func (o *legacyDocumentLifecycleContext) run(ctx context.Context, fn func(context.Context, context.Context) (doclife.Plan, error)) (doclife.Plan, error) {
	if fn == nil {
		return doclife.Plan{}, doclife.ErrDependency
	}
	if err := o.guard(ctx); err != nil {
		return doclife.Plan{}, err
	}
	base, baseErr := captureLegacyReindexOwner(ctx)
	if baseErr != nil {
		return doclife.Plan{}, baseErr
	}
	result := make(chan legacyDocumentLifecycleResult, 1)
	o.owner.repoMu.Lock()
	if o.owner.repoClosed || o.owner != Conf || o.data != util.DataDir || o.history != util.HistoryDir {
		o.owner.repoMu.Unlock()
		return doclife.Plan{}, conf.ErrBusinessUnbound
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	accepted := o.owner.repoTasks.Go(func(life context.Context) {
		life = context.WithValue(life, legacyAdmissionKey{}, base)
		out := legacyDocumentLifecycleResult{}
		op, cancel := context.WithCancel(context.WithValue(ctx, legacyAdmissionKey{}, base))
		stop := context.AfterFunc(life, cancel)
		defer func() {
			if recover() != nil {
				out = legacyDocumentLifecycleResult{err: errors.New("document_lifecycle_handler_panic")}
			}
			stop()
			cancel()
			result <- out
		}()
		out.plan, out.err = fn(op, life)
	})
	o.owner.repoMu.Unlock()
	if !accepted {
		return doclife.Plan{}, errors.New("document_lifecycle_owner_closed")
	}
	out := <-result
	return out.plan, out.err
}

func (o *legacyDocumentLifecycleContext) execute(ctx context.Context, q doclife.Request) (doclife.Plan, error) {
	return o.run(ctx, func(op, life context.Context) (doclife.Plan, error) {
		return doclife.Execute(op, q, o.services(life))
	})
}

func runLegacyDocumentLifecycle(ctx context.Context, q doclife.Request) (doclife.Plan, error) {
	o, err := captureLegacyDocumentLifecycle(ctx)
	if err != nil {
		return doclife.Plan{}, err
	}
	return o.execute(ctx, q)
}

func (o *legacyDocumentLifecycleContext) parser() *lute.Lute {
	return nativeparser.NewLute(o.markdown)
}

func (o *legacyDocumentLifecycleContext) children(ctx context.Context, box, sourcePath string) (ret []doclife.Location, err error) {
	if err = o.guard(ctx); err != nil {
		return nil, err
	}
	ioctx, closeIO, err := o.r.openRepairIOChecked(ctx, false)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	root := path.Join(box, util.StripDocExt(sourcePath))
	if err = ioctx.Source.Check(root, false); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var walk func(string) error
	walk = func(folder string) error {
		entries, readErr := ioctx.Source.ReadDir(ctx, folder)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if err := o.guard(ctx); err != nil {
				return err
			}
			name := path.Join(folder, entry.Name())
			if entry.IsDir() {
				if err := walk(name); err != nil {
					return err
				}
				continue
			}
			if util.HasDocExt(name) {
				rel := "/" + strings.TrimPrefix(name, box+"/")
				ret = append(ret, doclife.Location{Box: box, Path: rel, ID: util.GetTreeID(name)})
			}
		}
		return nil
	}
	return ret, walk(root)
}

func (o *legacyDocumentLifecycleContext) historyTree(ctx context.Context, name string) (*parse.Tree, error) {
	if err := o.guard(ctx); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(o.history)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, doclife.ErrMissing
	}
	historyRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	raw, err := historyRoot.ReadFile(filepath.ToSlash(rel))
	if err != nil {
		err = errors.Join(err, historyRoot.Close())
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	tree, parseErr := filesys.LoadTreeByData(raw, "", "/"+filepath.Base(abs), o.parser())
	return tree, errors.Join(parseErr, historyRoot.Close())
}

func (o *legacyDocumentLifecycleContext) services(life context.Context) doclife.Services {
	return doclife.Services{
		AllowDeeper: o.allowDeeper,
		Box: func(ctx context.Context, id string) (bool, error) {
			if err := o.guard(ctx); err != nil {
				return false, err
			}
			return o.owner.Box(id) != nil, o.guard(ctx)
		},
		Load: func(ctx context.Context, box, p string) (*parse.Tree, error) {
			return o.r.loadRepairTree(ctx, box, p, o.parser())
		},
		Lookup: func(ctx context.Context, id string) (doclife.Location, bool, error) {
			bt, err := o.r.referenceLookup(ctx, id)
			if err != nil || bt == nil {
				return doclife.Location{}, false, err
			}
			return doclife.Location{Box: bt.BoxID, Path: bt.Path, HPath: bt.HPath, ID: bt.ID, Document: bt.Type == "d"}, true, nil
		},
		Children: o.children,
		Exists: func(ctx context.Context, box, p string) (bool, error) {
			if err := o.guard(ctx); err != nil {
				return false, err
			}
			_, err := os.Lstat(filepath.Join(o.data, box, filepath.FromSlash(strings.TrimPrefix(p, "/"))))
			if os.IsNotExist(err) {
				return false, nil
			}
			return err == nil, err
		},
		History: o.historyTree,
		ExistingIDs: func(ctx context.Context, ids []string, exclude string) (map[string]bool, error) {
			rows, err := o.r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: append([]string(nil), ids...)})
			if err != nil {
				return nil, err
			}
			ret := make(map[string]bool, len(ids))
			for _, row := range rows {
				ret[row.ID] = row.RootID != exclude
			}
			return ret, o.guard(ctx)
		},
		Parser: func(ctx context.Context) (*lute.Lute, error) {
			if err := o.guard(ctx); err != nil {
				return nil, err
			}
			return o.parser(), nil
		},
		Language: o.language, PlainLanguage: o.plain, Now: util.CurrentTimeSecondsStr, NewID: newLifecycleID,
		Before: func(ctx context.Context, q doclife.Request) error {
			if err := o.guard(ctx); err != nil {
				return err
			}
			if q.Action != "create" && q.Action != "create-dom" {
				return FlushTxQueueContext(ctx)
			}
			return nil
		},
		Apply: func(ctx context.Context, p *doclife.Plan) error { return o.apply(ctx, life, p) },
	}
}

// The restore follow-up is admitted to the same captured owner, rather than
// the process task queue. Workspace close cancels pending work and joins any
// callback that already entered.
func (o *legacyDocumentLifecycleContext) scheduleRestoreFollowup(ctx context.Context, box, root string) error {
	if err := o.guard(ctx); err != nil {
		return err
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || o.owner != Conf || o.data != util.DataDir || o.history != util.HistoryDir {
		return errors.New("document_lifecycle_owner_closed")
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	if !o.owner.repoTasks.Go(func(life context.Context) {
		defer logging.Recover()
		if err := o.r.guard(life); err != nil {
			return
		}
		if err := o.r.queue.Flush(life); err != nil {
			return
		}
		bt, err := o.r.referenceLookup(life, root)
		if err != nil || bt == nil || bt.BoxID != box {
			return
		}
		tree, err := o.r.loadRepairTree(life, bt.BoxID, bt.Path, o.parser())
		if err != nil || tree == nil || tree.Root == nil || o.r.guard(life) != nil {
			return
		}
		events := o.sourceEvents(life)
		if err = reloadProtyleWithEnvironment(root, events); err != nil {
			return
		}
		o.owner.publishSyncUI(life, func() {
			evt := util.NewCmdResult("rename", 0, util.PushModeBroadcast)
			evt.Data = map[string]interface{}{"box": box, "id": tree.Root.ID, "path": tree.Path, "title": tree.Root.IALAttr("title"), "refText": getNodeRefText(tree.Root)}
			util.PushEvent(evt)
		})
		_ = o.owner.scheduleReferenceCounts(life, o.r, getRefDefIDs(tree.Root), util.SQLFlushInterval)
	}) {
		return errors.New("document_lifecycle_owner_closed")
	}
	return nil
}

func (o *legacyDocumentLifecycleContext) schedule(ctx context.Context, delay time.Duration, fn func() error) error {
	if fn == nil {
		return doclife.ErrDependency
	}
	if err := o.guard(ctx); err != nil {
		return err
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || o.owner != Conf || o.data != util.DataDir || o.history != util.HistoryDir {
		return errors.New("document_lifecycle_owner_closed")
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	if !o.owner.repoTasks.Go(func(life context.Context) {
		defer logging.Recover()
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-life.Done():
				return
			case <-timer.C:
			}
		}
		if err := o.guard(life); err != nil {
			return
		}
		if err := fn(); err != nil && life.Err() == nil {
			logging.LogErrorf("document lifecycle follow-up failed: %s", err)
		}
	}) {
		return errors.New("document_lifecycle_owner_closed")
	}
	return nil
}

func (o *legacyDocumentLifecycleContext) sourceEvents(ctx context.Context) *sourceEventEnvironment {
	e := legacySourceEventEnvironmentContext(ctx)
	e.enqueue = func(_, _ string, delay time.Duration, fn func() error) error {
		return o.schedule(ctx, delay, fn)
	}
	return e
}
