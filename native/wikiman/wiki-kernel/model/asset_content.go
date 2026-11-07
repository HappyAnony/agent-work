// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/88250/lute/ast"
	"github.com/klippa-app/go-pdfium"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/assetquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

type AssetContent = assetquery.Content

func GetAssetContent(id, query string, method int) *AssetContent {
	v, e := GetAssetContentContext(context.Background(), id, query, method)
	if e != nil {
		logging.LogErrorf("asset content: %s", e)
	}
	return v
}
func FullTextSearchAssetContent(query string, types map[string]bool, method, order, page, size int) ([]*AssetContent, int, int) {
	v, count, pages, e := FullTextSearchAssetContentContext(context.Background(), query, types, method, order, page, size)
	if e != nil {
		logging.LogErrorf("asset search: %s", e)
	}
	return v, count, pages
}

var assetContentSearcher = NewAssetsSearcher()

func removeIndexAssetContent(p string) {
	if e := IndexAssetContentContext(context.Background(), p, true); e != nil {
		logging.LogErrorf("remove asset index: %s", e)
	}
}

func indexAssetContent(p string) {
	if e := IndexAssetContentContext(context.Background(), p, false); e != nil {
		logging.LogErrorf("index asset content: %s", e)
	}
}

func ReindexAssetContent() {
	logOwnedTaskError("asset-content reindex admission", ReindexAssetContentContext(context.Background()))
}

func ReindexAssetContentContext(ctx context.Context) error {
	_, err := startAssetContentReindex(ctx)
	return err
}

func subscribeSQLAssetContentEvents() {
	subscribeLegacyEvent(util.EvtSQLAssetContentRebuild, func(payload map[string]interface{}) {
		withLegacySQLEvent(payload, func(ctx context.Context, _ *legacySQLEventSession) {
			if err := ReindexAssetContentContext(ctx); err != nil && ctx.Err() == nil {
				logging.LogErrorf("asset-content reindex admission failed: %s", err)
			}
		})
	})
}

var (
	AssetsSearchEnabled = true
)

type AssetsSearcher struct {
	parsers map[string]AssetParser
	lock    *sync.Mutex
}

func (searcher *AssetsSearcher) GetParser(ext string) AssetParser {
	searcher.lock.Lock()
	defer searcher.lock.Unlock()

	return searcher.parsers[strings.ToLower(ext)]
}

func (searcher *AssetsSearcher) FullIndex() {
	if err := ReindexAssetContentContext(context.Background()); err != nil {
		logging.LogErrorf("reindex asset content: %s", err)
	}
}

func (searcher *AssetsSearcher) scan(ctx context.Context, assetsDir string, guard func(context.Context) error) ([]indexdb.AssetContentEntry, error) {
	extraction, err := captureAssetExtraction(util.TempDir)
	if err != nil {
		return nil, err
	}
	return searcher.scanWithExtraction(ctx, assetsDir, guard, extraction)
}
func (searcher *AssetsSearcher) scanWithExtraction(ctx context.Context, assetsDir string, guard func(context.Context) error, extraction assetExtractionEnvironment) (items []indexdb.AssetContentEntry, err error) {
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if guard != nil {
			return guard(ctx)
		}
		return nil
	}
	if err = check(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(assetsDir)
	if errors.Is(err, os.ErrNotExist) {
		return []indexdb.AssetContentEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			items = nil
		}
	}()
	items = []indexdb.AssetContentEntry{}
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, walkErr error) error {
		if e := check(); e != nil {
			return e
		}
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		parser := searcher.GetParser(filepath.Ext(p))
		if parser == nil {
			return nil
		}
		result, e := extraction.read(ctx, root, p, assetParserKind(parser))
		if e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		if result == nil {
			return nil
		}
		name := "assets/" + p
		items = append(items, indexdb.AssetContentEntry{ID: ast.NewNodeID(), Name: util.RemoveID(filepath.Base(name)), Ext: strings.ToLower(filepath.Ext(p)), Path: name, Size: result.Size, Updated: result.Updated, Content: result.Content})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, check()
}

