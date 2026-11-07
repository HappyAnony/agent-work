package conf

import "testing"

func TestDefaultEditorsDoNotShareMarkdownPointer(t *testing.T) {
	a, b := NewEditor(), NewEditor()
	original := b.Markdown.InlineMath
	a.Markdown.InlineMath = !original
	if a.Markdown == b.Markdown || b.Markdown.InlineMath != original {
		t.Fatal("shared editor defaults")
	}
}
