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
	"strings"
	"wiki-kernel/internal/pdfdevice"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func PdfListLinks(ctx *model.Context) ([]model.LinkAnnotation, []model.LinkAnnotation, error) {
	return readPDFAssetLinks(ctx, func(uri string) bool {
		return strings.HasPrefix(uri, "http://127.0.0.1:") && strings.Contains(uri, "/assets/")
	}, func() error { return nil })
}

const PdfOutlineScheme = pdfdevice.OutlineScheme

func PdfListToCLinks(ctx *model.Context) ([]model.LinkAnnotation, error) {
	return readPDFOutlineLinks(ctx, func() error { return nil })
}
