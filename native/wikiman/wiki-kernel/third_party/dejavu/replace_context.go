// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type ReplaceResult struct{ Applied, Uncertain, CleanupPending bool }

// BindAdmission is set before sharing a legacy Repo. It rejects a repository
// captured before private key/owner replacement; callbacks must not reenter IO.
func (repo *Repo) BindAdmission(fn func() error) { repo.store.admission = fn }

// ReplaceRepositoryContext serializes with original Index/Sync/Checkout. A
// sibling quarantine keeps old cache until the private configuration commits.
// It is not a cross-file crash transaction: interrupted quarantines are retained
// for inspection, never automatically replayed or mistaken for source backups.
func ReplaceRepositoryContext(ctx context.Context, path string, commit func() error) (out ReplaceResult, err error) {
	if !filepath.IsAbs(path) || filepath.Dir(filepath.Clean(path)) == filepath.Clean(path) || commit == nil {
		return out, ErrSnapshot
	}
	path = filepath.Clean(path)
	if err = lock.LockContext(ctx); err != nil {
		return
	}
	defer lock.Unlock()
	if err = filepath.WalkDir(path, func(_ string, d fs.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrSnapshot
		}
		return ctx.Err()
	}); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	stage, e := os.MkdirTemp(filepath.Dir(filepath.Clean(path)), ".wiki-repo-replace-")
	if e != nil {
		return out, e
	}
	old, fresh := filepath.Join(stage, "old"), filepath.Join(stage, "new")
	hasOld := false
	published := false
	defer func() {
		if !out.Applied {
			if published {
				if e := os.Rename(path, fresh); e != nil {
					err = errors.Join(err, e)
					out.Uncertain = true
					out.CleanupPending = true
					return
				}
			}
			if hasOld {
				if e := os.Rename(old, path); e != nil {
					err = errors.Join(err, e)
					out.Uncertain = true
					out.CleanupPending = true
					return
				}
			}
		}
		// Cleanup is bounded by the operation context; failure keeps the quarantine
		// and the actual applied result. Never silently detach deletion workers.
		if e := removeRepositoryTree(ctx, stage); e != nil {
			err = errors.Join(err, e)
			out.CleanupPending = true
		}
	}()
	if err = os.Mkdir(fresh, 0700); err != nil {
		return
	}
	if _, e = os.Lstat(path); e == nil {
		if err = os.Rename(path, old); err != nil {
			return
		}
		hasOld = true
	} else if !os.IsNotExist(e) {
		return out, e
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if err = os.Rename(fresh, path); err != nil {
		return
	}
	published = true
	if err = ctx.Err(); err != nil {
		return
	}
	if err = commit(); err != nil {
		return
	}
	out.Applied = true
	return out, nil
}

func removeRepositoryTree(ctx context.Context, path string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if info.IsDir() {
		rows, e := os.ReadDir(path)
		if e != nil {
			return e
		}
		for _, row := range rows {
			if e = removeRepositoryTree(ctx, filepath.Join(path, row.Name())); e != nil {
				return e
			}
		}
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return os.Remove(path)
}
