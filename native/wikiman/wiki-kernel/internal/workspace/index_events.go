package workspace

import (
	"context"
	"wiki-kernel/internal/eventscope"
)

// IndexEvent is progress only, never a source commit receipt. It captures the
// publishing workspace identity/generation, not whichever UI tab is active.
type IndexEvent struct {
	StoreID, PageID string
	Generation      uint64
	Kind, Action    string
	Current, Total  int
	BlockCount      int
	Detail          string
}

func (l *Lease) SubscribeIndexEvents(ctx context.Context, fn func(IndexEvent)) (*eventscope.Subscription[IndexEvent], error) {
	pin, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pin.Release()
	var sub *eventscope.Subscription[IndexEvent]
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			var err error
			sub, err = pin.workspace.resources.indexEvents.SubscribeContext(ctx, fn)
			return err
		})
	})
	return sub, err
}

func (l *Lease) indexEvent(kind, action string, current, total int) IndexEvent {
	return IndexEvent{StoreID: l.StoreID(), PageID: l.PageID(), Generation: l.workspace.generation, Kind: kind, Action: action, Current: current, Total: total}
}
