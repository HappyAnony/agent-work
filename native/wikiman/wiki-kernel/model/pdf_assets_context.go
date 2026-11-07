// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/pdfdevice"
)

type pdfAssetURL struct{ path, uri string }
type pdfAssetFile struct{ path, name string }
type pdfAssetServices struct {
	guard            func() error
	classify         func(string) bool
	normalize        func(string) (pdfAssetURL, error)
	resolve          func(string) (pdfAssetFile, bool, error)
	embed            func(*pdfmodel.Context, pdfAssetFile) (*types.IndirectRef, error)
	now              time.Time
	decodeOther      bool
	skipEmbedFailure bool
}

func readPDFAssetLinks(pdf *pdfmodel.Context, classify func(string) bool, guard func() error) ([]pdfmodel.LinkAnnotation, []pdfmodel.LinkAnnotation, error) {
	return pdfdevice.ReadAssetLinks(pdf, classify, guard)
}
func normalizePDFAssetURL(value, origin string) (pdfAssetURL, error) {
	v, e := pdfdevice.NormalizeAssetURL(value, origin)
	if errors.Is(e, pdfdevice.ErrAssetPath) {
		e = indexrepair.ErrSourcePath
	}
	return pdfAssetURL{v.Path, v.URI}, e
}
func applyPDFAssetLinks(pdf *pdfmodel.Context, destinations []string, removeAssets bool, s pdfAssetServices) error {
	if s.guard == nil || s.classify == nil || s.normalize == nil || s.resolve == nil || s.embed == nil {
		return errors.New("pdf_assets_dependencies_unbound")
	}
	return pdfdevice.ApplyAssetLinks(pdf, destinations, removeAssets, pdfdevice.AssetServices{Guard: s.guard, Classify: s.classify, Normalize: func(uri string) (pdfdevice.AssetURL, error) {
		v, e := s.normalize(uri)
		return pdfdevice.AssetURL{Path: v.path, URI: v.uri}, e
	}, Resolve: func(relative string) (pdfdevice.AssetFile, bool, error) {
		v, ok, e := s.resolve(relative)
		return pdfdevice.AssetFile{Path: v.path, Name: v.name}, ok, e
	}, Embed: func(pdf *pdfmodel.Context, f pdfdevice.AssetFile) (*types.IndirectRef, error) {
		return s.embed(pdf, pdfAssetFile{f.Path, f.Name})
	}, Now: s.now, DecodeOther: s.decodeOther, SkipEmbedFailure: s.skipEmbedFailure})
}
func newPDFAssetAnnotation(pdf *pdfmodel.Context, link pdfmodel.LinkAnnotation, file types.IndirectRef, now time.Time) (*types.IndirectRef, error) {
	return pdfdevice.NewAssetAnnotation(pdf, link, file, now)
}

const pdfAssetFileLimit int64 = 64 << 20
const pdfAssetTotalLimit int64 = 256 << 20

type pdfAssetReader struct {
	reader    io.Reader
	guard     func() error
	remaining int64
	total     *int64
}

