// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/go-humanize"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/httpclient"
	"github.com/studio-b12/gowebdav"
	"net/http"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/util"
)

type metadataRepoPaths struct {
	Data, Repo, History, Temp, Agent string
	Workspace                        string
	Server, Endpoint                 string
	Transports                       *[]*http.Transport
	Ignore                           []string
}
type metadataRoundTripper struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t metadataRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(r.Clone(t.ctx))
}
func metadataTransport(ctx context.Context, skip bool, all *[]*http.Transport) http.RoundTripper {
	t := httpclient.NewTransport(skip)
	*all = append(*all, t)
	return metadataRoundTripper{ctx, t}
}
func metadataCloudConf(v conf.AppData) (ret *cloud.Conf, err error) {
	if v.Sync == nil {
		return nil, cloudrepo.ErrDependency
	}
	if !cloud.IsValidCloudDirName(v.Sync.CloudName) {
		v.Sync.CloudName = "main"
	}
	userId, token, availableSize := "0", "", int64(1024*1024*1024*1024*2)
	if nil != v.User && conf.ProviderSiYuan == v.Sync.Provider {
		u := v.User
		userId = u.UserId
		token = u.UserToken
		availableSize = u.GetCloudRepoAvailableSize()
	}

	ret = &cloud.Conf{
		Dir:           v.Sync.CloudName,
		UserID:        userId,
		Token:         token,
		AvailableSize: availableSize,
		Server:        util.GetCloudServerForRegion(v.CloudRegion),
	}

	switch v.Sync.Provider {
	case conf.ProviderSiYuan:
		ret.Endpoint = util.GetCloudSyncServerForRegion(v.CloudRegion)
	case conf.ProviderS3:
		if v.Sync.S3 == nil {
			return nil, cloudrepo.ErrDependency
		}
		ret.S3 = &cloud.ConfS3{
			Endpoint:       v.Sync.S3.Endpoint,
			AccessKey:      v.Sync.S3.AccessKey,
			SecretKey:      v.Sync.S3.SecretKey,
			Bucket:         v.Sync.S3.Bucket,
			Region:         v.Sync.S3.Region,
			PathStyle:      v.Sync.S3.PathStyle,
			SkipTlsVerify:  v.Sync.S3.SkipTlsVerify,
			Timeout:        v.Sync.S3.Timeout,
			ConcurrentReqs: v.Sync.S3.ConcurrentReqs,
		}
	case conf.ProviderWebDAV:
		if v.Sync.WebDAV == nil {
			return nil, cloudrepo.ErrDependency
		}
		ret.WebDAV = &cloud.ConfWebDAV{
			Endpoint:       v.Sync.WebDAV.Endpoint,
			Username:       v.Sync.WebDAV.Username,
			Password:       v.Sync.WebDAV.Password,
			SkipTlsVerify:  v.Sync.WebDAV.SkipTlsVerify,
			Timeout:        v.Sync.WebDAV.Timeout,
			ConcurrentReqs: v.Sync.WebDAV.ConcurrentReqs,
		}
	case conf.ProviderLocal:
		if v.Sync.Local == nil {
			return nil, cloudrepo.ErrDependency
		}
		ret.Local = &cloud.ConfLocal{
			Endpoint:       v.Sync.Local.Endpoint,
			Timeout:        v.Sync.Local.Timeout,
			ConcurrentReqs: v.Sync.Local.ConcurrentReqs,
		}
	default:
		err = fmt.Errorf("invalid provider [%d]", v.Sync.Provider)
		return
	}
	return
}

