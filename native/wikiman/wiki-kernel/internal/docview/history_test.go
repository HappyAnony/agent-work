package docview

import (
	"context"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestHistoryRenderOriginalHighlightLargeAndBorrowedParser(t *testing.T) {
	raw := []byte(`{"ID":"20261104203000-hist001","Type":"NodeDocument","Properties":{"id":"20261104203000-hist001","title":"history"},"Children":[{"ID":"20261104203001-hist002","Type":"NodeParagraph","Properties":{"id":"20261104203001-hist002","fold":"1"},"Children":[{"Type":"NodeText","Data":"history needle"}]}]}`)
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{Markdown: nativeparser.Defaults()})
	editable := parser.RenderOptions.ProtyleContenteditable
	out, e := RenderHistory(context.Background(), raw, "needle", true, false, parser)
	if e != nil || out.ID != "20261104203000-hist001" || out.Large || !strings.Contains(out.Content, "search-mark") || strings.Contains(out.Content, `contenteditable="true"`) || strings.Contains(out.Content, `fold="1"`) {
		t.Fatal(out, e)
	}
	if parser.RenderOptions.ProtyleContenteditable != editable {
		t.Fatal("borrowed parser modified")
	}
	padded := append(append([]byte(nil), raw...), []byte(strings.Repeat(" ", 1<<20))...)
	out, e = RenderHistory(context.Background(), padded, "needle", true, false, parser)
	if e != nil || !out.Large || !strings.Contains(out.Content, "history needle") || strings.Contains(out.Content, "search-mark") {
		t.Fatal(out, e)
	}
	if out, e = RenderHistory(context.Background(), []byte("corrupt"), "", false, false, parser); e == nil || out.ID != "" || out.Content != "" {
		t.Fatal(out, e)
	}
}
