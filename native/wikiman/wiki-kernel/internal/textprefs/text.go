// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original keyword and upload-name rules shared without runtime dependencies.
package textprefs

import (
	"bytes"
	"github.com/88250/gulu"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var newlineRegexp = regexp.MustCompile(`[\r\n]+`)

func ParseKeywords(keywordsStr string) (keywords []string) {
	keywords = []string{}
	keywordsStr = strings.TrimSpace(keywordsStr)
	if "" == keywordsStr {
		return
	}
	// 先处理转义的逗号
	keywordsStr = strings.ReplaceAll(keywordsStr, "\\,", "__comma@sep__")
	// 再将连续或单个换行符替换为一个逗号，避免把 `\\\n` 转换为 `\,`
	keywordsStr = newlineRegexp.ReplaceAllString(keywordsStr, ",")
	// 按逗号分隔
	for part := range strings.SplitSeq(keywordsStr, ",") {
		part = strings.TrimSpace(part) // 剔除前后的空白字符
		if "" == part {
			continue
		}
		// 恢复转义的逗号
		part = strings.ReplaceAll(part, "__comma@sep__", ",")
		keywords = append(keywords, part)
	}
	return
}
func AddKeywords(keywordsStr string, newKeywords []string) string {
	keywordsStr = strings.TrimSpace(keywordsStr)
	if 0 == len(newKeywords) {
		return keywordsStr
	}

	var builder strings.Builder
	if "" != keywordsStr {
		if !strings.HasSuffix(keywordsStr, "\\,") {
			keywordsStr = strings.TrimSuffix(keywordsStr, ",")
		}
		builder.WriteString(keywordsStr)
		builder.WriteString(",")
	}

	keywords := gulu.Str.RemoveDuplicatedElem(ParseKeywords(keywordsStr))
	newKeywords = gulu.Str.RemoveDuplicatedElem(newKeywords)
	allKeys := make(map[string]bool)

	// 添加新关键字
	for _, keyword := range newKeywords {
		keywordTrimmed := strings.TrimSpace(keyword)
		if "" == keywordTrimmed {
			continue
		}
		if gulu.Str.Contains(keywordTrimmed, keywords) {
			// 剔除已存在的关键字
			continue
		}
		if _, value := allKeys[keywordTrimmed]; value {
			// 剔除重复的关键字
			continue
		}
		allKeys[keywordTrimmed] = true
		builder.WriteString(strings.ReplaceAll(keyword, ",", "\\,")) // 字符串切片转换为字符串，需要转义逗号
		builder.WriteString(",")
	}

	return strings.TrimSuffix(builder.String(), ",")
}
func FilterUploadEmojiFileName(name string) string {
	if strings.HasPrefix(name, "api/icon/") {
		// 忽略动态图标 https://github.com/siyuan-note/siyuan/issues/15139
		return name
	}

	name = strings.ReplaceAll(name, "/", "_@slash@_")
	name = FilterUploadFileName(name)
	name = strings.ReplaceAll(name, "_@slash@_", "/")
	return name
}
func FilterUploadFileName(name string) string {
	ret := FilterFileName(name)

	// 插入资源文件时去除 `[`、`(` 等符号 https://github.com/siyuan-note/siyuan/issues/6708
	ret = strings.ReplaceAll(ret, "~", "")
	//ret = strings.ReplaceAll(ret, "_", "") // 插入资源文件时允许下划线 https://github.com/siyuan-note/siyuan/issues/3534
	ret = strings.ReplaceAll(ret, "[", "")
	ret = strings.ReplaceAll(ret, "]", "")
	ret = strings.ReplaceAll(ret, "(", "")
	ret = strings.ReplaceAll(ret, ")", "")
	ret = strings.ReplaceAll(ret, "!", "")
	ret = strings.ReplaceAll(ret, "`", "")
	ret = strings.ReplaceAll(ret, "&", "")
	ret = strings.ReplaceAll(ret, "{", "")
	ret = strings.ReplaceAll(ret, "}", "")
	ret = strings.ReplaceAll(ret, "=", "")
	ret = strings.ReplaceAll(ret, "#", "")
	ret = strings.ReplaceAll(ret, "%", "")
	ret = strings.ReplaceAll(ret, "$", "")
	ret = strings.ReplaceAll(ret, ";", "")
	ret = TruncateLenFileName(ret)
	return ret
}
func TruncateLenFileName(name string) (ret string) {
	// 插入资源文件时文件名长度最大限制 189 字节 https://github.com/siyuan-note/siyuan/issues/7099
	ext := filepath.Ext(name)
	var byteCount int
	truncated := false
	buf := bytes.Buffer{}
	for _, r := range name {
		byteCount += utf8.RuneLen(r)
		if 189-len(ext) < byteCount {
			truncated = true
			break
		}
		buf.WriteRune(r)
	}
	if truncated {
		buf.WriteString(ext)
	}
	ret = buf.String()
	return
}
func FilterFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "'", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".")
	name = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(name)) // Remove invisible characters from file names when uploading assets https://github.com/siyuan-note/siyuan/issues/11683
	return name
}
