// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func NormalizeSyncConcurrent(n, provider int) int {
	def, max := 8, 16
	switch provider {
	case ProviderWebDAV:
		def = 1
	case ProviderLocal:
		def, max = 16, 1024
	case ProviderSiYuan, ProviderS3:
	default:
		return n
	}
	if n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
func NormalizeSyncTimeout(n int) int {
	if n < 1 {
		return 60
	}
	if n < 7 {
		return 7
	}
	if n > 300 {
		return 300
	}
	return n
}
func NormalizeSyncEndpoint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "http://" + s
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}
func NormalizeSyncLocal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = filepath.ToSlash(filepath.Clean(s))
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}
func ValidSyncBucket(s string) bool {
	if !utf8.ValidString(s) || len(s) < 1 || len(s) > 63 || strings.ContainsAny(s, "~`!@#$%^&*()+=[]{}\\|;:'\"<,>.?/ ") {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// Called only through an explicit device service; no default workspace selector.
func ValidateSyncLocal(ctx context.Context, endpoint, workspace string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if endpoint == "" || workspace == "" {
		return ErrSetting
	}
	a, err := filepath.Abs(endpoint)
	if err != nil {
		return err
	}
	a, err = filepath.EvalSymlinks(a)
	if err != nil {
		return err
	}
	b, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	b, err = filepath.EvalSymlinks(b)
	if err != nil {
		return err
	}
	if _, err = os.Stat(a); err != nil {
		return err
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, e := filepath.Rel(pair[0], pair[1])
		if e == nil && (rel == "." || filepath.IsLocal(rel)) {
			return errors.New("sync_endpoint_overlaps_workspace")
		}
	}
	return ctx.Err()
}

func (d *AppData) prepareSyncSetting(ctx context.Context, kind string, raw json.RawMessage, services SettingServices) (target any, effects []SettingEffect, err error) {
	if d.Sync == nil {
		return nil, nil, ErrBusinessUnbound
	}
	v := d.Sync
	switch kind {
	case "syncEnabled":
		err = json.Unmarshal(raw, &v.Enabled)
		target = v.Enabled
	case "syncConflict":
		err = json.Unmarshal(raw, &v.GenerateConflictDoc)
		target = v.GenerateConflictDoc
	case "syncMode":
		err = json.Unmarshal(raw, &v.Mode)
		target = v.Mode
	case "syncProvider":
		err = json.Unmarshal(raw, &v.Provider)
		target = v.Provider
	case "syncInterval":
		err = json.Unmarshal(raw, &v.Interval)
		if v.Interval < 30 {
			v.Interval = 30
		}
		if v.Interval > 43200 {
			v.Interval = 43200
		}
		target = v.Interval
		effects = []SettingEffect{{Kind: "syncInterval", Int: v.Interval}}
	case "syncPerception":
		err = json.Unmarshal(raw, &v.Perception)
		if services.DisableSyncPerception {
			v.Perception = false
		}
		target = v.Perception
		effects = []SettingEffect{{Kind: "syncPerception", Bool: v.Perception}}
	case "syncS3":
		x := &S3{}
		err = json.Unmarshal(raw, x)
		if err != nil {
			return nil, nil, err
		}
		x.Endpoint = NormalizeSyncEndpoint(x.Endpoint)
		x.AccessKey = strings.TrimSpace(x.AccessKey)
		x.SecretKey = strings.TrimSpace(x.SecretKey)
		x.Bucket = strings.TrimSpace(x.Bucket)
		x.Region = strings.TrimSpace(x.Region)
		x.Timeout = NormalizeSyncTimeout(x.Timeout)
		x.ConcurrentReqs = NormalizeSyncConcurrent(x.ConcurrentReqs, ProviderS3)
		if !ValidSyncBucket(x.Bucket) {
			return nil, nil, ErrSetting
		}
		v.S3 = x
		target = x
	case "syncWebDAV":
		x := &WebDAV{}
		err = json.Unmarshal(raw, x)
		if err != nil {
			return nil, nil, err
		}
		x.Endpoint = NormalizeSyncEndpoint(x.Endpoint)
		if strings.Contains(strings.ToLower(x.Endpoint), "dav.jianguoyun.com") {
			return nil, nil, errors.New("unsupported_webdav_provider")
		}
		x.Username = strings.TrimSpace(x.Username)
		x.Password = strings.TrimSpace(x.Password)
		x.Timeout = NormalizeSyncTimeout(x.Timeout)
		x.ConcurrentReqs = NormalizeSyncConcurrent(x.ConcurrentReqs, ProviderWebDAV)
		v.WebDAV = x
		target = x
	case "syncLocal":
		x := &Local{}
		err = json.Unmarshal(raw, x)
		if err != nil {
			return nil, nil, err
		}
		x.Endpoint = NormalizeSyncLocal(x.Endpoint)
		if services.ValidateSyncLocal == nil {
			return nil, nil, ErrBusinessUnbound
		}
		if err = services.ValidateSyncLocal(ctx, x.Endpoint); err != nil {
			return nil, nil, err
		}
		x.Timeout = NormalizeSyncTimeout(x.Timeout)
		x.ConcurrentReqs = NormalizeSyncConcurrent(x.ConcurrentReqs, ProviderLocal)
		v.Local = x
		target = x
	default:
		return nil, nil, ErrSetting
	}
	return target, effects, err
}
