package conf

import (
	"testing"
	"time"
)

func TestFlashcardCalendarDifferenceUsesOwnedLabelSnapshot(t *testing.T) {
	labels := map[string]string{}
	for _, unit := range []string{"y", "M", "d", "h", "m"} {
		labels["1"+unit] = "%s" + unit
		labels["x"+unit] = "%d%s" + unit
	}
	b := NewBusiness(nil, nil, nil, map[string]map[string]string{"owner": labels})
	format, e := b.DiffTime("owner")
	if e != nil {
		t.Fatal(e)
	}
	start := time.Date(2024, 2, 29, 10, 15, 0, 0, time.UTC)
	end := time.Date(2026, 3, 30, 12, 17, 0, 0, time.UTC)
	want := FormatDiffTime(end, start, labels)
	if format(end, start) != want || want == "" {
		t.Fatal(format(end, start), want)
	}
	labels["xy"] = "changed"
	if format(end, start) != want {
		t.Fatal("borrowed labels")
	}
	if _, e = b.DiffTime("missing"); e == nil {
		t.Fatal("missing language accepted")
	}
}
