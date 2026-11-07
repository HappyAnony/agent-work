// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package pdfdevice

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// FontNames reads only the immutable built-in table and supplied font metadata.
// No API.ListFonts/global user metric access occurs in the parent process.
func FontNames(fonts []Font) []string {
	names := font.CoreFontNames()
	for _, f := range fonts {
		names = append(names, f.Name)
	}
	return names
}
func RequestedFont(description string) string {
	selected := ""
	for _, part := range strings.Split(description, ",") {
		kv := strings.Split(part, ":")
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == "fontname" {
			selected = strings.TrimSpace(kv[1])
		}
	}
	return selected
}
func FontDescription(description string, names []string) (string, bool) {
	parameters := map[string]string{}
	for _, part := range strings.Split(description, ",") {
		kv := strings.Split(part, ":")
		if len(kv) == 2 {
			parameters[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	found := false
	selected := parameters["fontname"]
	if selected != "" {
		for _, name := range names {
			if i := strings.IndexByte(name, '('); i >= 0 {
				name = name[:i]
			}
			name = strings.TrimSpace(name)
			if name == selected && !strings.Contains(name, ":") && name != "Corefonts" && name != "Userfonts" {
				found = true
				break
			}
		}
	}
	if !found {
		parameters["fontname"] = DefaultFont
	}
	keys := make([]string, 0, len(parameters))
	for k := range parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+":"+parameters[k])
	}
	return strings.Join(parts, ","), !found
}

// Parse preserves the original parser and byte-backed image/PDF semantics.
func Parse(mode, text, description string, media []byte) (wm *pdfmodel.Watermark, err error) {
	switch mode {
	case "text":
		return pdfcpu.ParseTextWatermarkDetails(text, description, false, types.POINTS)
	case "image":
		wm, err = pdfcpu.ParseImageWatermarkDetails("", description, false, types.POINTS)
		if err == nil {
			wm.Image = bytes.NewReader(media)
		}
	case "pdf":
		wm, err = pdfcpu.ParsePDFWatermarkDetails("", description, false, types.POINTS)
		if err == nil {
			wm.PDF = bytes.NewReader(media)
		}
	default:
		err = errors.New("pdf_watermark_mode_invalid")
	}
	return
}
func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Lm, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// Only Serve calls process: global assignments stay in the disposable child.
func process(r Request, dir string) ([]byte, error) {
	api.DisableConfigDir()
	font.UserFontDir = filepath.Join(dir, "fonts")
	if e := os.Mkdir(font.UserFontDir, 0700); e != nil {
		return nil, e
	}
	for _, f := range r.Fonts {
		if len(f.Cache) != 0 {
			if e := os.WriteFile(filepath.Join(font.UserFontDir, f.Name+".gob"), f.Cache, 0600); e != nil {
				return nil, e
			}
			continue
		}
		if e := installNamedTTF(font.UserFontDir, f); e != nil {
			return nil, e
		}
	}

	if e := api.LoadUserFonts(); e != nil {
		return nil, e
	}
	originalVersion := pdfmodel.VersionStr
	pdfmodel.VersionStr = "Wiki " + r.Version + " (pdfcpu " + originalVersion + ")"
	config := pdfmodel.NewDefaultConfiguration()
	pdf, e := api.ReadContext(bytes.NewReader(r.PDF), config)
	if e != nil {
		return nil, e
	}
	if e = api.ValidateContext(pdf); e != nil {
		return nil, e
	}
	if r.InspectOrigin != "" {
		return inspectPDF(pdf, r.InspectOrigin)
	}
	if e = applyPipeline(pdf, r); e != nil {
		return nil, e
	}
	if w := r.Watermark; w != nil && (w.Text != "" || w.Mode != "text") {
		description := w.Description
		if w.Mode == "text" && containsCJK(w.Text) {
			names, e := api.ListFonts()
			if e != nil {
				return nil, e
			}
			var fallback bool
			description, fallback = FontDescription(description, names)
			if fallback {
				if len(w.Fallback) == 0 {
					return nil, errors.New("pdf_device_font_unbound")
				}
				if e = font.InstallFontFromBytes(font.UserFontDir, DefaultFont, w.Fallback); e != nil {
					return nil, e
				}
				if e = api.LoadUserFonts(); e != nil {
					return nil, e
				}
			}
		}
		wm, e := Parse(w.Mode, w.Text, description, w.Media)
		if e != nil {
			return nil, e
		}
		wm.OnTop = true
		if e = pdfcpu.AddWatermarks(pdf, nil, wm); e != nil {
			return nil, e
		}
	}
	out := limitedBuffer{limit: MaxOutput}
	if e = api.WriteContext(pdf, &out); e != nil {
		return nil, e
	}
	verified, e := api.ReadContext(bytes.NewReader(out.Bytes()), config)
	if e != nil {
		return nil, e
	}
	if e = api.ValidateContext(verified); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}

// Keep the caller's captured inventory key: the original cache loader uses
// gob basenames, which can differ from the TTF's internal PostScript name.
func installNamedTTF(directory string, f Font) (err error) {
	stage, err := os.MkdirTemp(directory, ".install-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	if err = font.InstallFontFromBytes(stage, f.Name, f.TTF); err != nil {
		return err
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].Type().IsRegular() || filepath.Ext(entries[0].Name()) != ".gob" {
		return errors.New("pdf_device_font_invalid")
	}
	return os.Rename(filepath.Join(stage, entries[0].Name()), filepath.Join(directory, f.Name+".gob"))
}
