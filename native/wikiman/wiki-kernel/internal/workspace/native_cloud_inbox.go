// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"slices"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) nativeCloudInbox(ctx context.Context, id string, page int, detail bool) (map[string]any, error) {
	out := make(chan map[string]any, 1)
	e := l.runNativeCloudBusiness(ctx, func(c context.Context, pin *Lease, s *cloudaccount.Service) error {
		var p cloudaccount.InboxPresentation
		// Freeze the page-owned parser before network IO; never hold the source
		// admission/resource gate while waiting for a cloud response.
		e := pin.WithNativeIndexes(c, func(*indexdb.Bundle) error {
			parser, e := pin.workspace.resources.documentInfoParser(c, pin.workspace)
			if e != nil {
				return e
			}
			p = cloudaccount.InboxPresentation{Parser: parser, Location: time.Local}
			return c.Err()
		})
		if e != nil {
			return e
		}
		var v map[string]any
		if detail {
			v, e = s.GetCloudShorthand(c, id, p)
		} else {
			v, e = s.GetCloudShorthands(c, page, p)
		}
		if e == nil {
			out <- v
		}
		return e
	})
	if e != nil {
		return nil, e
	}
	return <-out, nil
}
func (l *Lease) NativeCloudGetShorthand(ctx context.Context, id string) (map[string]any, error) {
	return l.nativeCloudInbox(ctx, id, 0, true)
}
func (l *Lease) NativeCloudGetShorthands(ctx context.Context, page int) (map[string]any, error) {
	return l.nativeCloudInbox(ctx, "", page, false)
}
func (l *Lease) NativeCloudRemoveShorthands(ctx context.Context, ids []string) error {
	ids = slices.Clone(ids)
	return l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error {
		return s.RemoveCloudShorthands(c, ids)
	})
}
