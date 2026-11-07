// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package markup

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// GetDuplicateName preserves the original numbered-copy naming rule.
func GetDuplicateName(master string) (ret string) {
	if master == "" {
		return
	}
	ret = master + " (1)"
	m := regexp.MustCompile(`^(.*) \((\d+)\)$`).FindStringSubmatch(master)
	if len(m) < 3 {
		return
	}
	num, _ := strconv.Atoi(m[2])
	return fmt.Sprintf("%s (%d)", m[1], num+1)
}

func InsertElem[T any](s []T, index int, value T) []T {
	if len(s) == index {
		return append(s, value)
	}
	s = append(s[:index+1], s[index:]...)
	s[index] = value
	return s
}

func RemoveElem[T any](s []T, index int) []T {
	return append(s[:index], s[index+1:]...)
}

func TimeFromID(id string) string {
	if len(id) < 14 {
		return time.Now().Format("20060102150405")
	}
	return id[:14]
}
