package conf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestR03ReadBusinessRetainsInheritedLabelsAndTimeLanguages(t *testing.T) {
	files, e := filepath.Glob("../../wiki-app/appearance/langs/*.json")
	if e != nil || len(files) == 0 {
		t.Fatal(files, e)
	}
	business := NewReadBusiness(nil)
	for _, file := range files {
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var original struct {
			Kernel map[string]string `json:"_kernel"`
			Time   map[string]string `json:"_time"`
		}
		if e = json.Unmarshal(raw, &original); e != nil {
			t.Fatal(e)
		}
		lang := filepath.Base(file)
		lang = lang[:len(lang)-5]
		for _, id := range []int{53, 105, 259, 260, 261, 262, 263, 264, 265} {
			if got := business.Language(lang, id, false, 0); got != original.Kernel[strconv.Itoa(id)] {
				t.Fatal(lang, id, got)
			}
		}
		format, e := business.Duration(lang)
		if e != nil {
			t.Fatal(lang, e)
		}
		base := time.Unix(0, 0)
		if format(base, base.Add(time.Minute)) == "" {
			t.Fatal(lang, "missing original duration")
		}
		if _, e = business.HumanizeTime(lang); e != nil {
			t.Fatal(lang, e)
		}
	}
}
