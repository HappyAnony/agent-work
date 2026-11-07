// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package catalogue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func HTTPFetch(client *http.Client) func(context.Context, string) ([]byte, error) {
	return func(ctx context.Context, selected string) ([]byte, error) {
		if ctx == nil || client == nil {
			return nil, ErrDependency
		}
		u, e := url.Parse(selected)
		if e != nil {
			return nil, e
		}
		if u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
			return nil, ErrValue
		}
		q, e := http.NewRequestWithContext(ctx, http.MethodGet, selected, nil)
		if e != nil {
			return nil, e
		}
		q.Header.Set("User-Agent", "Mozilla/5.0 WikiCatalogue")
		response, e := client.Do(q)
		if e != nil {
			return nil, e
		}
		raw, e := io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
		e = errors.Join(e, response.Body.Close())
		if response.StatusCode != http.StatusOK {
			e = errors.Join(e, fmt.Errorf("catalogue_http_status_%d", response.StatusCode))
		}
		if e != nil {
			return nil, e
		}
		if len(raw) > MaxBytes {
			return nil, ErrValue
		}
		return raw, ctx.Err()
	}
}
