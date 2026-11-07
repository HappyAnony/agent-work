package workspace

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/eventscope"
)

// SubscribeOriginalSQLProgress binds the original progress-message business to
// this Context. Only the explicit sink receives it; no global boot details or
// active-window/WS broadcaster is used. The publishing lease owns callback time.
// The sink runs outside the DB gate and must not drain its own subscription.
func (l *Lease) SubscribeOriginalSQLProgress(ctx context.Context, policy conf.SQLProgressPolicy, sink func(IndexEvent, string)) (*eventscope.Subscription[IndexEvent], error) {
	if sink == nil {
		return nil, ErrIdentity
	}
	pin, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pin.Release()
	r := pin.workspace.resources
	var sub *eventscope.Subscription[IndexEvent]
	err = pin.sourceRead(ctx, func() error {
		return r.serial(ctx, func() error {
			if r.configurationBusiness == nil {
				return conf.ErrBusinessUnbound
			}
			var err error
			sub, err = r.indexEvents.SubscribeContext(ctx, func(event IndexEvent) {
				p := conf.SQLProgress{Current: event.Current, Total: event.Total, BlockCount: event.BlockCount, Detail: event.Detail}
				switch {
				case event.Kind == "history-index-progress" && event.Action == "index":
					p.Kind = "history"
				case event.Kind == "asset-index-progress" && event.Action == "index":
					p.Kind = "asset"
				case event.Kind == "main-index-flushed" && (event.Action == "delete_id" || event.Action == "delete_ids"):
					p.Kind = "delete"
				case event.Kind == "main-index-flushed" && (event.Action == "rename" || event.Action == "rename_sub_tree"):
					p.Kind = "rename"
				case event.Kind == "main-index-blocks" && (event.Action == "index" || event.Action == "upsert"):
					p.Kind = "blocks"
				default:
					return
				}
				var message string
				var enabled bool
				// This is an owned internal callback, called only while its publisher pins
				// the Context. Do not capture the caller's possibly-released lease or ctx.
				if err := r.serial(context.Background(), func() error {
					message, enabled = conf.SQLProgressMessage(p, policy, func(number int) string {
						return r.configurationBusiness.Language(r.configuration.Lang, number, true, r.configuration.CloudRegion)
					})
					return nil
				}); err == nil && enabled {
					sink(event, message)
				}
			})
			return err
		})
	})
	return sub, err
}
