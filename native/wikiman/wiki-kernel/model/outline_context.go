// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"time"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/pageprefs"
)

func OutlineContext(ctx context.Context, id string, preview bool) ([]*Path, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]*Path, error) {
		if e := FlushTxQueueContext(c); e != nil {
			return nil, e
		}
		tree, e := o.loadOptional(c, id)
		if e != nil {
			return nil, e
		}
		if tree == nil {
			return []*Path{}, nil
		}
		backend := pageprefs.Backend{Now: time.Now, Read: func(c context.Context, name string) ([]byte, error) { return o.raw(c, "storage/"+name) }, Write: func(context.Context, string, []byte) error { return ErrSettingReadOnly }}
		value, e := o.base.owner.preferencesService().Do(c, pageprefs.Request{Action: "outline.get", Key: id}, backend)
		if e != nil {
			return nil, e
		}
		storage, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.New("outline_storage_invalid")
		}
		if preview && o.settings.Export == nil {
			return nil, errors.New("outline_export_settings_missing")
		}
		addTitle := preview && o.settings.Export.AddTitle
		return blockview.OutlineView(c, tree, o.parser, addTitle, storage)
	})
}
