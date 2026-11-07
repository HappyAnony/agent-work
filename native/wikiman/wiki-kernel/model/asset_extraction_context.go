// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"wiki-kernel/internal/assetextract"
	"wiki-kernel/util"
)

type assetExtractionEnvironment struct {
	run    assetextract.Binding
	pdfMax uint64
	mobile bool
}

func captureAssetExtraction(temp string) (assetExtractionEnvironment, error) {
	exe, e := os.Executable()
	if e != nil {
		return assetExtractionEnvironment{}, e
	}
	run, e := assetextract.Bind(exe, temp)
	if e != nil {
		return assetExtractionEnvironment{}, e
	}
	max := PDFAssetContentMaxSize
	if value := os.Getenv("WIKI_PDF_ASSET_CONTENT_INDEX_MAX_SIZE"); value != "" {
		n, e := strconv.ParseUint(value, 10, 64)
		if e != nil {
			return assetExtractionEnvironment{}, e
		}
		max = n
	}
	if max > assetextract.MaxInput {
		return assetExtractionEnvironment{}, errors.New("asset_extraction_pdf_budget")
	}
	return assetExtractionEnvironment{run: run, pdfMax: max, mobile: util.Container == util.ContainerIOS || util.Container == util.ContainerAndroid || util.Container == util.ContainerHarmony}, nil
}
func assetParserKind(p AssetParser) string {
	switch p.(type) {
	case *TxtAssetParser:
		return "text"
	case *DocxAssetParser:
		return "docx"
	case *PptxAssetParser:
		return "pptx"
	case *XlsxAssetParser:
		return "xlsx"
	case *PdfAssetParser:
		return "pdf"
	case *EpubAssetParser:
		return "epub"
	}
	return ""
}
func (e assetExtractionEnvironment) read(ctx context.Context, root *os.Root, p, kind string) (out *AssetParseResult, err error) {
	if ctx == nil || e.run == nil || root == nil {
		return nil, errors.New("asset_extraction_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if strings.HasPrefix(filepath.Base(p), "~") || kind == "pdf" && e.mobile {
		return nil, nil
	}
	f, err := root.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			out = nil
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("asset_extraction_not_regular")
	}
	if info.Size() > assetextract.MaxInput || kind == "text" && info.Size() > assetextract.TextLimit || kind == "pdf" && uint64(info.Size()) > e.pdfMax {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(f, assetextract.MaxInput+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > assetextract.MaxInput {
		return nil, errors.New("asset_extraction_input_limit")
	}
	result, err := e.run(ctx, assetextract.Request{Kind: kind, Data: raw, PDFMaxBytes: e.pdfMax, PDFMaxPages: PDFAssetContentMaxPage})
	if err != nil {
		return nil, err
	}
	if result.Skipped {
		return nil, nil
	}
	return &AssetParseResult{Content: result.Content, Size: info.Size(), Updated: info.ModTime().Unix()}, ctx.Err()
}
func parseLegacyAsset(ctx context.Context, p, kind string) (out *AssetParseResult, err error) {
	e, e2 := captureAssetExtraction(util.TempDir)
	if e2 != nil {
		return nil, e2
	}
	p, err = filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	if kind != "text" && strings.ToLower(filepath.Ext(p)) != "."+kind {
		return nil, nil
	}
	root, err := os.OpenRoot(filepath.Dir(p))
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			out = nil
		}
	}()
	return e.read(ctx, root, filepath.Base(p), kind)
}
