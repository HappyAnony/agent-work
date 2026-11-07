// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"strings"
)

// SelectCreateSavePath preserves the original notebook/global fallback and
// document-only whitespace normalization, before any template execution.
func SelectCreateSavePath(current, globalBox, globalPath, localBox, localPath string, hasLocal, reference bool, opened func(string) bool) (box, template string) {
	template = globalPath
	if hasLocal {
		box, template = localBox, localPath
	}
	if box == "" && template == "" {
		box = globalBox
	}
	if box != "" && !opened(box) {
		box = current
	}
	if box == "" {
		box = current
	}
	if template == "" {
		template = globalPath
	}
	if !reference {
		template = strings.TrimSpace(template)
	}
	if box != current && template != "" && !strings.HasPrefix(template, "/") {
		template = "/" + template
	}
	return
}
