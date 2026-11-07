// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"fmt"
	"github.com/88250/go-humanize"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	util2 "github.com/88250/lute/util"
	"github.com/Masterminds/sprig/v3"
	"github.com/araddon/dateparse"
	"github.com/spf13/cast"
	"math"
	"text/template"
	"time"
)

// Original non-I/O functions; native services are explicit and never legacy globals.
// Unbound functions are registered to report dependency errors, not parse failures
// or silently substituted empty values. The legacy facade binds its full old map.
func BuiltInTemplateFuncs(now func() time.Time, parser *lute.Lute, services template.FuncMap) template.FuncMap {
	f := sprig.TxtFuncMap()
	for _, n := range []string{"env", "expandenv", "getHostByName"} {
		delete(f, n)
	}
	f["now"] = now
	f["Weekday"] = Weekday
	f["WeekdayCN"] = WeekdayCN
	f["WeekdayCN2"] = WeekdayCN2
	f["ISOWeek"] = ISOWeek
	f["ISOYear"] = ISOYear
	f["ISOMonth"] = ISOMonth
	f["ISOWeekDate"] = ISOWeekDate
	f["pow"] = pow
	f["powf"] = powf
	f["log"] = log
	f["logf"] = logf
	f["FormatFloat"] = FormatFloat
	f["runeCount"] = runeCount
	f["wordCount"] = wordCount
	f["parseTime"] = func(s string) time.Time {
		t, e := dateparse.ParseIn(s, now().Location())
		if e != nil {
			return now()
		}
		return t
	}
	f["markdown2text"] = func(s string) (string, error) {
		if parser == nil {
			return "", ErrEnvironmentUnbound
		}
		return parse.Parse("", []byte(s), parser.ParseOptions).Root.Text(), nil
	}
	f["markdown2content"] = func(s string) (string, error) {
		if parser == nil {
			return "", ErrEnvironmentUnbound
		}
		return parse.Parse("", []byte(s), parser.ParseOptions).Root.Content(), nil
	}
	for _, n := range []string{"getHPathByID", "statBlock", "queryBlocks", "getBlock", "querySpans", "querySQL"} {
		name := n
		f[name] = func(...any) (any, error) { return nil, fmt.Errorf("%w: template %s", ErrEnvironmentUnbound, name) }
	}
	for n, fn := range services {
		f[n] = fn
	}
	return f
}
func runeCount(s string) (ret int) {
	ret, _ = util2.WordCount(s)
	return
}
func wordCount(s string) (ret int) {
	_, ret = util2.WordCount(s)
	return
}
func pow(a, b interface{}) int64    { return int64(math.Pow(cast.ToFloat64(a), cast.ToFloat64(b))) }
func powf(a, b interface{}) float64 { return math.Pow(cast.ToFloat64(a), cast.ToFloat64(b)) }
func log(a, b interface{}) int64 {
	return int64(math.Log(cast.ToFloat64(a)) / math.Log(cast.ToFloat64(b)))
}
func logf(a, b interface{}) float64 { return math.Log(cast.ToFloat64(a)) / math.Log(cast.ToFloat64(b)) }
func FormatFloat(format string, n float64) string {
	return humanize.FormatFloat(format, n)
}
func Weekday(date time.Time) int {
	return int(date.Weekday())
}
func WeekdayCN(date time.Time) string {
	week := Weekday(date)
	weekdayCN := []string{"日", "一", "二", "三", "四", "五", "六"}
	return weekdayCN[week]
}
func WeekdayCN2(date time.Time) string {
	week := Weekday(date)
	weekdayCN2 := []string{"天", "一", "二", "三", "四", "五", "六"}
	return weekdayCN2[week]
}
func ISOWeek(date time.Time) int {
	_, week := date.ISOWeek()
	return week
}
func ISOYear(date time.Time) int {
	year, _ := date.ISOWeek()
	return year
}
func ISOMonth(date time.Time) int {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	daysToMonday := weekday - 1
	monday := date.AddDate(0, 0, -daysToMonday)
	return int(monday.Month())
}
func ISOWeekDate(day int, date time.Time) time.Time {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	daysToMonday := weekday - 1
	monday := date.AddDate(0, 0, -daysToMonday)
	return monday.AddDate(0, 0, day-1)
}
