package model

import (
	"context"
	"errors"
	"time"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

func RenameTagContext(ctx context.Context, old, next string) error {
	return mutateTagContext(ctx, old, next, true)
}
func RemoveTagContext(ctx context.Context, label string) error {
	return mutateTagContext(ctx, label, "", false)
}

func mutateTagContext(ctx context.Context, old, next string, rename bool) error {
	if ctx == nil {
		return errors.New("tag_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	o, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		return err
	}
	r, c := o.r, o.commit
	lifecycle := &legacyDocumentLifecycleContext{r: r, owner: r.owner, data: r.data, history: util.HistoryDir, language: r.language, plain: r.language}
	_, err = lifecycle.run(ctx, func(op, life context.Context) (doclife.Plan, error) {
		if err := FlushTxQueueContext(op); err != nil {
			return doclife.Plan{}, err
		}
		return doclife.Plan{}, legacyAttributeMutationGate.with(op, func() error { return mutateTagWithEnvironment(old, next, rename, tagMutationWithOwner(op, life, c)) })
	})
	return err
}

func tagMutationWithOwner(ctx, life context.Context, c *legacyCommitContext) *tagMutationEnvironment {
	r := c.r
	base := bookmarkMutationWithOwner(ctx, life, c)
	reload := sourceEventEnvironmentForOwner(ctx, &legacyAnchorContext{r: r, commit: c}, nil)
	return &tagMutationEnvironment{ctx: ctx, legacy: true,
		query: func(label string) ([]indexdb.SpanRecord, error) {
			if err := r.guard(ctx); err != nil {
				return nil, err
			}
			return indexdb.TagSpans(ctx, r.main.DB, "label", label, 0)
		},
		load: base.load, stage: base.stage,
		flush: func() error {
			if err := r.guard(ctx); err != nil {
				return err
			}
			return r.queue.Flush(ctx)
		},
		reload:   func(id string) error { return reloadProtyleWithEnvironment(id, reload) },
		progress: base.progress,
		clear: func(success bool) error {
			if !success {
				return base.clear()
			}
			if err := r.guard(ctx); err != nil {
				return err
			}
			accepted := false
			r.owner.publishSyncUI(ctx, func() { util.PushClearProgress(); accepted = true })
			if !accepted {
				return errors.New("tag_owner_closed")
			}
			return nil
		},
		delay: func(min, max int) error {
			timer := time.NewTimer(time.Duration((min+max)/2) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		invalid: base.invalid, empty: func() error { return errors.New(r.language(114)) },
		av: legacyAVTextEnvironment(ctx, bindAVOperation(c, ctx)),
	}
}
