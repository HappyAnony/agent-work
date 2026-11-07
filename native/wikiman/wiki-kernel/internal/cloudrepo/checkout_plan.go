// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"path"
	"strings"
)

type CheckoutFile struct {
	ID, Path, SHA256 string
	Size, Updated    int64
	Chunks           []string
}
type CheckoutPlan struct {
	ID    string
	Files []CheckoutFile
	Bytes int64
}

func (s *Service) PrepareCheckout(ctx context.Context, id string) (CheckoutPlan, error) {
	if s.life.Err() != nil {
		return CheckoutPlan{}, ErrClosed
	}
	if !ValidSnapshotID(id) {
		return CheckoutPlan{}, ErrSnapshotID
	}
	if s.o.PrepareCheckout == nil {
		return CheckoutPlan{}, ErrDependency
	}
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-c.Done():
		return CheckoutPlan{}, c.Err()
	}
	if s.life.Err() != nil {
		return CheckoutPlan{}, ErrClosed
	}
	if e := c.Err(); e != nil {
		return CheckoutPlan{}, e
	}
	v, e := s.o.Load(c)
	if e != nil {
		return CheckoutPlan{}, e
	}
	v, e = v.Clone()
	if e != nil {
		return CheckoutPlan{}, e
	}
	if v.Repo == nil {
		return CheckoutPlan{}, ErrDependency
	}
	if len(v.Repo.Key) == 0 {
		return CheckoutPlan{}, ErrKey
	}
	out, e := s.o.PrepareCheckout(c, v, id)
	if e != nil || c.Err() != nil {
		return CheckoutPlan{}, errors.Join(e, c.Err())
	}
	if out.ID != id || out.Bytes < 0 || len(out.Files) == 0 {
		return CheckoutPlan{}, ErrOperation
	}
	paths, ids := map[string]bool{}, map[string]bool{}
	var total int64
	out.Files = append([]CheckoutFile{}, out.Files...)
	for i := range out.Files {
		f := &out.Files[i]
		p := f.Path
		hash, err := hex.DecodeString(f.SHA256)
		if !ValidSnapshotID(f.ID) || ids[f.ID] || p == "" || p == "." || p == ".." || path.IsAbs(p) || path.Clean(p) != p || strings.HasPrefix(p, "../") || strings.ContainsAny(p, "\\\x00") || (len(p) > 1 && p[1] == ':') || paths[p] || f.Size < 0 || f.Size > math.MaxInt64-total || err != nil || len(hash) != 32 {
			return CheckoutPlan{}, ErrOperation
		}
		paths[p] = true
		ids[f.ID] = true
		if f.Size > 0 && len(f.Chunks) == 0 {
			return CheckoutPlan{}, ErrOperation
		}
		total += f.Size
		f.Chunks = append([]string{}, f.Chunks...)
		for _, chunk := range f.Chunks {
			if !ValidSnapshotID(chunk) {
				return CheckoutPlan{}, ErrOperation
			}
		}
	}
	for p := range paths {
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			if paths[dir] {
				return CheckoutPlan{}, ErrOperation
			}
		}
	}
	if total != out.Bytes {
		return CheckoutPlan{}, ErrOperation
	}
	return out, nil
}
