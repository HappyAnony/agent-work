// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"crypto/sha256"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/reminder"
)

type NativeReminderPlan struct {
	reminder.Plan
	SourceHashes map[string][32]byte
}

// Read-only preparation, like other R02 source candidates. No network, source
// commit, notification or receipt; R04 must bind the authoritative publication.
func (l *Lease) PlanNativeBlockReminder(ctx context.Context, id, timed string) (out NativeReminderPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		trees, e := r.documentInfoTrees(ctx, w, []string{id})
		if e != nil {
			return e
		}
		t := trees[id]
		content := r.nativeContent
		content.AttributeView = func(c context.Context, id string, _ bool) (string, error) {
			name, _, e := r.readNativeAV(c, w, id)
			return name, e
		}
		if t == nil || t.Root == nil {
			return reminder.ErrMissing
		}
		// Actual tree attributes belong to this owner's source, not process SQL.
		attrs := map[string]string{}
		for _, v := range t.Root.KramdownIAL {
			if len(v) == 2 {
				attrs[v[0]] = html.UnescapeAttrVal(v[1])
			}
		}
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.ID == id {
				attrs = map[string]string{}
				for _, v := range n.KramdownIAL {
					if len(v) == 2 {
						attrs[v[0]] = html.UnescapeAttrVal(v[1])
					}
				}
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		p, e := reminder.Prepare(ctx, id, timed, t, attrs, reminder.Services{Location: time.Local, Language: func(n int) string {
			return r.configurationBusiness.Language(r.configuration.Lang, n, true, r.configuration.CloudRegion)
		}, Content: func(c context.Context, n *ast.Node) (string, error) {
			return indexdb.StaticContent(c, n, nil, false, false, false, content)
		}})
		if e != nil {
			return e
		}
		rel, e := treeRelative(t.Box, t.Path)
		if e != nil {
			return e
		}
		raw, e := readNativeFile(ctx, w, rel, 16<<20)
		if e != nil {
			return e
		}
		out = NativeReminderPlan{Plan: p, SourceHashes: map[string][32]byte{"data/" + rel: sha256.Sum256(raw)}}
		return ctx.Err()
	})
	if err != nil {
		return NativeReminderPlan{}, err
	}
	return
}
func (l *Lease) NativeCloudSetBlockReminder(ctx context.Context, p reminder.Payload) error {
	return l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error { return s.SetCloudBlockReminder(c, p) })
}

func (l *Lease) StartNativeSubscriptionReminder(ctx context.Context, ios bool, notify func(context.Context, cloudaccount.ExpirationNotice) error) (*Task, error) {
	if notify == nil {
		return nil, cloudaccount.ErrDependency
	}
	var service *cloudaccount.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		service = l.workspace.resources.cloudAccount
		if service == nil {
			return cloudaccount.ErrDependency
		}
		return nil
	}); e != nil {
		return nil, e
	}
	return l.StartTask(ctx, func(c context.Context, _ *Lease) error {
		return service.RunExpirationReminder(c, ios, cloudaccount.WaitReminder, notify)
	})
}
