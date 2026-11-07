// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeAIAvailable(ctx context.Context) (available bool, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		available = l.workspace.resources.ai != nil
		return nil
	})
	return
}

// AI tasks hold creation identity while network IO runs outside source/resource
// admission. Action source text is frozen under a short owner read admission.
func (l *Lease) nativeAI(ctx context.Context, msg string, ids []string, action string, withAction bool) (string, error) {
	var service *aiflow.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		service = l.workspace.resources.ai
		if service == nil {
			return aiflow.ErrDependency
		}
		return nil
	}); e != nil {
		return "", e
	}
	result := make(chan string, 1)
	task, e := l.StartTask(ctx, func(c context.Context, pin *Lease) error {
		var out string
		var e error
		if !withAction {
			out, e = service.Chat(c, msg)
		} else {
			out, e = service.Action(c, ids, action, func(c context.Context, ids []string, v aiflow.Settings) (out string, e error) {
				if v.Markdown == nil {
					return "", aiflow.ErrConfig
				}
				e = pin.WithNativeIndexes(c, func(*indexdb.Bundle) error {
					trees, e := pin.workspace.resources.documentInfoTrees(c, pin.workspace, ids)
					if e != nil {
						return e
					}
					out, e = aiflow.Blocks(c, ids, trees, *v.Markdown)
					return e
				})
				return
			})
		}
		result <- out
		return e
	})
	if e != nil {
		return "", e
	}
	e = task.Wait(ctx)
	if e != nil {
		select {
		case text := <-result:
			return text, e
		default:
			return "", e
		}
	}
	return <-result, nil
}
func (l *Lease) NativeAIChat(ctx context.Context, msg string) (string, error) {
	return l.nativeAI(ctx, msg, nil, "", false)
}
func (l *Lease) NativeAIAction(ctx context.Context, ids []string, action string) (string, error) {
	return l.nativeAI(ctx, "", append([]string(nil), ids...), action, true)
}
func (l *Lease) NativeCloudChatGPT(ctx context.Context, msg string, history []string) (aiflow.Part, error) {
	history = append([]string(nil), history...)
	out := make(chan aiflow.Part, 1)
	e := l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error {
		part, e := s.CloudChatGPT(c, msg, history)
		if e == nil {
			out <- part
		}
		return e
	})
	if e != nil {
		return aiflow.Part{}, e
	}
	return <-out, nil
}
