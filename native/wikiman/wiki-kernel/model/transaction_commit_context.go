// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/88250/go-humanize"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// This adapter belongs to the selected compatibility runtime, never to Native.
// The original FIFO/close barrier pins its legacy AV and SQL adapters. No repo
// gate is acquired here: sync holds that gate while awaiting a FIFO create.
// Writes are sequential, not rollback-capable; R04 owns source atomicity.
type legacyCommitContext struct {
	r          *indexRepairContext
	parser     func() *lute.Lute
	interval   time.Duration
	relative   func(time.Time) string
	anchorMax  int
	duration   func(time.Time, time.Time) string
	avUntitled string
	avLanguage func(int) string
	avLabels   map[string]string
	avWarning  func(string)
	prod       bool
	refs       func(context.Context, string) ([]indexdb.RefRecord, error)
	ial        func(string, []string, map[string]string)
	dynamic    func(string, string) error
	// Side effects are explicit error-returning ports, including old AV business.
	refreshAV  func(context.Context, map[string]*ast.Node, func(string) error) error
	relatedAV  func(context.Context, []string, func(string) error) error
	publish    func(commitNotice)
	onComplete func(context.Context, []commitNotice) error
}
type commitNotice struct {
	kind, id string
	payload  json.RawMessage
	change   anchor.Change
	delay    time.Duration
}

func bindLegacyCommit(ctx context.Context, env *transactionEnvironment, r *indexRepairContext) error {
	var err error
	if r == nil {
		r, err = captureIndexRepair(ctx)
		if err != nil {
			return err
		}
	}
	if err = r.guard(ctx); err != nil {
		return err
	}
	c := &legacyCommitContext{r: r, parser: env.parser, prod: util.Mode == "prod", publish: publishCommitNotice, refs: sql.CaptureCommitReferences(), ial: cache.CaptureCommitIAL(), dynamic: treenode.DynamicRefTexts.Put}
	err = r.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Sync == nil || v.Editor == nil {
			return conf.ErrBusinessUnbound
		}
		c.avLabels = map[string]string{}
		for key, value := range util.AttrViewLangs[v.Lang] {
			if text, ok := value.(string); ok {
				c.avLabels[key] = text
			}
		}
		c.avUntitled = r.owner.business.Language(v.Lang, 105, false, v.CloudRegion)
		c.avLanguage = func(id int) string { return r.owner.business.Language(v.Lang, id, false, v.CloudRegion) }
		c.interval = time.Duration(v.Sync.Interval) * time.Second
		c.anchorMax = v.Editor.BlockRefDynamicAnchorTextMaxLen
		// Capture the actual compatibility language table, including before/after.
		labels := map[string]interface{}{}
		for k, v := range util.TimeLangs[v.Lang] {
			labels[k] = v
		}
		magnitudes, e := conf.RelativeMagnitudes(labels)
		if e != nil {
			return e
		}
		before, bok := labels["albl"].(string)
		after, aok := labels["blbl"].(string)
		if !bok || !aok {
			return conf.ErrBusinessUnbound
		}
		c.relative = func(t time.Time) string {
			return strings.TrimSpace(humanize.CustomRelTime(t, time.Now(), before, after, magnitudes))
		}
		c.duration = func(a, b time.Time) string {
			return strings.TrimSpace(humanize.CustomRelTime(a, b, "", "", magnitudes))
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Register the existing owner closer without entering its service gate.
	if _, err = r.owner.CloudRepoService(); err != nil {
		return err
	}
	c.refreshAV = func(ctx context.Context, nodes map[string]*ast.Node, reload func(string) error) error {
		return c.avContext(ctx).refresh(nodes, reload)
	}
	c.relatedAV = func(ctx context.Context, ids []string, reload func(string) error) error {
		return c.avContext(ctx).flush(ids, reload)
	}
	env.commitContext = c
	bindTransactionReferenceCounts(env, r)
	bindTransactionReadAndNoticePorts(env, c)
	bindTransactionMutationPorts(env, c)
	bindTransactionCachePorts(env, c)
	bindTransactionHistoryPort(env, c)
	bindTransactionHeadingEffects(env, c)
	bindTransactionLookup(env, c)
	bindTransactionAfterCommit(env, c)
	// Remember bytes at load time, before the original operation mutates its AST.
	env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
		return r.loadRepairTree(env.ctx, box, p, engine)
	}
	return nil
}

