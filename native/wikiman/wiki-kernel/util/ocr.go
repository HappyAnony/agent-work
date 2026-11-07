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

package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/88250/go-humanize"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"

	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/ocr"
)

var (
	TesseractBin     = "tesseract"
	TesseractEnabled bool
	TesseractMaxSize = 2 * 1000 * uint64(1000)
	TesseractLangs   []string

	assetsTexts        = map[string]string{}
	assetsTextsLock    = sync.Mutex{}
	assetsTextsChanged = atomic.Bool{}
)

func CleanNotExistAssetsTexts() {
	assetsTextsLock.Lock()
	defer assetsTextsLock.Unlock()

	assetsPath := GetDataAssetsAbsPath()
	var toRemoves []string
	for asset, _ := range assetsTexts {
		assetAbsPath := strings.TrimPrefix(asset, "assets")
		assetAbsPath = filepath.Join(assetsPath, assetAbsPath)
		if !filelock.IsExist(assetAbsPath) {
			toRemoves = append(toRemoves, asset)
		}
	}

	for _, asset := range toRemoves {
		delete(assetsTexts, asset)
		assetsTextsChanged.Store(true)
	}
	return
}

func LoadAssetsTexts() {
	assetsPath := GetDataAssetsAbsPath()
	assetsTextsPath := filepath.Join(assetsPath, "ocr-texts.json")
	if !filelock.IsExist(assetsTextsPath) {
		return
	}

	start := time.Now()
	data, err := filelock.ReadFile(assetsTextsPath)
	if err != nil {
		logging.LogErrorf("read assets texts failed: %s", err)
		return
	}

	var loaded map[string]string
	if err = gulu.JSON.UnmarshalJSON(data, &loaded); err != nil {
		logging.LogErrorf("unmarshal assets texts failed: %s", err)
		return
	}
	assetsTextsLock.Lock()
	assetsTexts = loaded
	assetsTextsLock.Unlock()
	debug.FreeOSMemory()

	if elapsed := time.Since(start).Seconds(); 2 < elapsed {
		logging.LogWarnf("read assets texts [%s] to [%s], elapsed [%.2fs]", humanize.BytesCustomCeil(uint64(len(data)), 2), assetsTextsPath, elapsed)
	}
	return
}

func SaveAssetsTexts() {
	if !assetsTextsChanged.Load() {
		return
	}

	start := time.Now()

	assetsTextsLock.Lock()
	data, err := gulu.JSON.MarshalIndentJSON(assetsTexts, "", "  ")
	if err != nil {
		logging.LogErrorf("marshal assets texts failed: %s", err)
		return
	}
	assetsTextsLock.Unlock()

	assetsPath := GetDataAssetsAbsPath()
	assetsTextsPath := filepath.Join(assetsPath, "ocr-texts.json")
	if err = filelock.WriteFile(assetsTextsPath, data); err != nil {
		logging.LogErrorf("write assets texts failed: %s", err)
		return
	}
	debug.FreeOSMemory()

	if elapsed := time.Since(start).Seconds(); 2 < elapsed {
		logging.LogWarnf("save assets texts [size=%s] to [%s], elapsed [%.2fs]", humanize.BytesCustomCeil(uint64(len(data)), 2), assetsTextsPath, elapsed)
	}

	assetsTextsChanged.Store(false)
}

func SetAssetText(asset, text string) {
	assetsTextsLock.Lock()
	oldText, ok := assetsTexts[asset]
	assetsTexts[asset] = text
	assetsTextsLock.Unlock()
	if !ok || oldText != text {
		assetsTextsChanged.Store(true)
	}
}

func ExistsAssetText(asset string) (ret bool) {
	assetsTextsLock.Lock()
	_, ret = assetsTexts[asset]
	assetsTextsLock.Unlock()
	return
}

func OcrAsset(asset string) (ret []map[string]interface{}, err error) {
	if !LegacyOCREnabled() {
		err = fmt.Errorf("%s", Langs[Lang][266])
		return
	}

	assetsPath := GetDataAssetsAbsPath()
	assetAbsPath := strings.TrimPrefix(asset, "assets")
	assetAbsPath = filepath.Join(assetsPath, assetAbsPath)
	ret = Tesseract(assetAbsPath)
	assetsTextsLock.Lock()
	ocrText := GetOcrJsonText(ret)
	assetsTexts[asset] = ocrText
	assetsTextsLock.Unlock()
	if "" != ocrText {
		assetsTextsChanged.Store(true)
	}
	return
}

func GetAssetText(asset string) (ret string) {
	assetsTextsLock.Lock()
	ret = assetsTexts[asset]
	assetsTextsLock.Unlock()
	return
}

