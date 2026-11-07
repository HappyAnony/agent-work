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
	"bytes"
	"io"
	"strings"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/textsort"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func NaturalCompare(a, b string) bool { return textsort.NaturalCompare(a, b) }
func EmojiPinYinCompare(str1, str2 string) bool {
	str1_ := strings.TrimSpace(RemoveEmojiInvisible(str1))
	str2_ := strings.TrimSpace(RemoveEmojiInvisible(str2))
	if str1_ == str2_ && 0 == len(str1_) {
		// 全部都是 emoji 的情况按 emoji 字符串排序
		return strings.Compare(str1, str2) < 0
	}
	return PinYinCompare(str1, str2)
}

func PinYinCompare(a, b string) bool          { return textsort.PinYinCompare(a, b) }
func PinYinCompare4FileTree(a, b string) bool { return textsort.FileTreeCompare(a, b) }

// UTF82GBK transform UTF8 rune into GBK byte array.
func UTF82GBK(src string) ([]byte, error) {
	GB18030 := simplifiedchinese.All[0]
	return io.ReadAll(transform.NewReader(bytes.NewReader([]byte(src)), GB18030.NewEncoder()))
}

// GBK2UTF8 transform GBK byte array into UTF8 string.
func GBK2UTF8(src []byte) (string, error) {
	GB18030 := simplifiedchinese.All[0]
	bytes, err := io.ReadAll(transform.NewReader(bytes.NewReader(src), GB18030.NewDecoder()))
	return string(bytes), err
}

const (
	SortModeNameASC         = configtypes.SortModeNameASC
	SortModeNameDESC        = configtypes.SortModeNameDESC
	SortModeUpdatedASC      = configtypes.SortModeUpdatedASC
	SortModeUpdatedDESC     = configtypes.SortModeUpdatedDESC
	SortModeAlphanumASC     = configtypes.SortModeAlphanumASC
	SortModeAlphanumDESC    = configtypes.SortModeAlphanumDESC
	SortModeCustom          = configtypes.SortModeCustom
	SortModeRefCountASC     = configtypes.SortModeRefCountASC
	SortModeRefCountDESC    = configtypes.SortModeRefCountDESC
	SortModeCreatedASC      = configtypes.SortModeCreatedASC
	SortModeCreatedDESC     = configtypes.SortModeCreatedDESC
	SortModeSizeASC         = configtypes.SortModeSizeASC
	SortModeSizeDESC        = configtypes.SortModeSizeDESC
	SortModeSubDocCountASC  = configtypes.SortModeSubDocCountASC
	SortModeSubDocCountDESC = configtypes.SortModeSubDocCountDESC
	SortModeFileTree        = configtypes.SortModeFileTree

	SortModeUnassigned = configtypes.SortModeUnassigned
)
