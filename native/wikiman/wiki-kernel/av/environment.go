// SPDX-License-Identifier: AGPL-3.0-or-later
package av

import (
	"github.com/siyuan-note/logging"
	"time"
	"wiki-kernel/util"
)

// Only the legacy handler binds device language and the old source reader.
func FormatDuration(a, b time.Time) string { return util.HumanizeRelTime(a, b, util.Lang) }
func reportError(err error)                { logging.LogErrorf("%s", err) }
func bindEnvironment(a *AttributeView) *AttributeView {
	copy := *a
	copy.ReadAttributeView = ParseAttributeView
	copy.ErrorSink = reportError
	copy.DurationFormatter = FormatDuration
	return &copy
}
