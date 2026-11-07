// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package pdfdevice

import (
	"bytes"
	"encoding/json"
	"errors"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strings"
	"time"
)

const MaxAsset = 64 << 20
const MaxAssetsTotal = 256 << 20
const maxEntries = 4096
const maxInspection = 4 << 20

type Outline struct{ Headings []Heading }
type Asset struct {
	Path, Name        string
	Exists, Directory bool
	Loaded            bool
	Data              []byte
	ModTime           time.Time
}
type Assets struct {
	Origin       string
	Destinations []string
	Entries      []Asset
	Embed        bool
	Now          time.Time
}
type Inspection struct{ URIs []string }

func validOrigin(origin string) bool {
	u, e := url.Parse(origin)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && len(origin) <= 4096
}
func IsAssetURL(uri, origin string) bool {
	return strings.HasPrefix(uri, origin+"/assets/") || strings.HasPrefix(uri, origin+"/export/temp/assets/")
}
func validAssetPath(p string) bool {
	return len(p) <= 4096 && strings.HasPrefix(p, "assets/") && fs.ValidPath(strings.TrimSuffix(p, "/")) && !strings.Contains(p, "\\")
}
func validatePipeline(r Request) error {
	if r.InspectOrigin != "" {
		if !validOrigin(r.InspectOrigin) || r.Outline != nil || r.Assets != nil || r.Watermark != nil || len(r.Fonts) != 0 {
			return errors.New("pdf_device_inspection_invalid")
		}
	}
	if o := r.Outline; o != nil {
		if len(o.Headings) > 8192 {
			return errors.New("pdf_device_outline_limit")
		}
		seen := map[string]bool{}
		total := 0
		for _, h := range o.Headings {
			if h.ID == "" || len(h.ID) > 256 || h.Level < 1 || h.Level > 6 || seen[h.ID] || len(h.Title) > 16<<10 {
				return errors.New("pdf_device_outline_invalid")
			}
			seen[h.ID] = true
			total += len(h.Title)
		}
		if total > 8<<20 {
			return errors.New("pdf_device_outline_limit")
		}
	}
	if a := r.Assets; a != nil {
		if (a.Now.Year() < 1000 || a.Now.Year() > 9999) || !validOrigin(a.Origin) || len(a.Entries) > maxEntries || len(a.Destinations) > maxEntries {
			return errors.New("pdf_device_assets_invalid")
		}
		seen := map[string]bool{}
		total := 0
		for _, v := range a.Entries {
			if !validAssetPath(v.Path) || seen[v.Path] || v.Name != path.Base(strings.TrimSuffix(v.Path, "/")) || len(v.Data) > MaxAsset || (!v.Exists || v.Directory || !a.Embed) && (len(v.Data) != 0 || v.Loaded) || len(v.Data) != 0 && !v.Loaded {
				return errors.New("pdf_device_asset_invalid")
			}
			seen[v.Path] = true
			total += len(v.Data)
		}
		for _, v := range a.Entries {
			if v.Loaded && (v.ModTime.Year() < 1000 || v.ModTime.Year() > 9999) {
				return errors.New("pdf_device_asset_date_invalid")
			}
		}
		if total > MaxAssetsTotal {
			return errors.New("pdf_device_assets_limit")
		}
		for _, v := range a.Destinations {
			if !seen[v] {
				return errors.New("pdf_device_asset_unbound")
			}
		}
	}
	return nil
}
func DecodeInspection(raw []byte) (Inspection, error) {
	var result Inspection
	if len(raw) > maxInspection {
		return result, errors.New("pdf_device_inspection_limit")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&result); e != nil {
		return Inspection{}, e
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Inspection{}, errors.New("pdf_device_inspection_invalid")
	}
	if len(result.URIs) > maxEntries {
		return Inspection{}, errors.New("pdf_device_inspection_limit")
	}
	for _, uri := range result.URIs {
		if len(uri) > 4096 {
			return Inspection{}, errors.New("pdf_device_inspection_limit")
		}
	}
	return result, nil
}
func inspectPDF(pdf *pdfmodel.Context, origin string) ([]byte, error) {
	links, _, e := ReadAssetLinks(pdf, func(uri string) bool { return IsAssetURL(uri, origin) }, func() error { return nil })
	if e != nil {
		return nil, e
	}
	result := Inspection{URIs: []string{}}
	for _, link := range links {
		result.URIs = append(result.URIs, link.URI)
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return nil, e
	}
	if _, e = DecodeInspection(raw); e != nil {
		return nil, e
	}
	return raw, nil
}
func applyPipeline(pdf *pdfmodel.Context, r Request) error {
	guard := func() error { return nil }
	if o := r.Outline; o != nil {
		byID := map[string]Heading{}
		for _, h := range o.Headings {
			byID[h.ID] = h
		}
		if e := ApplyBookmarks(pdf, o.Headings, func(id string) (string, bool, error) { h, ok := byID[id]; return h.Title, ok && h.Found, nil }, guard); e != nil {
			return e
		}
	}
	if a := r.Assets; a != nil {
		entries := map[string]Asset{}
		for _, v := range a.Entries {
			entries[v.Path] = v
		}
		return ApplyAssetLinks(pdf, a.Destinations, a.Embed, AssetServices{Guard: guard, Classify: func(uri string) bool { return IsAssetURL(uri, a.Origin) }, Normalize: func(uri string) (AssetURL, error) { return NormalizeAssetURL(uri, a.Origin) }, Resolve: func(name string) (AssetFile, bool, error) {
			v, ok := entries[name]
			if !ok {
				return AssetFile{}, false, errors.New("pdf_device_asset_unbound")
			}
			return AssetFile{Path: v.Path, Name: v.Name}, v.Exists, nil
		}, Embed: func(pdf *pdfmodel.Context, file AssetFile) (*types.IndirectRef, error) {
			v := entries[file.Path]
			if v.Directory || !v.Loaded {
				return nil, errors.New("pdf_asset_not_regular")
			}
			return pdf.NewEmbeddedStreamDict(bytes.NewReader(v.Data), v.ModTime)
		}, Now: a.Now})
	}
	return nil
}
