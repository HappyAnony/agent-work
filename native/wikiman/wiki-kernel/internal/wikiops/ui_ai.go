// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"strings"
	"time"

	"wiki-kernel/internal/workspace"
)

func runUIAI(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	status := r.Endpoint == "/api/ai/status"
	if view.Mode != "wikiDesktop" {
		if status {
			return false, nil
		}
		return nil, ErrUIEndpoint
	}
	l, err := m.Acquire(ctx, r.StoreID, r.PageID)
	if err != nil {
		return nil, err
	}
	defer l.Release()
	available, err := l.NativeAIAvailable(ctx)
	if status || err != nil {
		return available, err
	}
	if !available {
		return nil, ErrUIEndpoint
	}
	var q struct{ Msg string }
	if err = Decode(bytes.NewReader(r.Data), &q); err != nil {
		return nil, err
	}
	if len(q.Msg) > 32<<10 || strings.TrimSpace(q.Msg) == "" {
		return nil, ErrArgument
	}
	c, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	output, err := l.NativeAIChat(c, q.Msg)
	if len(output) > 128<<10 {
		return nil, ErrArgument
	}
	return output, err
}
