package workspace

import (
	"context"
	"errors"
	"time"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeEmptyEmbedBlocks(ctx context.Context) (ret []indexdb.BlockRecord, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, err := b.Database(indexdb.Main)
		if err != nil {
			return err
		}
		ret, err = indexdb.EmptyEmbedBlocks(ctx, db)
		return err
	})
	return
}
func (l *Lease) QueueNativeBlockContent(ctx context.Context, id, content string) error {
	return l.enqueueNativeMain(ctx, func(r *resources) error { return r.contentQueue.Update(id, content) })
}
func (l *Lease) FlushNativeBlockContents(ctx context.Context) error {
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	var events []IndexEvent
	err = pin.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		return pin.flushNativeMainLocked(ctx, b, &events)
	})
	// Even if a later operation fails, committed derived work is reported outside
	// the DB gate. The failed/unstarted tail remains queued, not acknowledged.
	for _, event := range events {
		err = errors.Join(err, pin.workspace.resources.indexEvents.Publish(event))
	}
	return err
}
func (l *Lease) StartNativeBlockContentFlush(ctx context.Context, interval time.Duration) (*Task, error) {
	return l.StartPeriodicTask(ctx, interval, func(ctx context.Context, lease *Lease) error { return lease.FlushNativeBlockContents(ctx) })
}

// These names include all audited main families, not just content updates.
func (l *Lease) QueueNativeMainMutation(ctx context.Context, m indexdb.MainMutation) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return l.enqueueNativeMain(ctx, func(r *resources) error { return r.contentQueue.Enqueue(m) })
}
func (l *Lease) FlushNativeMainQueue(ctx context.Context) error {
	return l.FlushNativeBlockContents(ctx)
}
func (l *Lease) StartNativeMainFlush(ctx context.Context, interval time.Duration) (*Task, error) {
	return l.StartPeriodicTask(ctx, interval, func(ctx context.Context, lease *Lease) error { return lease.FlushNativeMainQueue(ctx) })
}

func (l *Lease) enqueueNativeMain(ctx context.Context, enqueue func(*resources) error) error {
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	err = pin.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return enqueue(pin.workspace.resources) })
	if err != nil {
		return err
	}
	return pin.workspace.resources.indexEvents.Publish(pin.indexEvent("main-index-changed", "", 0, 0))
}

// Caller already owns this page's source admission and resource gate.
func (pin *Lease) flushNativeMainLocked(ctx context.Context, b *indexdb.Bundle, events *[]IndexEvent) error {
	db, err := b.Database(indexdb.Main)
	if err != nil {
		return err
	}
	r := pin.workspace.resources
	return r.contentQueue.FlushMainProgress(ctx, db, r.configuration.Search != nil && r.configuration.Search.CaseSensitive, func(p indexdb.MainProgress) {
		action := p.Action
		event := pin.indexEvent("main-index-flushed", p.Action, p.Current, p.Total)
		event.BlockCount, event.Detail = p.BlockCount, p.Detail
		if p.TreeBatch {
			event.Kind = "main-index-blocks"
		}
		*events = append(*events, event)
		if !p.TreeBatch {
			r.nativeBlocks.Clear()
			r.nativeRefs.Clear()
		}
		if r.ial != nil && (action == "index" || action == "upsert" || action == "rename" || action == "rename_sub_tree" || action == "delete" || action == "delete_id" || action == "delete_ids" || action == "delete_box") {
			r.ial.ClearBlocks()
			if action == "index" || action == "upsert" {
				r.ial.ClearDocs()
			}
		}
	})
}
