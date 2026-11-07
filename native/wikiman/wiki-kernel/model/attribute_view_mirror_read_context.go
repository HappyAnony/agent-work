// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
)

func AttributeViewMirrorContext(ctx context.Context, id string) (bool, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (bool, error) {
		av.AttributeViewBlocksLock.Lock()
		defer av.AttributeViewBlocksLock.Unlock()
		raw, e := o.raw(c, "storage/av/blocks.msgpack")
		if indexrepair.IsSourceNotExist(e) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		var rows map[string][]string
		if e = msgpack.Unmarshal(raw, &rows); e != nil {
			return false, e
		}
		return len(rows[id]) > 1, c.Err()
	})
}
