// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/pdfdevice"
)

// Two disposable children: inspect local link identifiers, then apply the shared
// original bookmark/assets/watermark pipeline using only captured byte material.
// This is deliberately not a file controller or an output publication capability.
func (d *pdfPostprocessDocument) processPDFBytes(input []byte, embed, watermark bool, device pdfDeviceBinding) ([]byte, error) {
	request, e := d.pdfDeviceRequest(input, watermark, device)
	if e != nil {
		return nil, e
	}
	headings := deviceHeadings(d.headings)
	r, ctx := d.m.read.binding.scope.commit.r, d.m.read.binding.scope.ctx
	for i, h := range headings {
		if e = d.guard(); e != nil {
			return nil, e
		}
		row, ok, err := (indexdb.CachedBlocks{DB: r.main.DB}).Get(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		headings[i].Title = row.Content
		headings[i].Found = ok
	}
	request.Outline = &pdfdevice.Outline{Headings: headings}
	origin := strings.TrimSuffix(d.m.htmlLocalAnchorPrefix, "/#")
	raw, e := device.execute(ctx, pdfdevice.Request{Protocol: 1, PDF: append([]byte(nil), request.PDF...), Version: request.Version, InspectOrigin: origin})
	if e != nil {
		return nil, e
	}
	if e = d.guard(); e != nil {
		return nil, e
	}
	inspected, e := pdfdevice.DecodeInspection(raw)
	if e != nil {
		return nil, e
	}
	assets, e := d.preparePDFDeviceAssets(inspected.URIs, embed)
	if e != nil {
		return nil, e
	}
	request.Assets = assets
	if e = pdfdevice.Validate(*request); e != nil {
		return nil, e
	}
	if e = d.guard(); e != nil {
		return nil, e
	}
	out, e := device.execute(ctx, *request)
	if e != nil {
		return nil, e
	}
	if e = d.guard(); e != nil {
		return nil, e
	}
	return out, nil
}
func (d *pdfPostprocessDocument) preparePDFDeviceAssets(uris []string, embed bool) (plan *pdfdevice.Assets, err error) {
	err = d.withPDFAssetSource(func(root *os.Root, source indexrepair.Source, resolve func(string) (pdfAssetFile, bool, error), destinations []string, guard func() error) error {
		origin := strings.TrimSuffix(d.m.htmlLocalAnchorPrefix, "/#")
		plan = &pdfdevice.Assets{Origin: origin, Destinations: destinations, Embed: embed, Now: time.Now()}
		entries := map[string]int{}
		hasSource := false
		add := func(name string) (int, error) {
			if i, ok := entries[name]; ok {
				return i, nil
			}
			file, exists, e := resolve(name)
			if e != nil {
				return 0, e
			}
			v := pdfdevice.Asset{Path: name, Name: path.Base(strings.TrimSuffix(name, "/")), Exists: exists}
			if exists {
				info, e := root.Stat(file.path)
				if e != nil {
					return 0, e
				}
				v.Directory = info.IsDir()
				v.ModTime = info.ModTime()
			}
			if len(plan.Entries) >= 4096 {
				return 0, errors.New("pdf_device_assets_limit")
			}
			i := len(plan.Entries)
			plan.Entries = append(plan.Entries, v)
			entries[name] = i
			return i, nil
		}
		for _, name := range destinations {
			i, e := add(name)
			if e != nil {
				return e
			}
			hasSource = hasSource || plan.Entries[i].Exists
		}
		if !hasSource {
			return guard()
		}
		total := int64(pdfdevice.MaxAssetsTotal)
		read := map[string]bool{}
		for _, uri := range uris {
			normalized, e := normalizePDFAssetURL(uri, origin)
			if e != nil {
				return e
			}
			i, e := add(normalized.path)
			if e != nil {
				return e
			}
			v := &plan.Entries[i]
			if !embed || !v.Exists || read[v.Path] {
				continue
			}
			file, _, e := resolve(v.Path)
			if e != nil {
				return e
			}
			if e = source.Check(file.path, false); e != nil {
				return e
			}
			selected := capturePDFWatermarkFile(filepath.Join(d.m.read.binding.scope.commit.r.data, file.path))
			limit := pdfAssetFileLimit
			if total < limit {
				limit = total
			}
			raw, e := selected.read(limit, guard)
			if e != nil {
				return e
			}
			total -= int64(len(raw))
			v.Data = raw
			v.Loaded = true
			v.ModTime = selected.info.ModTime()
			read[v.Path] = true
		}
		return guard()
	})
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, errors.New("pdf_device_assets_unbound")
	}
	return plan, nil
}
