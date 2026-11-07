package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/parse"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

func RenameBookmarkContext(ctx context.Context, old, next string) error {
	return mutateBookmarkContext(ctx, old, next, true)
}
func RemoveBookmarkContext(ctx context.Context, label string) error {
	return mutateBookmarkContext(ctx, label, "", false)
}

func mutateBookmarkContext(ctx context.Context, old, next string, rename bool) error {
	if ctx == nil {
		return errors.New("bookmark_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	o, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		return err
	}
	c := o.commit
	r := o.r
	lifecycle := &legacyDocumentLifecycleContext{r: r, owner: r.owner, data: r.data, history: util.HistoryDir, language: r.language, plain: r.language}
	_, err = lifecycle.run(ctx, func(op, life context.Context) (doclife.Plan, error) {
		if err := FlushTxQueueContext(op); err != nil {
			return doclife.Plan{}, err
		}
		e := bookmarkMutationWithOwner(op, life, c)
		return doclife.Plan{}, mutateBookmarkWithEnvironment(old, next, rename, e)
	})
	return err
}

func bookmarkMutationWithOwner(ctx, life context.Context, c *legacyCommitContext) *bookmarkMutationEnvironment {
	r := c.r
	publishIAL := cache.CaptureBlockIALPublisher()
	publish := func(ctx context.Context, fn func()) error {
		if err := r.guard(ctx); err != nil {
			return err
		}
		accepted := false
		r.owner.publishSyncUI(ctx, func() { fn(); accepted = true })
		if !accepted {
			return errors.New("bookmark_owner_closed")
		}
		return nil
	}
	return &bookmarkMutationEnvironment{ctx: ctx, legacy: true, gate: legacyAttributeMutationGate,
		query: func() ([]indexdb.BlockRecord, error) {
			if err := r.guard(ctx); err != nil {
				return nil, err
			}
			return indexdb.BookmarkBlocks(ctx, r.main.DB)
		},
		load: func(id string) (*parse.Tree, error) {
			bt, err := r.referenceLookup(ctx, id)
			if err != nil {
				return nil, err
			}
			if bt == nil {
				return nil, ErrBlockNotFound
			}
			return r.loadRepairTree(ctx, bt.BoxID, bt.Path, c.parser())
		},
		stage: func(tree *parse.Tree) (err error) {
			io, closeIO, err := r.openRepairIOChecked(ctx, true)
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, closeIO()) }()
			err = r.writeRepairTree(ctx, io, tree, c.parser())
			if err == nil {
				err = r.queue.Upsert(ctx, tree)
			}
			// Join all accepted derived writes even if a later stage fails/cancels.
			err = errors.Join(err, r.queue.Flush(context.WithoutCancel(ctx)))
			if err != nil {
				return err
			}
			if err = r.guard(ctx); err != nil {
				return err
			}
			r.owner.syncRuntime.same.Store(0)
			r.owner.planSyncAfter(c.interval)
			return nil
		},
		cache: func(id string, attrs map[string]string) error {
			if err := r.guard(ctx); err != nil {
				return err
			}
			publishIAL(id, attrs)
			return nil
		},
		progress: func(kind, value string) error {
			return publish(ctx, func() {
				switch kind {
				case "rename":
					util.PushEndlessProgress(r.language(110))
				case "remove":
					util.PushEndlessProgress(r.language(116))
				case "load":
					util.PushEndlessProgress("[" + value + "]")
				case "save":
					util.PushEndlessProgress(fmt.Sprintf(r.language(111), util.EscapeHTML(value)))
				}
			})
		},
		clear: func() error { return publish(life, func() { util.ClearPushProgress(100) }) },
		delay: func() error {
			timer := time.NewTimer(100 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		reload:  func() error { return publish(ctx, util.ReloadUI) },
		invalid: func(s string) error { return fmt.Errorf(r.language(112), s) }, empty: func() error { return errors.New(r.language(126)) },
	}
}
