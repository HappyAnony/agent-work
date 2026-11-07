// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original util/ocr.go recognition, TSV/text and language-selection rules.
package ocr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type Options struct {
	Enabled    bool
	Candidates []string
	Languages  []string
	MaxSize    int64
	Timeout    time.Duration
}

func Defaults() Options {
	return Options{Enabled: true, Candidates: []string{"tesseract", "/usr/local/bin/tesseract", "/opt/homebrew/bin/tesseract"}, MaxSize: 2000000, Timeout: 7 * time.Second}
}

type Engine struct {
	options                    Options
	Binary, Version, Signature string
	Languages                  []string
	Enabled                    bool
}

func (e *Engine) MaxSize() int64 { return e.options.MaxSize }

var ErrInvalid = errors.New("invalid_ocr_configuration")

func Open(ctx context.Context, o Options) (*Engine, error) {
	if o.MaxSize <= 0 || o.Timeout <= 0 {
		return nil, ErrInvalid
	}
	o.Candidates = append([]string(nil), o.Candidates...)
	o.Languages = append([]string(nil), o.Languages...)
	ret := &Engine{options: o}
	if !o.Enabled {
		return ret, nil
	}
	for _, binary := range o.Candidates {
		data, err := run(ctx, o.Timeout, binary, "--version")
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if !strings.HasPrefix(string(data), "tesseract ") {
			continue
		}
		ret.Binary = binary
		ret.Version = strings.TrimSpace(strings.TrimPrefix(strings.Split(string(data), "\n")[0], "tesseract "))
		break
	}
	if ret.Binary == "" {
		return nil, errors.New("ocr_executable_unavailable")
	}
	data, err := run(ctx, o.Timeout, ret.Binary, "--list-langs")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n")
	var available []string
	for _, line := range lines[1:] {
		if lang := strings.TrimSpace(line); lang != "" {
			available = append(available, lang)
		}
	}
	ret.Languages = SelectLanguages(available, o.Languages)
	if len(ret.Languages) == 0 {
		return nil, errors.New("ocr_languages_unavailable")
	}
	ret.Enabled = true
	ret.Signature = fmt.Sprintf("%x", sha256.Sum256([]byte(ret.Binary+"\x00"+ret.Version+"\x00"+strings.Join(ret.Languages, "+"))))
	return ret, nil
}
func SelectLanguages(available, wanted []string) (ret []string) {
	for _, lang := range available {
		if len(wanted) > 0 {
			for _, w := range wanted {
				if lang == w {
					ret = append(ret, lang)
					break
				}
			}
		} else if lang == "eng" || strings.HasPrefix(lang, "chi") || lang == "fra" || lang == "spa" || lang == "deu" || lang == "rus" || lang == "jpn" || lang == "osd" {
			ret = append(ret, lang)
		}
	}
	return
}
func Extractable(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range []string{".png", ".jpg", ".jpeg", ".tif", ".tiff", ".bmp", ".gif", ".webp", ".pbm", ".pgm", ".ppm", ".pnm"} {
		if ext == e {
			return true
		}
	}
	return false
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16<<20 {
		return 0, errors.New("ocr_output_budget")
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, timeout time.Duration, binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	gulu.CmdAttr(cmd)
	cmd.WaitDelay = 200 * time.Millisecond
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return append([]byte(nil), out.Bytes()...), nil
}
func (e *Engine) Recognize(ctx context.Context, name string, data []byte) ([]map[string]interface{}, error) {
	if !e.Enabled {
		return nil, nil
	}
	if !Extractable(name) || int64(len(data)) > e.options.MaxSize {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "wiki-ocr-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input"+filepath.Ext(name))
	if err = os.WriteFile(input, data, 0600); err != nil {
		return nil, err
	}
	return RunFile(ctx, e.options.Timeout, e.Binary, e.Languages, input)
}

func RunFile(ctx context.Context, timeout time.Duration, binary string, languages []string, input string) ([]map[string]interface{}, error) {
	output, err := run(ctx, timeout, binary, "-c", "debug_file=/dev/null", input, "stdout", "-l", strings.Join(languages, "+"), "tsv")
	if err != nil {
		return nil, err
	}
	return ParseTSV(string(output)), nil
}
func ParseTSV(tsv string) (ret []map[string]interface{}) {
	lines := strings.Split(strings.ReplaceAll(tsv, "\r", ""), "\n")
	if len(lines) == 0 {
		return nil
	}
	headers := strings.Split(lines[0], "\t")
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		row := map[string]interface{}{}
		for i, key := range headers {
			value := ""
			if i < len(fields) {
				value = fields[i]
			}
			row[key] = value
		}
		ret = append(ret, row)
	}
	return
}
func Text(rows []map[string]interface{}) string {
	var b strings.Builder
	for _, row := range rows {
		if s, ok := row["text"].(string); ok {
			b.WriteString(" " + strings.ReplaceAll(s, "\r", ""))
		}
	}
	return Normalize(b.String())
}
func Normalize(s string) string {
	s = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(s))
	var b strings.Builder
	chinese, space := false, false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if chinese || space {
				continue
			}
			b.WriteByte(' ')
			chinese = false
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
		chinese = unicode.Is(unicode.Han, r)
	}
	return b.String()
}

// ReadImage enforces the same size policy without giving a command source paths.
func (e *Engine) ReadImage(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, e.options.MaxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > e.options.MaxSize {
		return nil, errors.New("ocr_image_too_large")
	}
	return data, nil
}