func newMetadataRepository(ctx context.Context, v conf.AppData, paths metadataRepoPaths) (ret *dejavu.Repo, err error) {
	for _, p := range []string{paths.Data, paths.Repo, paths.History, paths.Temp} {
		if !filepath.IsAbs(p) {
			return nil, cloudrepo.ErrDependency
		}
	}
	cloudConf, err := metadataCloudConf(v)
	if err != nil {
		return
	}

	cloudConf.RequestContext = ctx
	cloudConf.UserAgent = paths.Agent
	if paths.Server != "" {
		cloudConf.Server = paths.Server
	}
	if paths.Endpoint != "" {
		cloudConf.Endpoint = paths.Endpoint
	}

	var cloudRepo cloud.Cloud
	switch v.Sync.Provider {
	case conf.ProviderSiYuan:
		cloudRepo = cloud.NewSiYuan(&cloud.BaseCloud{Conf: cloudConf})
	case conf.ProviderS3:
		s3HTTPClient := &http.Client{Transport: metadataTransport(ctx, cloudConf.S3.SkipTlsVerify, paths.Transports)}
		s3HTTPClient.Timeout = time.Duration(cloudConf.S3.Timeout) * time.Second
		cloudRepo = cloud.NewS3(&cloud.BaseCloud{Conf: cloudConf}, s3HTTPClient)
	case conf.ProviderWebDAV:
		webdavClient := gowebdav.NewClient(cloudConf.WebDAV.Endpoint, cloudConf.WebDAV.Username, cloudConf.WebDAV.Password)
		a := cloudConf.WebDAV.Username + ":" + cloudConf.WebDAV.Password
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(a))
		webdavClient.SetHeader("Authorization", auth)
		webdavClient.SetHeader("User-Agent", paths.Agent)
		webdavClient.SetTimeout(time.Duration(cloudConf.WebDAV.Timeout) * time.Second)
		webdavClient.SetTransport(metadataTransport(ctx, cloudConf.WebDAV.SkipTlsVerify, paths.Transports))
		cloudRepo = cloud.NewWebDAV(&cloud.BaseCloud{Conf: cloudConf}, webdavClient)
	case conf.ProviderLocal:
		cloudRepo = cloud.NewLocal(&cloud.BaseCloud{Conf: cloudConf})
	default:
		err = fmt.Errorf("unknown cloud provider [%d]", v.Sync.Provider)
		return
	}

	ignoreLines := append([]string{}, paths.Ignore...)
	ignoreLines = append(ignoreLines, "/.siyuan/conf.json") // 忽略旧版同步配置
	ret, err = dejavu.NewRepo(paths.Data, paths.Repo, paths.History, paths.Temp, v.System.ID, v.System.Name, v.System.OS, v.Repo.Key, ignoreLines, cloudRepo)
	if err != nil {
		// The caller handles errors without logging private provider URLs.
		return
	}
	return
}

