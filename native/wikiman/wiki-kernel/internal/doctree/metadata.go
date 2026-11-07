// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

import (
	"context"
	"errors"
	"github.com/88250/go-humanize"
	"time"
)

func Metadata(ctx context.Context, file Entry, ial map[string]string, location *time.Location, relative func(time.Time) (string, error)) (*File, error) {
	if ctx == nil || location == nil || relative == nil {
		return nil, errors.New("document_tree_dependencies_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ial["id"]) < 14 || file.Size < 0 {
		return nil, errors.New("invalid_document_tree_metadata")
	}
	ret := &File{Path: file.Path, Size: uint64(file.Size), Name: ial["title"] + ".docj", Icon: ial["icon"], ID: ial["id"], Name1: ial["name"], Alias: ial["alias"], Memo: ial["memo"], Bookmark: ial["bookmark"]}
	t, _ := time.ParseInLocation("20060102150405", ret.ID[:14], location)
	ret.CTime = t.Unix()
	human, err := relative(t)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	ret.HCtime = t.Format("2006-01-02 15:04:05") + ", " + human
	ret.HSize = humanize.BytesCustomCeil(ret.Size, 2)
	m := t
	if updated := ial["updated"]; updated != "" {
		if parsed, e := time.ParseInLocation("20060102150405", updated, location); e == nil {
			m = parsed
		}
	}
	ret.Mtime = m.Unix()
	human, err = relative(m)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	ret.HMtime = m.Format("2006-01-02 15:04:05") + ", " + human
	return ret, nil
}
