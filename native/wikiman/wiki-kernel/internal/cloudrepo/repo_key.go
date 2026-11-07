// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/siyuan-note/encryption"
	"strings"
	"time"
)

var ErrRepoKeyInput = errors.New("repository_key_invalid")

type RepoKeyRequest struct{ Kind, Input string }

// Key is private result material, never a log, evidence field or source receipt.
type RepoKeyResult struct {
	Key                                                    []byte
	Normalized                                             string
	Applied, Uncertain, CleanupPending, InitialIndexFailed bool
}

func (s *Service) RepoKey(ctx context.Context, r RepoKeyRequest) (RepoKeyResult, error) {
	if s.life.Err() != nil {
		return RepoKeyResult{}, ErrClosed
	}
	if r.Kind != "random" && r.Kind != "passphrase" && r.Kind != "import" && r.Kind != "reset" {
		return RepoKeyResult{}, ErrOperation
	}
	if s.o.RepoKey == nil {
		return RepoKeyResult{}, ErrDependency
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return RepoKeyResult{}, c.Err()
	}
	if s.life.Err() != nil {
		return RepoKeyResult{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return RepoKeyResult{}, e
	}
	v, e := s.o.Load(c)
	if e != nil {
		return RepoKeyResult{}, e
	}
	v, e = v.Clone()
	if e != nil {
		return RepoKeyResult{}, e
	}
	if v.Repo == nil || v.Sync == nil {
		return RepoKeyResult{}, ErrDependency
	}
	key, e := deriveRepoKey(c, &r)
	if e != nil {
		return RepoKeyResult{}, e
	}
	out, e := s.o.RepoKey(c, v, r, key)
	if out.Applied && r.Kind != "reset" {
		out.Key = append([]byte{}, key...)
		if r.Kind == "import" {
			out.Normalized = r.Input
		}
	}
	if out.Applied {
		s.repoAfterSync = false
		s.lastRepoPurge = time.Time{}
	}
	if c.Err() != nil {
		return out, errors.Join(e, c.Err())
	}
	return out, e
}

func deriveRepoKey(ctx context.Context, r *RepoKeyRequest) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if r.Kind == "reset" {
		return nil, nil
	}
	r.Input = strings.TrimSpace(gulu.Str.RemoveInvisible(r.Input))
	var key []byte
	var e error
	switch r.Kind {
	case "import":
		if r.Input == "" {
			return nil, ErrMemo
		}
		key, e = base64.StdEncoding.DecodeString(r.Input)
		if e != nil || len(key) != 32 {
			return nil, ErrRepoKeyInput
		}
	case "passphrase":
		if r.Input == "" {
			return nil, ErrMemo
		}
		if raw, err := base64.StdEncoding.DecodeString(r.Input); err == nil && len(raw) == 32 {
			key = raw
		} else {
			salt := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Input)))[:16]
			key, e = encryption.KDF(r.Input, salt)
		}
	case "random":
		password, salt := make([]byte, 16), make([]byte, 16)
		if _, e = rand.Read(password); e != nil {
			return nil, ErrRepoKeyInput
		}
		if _, e = rand.Read(salt); e != nil {
			return nil, ErrRepoKeyInput
		}
		key, e = encryption.KDF(string(password), string(salt))
	}
	if e != nil || len(key) != 32 {
		return nil, ErrRepoKeyInput
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return key, nil
}
