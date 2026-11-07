package docast

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const docID = "20261019090001-doc0001"
const blockID = "20261019090002-par0001"

func TestParserRejectsAmbiguityAndResetsMutableState(t *testing.T) {
	p := Parser{}
	ctx := context.Background()
	for _, raw := range []string{
		`null`, `[]`, fmt.Sprintf(`{"ID":%q,"ID":%q}`, docID, docID),
		fmt.Sprintf(`{"ID":%q,"Children":[{"ID":%q}]}`, docID, docID),
		fmt.Sprintf(`{"ID":%q,"Children":{}}`, docID),
		fmt.Sprintf(`{"ID":%q,"Children":[null]}`, docID),
		fmt.Sprintf(`{"ID":%q,"TextMarkBlockRefID":"../outside"}`, docID),
		fmt.Sprintf(`{"ID":%q,"Children":[%s{}%s]}`, docID, strings.Repeat(`{"Children":[`, 513), strings.Repeat(`]}`, 513)),
	} {
		if _, _, err := p.Parse(ctx, []byte(raw), docID); err == nil {
			t.Fatal("accepted ambiguous/invalid AST", raw[:min(len(raw), 120)])
		}
		if p.nodes != nil || p.ids != nil {
			t.Fatal("parser retained failed document")
		}
		if _, _, err := p.Parse(ctx, []byte(fmt.Sprintf(`{"ID":%q}`, docID)), docID); err != nil {
			t.Fatal("failed parser poisoned next document", err)
		}
	}
}

func TestParserIndexesBothNativeReferenceRepresentations(t *testing.T) {
	raw := []byte(fmt.Sprintf(`{"Children":[{"ID":%q,"Children":[{"Children":[{"Data":%q,"Type":"NodeBlockRefID"}],"Type":"NodeBlockRef"},{"TextMarkBlockRefID":%q,"Type":"NodeTextMark"}]}],"ID":%q}`, blockID, docID, docID, docID))
	p := Parser{}
	_, refs, err := p.Parse(context.Background(), raw, docID)
	if err != nil || len(refs) != 2 {
		t.Fatal(refs, err)
	}
	for _, ref := range refs {
		if ref.SourceID != blockID || ref.TargetID != docID {
			t.Fatal(ref)
		}
	}
}
