// SPDX-License-Identifier: AGPL-3.0-or-later
package archivepack

import (
	"mime"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/textprefs"
)

func UploadFilename(filename, mediaType string) string {
	name := "file-" + filename
	extensions, _ := mime.ExtensionsByType(mediaType)
	if len(extensions) > 0 && filepath.Ext(name) != extensions[0] {
		name += extensions[0]
	}
	return strings.ReplaceAll(textprefs.FilterFileName(name), "#", "_")
}
