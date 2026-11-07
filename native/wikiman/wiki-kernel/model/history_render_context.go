// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func GetDocHistoryContentContext(ctx context.Context, file, keyword string, highlight bool) (string, string, string, bool, error) {
	v, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out docview.HistoryResult, err error) {
		if !filepath.IsAbs(file) {
			return out, indexrepair.ErrSourcePath
		}
		relative, e := filepath.Rel(o.base.workspace, file)
		if e != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return out, indexrepair.ErrSourcePath
		}
		root, e := os.OpenRoot(o.base.workspace)
		if e != nil {
			return out, e
		}
		defer func() {
			err = errors.Join(err, root.Close())
			if err != nil {
				out = docview.HistoryResult{}
			}
		}()
		raw, e := (indexrepair.Source{Data: root}).Read(c, filepath.ToSlash(relative))
		if indexrepair.IsSourceNotExist(e) {
			return out, nil
		}
		if e != nil {
			return out, e
		}
		if o.settings.Search == nil {
			return out, errors.New("history_search_settings_unbound")
		}
		out, err = docview.RenderHistory(c, raw, keyword, highlight, o.settings.Search.CaseSensitive, o.parser)
		if err == nil && out.Large {
			o.base.owner.publishSyncUI(c, func() {
				util.PushMsg(o.base.owner.business.Language(o.settings.Lang, 36, true, o.settings.CloudRegion), 5000)
			})
		}
		return
	})
	return v.ID, v.RootID, v.Content, v.Large, e
}
