package textprefs

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOriginalKeywordEscapingWhitespaceAndDeduplication(t *testing.T) {
	old := ` First ,comma\,key,`
	got := AddKeywords(old, []string{" First ", " next,word ", " next,word ", "  ", "Last"})
	if got != `First ,comma\,key, next\,word ,Last` {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(ParseKeywords(got+"\r\nTail\n\n"), []string{"First", "comma,key", "next,word", "Last", "Tail"}) {
		t.Fatal(ParseKeywords(got))
	}
	if got = AddKeywords(`comma\,`, []string{"B"}); !reflect.DeepEqual(ParseKeywords(got), []string{"comma,", "B"}) {
		t.Fatal(got)
	}
	if AddKeywords(" old ", nil) != "old" {
		t.Fatal("empty-input trimming changed")
	}
}

func TestOriginalEmojiFilenameRulesAndUTF8Budget(t *testing.T) {
	for input, want := range map[string]string{"faces/a[bad].png": "faces/abad.png", "<x>&.png": "_x_.png", "zero\u200b.png": "zero.png", "api/icon/dynamic<.svg": "api/icon/dynamic<.svg"} {
		if got := FilterUploadEmojiFileName(input); got != want {
			t.Fatal(input, got, want)
		}
	}
	got := FilterUploadEmojiFileName(strings.Repeat("知", 100) + ".png")
	if !utf8.ValidString(got) || len(got) > 189 || !strings.HasSuffix(got, ".png") {
		t.Fatal(got)
	}
}
