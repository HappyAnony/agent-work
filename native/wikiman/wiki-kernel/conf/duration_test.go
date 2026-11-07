package conf

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestDurationOriginalLanguagesAreDetachedAndExplicit(t *testing.T) {
	all := map[string]map[string]string{}
	for _, lang := range []string{"en_US", "zh_CN"} {
		raw, e := os.ReadFile("../../wiki-app/appearance/langs/" + lang + ".json")
		if e != nil {
			t.Fatal(e)
		}
		var src struct {
			Time map[string]string `json:"_time"`
		}
		if e = json.Unmarshal(raw, &src); e != nil {
			t.Fatal(e)
		}
		all[lang] = src.Time
	}
	b := NewBusiness(nil, nil, nil, all)
	en, e := b.Duration("en_US")
	if e != nil {
		t.Fatal(e)
	}
	zh, e := b.Duration("zh_CN")
	if e != nil {
		t.Fatal(e)
	}
	a := time.Unix(1000, 0)
	for _, tc := range []struct {
		d      time.Duration
		en, zh string
	}{{time.Second, "1 second", "1 秒"}, {time.Minute, "1 minute", "1 分钟"}, {2 * time.Hour, "2 hours", "2 小时"}, {24 * time.Hour, "1 day", "1 天"}} {
		if en(a, a.Add(tc.d)) != tc.en || zh(a, a.Add(tc.d)) != tc.zh {
			t.Fatal(tc, en(a, a.Add(tc.d)), zh(a, a.Add(tc.d)))
		}
	}
	all["en_US"]["1m"] = "polluted"
	if en(a, a.Add(time.Minute)) != "1 minute" {
		t.Fatal("shared language input")
	}
	if _, e = b.Duration("missing"); !errors.Is(e, ErrBusinessUnbound) {
		t.Fatal(e)
	}
	if _, e = NewBusiness(nil, nil, nil, map[string]map[string]string{"en_US": {"now": "now"}}).Duration("en_US"); !errors.Is(e, ErrBusinessUnbound) {
		t.Fatal(e)
	}
}

func TestFileTreeHumanizeTimeIncludesOriginalSuffixesAndSnapshots(t *testing.T) {
	raw, err := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if err != nil {
		t.Fatal(err)
	}
	var src struct {
		Time map[string]string `json:"_time"`
	}
	if err = json.Unmarshal(raw, &src); err != nil {
		t.Fatal(err)
	}
	b := NewBusiness(nil, nil, nil, map[string]map[string]string{"en_US": src.Time})
	format, err := b.HumanizeTime("en_US")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100000, 0)
	if got := format(now.Add(-time.Minute), now); got != "1 minute ago" {
		t.Fatal(got)
	}
	if got := format(now.Add(time.Minute), now); got != "1 minute from now" {
		t.Fatal(got)
	}
	src.Time["albl"] = "changed"
	if got := format(now.Add(-time.Minute), now); got != "1 minute ago" {
		t.Fatal("language input aliases formatter", got)
	}
	if _, err = b.HumanizeTime("missing"); !errors.Is(err, ErrBusinessUnbound) {
		t.Fatal(err)
	}
}
