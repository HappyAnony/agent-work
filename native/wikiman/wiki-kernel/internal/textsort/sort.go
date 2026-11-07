// SPDX-License-Identifier: AGPL-3.0-or-later
// Original util tag comparison rules without runtime globals.
package textsort

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/facette/natsort"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"io"
	"regexp"
	"strings"
)

func NaturalCompare(str1, str2 string) bool {
	str1 = RemoveEmojiInvisible(str1)
	str2 = RemoveEmojiInvisible(str2)
	return natsort.Compare(str1, str2)
}

func PinYinCompare(str1, str2 string) bool {
	str1 = RemoveEmojiInvisible(str1)
	str2 = RemoveEmojiInvisible(str2)

	// Doc tree, backlinks, tags and templates ignores case when sorting alphabetically by name https://github.com/siyuan-note/siyuan/issues/8360
	str1 = strings.ToLower(str1)
	str2 = strings.ToLower(str2)

	a, _ := UTF82GBK(str1)
	b, _ := UTF82GBK(str2)
	bLen := len(b)
	for idx, chr := range a {
		if idx > bLen-1 {
			return false
		}
		if chr != b[idx] {
			return chr < b[idx]
		}
	}
	return true
}

func UTF82GBK(src string) ([]byte, error) {
	GB18030 := simplifiedchinese.All[0]
	return io.ReadAll(transform.NewReader(bytes.NewReader([]byte(src)), GB18030.NewEncoder()))
}

// GBK2UTF8 transform GBK byte array into UTF8 string.
var emojiRegex = regexp.MustCompile(`/([0-9#][\x{20E3}])|` +
	`[\x{00ae}\x{00a9}\x{203C}\x{2047}\x{2048}\x{2049}\x{3030}\x{303D}\x{2139}\x{2122}\x{3297}\x{3299}]|` +
	`[\x{2190}-\x{21FF}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{2300}-\x{23FF}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{2460}-\x{24FF}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{25A0}-\x{25FF}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{2600}-\x{27BF}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{2900}-\x{297F}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{2B00}-\x{2BF0}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{1F000}-\x{1F6FF}]|[\x{FE00}-\x{FEFF}]|` +
	`[\x{1F600}-\x{1F64F}]|` +
	`[\x{1F680}-\x{1F6FF}]|` +
	`[\x{1F900}-\x{1F9FF}]|` +
	`[\x{1F1E0}-\x{1F1FF}]|` +
	`[\x{1D100}-\x{1D1FF}]|` +
	`[\x{2600}-\x{26FF}]|` +
	`[\x{2700}-\x{27BF}]|` +
	`[\x{10000}-\x{E01EF}]`)

func RemoveEmojiInvisible(text string) (ret string) {
	ret = emojiRegex.ReplaceAllString(text, "")
	ret = RemoveInvalid(ret)
	return
}

func RemoveInvalid(text string) (ret string) {
	ret = gulu.Str.RemoveInvisible(text)
	ret = gulu.Str.RemovePUA(ret)
	return
}
