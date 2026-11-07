// SPDX-License-Identifier: AGPL-3.0-or-later
package docast

import (
	"bytes"
	"errors"
	"testing"
)

func TestR04SerializerRefusesAmbiguousNestedInlineExtensions(t *testing.T) {
	before := []byte(`{"ID":"doc","Type":"NodeDocument","Children":[{"ID":"p","Type":"NodeParagraph","Children":[{"Type":"NodeStrong","Children":[{"Type":"NodeText","Data":"same","Future":{"number":900719925474099312345}}]}]}]}`)
	same := []byte(`{"ID":"doc","Type":"NodeDocument","Children":[{"ID":"p","Type":"NodeParagraph","Children":[{"Type":"NodeStrong","Children":[{"Type":"NodeText","Data":"same"}]}]}]}`)
	got, e := SerializeDocument([][]byte{before}, same)
	if e != nil || !bytes.Contains(got, []byte("900719925474099312345")) {
		t.Fatal(string(got), e)
	}
	ambiguous := [][]byte{
		bytes.Replace(same, []byte(`"Type":"NodeStrong"`), []byte(`"Type":"NodeStrong","ID":"other"`), 1),
		bytes.Replace(same, []byte(`"Data":"same"`), []byte(`"Data":"changed"`), 1),
		bytes.Replace(same, []byte(`[{"Type":"NodeStrong"`), []byte(`[{"Type":"NodeText","Data":"new"},{"Type":"NodeStrong"`), 1),
	}
	for _, raw := range ambiguous {
		if _, e := SerializeDocument([][]byte{before}, raw); !errors.Is(e, ErrOpaqueEdit) {
			t.Fatal(string(raw), e)
		}
	}
}

func TestR04AttributeViewExtensionsFollowOptionIdentity(t *testing.T) {
	before := []byte(`{"keyValues":[{"key":{"id":"status","options":[{"name":"First","future":{"number":900719925474099312345}},{"name":"Second"}]}}]}`)
	reordered := []byte(`{"keyValues":[{"key":{"id":"status","options":[{"name":"Second","color":"2"},{"name":"First","color":"1"}]}}]}`)
	got, e := SerializeAttributeView(before, reordered)
	if e != nil || !bytes.Contains(got, []byte(`"future":{"number":900719925474099312345},"name":"First"`)) || bytes.Contains(got, []byte(`"future":{"number":900719925474099312345},"name":"Second"`)) {
		t.Fatal(string(got), e)
	}
	ambiguous := bytes.Replace(reordered, []byte("First"), []byte("New"), 1)
	if _, e = SerializeAttributeView(before, ambiguous); !errors.Is(e, ErrOpaqueEdit) {
		t.Fatal(e)
	}
}
