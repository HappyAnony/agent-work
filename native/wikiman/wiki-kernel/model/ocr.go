package model

import (
	"context"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/internal/ocr"

	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func OCRAssetsJob() { OCRAssetsJobContext(context.Background()) }
func OCRAssetsJobContext(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if err := util.WaitForTesseractInitContext(ctx); err != nil {
		return
	}
	if err := ocr.Cycle(ctx, autoOCRAssetsContext, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		util.SaveAssetsTexts()
		return nil
	}); err != nil && ctx.Err() == nil {
		logging.LogWarnf("OCR cycle failed: %s", err)
	}
}

func autoOCRAssets() { _ = autoOCRAssetsContext(context.Background()) }
func autoOCRAssetsContext(ctx context.Context) error {
	if !util.LegacyOCREnabled() {
		return nil
	}

	defer logging.Recover()

	assetsPath := util.GetDataAssetsAbsPath()
	assets := getUnOCRAssetsAbsPaths()
	if 0 < len(assets) {
		for i, assetAbsPath := range assets {
			if err := ctx.Err(); err != nil {
				return err
			}
			text := util.GetOcrJsonText(util.TesseractContext(ctx, assetAbsPath))
			if err := ctx.Err(); err != nil {
				return err
			}
			p := strings.TrimPrefix(assetAbsPath, assetsPath)
			p = "assets" + filepath.ToSlash(p)
			util.SetAssetText(p, text)
			if 7 <= i { // 一次任务中最多处理 7 张图片，防止长时间占用系统资源
				break
			}
		}
	}

	util.CleanNotExistAssetsTexts()

	// 刷新 OCR 结果到数据库
	util.NodeOCRQueueLock.Lock()
	defer util.NodeOCRQueueLock.Unlock()
	for _, id := range util.NodeOCRQueue {
		sql.IndexNodeQueue(id)
	}
	util.NodeOCRQueue = nil
	return ctx.Err()
}

func getUnOCRAssetsAbsPaths() (ret []string) {
	var assetsPaths []string
	assets := cache.GetAssets()
	for _, asset := range assets {
		if !util.IsTesseractExtractable(asset.Path) {
			continue
		}
		assetsPaths = append(assetsPaths, asset.Path)
	}

	assetsPath := util.GetDataAssetsAbsPath()
	for _, assetPath := range assetsPaths {
		if util.ExistsAssetText(assetPath) {
			continue
		}
		absPath := filepath.Join(assetsPath, strings.TrimPrefix(assetPath, "assets"))
		ret = append(ret, absPath)
	}
	return
}

func FlushAssetsTextsJob() {
	util.SaveAssetsTexts()
}
