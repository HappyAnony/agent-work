// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package assetextract

import "strings"

func TextExtensions() []string {
	return []string{".txt", ".md", ".markdown", ".json", ".log", ".sql", ".html", ".xml", ".java", ".h", ".c", ".cpp", ".go", ".rs", ".swift", ".kt", ".py", ".php", ".js", ".css", ".ts", ".sh", ".bat", ".cmd", ".ini", ".yaml", ".rst", ".adoc", ".textile", ".opml", ".org", ".wiki", ".cs"}
}

func Kind(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".docx", ".pptx", ".xlsx", ".pdf", ".epub":
		return strings.TrimPrefix(ext, ".")
	}
	for _, text := range TextExtensions() {
		if text == ext {
			return "text"
		}
	}
	return ""
}