func RemoveAssetText(asset string) {
	assetsTextsLock.Lock()
	delete(assetsTexts, asset)
	assetsTextsLock.Unlock()
	assetsTextsChanged.Store(true)
}

var tesseractExts = []string{
	".png",
	".jpg",
	".jpeg",
	".tif",
	".tiff",
	".bmp",
	".gif",
	".webp",
	".pbm",
	".pgm",
	".ppm",
	".pnm",
}

func IsTesseractExtractable(p string) bool { return ocr.Extractable(p) }

// tesseractOCRLock 用于 Tesseract OCR 加锁串行执行提升稳定性 https://github.com/siyuan-note/siyuan/issues/7265
var tesseractOCRLock = sync.Mutex{}
var tesseractRunGate = make(chan struct{}, 1)

func LegacyOCREnabled() bool {
	tesseractOCRLock.Lock()
	defer tesseractOCRLock.Unlock()
	return TesseractEnabled
}

func Tesseract(imgAbsPath string) []map[string]interface{} {
	return TesseractContext(context.Background(), imgAbsPath)
}
func TesseractContext(parent context.Context, imgAbsPath string) (ret []map[string]interface{}) {
	select {
	case tesseractRunGate <- struct{}{}:
	case <-parent.Done():
		return
	}
	defer func() { <-tesseractRunGate }()
	tesseractOCRLock.Lock()
	enabled, binary, maxSize, languages := TesseractEnabled, TesseractBin, TesseractMaxSize, append([]string(nil), TesseractLangs...)
	tesseractOCRLock.Unlock()
	if ContainerStd != Container || !enabled {
		return
	}
	defer logging.Recover()

	if !IsTesseractExtractable(imgAbsPath) {
		return
	}

	info, err := os.Stat(imgAbsPath)
	if err != nil {
		return
	}

	if maxSize < uint64(info.Size()) {
		return
	}

	defer logging.Recover()

	timeout := 7000
	timeoutEnv := os.Getenv("WIKI_TESSERACT_TIMEOUT")
	if "" != timeoutEnv {
		if timeoutParsed, parseErr := strconv.Atoi(timeoutEnv); nil == parseErr {
			timeout = timeoutParsed
		} else {
			logging.LogWarnf("parse tesseract timeout [%s] failed: %s", timeoutEnv, parseErr)
		}
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	ret, err = ocr.RunFile(ctx, time.Duration(timeout)*time.Millisecond, binary, languages, imgAbsPath)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		logging.LogWarnf("tesseract [path=%s, size=%d] timeout [%dms]", imgAbsPath, info.Size(), timeout)
		return
	}

	if err != nil {
		logging.LogWarnf("tesseract [path=%s, size=%d] failed: %s", imgAbsPath, info.Size(), err)
		return
	}

	msg := ocr.StatusMessage(info.Name(), GetOcrJsonText(ret))
	PushStatusBar(msg)
	return
}

// GetOcrJsonText 提取并连接所有 text 字段的函数
func GetOcrJsonText(jsonData []map[string]interface{}) string { return ocr.Text(jsonData) }

var tesseractInited = atomic.Bool{}

func WaitForTesseractInit() { _ = WaitForTesseractInitContext(context.Background()) }
func WaitForTesseractInitContext(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !tesseractInited.Load() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}
func InitTesseract() { InitTesseractContext(context.Background()) }

// Explicit compatibility bootstrap. Its caller owns cancellation/join; native
// WorkspaceContexts configure their own engine, never these compatibility globals.
func InitTesseractContext(ctx context.Context) {
	defer tesseractInited.Store(true)
	options, err := ocr.DeviceOptions(os.Getenv, ContainerStd == Container)
	if err != nil {
		logging.LogWarnf("invalid OCR device options: %s", err)
		return
	}
	engine, err := ocr.Open(ctx, options)
	if err != nil {
		logging.LogWarnf("OCR unavailable: %s", err)
		return
	}
	tesseractOCRLock.Lock()
	defer tesseractOCRLock.Unlock()
	TesseractEnabled = engine.Enabled
	TesseractBin = engine.Binary
	TesseractMaxSize = uint64(engine.MaxSize())
	TesseractLangs = append([]string(nil), engine.Languages...)
}

func filterTesseractLangs(langs []string) []string {
	var wanted []string
	if env := os.Getenv("WIKI_TESSERACT_LANGS"); env != "" {
		wanted = strings.Split(env, "+")
	}
	return ocr.SelectLanguages(langs, wanted)
}

var (
	NodeOCRQueue     []string
	NodeOCRQueueLock = sync.Mutex{}
)

func PushNodeOCRQueue(n *ast.Node) {
	if nil == n {
		return
	}

	NodeOCRQueueLock.Lock()
	defer NodeOCRQueueLock.Unlock()
	NodeOCRQueue = append(NodeOCRQueue, n.ID)
}
