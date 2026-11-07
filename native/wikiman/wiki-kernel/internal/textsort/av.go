// SPDX-License-Identifier: AGPL-3.0-or-later
package textsort

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
)

// Original AV numeric-text parsing and emoji comparison rules.
func Convert2Float(s string) (float64, bool) {
	s = RemoveInvalid(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", "")
	buf := bytes.Buffer{}
	for _, r := range s {
		if unicode.IsDigit(r) || '.' == r || '-' == r {
			buf.WriteRune(r)
		}
	}
	ret, err := strconv.ParseFloat(strings.TrimSpace(buf.String()), 64)
	if err != nil {
		return 0, false
	}
	return ret, true
}

func EmojiPinYinCompare(str1, str2 string) bool {
	a, b := strings.TrimSpace(RemoveEmojiInvisible(str1)), strings.TrimSpace(RemoveEmojiInvisible(str2))
	if a == b && len(a) == 0 {
		return strings.Compare(str1, str2) < 0
	}
	return PinYinCompare(str1, str2)
}