// executeMetadata preserves the pinned repository algorithms. It never checks
// out source data or starts the old global sync/lock-refresh machinery.
func executeMetadata(ctx context.Context, v conf.AppData, r cloudrepo.Request, p metadataRepoPaths) (out cloudrepo.Result, err error) {
	defer func() {
		if err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			} else if !errors.Is(err, cloudrepo.ErrDependency) && !errors.Is(err, cloudrepo.ErrOperation) && !errors.Is(err, cloud.ErrCloudAuthFailed) {
				err = cloudrepo.ErrProvider
			}
		}
	}()
	if v.Sync == nil || v.Repo == nil || v.System == nil {
		return out, cloudrepo.ErrDependency
	}
	var transports []*http.Transport
	p.Transports = &transports
	defer func() {
		for _, t := range transports {
			t.CloseIdleConnections()
		}
	}()
	if r.Kind == "logs" || r.Kind == "tags" {
		if len(v.Repo.Key) == 0 {
			return out, errors.New("repository key is not initialized")
		}
		u := v.User
		subscriber := u != nil && (u.UserSiYuanProExpireTime == -1 || u.UserSiYuanProExpireTime > 0) && u.UserSiYuanSubscriptionStatus == 0
		paid := subscriber || u != nil && u.UserSiYuanOneTimePayStatus == 1
		if v.Sync.Provider == conf.ProviderSiYuan && !subscriber || v.Sync.Provider != conf.ProviderSiYuan && !paid {
			out.Notice = 29
			if v.Sync.Provider != conf.ProviderSiYuan {
				out.Notice = 214
			}
			data := map[string]any{"snapshots": []any{}}
			if r.Kind == "logs" {
				data["pageCount"] = 0
				data["totalCount"] = 0
			}
			out.Data, err = json.Marshal(data)
			return
		}
	}
	repo, e := newMetadataRepository(ctx, v, p)
	if e != nil {
		return out, e
	}
	defer repo.CloseMetadata()
	var data any
	switch r.Kind {
	case "space":
		st, e := repo.GetCloudRepoStat()
		if e != nil {
			return out, e
		}
		if st == nil || st.Sync == nil || st.Backup == nil || st.Sync.Size < 0 || st.Backup.Size < 0 || st.AssetSize < 0 {
			return out, cloudrepo.ErrOperation
		}
		s := &Sync{Size: st.Sync.Size, HSize: "-", Updated: st.Sync.Updated}
		b := &Backup{Size: st.Backup.Size, HSize: "-", Updated: st.Backup.Updated}
		d := map[string]any{"sync": s, "backup": b, "hSize": "-", "hAssetSize": "-", "hTotalSize": "-", "hExchangeSize": "-", "hTrafficUploadSize": "-", "hTrafficDownloadSize": "-", "hTrafficAPIGet": "-", "hTrafficAPIPut": "-"}
		if v.Sync.Provider == conf.ProviderSiYuan {
			s.HSize = humanize.BytesCustomCeil(uint64(s.Size), 2)
			b.HSize = humanize.BytesCustomCeil(uint64(b.Size), 2)
			d["hAssetSize"] = humanize.BytesCustomCeil(uint64(st.AssetSize), 2)
			d["hSize"] = humanize.BytesCustomCeil(uint64(s.Size)+uint64(b.Size)+uint64(st.AssetSize), 2)
			if u := v.User; u != nil {
				d["hTotalSize"] = humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2)
				d["hExchangeSize"] = humanize.BytesCustomCeil(uint64(u.UserSiYuanPointExchangeRepoSize), 2)
				d["hTrafficUploadSize"] = humanize.BytesCustomCeil(uint64(u.UserTrafficUpload), 2)
				d["hTrafficDownloadSize"] = humanize.BytesCustomCeil(uint64(u.UserTrafficDownload), 2)
				d["hTrafficAPIGet"] = humanize.SIWithDigits(u.UserTrafficAPIGet, 2, "")
				d["hTrafficAPIPut"] = humanize.SIWithDigits(u.UserTrafficAPIPut, 2, "")
			}
		}
		data = d
	case "repos":
		dirs, size, e := repo.GetCloudRepos()
		if e != nil {
			return out, e
		}
		if len(dirs) == 0 {
			dirs = []*cloud.Repo{{Name: "main", Updated: time.Now().Format("2006-01-02 15:04:05")}}
		}
		list := []*Sync{}
		for _, d := range dirs {
			if d == nil || d.Size < 0 {
				return out, cloudrepo.ErrOperation
			}
			x := &Sync{Size: d.Size, HSize: "-", Updated: d.Updated, CloudName: d.Name}
			if v.Sync.Provider == conf.ProviderSiYuan {
				x.HSize = humanize.BytesCustomCeil(uint64(x.Size), 2)
			}
			list = append(list, x)
		}
		h := "-"
		if v.Sync.Provider == conf.ProviderSiYuan {
			h = humanize.BytesCustomCeil(uint64(size), 2)
		}
		checked := v.Sync.CloudName
		if v.Sync.Provider == conf.ProviderS3 {
			checked = list[0].CloudName
		}
		data = map[string]any{"syncDirs": list, "hSize": h, "checkedSyncDir": checked}
	case "logs":
		logs, pc, tc, e := repo.GetCloudRepoLogs(r.Page)
		if e != nil {
			return out, e
		}
		if logs == nil {
			logs = []*dejavu.Log{}
		}
		data = map[string]any{"snapshots": logs, "pageCount": pc, "totalCount": tc}
	case "tags":
		logs, e := repo.GetCloudRepoTagLogs(nil)
		if e != nil {
			return out, e
		}
		if logs == nil {
			logs = []*dejavu.Log{}
		}
		data = map[string]any{"snapshots": logs}
	default:
		return out, cloudrepo.ErrOperation
	}
	out.Data, err = json.Marshal(data)
	return
}
