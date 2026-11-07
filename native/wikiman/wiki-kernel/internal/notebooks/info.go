// SPDX-License-Identifier: AGPL-3.0-or-later
package notebooks

import (
	"context"
	"github.com/88250/go-humanize"
	"html"
	"time"
	"wiki-kernel/internal/indexrepair"
)

type Info struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DocCount int    `json:"docCount"`
	Size     uint64 `json:"size"`
	HSize    string `json:"hSize"`
	Mtime    int64  `json:"mtime"`
	CTime    int64  `json:"ctime"`
	HMtime   string `json:"hMtime"`
	HCtime   string `json:"hCtime"`
}

func Information(ctx context.Context, source indexrepair.Source, id, name string, location *time.Location, relative func(time.Time) string) (out *Info, err error) {
	if ctx == nil || !idPattern.MatchString(id) || location == nil || relative == nil {
		return nil, ErrPath
	}
	docs, e := source.Documents(ctx, id)
	if e != nil {
		return nil, e
	}
	created, _ := time.ParseInLocation("20060102150405", id[:14], location)
	latest := created
	out = &Info{ID: id, Name: html.EscapeString(name), CTime: created.Unix(), HCtime: created.Format("2006-01-02 15:04:05") + ", " + relative(created)}
	for _, doc := range docs {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if e = source.Check(doc.Relative, false); e != nil {
			return nil, e
		}
		info, e := source.Data.Lstat(doc.Relative)
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() || info.Size() < 0 {
			return nil, ErrPath
		}
		out.DocCount++
		out.Size += uint64(info.Size())
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	out.HSize = humanize.BytesCustomCeil(out.Size, 2)
	out.Mtime = latest.Unix()
	out.HMtime = latest.Format("2006-01-02 15:04:05") + ", " + relative(latest)
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
