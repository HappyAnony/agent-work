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
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"github.com/mssola/useragent"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func exportAttributeView(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	avID := arg["id"].(string)
	blockID := arg["blockID"].(string)
	zipPath, err := model.ExportAv2CSVContext(c.Request.Context(), avID, blockID)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}

	ret.Data = map[string]interface{}{
		"zip": zipPath,
	}
}

func exportEPUB(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "epub", ".epub", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportRTF(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "rtf", ".rtf", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportODT(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "odt", ".odt", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportMediaWiki(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "mediawiki", ".wiki", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportOrgMode(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "org", ".org", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportOPML(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "opml", ".opml", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportTextile(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "textile", ".textile", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportAsciiDoc(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "asciidoc", ".adoc", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportReStructuredText(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "rst", ".rst", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func export2Liandi(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	err := model.Export2LiandiContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func exportDataInFolder(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	exportFolder := arg["folder"].(string)
	name, err := model.ExportDataInFolderContext(c.Request.Context(), exportFolder)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
	ret.Data = map[string]interface{}{
		"name": name,
	}
}

func exportData(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	zipPath, err := model.ExportDataContext(c.Request.Context())
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
	ret.Data = map[string]interface{}{
		"zip": zipPath,
	}
}

func exportResources(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var name string
	if nil != arg["name"] {
		name = util.TruncateLenFileName(arg["name"].(string))
	}
	if name == "" {
		name = time.Now().Format("export-2006-01-02_15-04-05") // 生成的 *.zip 文件主文件名
	}

	if nil == arg["paths"] {
		ret.Code = 1
		ret.Data = ""
		ret.Msg = "paths is required"
		return
	}

	var resourcePaths []string // 文件/文件夹在工作空间中的路径
	for _, resourcePath := range arg["paths"].([]interface{}) {
		resourcePaths = append(resourcePaths, resourcePath.(string))
	}

	zipFilePath, err := model.ExportResourcesContext(c.Request.Context(), resourcePaths, name)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
	ret.Data = map[string]interface{}{
		"path": zipFilePath, // 相对于工作空间目录的路径
	}
}

func exportNotebookMd(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	zipPath := model.ExportNotebookMarkdown(notebook, c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": path.Base(zipPath),
		"zip":  zipPath,
	}
}

func exportMds(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["ids"].([]interface{})
	var ids []string
	for _, id := range idsArg {
		ids = append(ids, id.(string))
	}

	name, zipPath := model.ExportPandocConvertZip(ids, "", ".md", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportMd(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportPandocConvertZip([]string{id}, "", ".md", c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportNotebookSY(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	zipPath := model.ExportNotebookSY(id, c.Request.Context())
	ret.Data = map[string]interface{}{
		"zip": zipPath,
	}
}

func exportSY(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	name, zipPath := model.ExportSY(id, c.Request.Context())
	ret.Data = map[string]interface{}{
		"name": name,
		"zip":  zipPath,
	}
}

func exportMdContent(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	var refMode *int
	if nil != arg["refMode"] {
		v := int(arg["refMode"].(float64))
		refMode = &v
	}

	var embedMode *int
	if nil != arg["embedMode"] {
		v := int(arg["embedMode"].(float64))
		embedMode = &v
	}

	yfm := true
	if nil != arg["yfm"] {
		yfm = arg["yfm"].(bool)
	}

	fillCSSVar := false
	if nil != arg["fillCSSVar"] {
		fillCSSVar = arg["fillCSSVar"].(bool)
	}

	adjustHeadingLevel := false
	if nil != arg["adjustHeadingLevel"] {
		adjustHeadingLevel = arg["adjustHeadingLevel"].(bool)
	}

	imgTag := false
	if nil != arg["imgTag"] {
		imgTag = arg["imgTag"].(bool)
	}

	hPath, content, err := model.ExportMarkdownContentContext(c.Request.Context(), id, model.MarkdownContentOptions{RefMode: refMode, EmbedMode: embedMode, YFM: yfm, FillCSSVar: fillCSSVar, AdjustHeadingLevel: adjustHeadingLevel, ImgTag: imgTag})
	if err != nil && !errors.Is(err, model.ErrBlockNotFound) {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"hPath":   hPath,
		"content": content,
	}
}

func exportDocx(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	savePath := arg["savePath"].(string)
	removeAssets := arg["removeAssets"].(bool)
	merge := false
	if nil != arg["merge"] {
		merge = arg["merge"].(bool)
	}

	fullPath, err := model.ExportDocxContext(c.Request.Context(), id, savePath, removeAssets, merge)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
	ret.Data = map[string]interface{}{
		"path": fullPath,
	}
}

func exportMdHTML(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	savePath := arg["savePath"].(string)

	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		result, err := model.ExportHTMLFolderContext(c.Request.Context(), id, true, false, false, false)
		if err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
			return
		}
		ret.Data = map[string]interface{}{"id": id, "name": result.Name, "content": result.Content, "folder": result.Folder}
		return
	}

	result, err := model.ExportHTMLDesktopContext(c.Request.Context(), id, savePath, true, false, false, false)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"id":      id,
		"name":    result.Name,
		"content": result.Content,
	}
}

func exportTempContent(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	address, err := model.ExportTempContentContext(c.Request.Context(), arg["content"].(string))
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 7000}
		return
	}
	ret.Data = map[string]interface{}{"url": address}
}

func exportBrowserHTML(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	folder := arg["folder"].(string)
	htmlContent := arg["html"].(string)
	name := arg["name"].(string)

	zipURL, err := model.ExportBrowserHTMLContext(c.Request.Context(), folder, htmlContent, name)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{"zip": zipURL}
}

func exportPreviewHTML(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	keepFold := false
	if nil != arg["keepFold"] {
		keepFold = arg["keepFold"].(bool)
	}
	merge := false
	if nil != arg["merge"] {
		merge = arg["merge"].(bool)
	}
	image := false
	if nil != arg["image"] {
		image = arg["image"].(bool)
	}
	result, err := model.ExportPreviewHTMLContext(c.Request.Context(), id, model.ExportHTMLPreviewOptions{Image: image, KeepFold: keepFold, Merge: merge})
	if err != nil && !errors.Is(err, model.ErrBlockNotFound) {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	if result.Attributes == nil {
		result.Attributes = map[string]string{}
	}

	ret.Data = map[string]interface{}{
		"id":      id,
		"name":    result.Name,
		"content": result.Content,
		"attrs":   result.Attributes,
		"type":    result.Type,
	}
}

func exportHTML(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	pdf := arg["pdf"].(bool)
	savePath := arg["savePath"].(string)
	keepFold := false
	if nil != arg["keepFold"] {
		keepFold = arg["keepFold"].(bool)
	}
	merge := false
	if nil != arg["merge"] {
		merge = arg["merge"].(bool)
	}

	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		result, err := model.ExportHTMLFolderContext(c.Request.Context(), id, false, pdf, keepFold, merge)
		if err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
			return
		}
		ret.Data = map[string]interface{}{"id": id, "name": result.Name, "content": result.Content, "folder": result.Folder}
		return
	}

	result, err := model.ExportHTMLDesktopContext(c.Request.Context(), id, savePath, false, pdf, keepFold, merge)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"id":      id,
		"name":    result.Name,
		"content": result.Content,
	}
}

func processPDF(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id, idOK := arg["id"].(string)
	path, pathOK := arg["path"].(string)
	embed, embedOK := arg["removeAssets"].(bool)
	watermark, watermarkOK := arg["watermark"].(bool)
	merge := false
	mergeOK := true
	if value, exists := arg["merge"]; exists {
		merge, mergeOK = value.(bool)
	}
	if !idOK || id == "" || !pathOK || path == "" || !embedOK || !watermarkOK || !mergeOK {
		ret.Code = -1
		ret.Msg = "pdf_request_invalid"
		return
	}
	err := model.ProcessPDFContext(c.Request.Context(), id, path, merge, embed, watermark)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = pdfProcessFailureData(err)
		return
	}

}

func exportPreview(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)

	userAgentStr := c.GetHeader("User-Agent")
	fillCSSVar := true
	if userAgentStr != "" {
		ua := useragent.New(userAgentStr)
		name, _ := ua.Browser()
		// Chrome、Edge、SiYuan 桌面端不需要替换 CSS 变量
		if !ua.Mobile() && (name == "Chrome" || name == "Edge" || strings.Contains(userAgentStr, "Electron") || strings.Contains(userAgentStr, "SiYuan/")) {
			fillCSSVar = false
		}
	}

	stdHTML, err := model.PreviewContext(c.Request.Context(), id, fillCSSVar)
	if err != nil && !errors.Is(err, model.ErrBlockNotFound) {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"html":       stdHTML,
		"fillCSSVar": fillCSSVar,
	}
}

func exportAsFile(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	file, err := model.ExportAsFileRequest(c.Request.Context(), c.Request)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{"file": file}
}

func pdfProcessFailureData(err error) interface{} {
	if model.PDFProcessCommitted(err) {
		return map[string]interface{}{"committed": true}
	}
	return nil
}