func NewAssetsSearcher() *AssetsSearcher {
	txtAssetParser := &TxtAssetParser{}
	searcher := &AssetsSearcher{
		parsers: map[string]AssetParser{
			".docx": &DocxAssetParser{},
			".pptx": &PptxAssetParser{},
			".xlsx": &XlsxAssetParser{},
			".pdf":  &PdfAssetParser{},
			".epub": &EpubAssetParser{},
		},

		lock: &sync.Mutex{},
	}
	for _, ext := range assetextract.TextExtensions() {
		searcher.parsers[ext] = txtAssetParser
	}
	return searcher
}

const (
	TxtAssetContentMaxSize = 1024 * 1024 * 4
	PDFAssetContentMaxPage = 1024
)

var (
	PDFAssetContentMaxSize uint64 = 1024 * 1024 * 128
)

type AssetParseResult struct {
	Path    string
	Size    int64
	Updated int64
	Content string
}

type AssetParser interface {
	Parse(absPath string) *AssetParseResult
}

type TxtAssetParser struct {
}

func (parser *TxtAssetParser) Parse(p string) *AssetParseResult {
	result, err := parseLegacyAsset(context.Background(), p, "text")
	if err != nil {
		logging.LogErrorf("extract asset content: %s", err)
	}
	return result
}

type DocxAssetParser struct {
}

func (parser *DocxAssetParser) Parse(p string) *AssetParseResult {
	result, err := parseLegacyAsset(context.Background(), p, "docx")
	if err != nil {
		logging.LogErrorf("extract asset content: %s", err)
	}
	return result
}

type PptxAssetParser struct {
}

func (parser *PptxAssetParser) Parse(p string) *AssetParseResult {
	result, err := parseLegacyAsset(context.Background(), p, "pptx")
	if err != nil {
		logging.LogErrorf("extract asset content: %s", err)
	}
	return result
}

type XlsxAssetParser struct {
}

func (parser *XlsxAssetParser) Parse(p string) *AssetParseResult {
	result, err := parseLegacyAsset(context.Background(), p, "xlsx")
	if err != nil {
		logging.LogErrorf("extract asset content: %s", err)
	}
	return result
}

// PdfAssetParser parser factory product
type PdfAssetParser struct {
}

// pdfPage struct defines a worker job for text extraction
type pdfPage struct {
	pageNo int     // page number for text extraction
	data   *[]byte // pointer to PDF document data
}

// pdfTextResult struct defines the extracted PDF text result
type pdfTextResult struct {
	pageNo int    // page number of PDF document
	text   string // text of converted page
	err    error  // processing error
}

// getTextPageWorker will extract the text from a given PDF page and return its result
// All instances in this pool share a compiled WASM module. wazero v1.9.0
// lazily mutates FunctionType keys during indirect calls; concurrent instances
// can race even though their PDF documents are distinct. Serialize entry per
// pool (not process-wide), including instance cleanup, until that dependency is
// safe. This is a compat worker fix, not a contextual PDF service.
func (parser *PdfAssetParser) getTextPageWorker(instance pdfium.Pdfium, page <-chan *pdfPage, result chan<- *pdfTextResult, runtimeMu *sync.Mutex, workers *sync.WaitGroup) {
	defer workers.Done()
	defer func() { runtimeMu.Lock(); defer runtimeMu.Unlock(); instance.Close() }()
	for pd := range page {
		runtimeMu.Lock()
		text, err := assetextract.PageText(instance, pd.data, pd.pageNo)
		runtimeMu.Unlock()
		converted := &pdfTextResult{pageNo: pd.pageNo, text: text, err: err}
		result <- converted
	}
}

// Parse will parse a PDF document using PDFium webassembly module using a worker pool
func (parser *PdfAssetParser) Parse(p string) *AssetParseResult {
	result, err := parseLegacyAsset(context.Background(), p, "pdf")
	if err != nil {
		logging.LogErrorf("extract asset content: %s", err)
	}
	return result
}

type EpubAssetParser struct {
}

func (parser *EpubAssetParser) Parse(p string) *AssetParseResult {
	result, err := parseLegacyAsset(context.Background(), p, "epub")
	if err != nil {
		logging.LogErrorf("extract asset content: %s", err)
	}
	return result
}
