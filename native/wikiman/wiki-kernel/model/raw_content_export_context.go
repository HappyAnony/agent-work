// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/util"
)

func exportOwnedContent(ctx context.Context, name string, raw []byte, temp bool) (string, error) {
	if len(raw) > 512<<20 {
		return "", errors.New("export_content_budget")
	}
	raw = append([]byte(nil), raw...)
	return runRawArchive(ctx, func(a *rawArchiveContext) (string, error) {
		clean := util.FilterFileName(name)
		ext := path.Ext(clean)
		title := strings.TrimSuffix(clean, ext)
		o, e := a.newOutput(title)
		if e != nil {
			return "", e
		}
		o.filename = strings.TrimSuffix(o.filename, ".zip") + ext
		if temp {
			if e = o.useSubdirectory("temp"); e != nil {
				return "", e
			}
		}
		if e = o.publishContent(func(w io.Writer) error { return archivepack.CopyContext(a.ctx, w, bytes.NewReader(raw)) }); e != nil {
			return "", e
		}
		a.keep = true
		// Preserve slashes in the checked optional route component.
		pieces := strings.Split(o.filename, "/")
		for i := range pieces {
			pieces[i] = url.PathEscape(pieces[i])
		}
		return "/export/" + strings.Join(pieces, "/"), a.guard()
	})
}
func ExportTempContentContext(ctx context.Context, content string) (string, error) {
	origin := "http://" + util.LocalHost + ":" + util.ServerPort
	relative, e := exportOwnedContent(ctx, "temp", []byte(content), true)
	if e != nil {
		return "", e
	}
	return origin + relative, nil
}
func ExportAsFileRequest(ctx context.Context, request *http.Request) (string, error) {
	return runSingleUpload(ctx, request, "file", 512<<20, func(c context.Context, base *legacyReindexOwner, input singleUpload) (string, error) {
		mediaType := ""
		if values := input.Values["type"]; len(values) > 0 {
			mediaType = values[0]
		}
		if e := base.guard(c); e != nil {
			return "", e
		}
		return exportOwnedContent(c, archivepack.UploadFilename(input.Name, mediaType), input.Data, false)
	})
}