func (c *legacyCommitContext) finish(ctx context.Context, tx *Transaction) (err error) {
	if c == nil || c.r == nil || c.parser == nil || c.relative == nil || c.publish == nil || c.refreshAV == nil || c.relatedAV == nil || c.refs == nil || c.ial == nil || c.dynamic == nil {
		return errors.New("transaction_commit_context_unbound")
	}
	r := c.r
	if err = r.guard(ctx); err != nil {
		return err
	}
	ioctx, closeIO, err := r.openRepairIOChecked(ctx, true)
	if err != nil {
		return err
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	notices := []commitNotice{}
	c.avWarning = func(text string) {
		notices = append(notices, commitNotice{kind: "guide", payload: mustCommitJSON(text)})
	}
	defer func() { c.avWarning = nil }()
	// All payloads are values; delayed jobs must not retain the mutable tx/AST.
	source, err := json.Marshal([]interface{}{tx})
	if err != nil {
		return err
	}
	info := func(t *parse.Tree, size uint64) error {
		count := 0
		dir := t.Box + util.StripDocExt(t.Path)
		var entries []fs.DirEntry
		checkErr := ioctx.Source.Check(dir, false)
		if checkErr != nil && !os.IsNotExist(checkErr) {
			return checkErr
		}
		handle, e := ioctx.Source.Data.Open(dir)
		if e == nil {
			entries, e = handle.ReadDir(-1)
			e = errors.Join(e, handle.Close())
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if t.Root.IALAttr("custom-hidden") != "true" {
			for _, f := range entries {
				if util.HasDocExt(f.Name()) {
					count++
				}
			}
		}
		data, e := json.Marshal(projectReloadDocInfo(t, size, count, c.relative))
		if e != nil {
			return e
		}
		notices = append(notices, commitNotice{kind: "info", payload: data, delay: 500 * time.Millisecond})
		return nil
	}
	publishIAL := c.ial
	ioctx.Written = func(ctx context.Context, t *parse.Tree, size uint64) error {
		if e := r.guard(ctx); e != nil {
			return e
		}
		if r.warningSize > 0 && size > uint64(r.warningSize)*1024*1024 {
			notices = append(notices, commitNotice{kind: "guide", payload: mustCommitJSON(fmt.Sprintf(r.language(268), t.Root.IALAttr("title")+" "+path.Base(t.Path), r.warningSize))})
		}
		// Drain stale IAL admissions before publishing the new document properties.
		ids := []string{}
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.ID != "" {
				ids = append(ids, n.ID)
			}
			return ast.WalkContinue
		})
		publishIAL(t.Path, ids, parse.IAL2MapUnEsc(t.Root.KramdownIAL))
		if e := info(t, size); e != nil {
			return e
		}
		if parent := path.Dir(t.Path); parent != "/" && parent != "." {
			// Metadata reads never repair/migrate the parent source.
			p, e := ioctx.Load(ctx, t.Box, parent+util.DocExt, c.parser(), false)
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			if e == nil {
				if e = info(p, uint64(len(render.NewJSONRenderer(p, lute.New().RenderOptions).Render()))); e != nil {
					return e
				}
			}
		}
		return nil
	}
	q := r.queue
	// Always join admitted derived writes, even when a later source/AV step fails.
	drained := false
	defer func() {
		if !drained {
			err = errors.Join(err, indexrepairDrain(context.WithoutCancel(ctx), q))
		}
	}()
	save := func(t *parse.Tree, create bool) error {
		if e := r.guard(ctx); e != nil {
			return e
		}
		var e error
		if create {
			e = ioctx.Write(ctx, t, nil, c.parser())
		} else if _, known := r.revisions.Load(t.Box + t.Path); known {
			e = r.writeRepairTree(ctx, ioctx, t, c.parser())
		} else {
			// Some old move/insert operations stage existing trees without load().
			// Bind that source revision explicitly before writing, never overwrite a create.
			raw, re := ioctx.Source.Read(ctx, t.Box+t.Path)
			if re != nil {
				return re
			}
			ioctx.Remember(t, raw)
			e = r.writeRepairTree(ctx, ioctx, t, c.parser())
		}
		if e != nil {
			return e
		}
		if e = r.registry.Upsert(ctx, t); e != nil {
			return e
		}
		return q.Upsert(context.WithoutCancel(ctx), t)
	}
	creates := map[string]bool{}
	for _, op := range tx.DoOperations {
		if op != nil && op.Action == "create" {
			if t, ok := op.Data.(*parse.Tree); ok {
				creates[t.ID] = true
			}
		}
	}
	ids := make([]string, 0, len(tx.trees))
	for id := range tx.trees {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := tx.trees[id]
		if e := save(t, creates[id]); e != nil {
			return e
		}
		notices = append(notices, commitNotice{kind: "saved", id: t.ID, payload: append(json.RawMessage(nil), source...)})
		if c.prod && IsUserGuide(t.Box) {
			notices = append(notices, commitNotice{kind: "guide", payload: json.RawMessage(mustCommitJSON(r.language(52)))})
		}
	}
	reload := func(id string) error {
		notices = append(notices, commitNotice{kind: "av", id: id, delay: 200 * time.Millisecond})
		return ctx.Err()
	}
	refs := c.refs
	pendingRefs := map[string]*parse.Tree{}
	services := anchor.Services{Embedded: treenode.GetEmbedBlockRef, StrictLoad: true}
	services.References = func(ctx context.Context, n *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error) {
		if e := r.guard(ctx); e != nil {
			return nil, nil, e
		}
		return anchor.References(ctx, n, refs)
	}
	services.Load = func(ctx context.Context, id string) (*parse.Tree, error) {
		bt, e := r.referenceLookup(ctx, id)
		if e != nil {
			return nil, e
		}
		if bt == nil {
			return nil, ErrBlockNotFound
		}
		return r.loadRepairTree(ctx, bt.BoxID, bt.Path, c.parser())
	}
	services.Text = func(ctx context.Context, n *ast.Node) (string, error) {
		return anchor.Text(ctx, n, c.anchorMax, indexdb.ContentServices{AttributeView: func(ctx context.Context, id string, full bool) (string, error) {
			if e := r.guard(ctx); e != nil {
				return "", e
			}
			return c.avContext(ctx).title(id)
		}})
	}
	services.Set = func(n *ast.Node, text string) error {
		if e := r.guard(ctx); e != nil {
			return e
		}
		return anchor.SetText(n, text, c.dynamic)
	}
	services.Notify = func(ctx context.Context, v anchor.Change) error {
		notices = append(notices, commitNotice{kind: "anchor", change: v, delay: 200 * time.Millisecond})
		return ctx.Err()
	}
	services.IndexRefs = func(ctx context.Context, t *parse.Tree) error {
		if e := r.guard(ctx); e != nil {
			return e
		}
		pendingRefs[t.ID] = t
		return nil
	}
	services.Stage = func(ctx context.Context, t *parse.Tree) error {
		if e := save(t, false); e != nil {
			return e
		}
		pending := pendingRefs[t.ID]
		if pending == nil {
			return nil
		}
		if e := q.UpdateRefs(context.WithoutCancel(ctx), pending); e != nil {
			return e
		}
		delete(pendingRefs, t.ID)
		return nil
	}
	services.UpdateAVs = func(ctx context.Context, nodes map[string]*ast.Node) error { return c.refreshAV(ctx, nodes, reload) }
	if err = anchor.Refresh(ctx, tx.nodes, tx.trees, services); err != nil {
		return err
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range tx.relatedAvIDs {
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	if err = c.relatedAV(ctx, unique, reload); err != nil {
		return err
	}
	if err = q.Flush(ctx); err != nil {
		return err
	}
	drained = true
	if err = closeIO(); err != nil {
		return err
	}
	if err = r.guard(ctx); err != nil {
		return err
	}
	if c.onComplete != nil {
		return c.onComplete(ctx, notices)
	}
	return c.complete(ctx, notices)
}
func mustCommitJSON(v string) []byte { b, _ := json.Marshal(v); return b }

// Admit one owned notification batch only after all source/derived work succeeds.
// The original delays are preserved relative to completion. Close cancels pending
// timers and joins in-flight publication through repoMu and the owner task group.
func (c *legacyCommitContext) complete(ctx context.Context, notices []commitNotice) error {
	return c.completeUI(ctx, notices, true)
}

func (c *legacyCommitContext) completeUI(ctx context.Context, notices []commitNotice, planSync bool) error {
	if e := c.r.guard(ctx); e != nil {
		return e
	}
	owner := c.r.owner
	owner.repoMu.Lock()
	defer owner.repoMu.Unlock()
	// Cancellation while waiting for admission must not create a success task.
	if e := c.r.guard(ctx); e != nil {
		return e
	}
	if owner.repoClosed {
		return errors.New("transaction_commit_owner_closed")
	}
	if owner.repoTasks == nil {
		owner.repoTasks = taskgroup.Open()
	}
	copyNotices := append([]commitNotice(nil), notices...)
	for i := range copyNotices {
		copyNotices[i].payload = append(json.RawMessage(nil), copyNotices[i].payload...)
	}
	sort.SliceStable(copyNotices, func(i, j int) bool { return copyNotices[i].delay < copyNotices[j].delay })
	var priorIDNotice <-chan struct{}
	var doneIDNotice chan struct{}
	for _, n := range copyNotices {
		if n.kind == "ids" {
			priorIDNotice = owner.archiveIDNoticeTail
			doneIDNotice = make(chan struct{})
			break
		}
	}
	start := time.Now()
	if !owner.repoTasks.Go(func(life context.Context) {
		if doneIDNotice != nil {
			defer close(doneIDNotice)
		}
		defer func() {
			if recover() != nil {
				logging.LogErrorf("commit UI publication failed")
			}
		}()
		if priorIDNotice != nil {
			select {
			case <-priorIDNotice:
			case <-life.Done():
				return
			}
		}
		for _, n := range copyNotices {
			wait := time.Until(start.Add(n.delay))
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-life.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			if c.r.guard(life) != nil {
				return
			}
			owner.publishSyncUI(life, func() { c.publish(n) })
		}
	}) {
		return errors.New("transaction_commit_owner_closed")
	}
	if doneIDNotice != nil {
		owner.archiveIDNoticeTail = doneIDNotice
	}
	if planSync {
		owner.syncRuntime.same.Store(0)
		owner.syncRuntime.planMu.Lock()
		owner.syncRuntime.plan = time.Now().Add(c.interval)
		owner.syncRuntime.planMu.Unlock()
	}
	return nil
}
func publishCommitNotice(n commitNotice) {
	switch n.kind {
	case "filetree-sort":
		publishMarkdownSortChange(n.payload)
	case "ids":
		var ids map[string]string
		if json.Unmarshal(n.payload, &ids) == nil {
			util.PushUpdateIDs(ids)
		}
	case "saved":
		util.PushSaveDoc(n.id, "tx", n.payload)
	case "info":
		var data map[string]interface{}
		if json.Unmarshal(n.payload, &data) == nil {
			util.PushReloadDocInfo(data)
		}
	case "anchor":
		v := n.change
		util.PushSetRefDynamicText(v.RootID, v.BlockID, v.DefinitionID, v.Text)
	case "av":
		pushReloadAttrView(n.id)
	case "guide":
		var text string
		if json.Unmarshal(n.payload, &text) == nil {
			util.PushErrMsg(text, 7000)
		}
	}
}