func (r *pdfAssetReader) Read(p []byte) (int, error) {
	if e := r.guard(); e != nil {
		return 0, e
	}
	if len(p) > 64*1024 {
		p = p[:64*1024]
	}
	n, e := r.reader.Read(p)
	r.remaining -= int64(n)
	*r.total -= int64(n)
	if r.remaining < 0 || *r.total < 0 {
		return 0, errors.New("pdf_asset_read_limit")
	}
	if err := r.guard(); err != nil {
		return 0, err
	}
	return n, e
}
func (d *pdfPostprocessDocument) withPDFAssetSource(use func(*os.Root, indexrepair.Source, func(string) (pdfAssetFile, bool, error), []string, func() error) error) (err error) {
	if err = d.guard(); err != nil {
		return err
	}
	data := d.m.read.binding.scope.commit.r.data
	root, err := openArchivePublicationSource(data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	identity, err := root.Stat(".")
	if err != nil {
		return err
	}
	guard := func() error {
		if e := d.guard(); e != nil {
			return e
		}
		info, e := os.Lstat(data)
		return sameArchiveDirectory(info, e, identity)
	}
	source := indexrepair.Source{Data: root}
	index, err := d.m.read.assetIndex(source)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	var directories []string
	for _, raw := range d.assets {
		if !strings.HasPrefix(raw, "assets/") {
			continue
		}
		if i := strings.IndexByte(raw, '#'); i >= 0 {
			raw = raw[:i]
		}
		relative, e := normalizeExportAsset(raw)
		if e != nil {
			return e
		}
		allowed[relative] = true
		from, ok := index[relative]
		if !ok {
			from, ok = index[strings.TrimSuffix(relative, "/")+"/"]
		}
		if ok {
			info, e := root.Stat(from)
			if e != nil {
				return e
			}
			if info.IsDir() {
				directories = append(directories, strings.TrimSuffix(relative, "/")+"/")
			}
		}
	}
	resolve := func(relative string) (pdfAssetFile, bool, error) {
		if e := guard(); e != nil {
			return pdfAssetFile{}, false, e
		}
		// Both the document allowlist and link path are decoded once by their
		// respective callers. Never decode a literal percent sign a second time.
		if !allowed[relative] {
			permitted := false
			for _, prefix := range directories {
				permitted = permitted || strings.HasPrefix(relative, prefix) || relative == strings.TrimSuffix(prefix, "/")
			}
			if !permitted {
				return pdfAssetFile{}, false, indexrepair.ErrSourcePath
			}
		}
		from, exists := index[relative]
		if !exists {
			from, exists = index[strings.TrimSuffix(relative, "/")+"/"]
		}
		if !exists {
			return pdfAssetFile{}, false, nil
		}
		if e := source.Check(from, false); e != nil {
			return pdfAssetFile{}, false, e
		}
		return pdfAssetFile{from, path.Base(strings.TrimSuffix(relative, "/"))}, true, nil
	}
	destinations := make([]string, 0, len(allowed))
	for name := range allowed {
		destinations = append(destinations, name)
	}
	sort.Strings(destinations)
	return use(root, source, resolve, destinations, guard)
}
func (d *pdfPostprocessDocument) assetLinks(pdf *pdfmodel.Context, removeAssets bool) error {
	return d.withPDFAssetSource(func(root *os.Root, source indexrepair.Source, resolve func(string) (pdfAssetFile, bool, error), destinations []string, guard func() error) error {
		origin := strings.TrimSuffix(d.m.htmlLocalAnchorPrefix, "/#")
		total := pdfAssetTotalLimit
		embed := func(pdf *pdfmodel.Context, file pdfAssetFile) (ref *types.IndirectRef, err error) {
			if err = guard(); err != nil {
				return nil, err
			}
			if err = source.Check(file.path, false); err != nil {
				return nil, err
			}
			info, err := root.Stat(file.path)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, errors.New("pdf_asset_not_regular")
			}
			f, err := root.Open(file.path)
			if err != nil {
				return nil, err
			}
			defer func() {
				err = errors.Join(err, f.Close(), guard())
				if err != nil {
					ref = nil
				}
			}()
			info, err = f.Stat()
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, errors.New("pdf_asset_not_regular")
			}
			if info.Size() > pdfAssetFileLimit || info.Size() > total {
				return nil, errors.New("pdf_asset_read_limit")
			}
			return pdf.NewEmbeddedStreamDict(&pdfAssetReader{reader: f, guard: guard, remaining: pdfAssetFileLimit, total: &total}, info.ModTime())
		}
		return applyPDFAssetLinks(pdf, destinations, removeAssets, pdfAssetServices{guard: guard, classify: func(uri string) bool {
			return strings.HasPrefix(uri, origin+"/assets/") || strings.HasPrefix(uri, origin+"/export/temp/assets/")
		}, normalize: func(uri string) (pdfAssetURL, error) { return normalizePDFAssetURL(uri, origin) }, resolve: resolve, embed: embed, now: time.Now()})
	})
}
