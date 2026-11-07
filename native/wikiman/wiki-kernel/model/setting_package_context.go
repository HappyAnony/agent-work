// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/settingpack"
	"wiki-kernel/util"
)

type SettingPackageResult struct {
	Name string `json:"name"`
	ZIP  string `json:"zip"`
}

func ExportSettingPackageContext(ctx context.Context, kind string) (SettingPackageResult, error) {
	key := append([]byte(nil), util.SK...)
	return runRawArchive(ctx, func(a *rawArchiveContext) (SettingPackageResult, error) {
		var raw []byte
		e := a.base.owner.WithSettingSnapshot(a.ctx, func(v conf.AppData) error { var e error; raw, e = settingpack.Projection(a.ctx, kind, v); return e })
		if e != nil {
			return SettingPackageResult{}, e
		}
		suffix := ""
		switch kind {
		case "conf":
			suffix = "conf"
		case "syncWebDAV":
			suffix = "webdav"
		case "syncS3":
			suffix = "s3"
		default:
			return SettingPackageResult{}, settingpack.ErrPackage
		}
		if kind != "conf" {
			raw, e = settingpack.EncodeProvider(a.ctx, raw, key)
			if e != nil {
				return SettingPackageResult{}, e
			}
		}
		name := "siyuan-" + suffix + "-" + time.Now().Format("20060102150405") + ".json"
		o, e := a.newOutput(name)
		if e != nil {
			return SettingPackageResult{}, e
		}
		source := indexrepair.Source{Data: o.payload, Authorize: func(context.Context) error { return a.guard() }}
		if e = source.Rewrite(a.ctx, name, nil, raw); e != nil {
			return SettingPackageResult{}, e
		}
		o.prefix = ""
		if e = o.pack(); e != nil {
			return SettingPackageResult{}, e
		}
		a.keep = true
		return SettingPackageResult{Name: name, ZIP: "/export/" + url.PathEscape(o.filename)}, a.guard()
	})
}
func ImportSyncProviderRequest(ctx context.Context, request *http.Request, kind string) (json.RawMessage, error) {
	key := append([]byte(nil), util.SK...)
	return runSingleUpload(ctx, request, "file", 16<<20, func(c context.Context, base *legacyReindexOwner, input singleUpload) (json.RawMessage, error) {
		if kind != "syncWebDAV" && kind != "syncS3" {
			return nil, settingpack.ErrPackage
		}
		encoded, e := conf.ReadImport(c, input.Name, bytes.NewReader(input.Data))
		if e != nil {
			return nil, e
		}
		raw, e := settingpack.DecodeProvider(c, encoded, key)
		if e != nil {
			return nil, e
		}
		if e = base.guard(c); e != nil {
			return nil, e
		}
		return SetSyncSettingContext(c, kind, raw)
	})
}
func ImportConfigurationRequest(ctx context.Context, request *http.Request) error {
	_, e := runSingleUpload(ctx, request, "file", 16<<20, func(c context.Context, base *legacyReindexOwner, input singleUpload) (struct{}, error) {
		if e := base.guard(c); e != nil {
			return struct{}{}, e
		}
		return struct{}{}, base.owner.ImportConfiguration(c, input.Name, bytes.NewReader(input.Data))
	})
	return e
}
