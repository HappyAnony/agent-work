// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloud

import (
	"context"
	"encoding/base64"
	"github.com/siyuan-note/httpclient"
	"github.com/studio-b12/gowebdav"
	"net/http"
	"time"
)

// OpenContext creates an immutable request-owned provider. Cleanup/renewal never
// rewrites a Conf that another request or provider is concurrently reading.
func OpenContext(source Cloud, ctx context.Context) (Cloud, func(), error) {
	cfg := *source.GetConf()
	cfg.RequestContext = ctx
	base := &BaseCloud{Conf: &cfg}
	switch source.(type) {
	case *SiYuan:
		return NewSiYuan(base), func() {}, nil
	case *Local:
		return NewLocal(base), func() {}, nil
	case *S3:
		if cfg.S3 == nil {
			return nil, nil, ErrCloudCheckFailed
		}
		t := httpclient.NewTransport(cfg.S3.SkipTlsVerify)
		h := &http.Client{Transport: t, Timeout: time.Duration(cfg.S3.Timeout) * time.Second}
		return NewS3(base, h), t.CloseIdleConnections, nil
	case *WebDAV:
		if cfg.WebDAV == nil {
			return nil, nil, ErrCloudCheckFailed
		}
		v := cfg.WebDAV
		c := gowebdav.NewClient(v.Endpoint, v.Username, v.Password)
		c.SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(v.Username+":"+v.Password)))
		c.SetHeader("User-Agent", cfg.UserAgent)
		c.SetTimeout(time.Duration(v.Timeout) * time.Second)
		t := httpclient.NewTransport(v.SkipTlsVerify)
		c.SetTransport(contextTransport{ctx, t})
		return NewWebDAV(base, c), t.CloseIdleConnections, nil
	default:
		return nil, nil, ErrCloudCheckFailed
	}
}

type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func providerRetry(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (t contextTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(r.Clone(t.ctx))
}
