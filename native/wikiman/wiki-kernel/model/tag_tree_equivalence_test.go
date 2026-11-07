package model

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/tagquery"
)

func TestOriginalTagTreeAllSortModesAndLimit(t *testing.T) {
	old := Conf
	Conf = NewAppConfWithBusiness(nil)
	Conf.Tag = conf.NewTag()
	defer func() { Conf = old }()
	labels := map[string]int{"Root": 2, "Root/child2": 1, "Root/child10": 3, "中文": 4, "AGI2": 5, "AGI10": 6, "R&X": 7}
	for _, mode := range []int{0, 1, 4, 5, 7, 8, 99} {
		Conf.Tag.Sort = mode
		originalLabels := map[string]Tags{}
		for k, count := range labels {
			originalLabels[k] = make(Tags, count-1)
		}
		original := Tags{}
		for label := range originalLabels {
			original = buildTags(original, strings.Split(label, "/"), 0)
		}
		appendTagChildren(&original, originalLabels)
		sortTags(original)
		for _, limit := range []int{1, 100} {
			for _, ignore := range []bool{true, false} {
				want := Tags{}
				total := 0
				exceeded := false
				for _, n := range original {
					want = append(want, n)
					countTag(n, &total)
					if limit < total && !ignore {
						exceeded = true
						break
					}
				}
				got, over, e := tagquery.Build(context.Background(), labels, mode, limit, ignore)
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(want)
				if e != nil || over != exceeded || string(a) != string(b) {
					t.Fatalf("mode=%d limit=%d ignore=%v got=%s want=%s err=%v", mode, limit, ignore, a, b, e)
				}
			}
		}
	}
}
