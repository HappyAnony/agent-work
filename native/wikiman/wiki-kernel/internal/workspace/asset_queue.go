package workspace

import (
	"context"
	"time"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeAssetContents(ctx context.Context, statement string, limit int) (ret []indexdb.AssetContentEntry, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, err := b.Database(indexdb.Assets)
		if err != nil {
			return err
		}
		ret, err = indexdb.SelectAssetContents(ctx, db, statement, limit)
		return err
	})
	return
}

func (l *Lease) QueueNativeAssetContents(ctx context.Context, items []indexdb.AssetContentEntry) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return l.workspace.resources.assetQueue.Index(items) })
}
func (l *Lease) QueueNativeAssetContentDelete(ctx context.Context, path string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return l.workspace.resources.assetQueue.DeletePath(path) })
}
func (l *Lease) FlushNativeAssetContents(ctx context.Context) error {
	pin, err := l.pin(true)
	if err != nil {
		return err
	}
	defer pin.Release()
	var events []IndexEvent
	err = pin.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, err := bundle.Database(indexdb.Assets)
		if err != nil {
			return err
		}
		return pin.workspace.resources.assetQueue.Flush(ctx, db, func(p indexdb.AssetProgress) {
			events = append(events, pin.indexEvent("asset-index-progress", p.Action, p.Current, p.Total))
		})
	})
	for _, event := range events {
		if publishErr := pin.workspace.resources.indexEvents.Publish(event); publishErr != nil && err == nil {
			err = publishErr
		}
	}
	return err
}
func (l *Lease) StartNativeAssetContentFlush(ctx context.Context, interval time.Duration) (*Task, error) {
	return l.StartPeriodicTask(ctx, interval, func(ctx context.Context, lease *Lease) error { return lease.FlushNativeAssetContents(ctx) })
}
