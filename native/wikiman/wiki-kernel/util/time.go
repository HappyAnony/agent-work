// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package util

import (
	"strings"
	"time"
	"wiki-kernel/conf"

	"github.com/88250/go-humanize"
)

func GetTodayStart() (ret time.Time) {
	ret = time.Now()
	ret = time.Date(ret.Year(), ret.Month(), ret.Day(), 0, 0, 0, 0, time.Local)
	return
}

// Weekday returns the day of the week specified by date.
// Sunday=0, Monday=1, ..., Saturday=6.
func Weekday(date time.Time) int {
	return int(date.Weekday())
}

// WeekdayCN returns the day of the week specified by date.
// Sunday=日, Monday=一, ..., Saturday=六.
func WeekdayCN(date time.Time) string {
	week := Weekday(date)
	weekdayCN := []string{"日", "一", "二", "三", "四", "五", "六"}
	return weekdayCN[week]
}

// WeekdayCN2 returns the day of the week specified by date.
// Sunday=天, Monday=一, ..., Saturday=六.
func WeekdayCN2(date time.Time) string {
	week := Weekday(date)
	weekdayCN2 := []string{"天", "一", "二", "三", "四", "五", "六"}
	return weekdayCN2[week]
}

// ISOWeek returns the ISO 8601 year and week number in which date occurs.
// Week ranges from 1 to 53. Jan 01 to Jan 03 of year n might belong to week 52 or 53 of year n-1,
// and Dec 29 to Dec 31 might belong to week 1 of year n+1.
func ISOWeek(date time.Time) int {
	_, week := date.ISOWeek()
	return week
}

// ISOYear returns the ISO 8601 year in which date occurs.
func ISOYear(date time.Time) int {
	year, _ := date.ISOWeek()
	return year
}

// ISOMonth returns the month in which the first day of the ISO 8601 week of date occurs.
func ISOMonth(date time.Time) int {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	daysToMonday := weekday - 1
	monday := date.AddDate(0, 0, -daysToMonday)
	return int(monday.Month())
}

// ISOWeekDate returns the date of the specified day of the week in the ISO 8601 week of date.
// day: Monday=1, ..., Sunday=7.
func ISOWeekDate(day int, date time.Time) time.Time {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	daysToMonday := weekday - 1
	monday := date.AddDate(0, 0, -daysToMonday)
	return monday.AddDate(0, 0, day-1)
}

func Millisecond2Time(t int64) time.Time {
	sec := t / 1000
	msec := t % 1000
	return time.Unix(sec, msec*int64(time.Millisecond))
}

func CurrentTimeMillis() int64 {
	return time.Now().UnixMilli()
}

func CurrentTimeSecondsStr() string {
	return time.Now().Format("20060102150405")
}

func HumanizeDiffTime(a, b time.Time, lang string) string {
	labels := map[string]string{}
	for key, value := range TimeLangs[lang] {
		if str, ok := value.(string); ok {
			labels[key] = str
		}
	}
	return conf.FormatDiffTime(a, b, labels)
}

func HumanizeRelTime(a time.Time, b time.Time, lang string) string {
	_, magnitudes := humanizeTimeMagnitudes(lang)
	return strings.TrimSpace(humanize.CustomRelTime(a, b, "", "", magnitudes))
}

func HumanizeTime(then time.Time, lang string) string {
	labels, magnitudes := humanizeTimeMagnitudes(lang)
	return strings.TrimSpace(humanize.CustomRelTime(then, time.Now(), labels["albl"].(string), labels["blbl"].(string), magnitudes))
}

func humanizeTimeMagnitudes(lang string) (labels map[string]interface{}, magnitudes []humanize.RelTimeMagnitude) {
	labels = TimeLangs[lang]
	var e error
	magnitudes, e = conf.RelativeMagnitudes(labels)
	if e != nil {
		panic(e)
	}
	return
}
