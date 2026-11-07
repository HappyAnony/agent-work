// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original util/time.go calendar difference rules, with explicit owner labels.
package conf

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

func FormatDiffTime(a, b time.Time, labels map[string]string) string {
	year, month, day, hour, min, _ := humanizeDiffTime(a, b)
	buf := bytes.Buffer{}
	if 0 < year {
		if 1 == year {
			buf.WriteString(fmt.Sprintf(labels["1y"], " "))
		} else {
			buf.WriteString(fmt.Sprintf(labels["xy"], year, " "))
		}
	}
	if 0 < month {
		if 1 == month {
			buf.WriteString(fmt.Sprintf(labels["1M"], " "))
		} else {
			buf.WriteString(fmt.Sprintf(labels["xM"], month, " "))
		}
	}
	if 0 < day {
		if 1 == day {
			buf.WriteString(fmt.Sprintf(labels["1d"], " "))
		} else {
			buf.WriteString(fmt.Sprintf(labels["xd"], day, " "))
		}
	}
	if 0 < hour {
		if 1 == hour {
			buf.WriteString(fmt.Sprintf(labels["1h"], " "))
		} else {
			buf.WriteString(fmt.Sprintf(labels["xh"], hour, " "))
		}
	}
	if 0 < min {
		if 1 == min {
			buf.WriteString(fmt.Sprintf(labels["1m"], " "))
		} else {
			buf.WriteString(fmt.Sprintf(labels["xm"], min, " "))
		}
	}
	return strings.TrimSpace(buf.String())
}

func humanizeDiffTime(a, b time.Time) (year, month, day, hour, min, sec int) {
	// 感谢 https://stackoverflow.com/a/36531443/1043233

	if a.Location() != b.Location() {
		b = b.In(a.Location())
	}
	if a.After(b) {
		a, b = b, a
	}
	y1, M1, d1 := a.Date()
	y2, M2, d2 := b.Date()

	h1, m1, s1 := a.Clock()
	h2, m2, s2 := b.Clock()

	year = y2 - y1
	month = int(M2 - M1)
	day = d2 - d1
	hour = h2 - h1
	min = m2 - m1
	sec = s2 - s1

	// Normalize negative values
	if sec < 0 {
		sec += 60
		min--
	}
	if min < 0 {
		min += 60
		hour--
	}
	if hour < 0 {
		hour += 24
		day--
	}
	if day < 0 {
		// days in month:
		t := time.Date(y1, M1, 32, 0, 0, 0, 0, time.UTC)
		day += 32 - t.Day()
		month--
	}
	if month < 0 {
		month += 12
		year--
	}
	return
}

func (b *Business) DiffTime(lang string) (func(time.Time, time.Time) string, error) {
	if b == nil {
		return nil, ErrBusinessUnbound
	}
	labels := map[string]string{}
	for _, key := range []string{"1y", "xy", "1M", "xM", "1d", "xd", "1h", "xh", "1m", "xm"} {
		value := b.timeLanguages[lang][key]
		if value == "" {
			return nil, ErrBusinessUnbound
		}
		labels[key] = value
	}
	return func(a, b time.Time) string { return FormatDiffTime(a, b, labels) }, nil
}
