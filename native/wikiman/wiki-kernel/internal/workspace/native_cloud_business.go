// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
)

// Network callbacks run outside source/read-resource locks, but remain registered
// tasks pinned to their creation identity. Drain cancels and joins actual IO.
func (l *Lease) runNativeCloudBusiness(ctx context.Context, fn func(context.Context, *Lease, *cloudaccount.Service) error) error {
	var service *cloudaccount.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		service = l.workspace.resources.cloudAccount
		if service == nil {
			return cloudaccount.ErrDependency
		}
		return nil
	}); e != nil {
		return e
	}
	task, e := l.StartTask(ctx, func(c context.Context, pin *Lease) error { return fn(c, pin, service) })
	if e != nil {
		return e
	}
	return task.Wait(ctx)
}
func (l *Lease) NativeCloudStartFreeTrial(ctx context.Context) error {
	return l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error { return s.StartFreeTrial(c) })
}
func (l *Lease) NativeCloudDeactivate(ctx context.Context) error {
	return l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error { return s.Deactivate(c) })
}
func (l *Lease) NativeCloudUploadToken(ctx context.Context) (string, error) {
	result := make(chan string, 1)
	e := l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error {
		token, e := s.UploadToken(c)
		if e == nil {
			result <- token
		}
		return e
	})
	if e != nil {
		return "", e
	}
	return <-result, nil
}

type frozenUploadAsset struct {
	data []byte
	size int64
}

// Explicit paths, not a new document/AV/embed extraction implementation. Freeze
// at most 128 page-owned assets/64 MiB before network IO, retaining only metadata
// for over-10-MiB files so the shared upload executor can apply original skips.
func (pin *Lease) freezeCloudUpload(ctx context.Context, paths []string) (map[string]frozenUploadAsset, error) {
	if len(paths) > 128 {
		return nil, ErrBudget
	}
	out := map[string]frozenUploadAsset{}
	var total int64
	e := pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error {
			for _, p := range paths {
				if e := ctx.Err(); e != nil {
					return e
				}
				if _, ok := out[p]; ok {
					continue
				}
				parts := strings.Split(p, "/")
				allowed := strings.HasPrefix(p, "assets/") || len(parts) > 2 && documentID.MatchString(parts[0]) && strings.Contains(p, "/assets/")
				if !allowed || !fs.ValidPath(p) || strings.Contains(p, "\\") {
					return ErrDocument
				}
				f, e := openNativeFile(pin.workspace, p)
				if errors.Is(e, fs.ErrNotExist) {
					continue
				}
				if e != nil {
					return e
				}
				stat, e := f.Stat()
				if e != nil {
					f.Close()
					return e
				}
				size := stat.Size()
				if size > 10<<20 {
					f.Close()
					out[p] = frozenUploadAsset{size: size}
					continue
				}
				total += size
				if total > 64<<20 {
					f.Close()
					return ErrBudget
				}
				raw, e := io.ReadAll(io.LimitReader(f, (10<<20)+1))
				closeErr := f.Close()
				if e = errors.Join(e, closeErr); e != nil {
					return e
				}
				if int64(len(raw)) != size || len(raw) > 10<<20 {
					return cloudaccount.ErrAssetChanged
				}
				out[p] = frozenUploadAsset{data: raw, size: size}
			}
			return ctx.Err()
		})
	})
	return out, e
}
func (l *Lease) NativeCloudUploadAssets(ctx context.Context, paths []string, biz string) (int, error) {
	paths = append([]string(nil), paths...)
	type result struct {
		n int
		e error
	}
	out := make(chan result, 1)
	e := l.runNativeCloudBusiness(ctx, func(c context.Context, pin *Lease, s *cloudaccount.Service) error {
		frozen, e := pin.freezeCloudUpload(c, paths)
		if e != nil {
			out <- result{e: e}
			return e
		}
		n, e := s.UploadAssets(c, paths, biz, cloudaccount.UploadPorts{
			Resolve: func(ctx context.Context, p string) (string, error) {
				if _, ok := frozen[p]; !ok {
					return "", nil
				}
				return p, ctx.Err()
			},
			Open: func(ctx context.Context, p string) (io.ReadCloser, int64, error) {
				v, ok := frozen[p]
				if !ok {
					return nil, 0, ErrDocument
				}
				return io.NopCloser(bytes.NewReader(v.data)), v.size, ctx.Err()
			},
		})
		out <- result{n, e}
		return e
	})
	// A canceled wait may finish before a callback has returned. Only read the
	// buffered result when available; the task still owns the lease until exit.
	select {
	case r := <-out:
		return r.n, r.e
	default:
		return 0, e
	}
}

func (l *Lease) NativeCloudUseActivationcode(ctx context.Context, value string) error {
	return l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error { return s.UseActivationcode(c, value) })
}
func (l *Lease) NativeCloudCheckActivationcode(ctx context.Context, value string) (cloudaccount.Result, error) {
	out := make(chan cloudaccount.Result, 1)
	e := l.runNativeCloudBusiness(ctx, func(c context.Context, _ *Lease, s *cloudaccount.Service) error {
		v, e := s.CheckActivationcode(c, value)
		if e == nil {
			out <- v
		}
		return e
	})
	if e != nil {
		return cloudaccount.Result{}, e
	}
	return <-out, nil
}
