package model

import (
	"encoding/json"
	"os"
	"testing"
	"time"
	"wiki-kernel/util"
)

func TestLegacyBusinessCapturesBootTimeResources(t *testing.T) {
	raw, e := os.ReadFile("../../wiki-app/appearance/langs/en_US.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Time map[string]interface{} `json:"_time"`
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	before := util.TimeLangs
	util.TimeLangs = map[string]map[string]interface{}{"en_US": v.Time}
	defer func() { util.TimeLangs = before }()
	b := legacyConfigurationBusiness()
	f, e := b.HumanizeTime("en_US")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	want := f(now.Add(-time.Hour), now)
	util.TimeLangs["en_US"]["1h"] = "changed"
	if got := f(now.Add(-time.Hour), now); got != want {
		t.Fatal("admitted labels changed", got, want)
	}
}
