// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import "strings"

func SearchIgnoreLines(raw []byte) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	if len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	return out
}
