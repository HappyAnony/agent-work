// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"errors"
	"time"
)

type DurationFormatter func(time.Time, time.Time) string

func (a *AttributeView) reportError(err error) {
	if a.ErrorSink != nil {
		a.ErrorSink(err)
	}
}

var ErrRelatedReadUnbound = errors.New("AV related source reader is not bound")
var ErrDurationFormatterUnbound = errors.New("AV duration formatter is not bound")

// Duration formatting has always been language-specific. No device-language
// global is imported here. Checked callers must bind it before duration work.
func formatDuration(formatters []DurationFormatter, a, b time.Time) string {
	if len(formatters) == 0 || formatters[0] == nil {
		panic(ErrDurationFormatterUnbound)
	}
	return formatters[0](a, b)
}

func (a *AttributeView) readRelated(id string) *AttributeView {
	if a.ReadAttributeView == nil {
		a.readError = ErrRelatedReadUnbound
		return nil
	}
	ret, err := a.ReadAttributeView(id)
	if err != nil {
		a.readError = err
	}
	return ret
}

// FilterChecked reports a failed dependency rather than publishing a partially
// filtered candidate. Both source and collection must be detached request data.
func FilterChecked(viewable Viewable, a *AttributeView, further map[string]Collection, cached map[string]*AttributeView) error {
	if cached == nil {
		cached = map[string]*AttributeView{}
	}
	a.readError = nil
	Filter(viewable, a, further, cached)
	return a.readError
}
