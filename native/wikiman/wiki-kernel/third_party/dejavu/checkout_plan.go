// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/dejavu/util"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type CheckoutFile struct {
	ID, Path, SHA256 string
	Size, Updated    int64
	Chunks           []string
}

// CheckoutPlan describes verified repository objects only. It carries no source
// diff, authority to overwrite files, backup guarantee or transaction receipt.
type CheckoutPlan struct {
	ID    string
	Files []CheckoutFile
	Bytes int64
}

func checkoutPath(p string) (string, error) {
	if strings.ContainsAny(p, "\\\x00") || strings.HasPrefix(p, "//") {
		return "", ErrSnapshot
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." || p == ".." || strings.HasPrefix(p, "../") || path.Clean(p) != p || (len(p) > 1 && p[1] == ':') {
		return "", ErrSnapshot
	}
	return p, nil
}

// PrepareCheckoutContext reads/decrypts original index/file/chunk codecs without
// reading or changing DataPath, publishing refs or making network requests.
func (repo *Repo) PrepareCheckoutContext(ctx context.Context, id string) (CheckoutPlan, error) {
	if !validScopedObjectID(id) {
		return CheckoutPlan{}, ErrSnapshot
	}
	if e := lock.LockContext(ctx); e != nil {
		return CheckoutPlan{}, e
	}
	defer lock.Unlock()
	if e := checkoutNoSymlinks(ctx, repo.Path); e != nil {
		return CheckoutPlan{}, e
	}
	previous, scoped := repo.store.operation, repo.store.scoped
	repo.store.operation = ctx
	repo.store.scoped = true
	defer func() { repo.store.operation = previous; repo.store.scoped = scoped }()
	idx, e := repo.store.GetIndex(id)
	if e != nil {
		return CheckoutPlan{}, e
	}
	if idx.ID != id {
		return CheckoutPlan{}, ErrSnapshot
	}
	return repo.checkoutPlan(ctx, idx)
}

func checkoutNoSymlinks(ctx context.Context, root string) error {
	return filepath.WalkDir(filepath.Clean(root), func(_ string, d fs.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return ctx.Err()
		}
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrSnapshot
		}
		return ctx.Err()
	})
}

func (repo *Repo) validateCheckoutTarget(idx *entity.Index) error {
	ctx := repo.store.operation
	if ctx == nil {
		ctx = context.Background()
	}
	_, e := repo.checkoutPlan(ctx, idx)
	return e
}

func (repo *Repo) validateCheckoutSource() error {
	ctx := repo.store.operation
	if ctx == nil {
		ctx = context.Background()
	}
	return checkoutNoSymlinks(ctx, repo.DataPath)
}

func (repo *Repo) checkoutPlan(ctx context.Context, idx *entity.Index) (out CheckoutPlan, err error) {
	if idx == nil || !validScopedObjectID(idx.ID) {
		return out, ErrSnapshot
	}
	if len(idx.Files) == 0 {
		return out, ErrEmptyIndex
	}
	out.ID = idx.ID
	out.Files = []CheckoutFile{}
	paths := map[string]bool{}
	ids := map[string]bool{}
	for _, id := range idx.Files {
		if e := ctx.Err(); e != nil {
			return CheckoutPlan{}, e
		}
		if !validScopedObjectID(id) || ids[id] {
			return CheckoutPlan{}, ErrSnapshot
		}
		ids[id] = true
		f, e := repo.store.GetFile(id)
		if e != nil {
			return CheckoutPlan{}, e
		}
		if !validFile(f) || f.ID != id {
			return CheckoutPlan{}, ErrSnapshot
		}
		p, e := checkoutPath(f.Path)
		if e != nil || paths[p] {
			return CheckoutPlan{}, ErrSnapshot
		}
		paths[p] = true
		if f.Size > math.MaxInt64-out.Bytes {
			return CheckoutPlan{}, ErrSnapshot
		}
		h := sha256.New()
		var size int64
		for _, chunk := range f.Chunks {
			if e = ctx.Err(); e != nil {
				return CheckoutPlan{}, e
			}
			c, e := repo.store.GetChunk(chunk)
			if e != nil {
				return CheckoutPlan{}, e
			}
			if c == nil || util.Hash(c.Data) != chunk || int64(len(c.Data)) > f.Size-size {
				return CheckoutPlan{}, ErrSnapshot
			}
			if _, e = h.Write(c.Data); e != nil {
				return CheckoutPlan{}, e
			}
			size += int64(len(c.Data))
		}
		if size != f.Size {
			return CheckoutPlan{}, io.ErrUnexpectedEOF
		}
		out.Bytes += size
		out.Files = append(out.Files, CheckoutFile{ID: f.ID, Path: p, SHA256: hex.EncodeToString(h.Sum(nil)), Size: f.Size, Updated: f.Updated, Chunks: append([]string{}, f.Chunks...)})
	}
	for p := range paths {
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			if paths[dir] {
				return CheckoutPlan{}, ErrSnapshot
			}
		}
	}
	if e := ctx.Err(); e != nil {
		return CheckoutPlan{}, e
	}
	return out, nil
}
