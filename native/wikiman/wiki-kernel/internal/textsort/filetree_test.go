package textsort

import "testing"

func TestFileTreeAlphabetRulePrefixesEmojiSuffixAndCase(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"a.docj", "ab.docj", true}, {"ab.docj", "a.docj", false}, {"A.docj", "a.docj", false}, {"🚀a.docj", "b.docj", true}, {"b.docj", "🚀a.docj", false}, {"Alpha.docj", "Beta.docj", true}} {
		if got := FileTreeCompare(tc.a, tc.b); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
