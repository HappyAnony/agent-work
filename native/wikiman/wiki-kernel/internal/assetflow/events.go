// Package assetflow shares original asset watcher side-effect order. No path,
// database, global broadcaster or goroutine is chosen here.
package assetflow

import "context"

type Event struct {
	Path             string
	Removed, Written bool
}
type Handler struct {
	Refresh             func(context.Context) error
	Index               func(context.Context, Event) error
	InvalidateThumbnail func(context.Context, string) error
	MarkSync            func(context.Context) error
}

func Dispatch(ctx context.Context, e Event, h Handler) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.Written && h.MarkSync != nil {
		if err := h.MarkSync(ctx); err != nil {
			return err
		}
	}
	if h.Refresh != nil {
		if err := h.Refresh(ctx); err != nil {
			return err
		}
	}
	if h.Index != nil {
		if err := h.Index(ctx, e); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.InvalidateThumbnail != nil {
		return h.InvalidateThumbnail(ctx, e.Path)
	}
	return nil
}
