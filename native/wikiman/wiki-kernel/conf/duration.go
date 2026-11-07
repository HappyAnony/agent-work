// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

import (
	"fmt"
	"github.com/88250/go-humanize"
	"math"
	"strings"
	"time"
)

// RelativeMagnitudes is the original time formatter's threshold table. Labels
// are explicit owner resources, not a device-global language or disk lookup.
func RelativeMagnitudes(labels map[string]interface{}) ([]humanize.RelTimeMagnitude, error) {
	keys := []string{"now", "1s", "xs", "1m", "xm", "1h", "xh", "1d", "xd", "1w", "xw", "1M", "xM", "1y", "2y", "xy", "max"}
	limits := []time.Duration{time.Second, 2 * time.Second, time.Minute, 2 * time.Minute, time.Hour, 2 * time.Hour, humanize.Day, 2 * humanize.Day, humanize.Week, 2 * humanize.Week, humanize.Month, 2 * humanize.Month, humanize.Year, 23 * humanize.Month, 2 * humanize.Year, humanize.LongTime, math.MaxInt64}
	units := []time.Duration{time.Second, 1, time.Second, 1, time.Minute, 1, time.Hour, 1, humanize.Day, 1, humanize.Week, 1, humanize.Month, 1, 1, humanize.Year, 1}
	ret := make([]humanize.RelTimeMagnitude, len(keys))
	for i, k := range keys {
		s, ok := labels[k].(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("%w: time label %s", ErrBusinessUnbound, k)
		}
		ret[i] = humanize.RelTimeMagnitude{D: limits[i], Format: s, DivBy: units[i]}
	}
	return ret, nil
}

// Duration snapshots all labels before returning a formatter; later changes to
// the caller's input maps cannot change an admitted operation's language.
func (b *Business) Duration(lang string) (func(time.Time, time.Time) string, error) {
	if b == nil {
		return nil, ErrBusinessUnbound
	}
	labels := map[string]interface{}{}
	for k, v := range b.timeLanguages[lang] {
		labels[k] = v
	}
	magnitudes, e := RelativeMagnitudes(labels)
	if e != nil {
		return nil, e
	}
	return func(a, b time.Time) string {
		return strings.TrimSpace(humanize.CustomRelTime(a, b, "", "", magnitudes))
	}, nil
}

// HumanizeTime keeps the file tree's original before/after labels.
func (b *Business) HumanizeTime(lang string) (func(time.Time, time.Time) string, error) {
	if b == nil {
		return nil, ErrBusinessUnbound
	}
	labels := map[string]interface{}{}
	for k, v := range b.timeLanguages[lang] {
		labels[k] = v
	}
	magnitudes, err := RelativeMagnitudes(labels)
	if err != nil {
		return nil, err
	}
	before, ok := labels["albl"].(string)
	if !ok {
		return nil, ErrBusinessUnbound
	}
	after, ok := labels["blbl"].(string)
	if !ok {
		return nil, ErrBusinessUnbound
	}
	return func(then, now time.Time) string {
		return strings.TrimSpace(humanize.CustomRelTime(then, now, before, after, magnitudes))
	}, nil
}
