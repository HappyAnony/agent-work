// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/go-humanize"
	"github.com/siyuan-note/httpclient"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/util"
)

func requestCloudUpload(ctx context.Context, region int, token string, file cloudaccount.UploadFile, biz, meta string) (cloudaccount.Response, error) {
	return requestCloudUploadURL(ctx, util.GetCloudServerForRegion(region)+"/apis/siyuan/upload?ver="+url.QueryEscape(util.Ver), token, file, biz, meta)
}
func requestCloudUploadURL(ctx context.Context, address, token string, file cloudaccount.UploadFile, biz, meta string) (cloudaccount.Response, error) {
	result := map[string]any{}
	resp, e := httpclient.NewCloudFileRequest2m().SetContext(ctx).SetSuccessResult(&result).SetFileBytes("file[]", file.Name, file.Data).SetCookies(&http.Cookie{Name: "symphony", Value: token}).SetHeader("meta-type", meta).SetHeader("biz-type", biz).Post(address)
	if e != nil {
		return cloudaccount.Response{}, e
	}
	if resp == nil {
		return cloudaccount.Response{}, cloudaccount.ErrResponse
	}
	return cloudaccount.Response{Status: resp.StatusCode, Body: result}, nil
}
func cloudBusinessError(owner *AppConf, e error) error {
	if errors.Is(e, cloudaccount.ErrAuthentication) {
		if owner == nil {
			return e
		}
		return errors.New(owner.Language(31))
	}
	if errors.Is(e, cloudaccount.ErrNetwork) {
		return ErrFailedToConnectCloudServer
	}
	return e
}
func StartFreeTrialContext(ctx context.Context) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	return cloudBusinessError(owner, s.StartFreeTrial(ctx))
}
func DeactivateUserContext(ctx context.Context) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	return cloudBusinessError(owner, s.Deactivate(ctx))
}
func LoadUploadTokenContext(ctx context.Context) (string, error) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return "", e
	}
	token, e := s.UploadToken(ctx)
	return token, cloudBusinessError(owner, e)
}

