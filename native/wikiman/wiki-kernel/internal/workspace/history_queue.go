package workspace

import (
	"context"
	"time"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) QueueNativeHistories(ctx context.Context, items []indexdb.HistoryEntry) error {
	return l.withHistoryQueue(ctx, func(q *indexdb.HistoryQueue, _ *indexdb.Bundle) error { return q.Index(items) })
}

func (l *Lease) QueueNativeHistoryPurge(ctx context.Context, before int64) error {
	return l.withHistoryQueue(ctx, func(q *indexdb.HistoryQueue, _ *indexdb.Bundle) error { return q.DeleteBefore(before) })
}

func (l *Lease) FlushNativeHistories(ctx context.Context) error {
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	var events []IndexEvent
	err = pin.withHistoryQueue(ctx, func(q *indexdb.HistoryQueue, bundle *indexdb.Bundle) error {
		db, err := bundle.Database(indexdb.History)
		if err != nil {
			return err
		}
		return q.Flush(ctx, db, func(p indexdb.HistoryProgress) {
			events = append(events, pin.indexEvent("history-index-progress", p.Action, p.Current, p.Total))
		})
	})
	for _, event := range events {
		if publishErr := pin.workspace.resources.indexEvents.Publish(event); publishErr != nil && err == nil {
			err = publishErr
		}
	}
	return err
}

// The actual original history-index flush business, not a placeholder callback,
// runs on the creating page lease and stops/joins with that owner's tasks.
func (l *Lease) StartNativeHistoryFlush(ctx context.Context, interval time.Duration) (*Task, error) {
	return l.StartPeriodicTask(ctx, interval, func(ctx context.Context, lease *Lease) error { return lease.FlushNativeHistories(ctx) })
}

func (l *Lease) withHistoryQueue(ctx context.Context, fn func(*indexdb.HistoryQueue, *indexdb.Bundle) error) error {
	// WithNativeIndexes pins source admission and serializes resource ownership.
	return l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		return fn(&l.workspace.resources.historyQueue, bundle)
	})
}
