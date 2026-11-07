// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"crypto/sha256"
	"io/fs"
	"os"
	"path"
	"strings"
	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/indexdb"
)

type NativeAssetExtraction struct {
	Content    string
	Skipped    bool
	SourceHash [32]byte
}

// Source bytes and temporary state are owned by this admission. Closing the
// workspace cancels, kills and waits for the converter before releasing files.
func (l *Lease) ExtractNativeAsset(ctx context.Context, p string) (ret NativeAssetExtraction, err error) {
	if !strings.HasPrefix(p, "assets/") || !fs.ValidPath(p) || strings.Contains(p, "\\") {
		return ret, ErrDocument
	}
	kind := assetextract.Kind(path.Ext(p))
	if kind == "" {
		return ret, ErrDocument
	}
	var raw []byte
	var run assetextract.Binding
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		w := l.workspace
		var e error
		raw, e = readNativeFile(ctx, w, p, assetextract.MaxInput)
		if e != nil {
			return e
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		run, e = assetextract.Bind(exe, w.paths.TempDir)
		return e
	})
	if err != nil {
		return NativeAssetExtraction{}, err
	}
	result := make(chan assetextract.Result, 1)
	task, e := l.StartTask(ctx, func(c context.Context, _ *Lease) error {
		out, e := run(c, assetextract.Request{Kind: kind, Data: raw, PDFMaxBytes: assetextract.MaxInput, PDFMaxPages: assetextract.PageLimit})
		result <- out
		return e
	})
	if e != nil {
		return NativeAssetExtraction{}, e
	}
	// Caller cancellation reaches the child through StartTask. Still join its
	// real exit and cleanup before reporting completion to this caller.
	if e = task.Wait(context.Background()); e != nil {
		return NativeAssetExtraction{}, e
	}
	out := <-result
	return NativeAssetExtraction{Content: out.Content, Skipped: out.Skipped, SourceHash: sha256.Sum256(raw)}, nil
}
