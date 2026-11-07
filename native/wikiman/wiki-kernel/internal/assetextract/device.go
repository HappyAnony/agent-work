// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package assetextract runs the original document text converters in a joined
// one-job child. Input is bytes, never source paths or workspace capabilities.
package assetextract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const WorkerArgument = "--internal-asset-extraction"
const MaxInput = 128 << 20
const MaxText = 32 << 20
const TextLimit = 4 << 20
const PageLimit = 1024

type Request struct {
	Kind        string
	Data        []byte
	PDFMaxBytes uint64
	PDFMaxPages int
}
type Result struct {
	Content string
	Skipped bool
}
type Binding func(context.Context, Request) (Result, error)

func validate(q Request) error {
	if len(q.Data) > MaxInput || q.PDFMaxBytes > MaxInput || q.PDFMaxPages > PageLimit || q.PDFMaxPages < 0 {
		return errors.New("asset_extraction_input_limit")
	}
	switch q.Kind {
	case "text", "docx", "pptx", "xlsx", "pdf", "epub":
		return nil
	}
	return errors.New("asset_extraction_type_invalid")
}
func Bind(executable, temp string) (Binding, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(temp) {
		return nil, errors.New("asset_extraction_unbound")
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
		return nil, errors.New("asset_extraction_executable_invalid")
	}
	check := func() error {
		p, e := filepath.EvalSymlinks(executable)
		if e != nil {
			return e
		}
		now, e := os.Lstat(physical)
		if e != nil {
			return e
		}
		if p != physical || !os.SameFile(info, now) || now.Mode() != info.Mode() || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) {
			return errors.New("asset_extraction_executable_changed")
		}
		return nil
	}
	return func(ctx context.Context, q Request) (out Result, err error) {
		if ctx == nil {
			return out, errors.New("asset_extraction_context_missing")
		}
		if e := ctx.Err(); e != nil {
			return out, e
		}
		if e := validate(q); e != nil {
			return out, e
		}
		if e := check(); e != nil {
			return out, e
		}
		// UTF-8 text conversion has no foreign runtime, filesystem or worker.
		if q.Kind == "text" {
			return ReadText(q.Data), ctx.Err()
		}
		if e := os.MkdirAll(temp, 0700); e != nil {
			return out, e
		}
		dir, e := os.MkdirTemp(temp, "asset-extract-")
		if e != nil {
			return out, e
		}
		defer func() {
			err = errors.Join(err, os.RemoveAll(dir))
			if err != nil {
				out = Result{}
			}
		}()
		wire, e := json.Marshal(q)
		if e != nil {
			return out, e
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, physical, WorkerArgument)
		cmd.Dir = dir
		cmd.Env = []string{"HOME=" + dir, "TMPDIR=" + dir, "TMP=" + dir, "TEMP=" + dir}
		if root := os.Getenv("SystemRoot"); root != "" {
			cmd.Env = append(cmd.Env, "SystemRoot="+root)
		}
		cmd.Stdin = bytes.NewReader(wire)
		stdout := limitedBuffer{limit: MaxText*6 + 1024}
		stderr := limitedBuffer{limit: 4096}
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.WaitDelay = 200 * time.Millisecond
		if e = cmd.Run(); e != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			return out, errors.New("asset_extraction_failed")
		}
		if e = ctx.Err(); e != nil {
			return out, e
		}
		if e = check(); e != nil {
			return out, e
		}
		if e = json.Unmarshal(stdout.Bytes(), &out); e != nil {
			return Result{}, errors.New("asset_extraction_output_invalid")
		}
		if len(out.Content) > MaxText || out.Skipped && out.Content != "" {
			return Result{}, errors.New("asset_extraction_output_limit")
		}
		return out, nil
	}, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
	err   error
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	if len(p) > b.limit-b.Len() {
		b.err = errors.New("asset_extraction_output_limit")
		return 0, b.err
	}
	return b.Buffer.Write(p)
}
func Serve(in io.Reader, out io.Writer) error {
	var q Request
	dec := json.NewDecoder(io.LimitReader(in, MaxInput*2+4096))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&q); e != nil {
		return e
	}
	if e := validate(q); e != nil {
		return e
	}
	var trailing any
	if e := dec.Decode(&trailing); e != io.EOF {
		return errors.New("asset_extraction_trailing_input")
	}
	r, e := convert(q)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(r)
}
