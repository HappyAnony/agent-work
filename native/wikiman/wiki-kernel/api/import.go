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

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

type archiveImportHandler interface {
	ImportReader(io.Reader, string, string) error
}

func importSY(c *gin.Context) {
	importSYWithCapture(c, func(ctx context.Context) (archiveImportHandler, error) { return model.CaptureArchiveImport(ctx) })
}
func importSYWithCapture(c *gin.Context, capture func(context.Context) (archiveImportHandler, error)) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	importer, err := capture(c.Request.Context())
	if err == nil {
		err = importArchiveUpload(c, true, importer.ImportReader)
	}
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

type dataImportHandler interface{ ImportDataReader(io.Reader) error }

func importData(c *gin.Context) {
	importDataWithCapture(c, func(ctx context.Context) (dataImportHandler, error) { return model.CaptureDataImport(ctx) })
}
func importDataWithCapture(c *gin.Context, capture func(context.Context) (dataImportHandler, error)) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	importer, err := capture(c.Request.Context())
	if err == nil {
		err = importArchiveUpload(c, false, func(reader io.Reader, _, _ string) error { return importer.ImportDataReader(reader) })
	}
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

// Capture precedes multipart parsing. Both import controllers join request-owned
// reader/form cleanup failures before returning the original HTTP envelope.
func importArchiveUpload(c *gin.Context, destination bool, invoke func(io.Reader, string, string) error) (err error) {
	return importArchiveUploadNamed(c, destination, func(reader io.Reader, box, to, _ string) error { return invoke(reader, box, to) })
}
func importArchiveUploadNamed(c *gin.Context, destination bool, invoke func(io.Reader, string, string, string) error) (err error) {
	form, err := c.MultipartForm()
	if form != nil {
		defer func() {
			if cleanup := form.RemoveAll(); cleanup != nil {
				err = errors.Join(err, fmt.Errorf("multipart cleanup: %w", cleanup))
			}
		}()
	}
	if err != nil {
		return err
	}
	if form == nil || len(form.File["file"]) == 0 {
		return errors.New("no file found")
	}
	var box, to string
	if destination {
		if len(form.Value["notebook"]) == 0 || len(form.Value["toPath"]) == 0 {
			return errors.New("missing import destination")
		}
		box, to = form.Value["notebook"][0], form.Value["toPath"][0]
	}
	reader, err := form.File["file"][0].Open()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("upload close: %w", closeErr))
		}
	}()
	return invoke(reader, box, to, form.File["file"][0].Filename)
}

func importStdMd(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	localPath := arg["localPath"].(string)
	toPath := arg["toPath"].(string)
	err := model.ImportFromLocalPath(notebook, localPath, toPath, c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

type markdownZIPImportHandler interface {
	ImportZIPReader(io.Reader, string, string, string) error
}

func importZipMd(c *gin.Context) {
	importZipMdWithCapture(c, func(ctx context.Context) (markdownZIPImportHandler, error) {
		return model.CaptureMarkdownZIPImport(ctx)
	})
}
func importZipMdWithCapture(c *gin.Context, capture func(context.Context) (markdownZIPImportHandler, error)) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	importer, err := capture(c.Request.Context())
	if err == nil {
		err = importArchiveUploadNamed(c, true, importer.ImportZIPReader)
	}
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}
