// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io"
	"os"
	"path"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func NetAssets2LocalAssetsContext(ctx context.Context, id string, onlyImg bool, referer string) error {
	// Transport is a selected device capability; the shared algorithm receives
	// response bytes and never discovers a workspace or starts detached work.
	client := util.NewCustomReqClient()
	defer client.GetTransport().CloseIdleConnections()
	return runReferenceChange(ctx, func(c, life context.Context, o *workspaceReadOwner, a *legacyAnchorContext) (err error) {
		tree, e := loadReferenceChangeTree(c, a, id, map[string]*parse.Tree{})
		if e != nil {
			return e
		}
		ioContext, closeIO, e := a.r.openRepairIOChecked(c, true)
		if e != nil {
			return e
		}
		defer func() { err = joinRepairCloseError(err, closeIO()) }()
		source := ioContext.Source
		target, e := assetflow.TargetDirectory(c, source, tree.Box, tree.Path)
		if e != nil {
			return e
		}
		if e = source.EnsureDirectory(c, target); e != nil {
			return e
		}
		views := map[string]*av.AttributeView{}
		before := map[string][]byte{}
		readAV := func(id string) (*av.AttributeView, error) {
			if v, ok := views[id]; ok {
				return v, nil
			}
			if !ast.IsNodeIDPattern(id) {
				return nil, indexrepair.ErrSourcePath
			}
			raw, e := source.Read(c, "storage/av/"+id+".json")
			if indexrepair.IsSourceNotExist(e) {
				views[id] = nil
				return nil, nil
			}
			if e != nil {
				return nil, e
			}
			var v *av.AttributeView
			if e = json.Unmarshal(raw, &v); e != nil {
				return nil, e
			}
			if v == nil || v.ID != id {
				return nil, indexrepair.ErrSourcePath
			}
			views[id] = v
			before[id] = raw
			return v, nil
		}
		message := ast.NewNodeID()
		defer o.base.owner.publishSyncUI(life, func() { util.PushClearMsg(message) })
		changed := false
		defer func() {
			if changed && o.settings.Sync != nil {
				o.base.owner.syncRuntime.same.Store(0)
				o.base.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
			}
		}()
		result, e := assetflow.ConvertRemote(c, tree.Root, onlyImg, referer, assetflow.RemotePorts{
			NewID: ast.NewNodeID, CommonExt: util.IsCommonExt, ReadAV: readAV,
			SaveAV: func(v *av.AttributeView) error {
				av.UpgradeSpec(v)
				raw, e := av.EncodeAttributeView(v, a.r.singleLine)
				if e != nil {
					return e
				}
				if e = source.Rewrite(c, "storage/av/"+v.ID+".json", before[v.ID], raw); e != nil {
					return e
				}
				before[v.ID] = raw
				changed = true
				return nil
			},
			Write: func(c context.Context, name string, raw []byte) error {
				if e := source.Rewrite(c, path.Join(target, name), nil, raw); e != nil {
					return e
				}
				changed = true
				return nil
			},
			ReadLocal: func(c context.Context, name string) ([]byte, bool, error) {
				if e := a.r.guard(c); e != nil {
					return nil, false, e
				}
				info, e := os.Stat(name)
				if os.IsNotExist(e) {
					return nil, false, nil
				}
				if e != nil {
					return nil, false, e
				}
				if info.IsDir() {
					return nil, false, nil
				}
				raw, e := capturePDFWatermarkFile(name).read(512<<20, func() error { return a.r.guard(c) })
				return raw, e == nil, e
			},
			Fetch: func(c context.Context, address, referer string) (out assetflow.RemoteResponse, err error) {
				request := client.R().SetContext(c).DisableAutoReadResponse().SetRetryCount(1).SetRetryFixedInterval(3 * time.Second)
				if referer != "" {
					request.SetHeader("Referer", referer)
				}
				response, e := request.Get(address)
				if response != nil && response.Response != nil && response.Body != nil {
					defer func() { err = errors.Join(err, response.Body.Close()) }()
				}
				if e != nil {
					return out, e
				}
				if response == nil || response.Response == nil || response.Body == nil {
					return out, errors.New("asset_remote_response_missing")
				}
				out.Status = response.StatusCode
				out.ContentType = response.GetContentType()
				if out.Status != 200 || response.ContentLength > 96<<20 {
					return out, nil
				}
				out.Data, err = io.ReadAll(io.LimitReader(extensionGuardedBody{ReadCloser: response.Body, guard: func() error { return a.r.guard(c) }}, (96<<20)+1))
				return
			},
			Progress: func(address string) {
				if len(address) > 64 {
					address = address[:64] + "..."
				}
				o.base.owner.publishSyncUI(c, func() { util.PushUpdateMsg(message, fmt.Sprintf(a.r.language(119), address), 15000) })
			},
		})
		if e != nil {
			return e
		}
		if result.Files > 0 {
			if e = a.stage(c, tree, tree); e != nil {
				return e
			}
			o.base.owner.publishSyncUI(life, func() { util.PushMsg(fmt.Sprintf(a.r.language(120), result.Files), 5000) })
		} else if result.Forbidden == 0 {
			o.base.owner.publishSyncUI(life, func() { util.PushMsg(a.r.language(121), 3000) })
		}
		if result.Forbidden > 0 {
			o.base.owner.publishSyncUI(life, func() { util.PushErrMsg(fmt.Sprintf(a.r.language(255), result.Forbidden), 5000) })
		}
		return c.Err()
	})
}