func (owner *AppConf) uploadCloudAssets(ctx context.Context, paths []string, biz string) (int, error) {
	return owner.uploadCloudAssetsWithIO(ctx, paths, biz, cloudUploadIO{})
}
func (owner *AppConf) uploadCloudAssetsWithIO(ctx context.Context, paths []string, biz string, op cloudUploadIO) (count int, err error) {
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	if len(paths) == 0 {
		return 0, ctx.Err()
	}
	if owner == nil {
		return 0, cloudaccount.ErrDependency
	}
	s, e := owner.CloudAccount()
	if e != nil {
		return 0, e
	}
	dataDir, workspace := util.DataDir, util.WorkspaceDir
	op = op.defaults()
	base, e := filepath.Abs(dataDir)
	if e != nil {
		return 0, e
	}
	info, e := os.Lstat(base)
	if e != nil {
		return 0, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, cloudaccount.ErrAssetChanged
	}
	guard := func(ctx context.Context) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if owner != Conf || util.DataDir != dataDir || util.WorkspaceDir != workspace {
			return conf.ErrBusinessUnbound
		}
		owner.cloudMu.Lock()
		closed := owner.cloudClosed
		owner.cloudMu.Unlock()
		owner.repoMu.Lock()
		closed = closed || owner.repoClosed
		owner.repoMu.Unlock()
		if closed {
			return conf.ErrBusinessUnbound
		}
		now, e := os.Lstat(base)
		if e != nil {
			return e
		}
		if !now.IsDir() || now.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, now) {
			return cloudaccount.ErrAssetChanged
		}
		return nil
	}
	if e = guard(ctx); e != nil {
		return 0, e
	}
	root, e := op.openRoot(base)
	if e != nil {
		return 0, e
	}
	closeIO := newRepairIOCloser([]*os.Root{root}, op.closeRoot)
	defer func() { err = joinRepairCloseError(err, closeIO()); err = joinRepairCloseError(err, guard(ctx)) }()
	defer recoverRepositoryWorkflow(&err, "cloud_upload")
	opened, e := root.Stat(".")
	if e != nil {
		return 0, e
	}
	if !os.SameFile(info, opened) {
		return 0, cloudaccount.ErrAssetChanged
	}
	if e = guard(ctx); e != nil {
		return 0, e
	}
	msgID := ""
	count, e = s.UploadAssets(ctx, paths, biz, cloudaccount.UploadPorts{
		Resolve: func(ctx context.Context, p string) (string, error) {
			if e := guard(ctx); e != nil {
				return "", e
			}
			clean := strings.TrimSpace(p)
			if i := strings.IndexByte(clean, '?'); i >= 0 {
				clean = clean[:i]
			}
			if !strings.HasPrefix(clean, "assets/") || !fs.ValidPath(clean) || strings.Contains(clean, "\\") {
				return "", cloudaccount.ErrAssetChanged
			}
			name, e := GetAssetAbsPathContext(ctx, clean)
			if e != nil {
				return "", e
			}
			if e = guard(ctx); e != nil {
				return "", e
			}
			return name, nil
		},
		Open: func(ctx context.Context, p string) (io.ReadCloser, int64, error) {
			if e := guard(ctx); e != nil {
				return nil, 0, e
			}
			relative, e := filepath.Rel(dataDir, p)
			if e != nil {
				return nil, 0, e
			}
			parts := strings.Split(filepath.ToSlash(relative), "/")
			for i := range parts {
				v, e := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
				if e != nil {
					return nil, 0, e
				}
				if v.Mode()&os.ModeSymlink != 0 {
					return nil, 0, cloudaccount.ErrAssetChanged
				}
			}
			stat, e := root.Lstat(relative)
			if e != nil {
				return nil, 0, e
			}
			if !stat.Mode().IsRegular() {
				return nil, 0, cloudaccount.ErrAssetChanged
			}
			return &cloudUploadFile{ctx: ctx, root: root, name: filepath.ToSlash(relative), identity: stat, guard: func() error { return guard(ctx) }, open: op.openFile}, stat.Size(), nil
		},
		Event: func(ctx context.Context, event cloudaccount.UploadEvent) error {
			if e := guard(ctx); e != nil {
				return e
			}
			switch event.Kind {
			case "begin":
				msgID = util.PushMsg(fmt.Sprintf(owner.Language(27), event.Total), 3000)
			case "end":
				if msgID != "" {
					util.PushClearMsg(msgID)
				}
			case "oversize":
				util.PushErrMsg(fmt.Sprintf(owner.Language(247), filepath.Base(event.Asset), humanize.IBytes(uint64(event.Limit))), 30000)
			case "upload":
				msg := fmt.Sprintf(owner.Language(27), html.EscapeString(event.Asset))
				util.PushStatusBar(msg)
				util.PushUpdateMsg(msgID, msg, 3000)
			}
			return nil
		},
	})
	var remote *cloudaccount.BusinessError
	if errors.As(e, &remote) {
		return count, &cloudUploadError{message: fmt.Sprintf(owner.Language(94), remote.Message), cause: e}
	}
	if e != nil {
		friendly := cloudBusinessError(owner, e)
		return count, &cloudUploadError{message: friendly.Error(), cause: errors.Join(e, friendly)}
	}
	return count, nil
}
func uploadAssets2CloudContext(ctx context.Context, paths []string, biz string) (int, error) {
	return Conf.uploadCloudAssets(ctx, paths, biz)
}

func UseActivationcodeContext(ctx context.Context, value string) error {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return e
	}
	return cloudBusinessError(owner, s.UseActivationcode(ctx, value))
}
func CheckActivationcodeContext(ctx context.Context, value string) (int, string) {
	owner := Conf
	s, e := owner.CloudAccount()
	if e != nil {
		return 1, e.Error()
	}
	v, e := s.CheckActivationcode(ctx, value)
	if e != nil {
		return 1, cloudBusinessError(owner, e).Error()
	}
	return v.Code, v.Msg
}
