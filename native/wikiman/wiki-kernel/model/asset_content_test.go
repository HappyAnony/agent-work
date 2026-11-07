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
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"wiki-kernel/util"
)

func TestPDFParser(t *testing.T) {
	oldTemp := util.TempDir
	util.TempDir = t.TempDir()
	defer func() { util.TempDir = oldTemp }()
	p := &PdfAssetParser{}
	res := p.Parse("../testdata/parsertest.pdf")
	if res == nil || res.Content == "" {
		t.Fatalf("empty or nil PDF content result")
	}
}

type pdfWorkerFixture struct {
	pdfium.Pdfium
	failOpen, failText         bool
	closeStarted, releaseClose chan struct{}
	documentsClosed            atomic.Int32
}

func (f *pdfWorkerFixture) OpenDocument(*requests.OpenDocument) (*responses.OpenDocument, error) {
	if f.failOpen {
		return nil, errors.New("synthetic open failure")
	}
	return &responses.OpenDocument{}, nil
}
func (f *pdfWorkerFixture) GetPageText(*requests.GetPageText) (*responses.GetPageText, error) {
	if f.failText {
		return nil, errors.New("synthetic text failure")
	}
	return &responses.GetPageText{Text: "page text"}, nil
}
func (f *pdfWorkerFixture) FPDF_CloseDocument(*requests.FPDF_CloseDocument) (*responses.FPDF_CloseDocument, error) {
	f.documentsClosed.Add(1)
	return &responses.FPDF_CloseDocument{}, nil
}
func (f *pdfWorkerFixture) Close() error {
	close(f.closeStarted)
	<-f.releaseClose
	return nil
}

func TestPDFWorkerFailureAndCleanupJoin(t *testing.T) {
	for _, mode := range []string{"success", "open failure", "text failure"} {
		t.Run(mode, func(t *testing.T) {
			f := &pdfWorkerFixture{failOpen: mode == "open failure", failText: mode == "text failure", closeStarted: make(chan struct{}), releaseClose: make(chan struct{})}
			pages, results := make(chan *pdfPage, 1), make(chan *pdfTextResult, 1)
			var runtimeMu sync.Mutex
			var workers sync.WaitGroup
			workers.Add(1)
			go (&PdfAssetParser{}).getTextPageWorker(f, pages, results, &runtimeMu, &workers)
			pages <- &pdfPage{pageNo: 7}
			close(pages)
			select {
			case got := <-results:
				if got.pageNo != 7 || (got.err != nil) != (mode != "success") || (mode == "success" && got.text != "page text") {
					t.Error(got)
				}
			case <-time.After(5 * time.Second):
				t.Error("worker did not report result")
			}
			select {
			case <-f.closeStarted:
			case <-time.After(5 * time.Second):
				t.Error("instance cleanup not started")
			}
			joined := make(chan struct{})
			go func() { workers.Wait(); close(joined) }()
			select {
			case <-joined:
				t.Error("joined before instance cleanup finished")
			default:
			}
			close(f.releaseClose)
			select {
			case <-joined:
			case <-time.After(5 * time.Second):
				t.Fatal("cleanup not joined")
			}
			want := int32(1)
			if f.failOpen {
				want = 0
			}
			if f.documentsClosed.Load() != want {
				t.Fatal("document cleanup count", f.documentsClosed.Load(), want)
			}
		})
	}
}
