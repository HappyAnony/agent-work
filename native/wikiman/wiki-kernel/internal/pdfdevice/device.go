// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package pdfdevice runs pdfcpu's process-global state in a one-job child.
// It has no knowledge-root, workspace, HTTP or file-publication capability.
package pdfdevice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const WorkerArgument = "--internal-pdf-device"
const MaxPDF = 64 << 20
const MaxOutput = 128 << 20
const MaxMedia = 32 << 20
const MaxFont = 16 << 20
const maxFonts = 32 << 20
const maxWire = 640 << 20
const DefaultFont = "LXGWWenKaiLite-Regular"

type Font struct {
	Name  string
	TTF   []byte
	Cache []byte
}
type Watermark struct {
	Mode, Text, Description string
	Media                   []byte
	Fallback                []byte
}
type Request struct {
	InspectOrigin string
	Outline       *Outline
	Assets        *Assets
	Protocol      int
	PDF           []byte
	Watermark     *Watermark
	Fonts         []Font
	Version       string
}

// A caller must capture executable identity/configuration in its host binding.
// Run never discovers a binary in PATH or silently falls back to in-process PDF.
func Run(ctx context.Context, executable string, request Request) (out []byte, err error) {
	if ctx == nil || !filepath.IsAbs(executable) {
		return nil, errors.New("pdf_device_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = Validate(request); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxWire {
		return nil, errors.New("pdf_device_request_limit")
	}
	dir, err := os.MkdirTemp("", "wiki-pdf-device-")
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, os.RemoveAll(dir))
		if err != nil {
			out = nil
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, WorkerArgument)
	cmd.Dir = dir
	// No inherited tokens, proxy settings, font path or user configuration.
	cmd.Env = []string{"HOME=" + dir, "TMPDIR=" + dir}
	if value := os.Getenv("SystemRoot"); value != "" {
		cmd.Env = append(cmd.Env, "SystemRoot="+value)
	}
	cmd.Stdin = bytes.NewReader(raw)
	stdout := limitedBuffer{limit: MaxOutput}
	stderr := limitedBuffer{limit: 4096}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 200 * time.Millisecond
	if e := cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("pdf_device_failed")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if request.InspectOrigin != "" {
		if _, e := DecodeInspection(stdout.Bytes()); e != nil {
			return nil, e
		}
		return append([]byte(nil), stdout.Bytes()...), nil
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte("%PDF-")) {
		return nil, errors.New("pdf_device_output_invalid")
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("pdf_device_output_limit")
	}
	return b.Buffer.Write(p)
}

var fontName = regexp.MustCompile(`^[\pL\pN_][\pL\pN_. -]{0,127}$`)

// Validate checks bounded, path-free job data before material is copied or sent.
func Validate(r Request) error {
	if r.Protocol != 1 || len(r.PDF) == 0 || len(r.PDF) > MaxPDF || len(r.Version) > 128 || strings.ContainsAny(r.Version, "\x00\r\n") {
		return errors.New("pdf_device_request_invalid")
	}
	if len(r.Fonts) > 8 {
		return errors.New("pdf_device_font_limit")
	}
	total := 0
	seen := map[string]bool{}
	for _, f := range r.Fonts {
		if !fontName.MatchString(f.Name) || (len(f.TTF) == 0) == (len(f.Cache) == 0) || len(f.TTF)+len(f.Cache) > MaxFont || seen[f.Name] {
			return errors.New("pdf_device_font_invalid")
		}
		seen[f.Name] = true
		total += len(f.TTF) + len(f.Cache)
	}
	if total > maxFonts {
		return errors.New("pdf_device_font_limit")
	}
	if w := r.Watermark; w != nil {
		if len(w.Media) > MaxMedia || len(w.Fallback) > MaxFont || len(w.Text) > 1<<20 || len(w.Description) > 16<<10 {
			return errors.New("pdf_device_watermark_limit")
		}
		if w.Mode != "text" && w.Mode != "image" && w.Mode != "pdf" {
			return errors.New("pdf_device_watermark_invalid")
		}
		if w.Mode == "text" && len(w.Media) != 0 || w.Mode != "text" && (len(w.Media) == 0 || w.Text != "") {
			return errors.New("pdf_device_watermark_invalid")
		}
	}
	return validatePipeline(r)
}

// Serve is only called at the explicit executable entrypoint, before server/CLI
// boot. One request, one fresh process. The input contains bytes, never paths.
func Serve(input io.Reader, output io.Writer) error {
	dir, e := os.Getwd()
	if e != nil {
		return e
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("pdf_device_private_directory_required")
	}
	raw, e := io.ReadAll(io.LimitReader(input, maxWire+1))
	if e != nil {
		return e
	}
	if len(raw) > maxWire {
		return errors.New("pdf_device_request_limit")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r Request
	if e = dec.Decode(&r); e != nil {
		return errors.New("pdf_device_protocol_invalid")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("pdf_device_protocol_invalid")
	}
	if e = Validate(r); e != nil {
		return e
	}
	data, e := process(r, dir)
	if e != nil {
		return errors.New("pdf_device_process_failed")
	}
	_, e = io.Copy(output, bytes.NewReader(data))
	return e
}

// Bind captures a host-selected executable. A later replacement or alias change
// fails before launch or discards the result. This is identity checking, not an
// OS-level executable CAS or protection from trusted-code writes preserving mtime.
func Bind(executable string) (func(context.Context, Request) ([]byte, error), error) {
	if !filepath.IsAbs(executable) {
		return nil, errors.New("pdf_device_unbound")
	}
	physical, e := filepath.EvalSymlinks(executable)
	if e != nil {
		return nil, e
	}
	info, e := os.Lstat(physical)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, errors.New("pdf_device_executable_invalid")
	}
	check := func() error {
		current, e := filepath.EvalSymlinks(executable)
		if e != nil {
			return e
		}
		if current != physical {
			return errors.New("pdf_device_executable_changed")
		}
		now, e := os.Lstat(physical)
		if e != nil {
			return e
		}
		if !now.Mode().IsRegular() || !os.SameFile(now, info) || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) || now.Mode() != info.Mode() {
			return errors.New("pdf_device_executable_changed")
		}
		return nil
	}
	return func(ctx context.Context, r Request) ([]byte, error) {
		if e := check(); e != nil {
			return nil, e
		}
		out, e := Run(ctx, physical, r)
		if e != nil {
			return nil, e
		}
		if e = check(); e != nil {
			return nil, e
		}
		return out, nil
	}, nil
}
