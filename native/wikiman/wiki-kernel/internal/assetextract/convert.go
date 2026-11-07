// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package assetextract

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"code.sajari.com/docconv"
	"github.com/88250/epub"
	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/xuri/excelize/v2"
)

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

// ReadText is the inherited bounded UTF-8 text projection, shared by read indexing
// and the isolated converter worker. It has no filesystem or process effects.
func ReadText(raw []byte) Result {
	if len(raw) > TextLimit || !utf8.Valid(raw) {
		return Result{Skipped: true}
	}
	return Result{Content: string(raw)}
}
func convert(q Request) (out Result, err error) {
	if q.Kind == "text" {
		return ReadText(q.Data), nil
	}
	var content string
	if q.Kind == "docx" || q.Kind == "pptx" || q.Kind == "xlsx" || q.Kind == "epub" {
		z, e := zip.NewReader(bytes.NewReader(q.Data), int64(len(q.Data)))
		if e != nil {
			return out, e
		}
		if len(z.File) > 10000 {
			return out, errors.New("asset_extraction_archive_limit")
		}
		var total uint64
		for _, f := range z.File {
			if f.UncompressedSize64 > MaxInput || total > MaxInput-f.UncompressedSize64 {
				return out, errors.New("asset_extraction_archive_limit")
			}
			total += f.UncompressedSize64
		}
	}
	switch q.Kind {
	case "docx":
		content, _, err = docconv.ConvertDocx(bytes.NewReader(q.Data))
	case "pptx":
		content, _, err = docconv.ConvertPptx(bytes.NewReader(q.Data))
	case "xlsx":
		var x *excelize.File
		x, err = excelize.OpenReader(bytes.NewReader(q.Data))
		if err != nil {
			return out, err
		}
		defer func() { err = errors.Join(err, x.Close()) }()
		buf := limitedBuffer{limit: MaxText}
		sheets := x.GetSheetMap()
		indexes := make([]int, 0, len(sheets))
		for id := range sheets {
			indexes = append(indexes, id)
		}
		slices.Sort(indexes)
		for _, id := range indexes {
			rows, e := x.GetRows(sheets[id])
			if e != nil {
				return out, e
			}
			for _, row := range rows {
				for _, cell := range row {
					if _, e = buf.Write([]byte(cell + " ")); e != nil {
						return out, e
					}
				}
			}
		}
		content = buf.String()
	case "epub":
		// epub's converter requires a local zip name. The child has only its
		// private working directory; neither parent paths nor credentials exist.
		if err = os.WriteFile("input.epub", q.Data, 0600); err != nil {
			return out, err
		}
		buf := limitedBuffer{limit: MaxText}
		err = epub.ToTxt("input.epub", &buf)
		err = errors.Join(err, buf.err)
		content = buf.String()
	case "pdf":
		return pdf(q)
	default:
		return out, errors.New("asset_extraction_type_invalid")
	}
	if err != nil {
		return Result{}, err
	}
	if len(content) > MaxText {
		return Result{}, errors.New("asset_extraction_output_limit")
	}
	return Result{Content: normalize(content)}, nil
}

// PageText is also used by the compatibility worker facade; errors from actual
// document cleanup are part of completion rather than detached cleanup work.
func PageText(instance pdfium.Pdfium, data *[]byte, page int) (text string, err error) {
	doc, e := instance.OpenDocument(&requests.OpenDocument{File: data})
	if e != nil {
		return "", e
	}
	defer func() {
		_, e := instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		err = errors.Join(err, e)
	}()
	res, e := instance.GetPageText(&requests.GetPageText{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: page}}})
	if e != nil {
		return "", e
	}
	return res.Text, nil
}
func pdf(q Request) (out Result, err error) {
	max := q.PDFMaxBytes
	if max == 0 {
		max = MaxInput
	}
	pages := q.PDFMaxPages
	if pages == 0 {
		pages = PageLimit
	}
	if uint64(len(q.Data)) > max {
		return Result{Skipped: true}, nil
	}
	pool, e := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1})
	if e != nil {
		return out, e
	}
	defer func() { err = errors.Join(err, pool.Close()) }()
	instance, e := pool.GetInstance(30 * time.Second)
	if e != nil {
		return out, e
	}
	defer func() { err = errors.Join(err, instance.Close()) }()
	doc, e := instance.OpenDocument(&requests.OpenDocument{File: &q.Data})
	if e != nil {
		return out, e
	}
	defer func() {
		_, e := instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		err = errors.Join(err, e)
	}()
	count, e := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if e != nil {
		return out, e
	}
	if count.PageCount > pages {
		return Result{Skipped: true}, nil
	}
	buf := limitedBuffer{limit: MaxText}
	for page := 0; page < count.PageCount; page++ {
		res, e := instance.GetPageText(&requests.GetPageText{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: page}}})
		if e != nil {
			return Result{}, e
		}
		if _, e = buf.Write([]byte(" " + normalize(res.Text))); e != nil {
			return Result{}, e
		}
	}
	return Result{Content: buf.String()}, nil
}
