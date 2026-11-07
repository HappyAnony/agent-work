// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package textsort

import (
	"bytes"
	"strings"
)

// Original file-tree alphabet order differs from the general pinyin comparer:
// ignore emoji/invisibles, suffix and case, then compare encoded bytes/prefixes.
func FileTreeCompare(a, b string) bool {
	a = strings.ToLower(strings.TrimSuffix(RemoveEmojiInvisible(a), ".docj"))
	b = strings.ToLower(strings.TrimSuffix(RemoveEmojiInvisible(b), ".docj"))
	x, _ := UTF82GBK(a)
	y, _ := UTF82GBK(b)
	return bytes.Compare(x, y) < 0
}
