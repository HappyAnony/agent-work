// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/internal/docquery"
)

type indexingNoticeError struct{ message string }

func (e indexingNoticeError) Error() string { return e.message }
func (e indexingNoticeError) Unwrap() error { return ErrIndexing }

func GetBlockInfoContext(ctx context.Context, id string) (map[string]string, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (map[string]string, error) {
		tree, e := o.loadByBlockID(c, id, true)
		if errors.Is(e, ErrIndexing) {
			return nil, indexingNoticeError{message: o.base.owner.business.Language(o.settings.Lang, 56, true, o.settings.CloudRegion)}
		}
		if e != nil {
			return nil, e
		}
		return docquery.BlockLocation(c, id, tree, o.parser, o.content())
	})
}
