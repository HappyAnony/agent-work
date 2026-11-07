// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original util/etag.go Qiniu-compatible hash (not a SHA-256 replacement).
package indexdb

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"io"
)

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func AssetEtag(ctx context.Context, r io.Reader, size int64) (string, error) {
	const width int64 = 1 << 22
	input := contextReader{ctx, r}
	count := (size + width - 1) / width
	var digest []byte
	if count <= 1 {
		h := sha1.New()
		if _, err := io.Copy(h, input); err != nil {
			return "", err
		}
		digest = append([]byte{0x16}, h.Sum(nil)...)
	} else {
		combined := sha1.New()
		for i := int64(0); i < count; i++ {
			h := sha1.New()
			if _, err := io.Copy(h, io.LimitReader(input, width)); err != nil {
				return "", err
			}
			combined.Write(h.Sum(nil))
		}
		digest = append([]byte{0x96}, combined.Sum(nil)...)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(digest), nil
}
