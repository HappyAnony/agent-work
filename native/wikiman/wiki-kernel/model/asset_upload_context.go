// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func ownedAssetHashLookup(c context.Context, r *indexRepairContext, source indexrepair.Source) func(context.Context, string) (string, error) {
	verify := func(ctx context.Context, p, hash string) (bool, error) {
		if e := r.guard(ctx); e != nil {
			return false, e
		}
		relative, e := assetflow.Resolve(ctx, source, nil, p)
		if errors.Is(e, assetflow.ErrNotFound) {
			boxes, err := r.owner.business.Notebooks(ctx)
			if err != nil {
				return false, err
			}
			ids := []string{}
			for _, box := range boxes {
				ids = append(ids, box.ID)
			}
			relative, e = assetflow.Resolve(ctx, source, ids, p)
		}
		if errors.Is(e, assetflow.ErrNotFound) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		info, e := source.Data.Lstat(relative)
		if e != nil {
			return false, e
		}
		if info.IsDir() {
			return false, nil
		}
		raw, e := source.ReadLimited(ctx, relative, assetflow.UploadLimit)
		if e != nil {
			return false, e
		}
		got, e := indexdb.AssetEtag(ctx, bytes.NewReader(raw), int64(len(raw)))
		return got == hash, e
	}
	return func(ctx context.Context, hash string) (string, error) {
		if e := r.guard(ctx); e != nil {
			return "", e
		}
		if hit, ok := cache.AssetHashCandidate(hash); ok {
			valid, e := verify(ctx, hit.Path, hash)
			if e != nil {
				return "", e
			}
			if valid {
				return hit.Path, nil
			}
		}
		var p string
		e := r.main.DB.QueryRowContext(ctx, "SELECT path FROM assets WHERE hash=? LIMIT 1", hash).Scan(&p)
		if errors.Is(e, stdsql.ErrNoRows) {
			return "", nil
		}
		if e != nil {
			return "", e
		}
		valid, e := verify(ctx, p, hash)
		if e != nil {
			return "", e
		}
		if valid {
			return p, nil
		}
		return "", nil
	}
}

func UploadAssetsRequest(ctx context.Context, request *http.Request, maxMemory int64) (out assetflow.UploadResult, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return out, e
	}
	if r.owner != base.owner || r.data != base.data {
		return out, conf.ErrBusinessUnbound
	}
	if request == nil || request.Body == nil {
		return out, errors.New("asset_upload_request_unbound")
	}
	request.Body = http.MaxBytesReader(nil, request.Body, assetflow.UploadLimit+(4<<20))
	var form *multipart.Form
	err = base.runFinalized(ctx, func(c, life context.Context) (result error) {
		if e := r.guard(c); e != nil {
			return e
		}
		finishBody := requestBodyCancellation(c, request.Body)
		defer func() { result = errors.Join(result, finishBody()) }()
		if e := request.ParseMultipartForm(maxMemory); e != nil {
			form = request.MultipartForm
			return errors.Join(e, c.Err())
		}
		form = request.MultipartForm
		if form == nil {
			return errors.New("asset_upload_form_missing")
		}
		ioContext, closeIO, e := r.openRepairIOChecked(c, true)
		if e != nil {
			return e
		}
		defer func() { result = joinRepairCloseError(result, closeIO()) }()
		source := ioContext.Source
		target := "assets"
		relative := "assets"
		value := func(key string) string {
			if values := form.Value[key]; len(values) > 0 {
				return values[0]
			}
			return ""
		}
		if id := value("id"); id != "" {
			loc, e := r.referenceLookup(c, id)
			if e != nil {
				return e
			}
			if loc != nil {
				target, e = assetflow.TargetDirectory(c, source, loc.BoxID, loc.Path)
				if e != nil {
					return e
				}
			}
		}
		if selected := value("assetsDirPath"); selected != "" {
			if !fs.ValidPath(selected) || selected == "." || strings.Contains(selected, "\\") {
				return indexrepair.ErrSourcePath
			}
			target = selected
			relative = selected
		}
		if e = source.EnsureDirectory(c, target); e != nil {
			return e
		}
		files := []assetflow.UploadFile{}
		for _, header := range form.File["file[]"] {
			header := header
			files = append(files, assetflow.UploadFile{Name: header.Filename, Size: header.Size, Open: func() (io.ReadCloser, error) { return header.Open() }})
		}
		byHash := ownedAssetHashLookup(c, r, source)
		remembered := map[string]string{}
		out, e = assetflow.Upload(c, files, relative, value("skipIfDuplicated") == "true", runtime.GOOS == "darwin", assetflow.UploadPorts{
			NewID: ast.NewNodeID,
			ByHash: func(c context.Context, hash string) (string, error) {
				if e := r.guard(c); e != nil {
					return "", e
				}
				if p, ok := remembered[hash]; ok {
					return p, nil
				}
				return byHash(c, hash)
			},
			Match: func(c context.Context, pattern string) (string, error) {
				entries, e := source.ReadDir(c, target)
				if e != nil {
					return "", e
				}
				names := []string{}
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				sort.Strings(names)
				for _, name := range names {
					matched, e := path.Match(pattern, name)
					if e != nil {
						return "", e
					}
					if matched {
						if e = source.Check(path.Join(target, name), false); e != nil {
							return "", e
						}
						return name, nil
					}
				}
				return "", nil
			},
			Write: func(c context.Context, name string, raw []byte) error {
				return source.Rewrite(c, path.Join(target, name), nil, raw)
			},
			Directory: func(c context.Context, name string, entries []assetflow.UploadEntry) error {
				root := path.Join(target, name)
				if e := source.EnsureDirectory(c, root); e != nil {
					return e
				}
				for _, entry := range entries {
					dest := path.Join(root, entry.Name)
					if entry.Directory {
						if e := source.EnsureDirectory(c, dest); e != nil {
							return e
						}
					} else if e := source.Rewrite(c, dest, nil, entry.Data); e != nil {
						return e
					}
				}
				return c.Err()
			},
			Remember: func(c context.Context, hash, p string) error {
				if e := r.guard(c); e != nil {
					return e
				}
				remembered[hash] = p
				cache.SetAssetHash(hash, p)
				return nil
			},
		})
		// Preserve already written file prefixes even if a later selection failed.
		syncErr := base.owner.WithSettingSnapshot(context.WithoutCancel(c), func(v conf.AppData) error {
			if v.Sync == nil {
				return conf.ErrBusinessUnbound
			}
			base.owner.syncRuntime.same.Store(0)
			base.owner.planSyncAfter(time.Duration(v.Sync.Interval) * time.Second)
			return nil
		})
		return errors.Join(e, syncErr)
	}, func(error) error {
		if form != nil {
			return form.RemoveAll()
		}
		return nil
	})
	return
}
