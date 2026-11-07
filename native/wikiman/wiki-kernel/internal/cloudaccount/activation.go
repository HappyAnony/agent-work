// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"net/http"
	"strings"

	"github.com/88250/gulu"
)

// Preserve the original trim -> invisible -> PUA normalization for both handlers.
func normalizeActivationcode(value string) string {
	return gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(strings.TrimSpace(value)))
}

func (s *Service) activation(ctx context.Context, value string, use bool) (Result, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return Result{}, e
	}
	defer done()
	v, e := s.state(ctx)
	if e != nil {
		return Result{}, e
	}
	if e = authenticated(v); e != nil {
		return Result{}, e
	}
	route := "/apis/siyuan/checkActivationcode"
	if use {
		route = "/apis/siyuan/useActivationcode"
		// Redemption can change entitlements even if its response is lost. Never
		// reuse a credential minted before this attempt or retry it automatically.
		s.upload = uploadCache{}
	}
	cookie := (&http.Cookie{Name: "symphony", Value: v.User.UserToken}).String()
	r, e := s.options.Request(ctx, v.Region, route, map[string]string{"Cookie": cookie}, map[string]string{"data": normalizeActivationcode(value)})
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if e != nil {
		return Result{}, ErrNetwork
	}
	if r.Status == http.StatusUnauthorized {
		s.upload = uploadCache{}
	}
	// Both original activation endpoints display non-200 responses as a
	// connection failure, unlike the deactivation endpoint's 401 handling.
	if r.Status != http.StatusOK {
		return Result{}, ErrNetwork
	}
	n, e := code(r.Body)
	if e != nil {
		return Result{}, e
	}
	msg, ok := r.Body["msg"].(string)
	if !ok {
		return Result{}, ErrResponse
	}
	if n != 0 {
		if use {
			return Result{}, &BusinessError{Message: msg}
		}
		// The original check deliberately collapses every business rejection to 1.
		return Result{Code: 1, Msg: msg}, nil
	}
	return Result{Code: 0, Msg: msg}, nil
}

func (s *Service) CheckActivationcode(ctx context.Context, value string) (Result, error) {
	return s.activation(ctx, value, false)
}

func (s *Service) UseActivationcode(ctx context.Context, value string) error {
	_, e := s.activation(ctx, value, true)
	return e
}
