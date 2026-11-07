// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"wiki-kernel/internal/pdfdevice"
	"wiki-kernel/util"
)

// This binding is assembled by the trusted host, not deserialized from a request.
// Fonts are captured TTF bytes; no ambient parent font cache is consulted.
type pdfDeviceBinding struct {
	execute func(context.Context, pdfdevice.Request) ([]byte, error)
	fonts   []pdfdevice.Font
	version string
}

// watermarkBytes is the isolated read/watermark/write phase, not the final
// ProcessPDF controller. It receives a caller-private PDF and returns bytes;
// no source/output path, publication capability or implicit legacy adapter.
func (d *pdfPostprocessDocument) watermarkBytes(input []byte, enabled bool, device pdfDeviceBinding) ([]byte, error) {
	request, e := d.pdfDeviceRequest(input, enabled, device)
	if e != nil {
		return nil, e
	}
	out, e := device.execute(d.m.read.binding.scope.ctx, *request)
	if e != nil {
		return nil, e
	}
	if e = d.guard(); e != nil {
		return nil, e
	}
	return out, nil
}
func (d *pdfPostprocessDocument) pdfDeviceRequest(input []byte, enabled bool, device pdfDeviceBinding) (*pdfdevice.Request, error) {
	if e := d.guard(); e != nil {
		return nil, e
	}
	if device.execute == nil {
		return nil, errors.New("pdf_device_unbound")
	}
	if len(input) == 0 || len(input) > pdfdevice.MaxPDF {
		return nil, errors.New("pdf_device_input_limit")
	}
	if e := pdfdevice.Validate(pdfdevice.Request{Protocol: 1, PDF: input, Version: device.version, Fonts: device.fonts}); e != nil {
		return nil, e
	}
	request := pdfdevice.Request{Protocol: 1, PDF: append([]byte(nil), input...), Version: device.version}
	for _, f := range device.fonts {
		request.Fonts = append(request.Fonts, pdfdevice.Font{Name: f.Name, TTF: append([]byte(nil), f.TTF...), Cache: append([]byte(nil), f.Cache...)})
	}
	snapshot := d.m.pdfWatermark
	if enabled {
		if !snapshot.bound {
			return nil, errors.New("pdf_watermark_dependencies_unbound")
		}
		if snapshot.value != "" {
			if snapshot.err != nil {
				return nil, snapshot.err
			}
			w := &pdfdevice.Watermark{Mode: snapshot.mode, Text: snapshot.value, Description: snapshot.description}
			if w.Mode == "text" {
				selected := pdfdevice.RequestedFont(w.Description)
				_, missing := pdfdevice.FontDescription(w.Description, pdfdevice.FontNames(request.Fonts))
				if selected != "" && missing {
					f, ok, e := d.m.pdfFonts.selected(selected, d.guard)
					if e != nil {
						return nil, e
					}
					if ok {
						request.Fonts = append(request.Fonts, f)
					}
				}
				_, fallback := pdfdevice.FontDescription(w.Description, pdfdevice.FontNames(request.Fonts))
				if fallback && util.ContainsCJK(w.Text) {
					raw, e := snapshot.fallback.read(pdfWatermarkFontLimit, d.guard)
					if e != nil {
						return nil, e
					}
					w.Fallback = raw
				}
			}
			if w.Mode != "text" {
				if w.Mode == "image" && !model.ImageFileName(snapshot.value) {
					return nil, errors.New("pdf_watermark_image_type_invalid")
				}
				raw, e := snapshot.media.read(pdfWatermarkMediaLimit, d.guard)
				if e != nil {
					return nil, e
				}
				w.Media = raw
				w.Text = ""
			}
			request.Watermark = w
		}
	}
	if e := pdfdevice.Validate(request); e != nil {
		return nil, e
	}
	return &request, d.guard()
}
