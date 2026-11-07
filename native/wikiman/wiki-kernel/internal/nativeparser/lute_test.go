package nativeparser

import (
	"github.com/88250/lute"
	"testing"
)

func TestOriginalOptionsAreDetachedAndPreserved(t *testing.T) {
	settings := Defaults()
	a := NewEditor(EditorOptions{Markdown: settings, Spellcheck: true, ParagraphBeginningSpace: true, Emojis: map[string]string{"mine": "original"}})
	settings.InlineMath = false
	settings.InlineAsterisk = false
	b := NewEditor(EditorOptions{Markdown: settings})
	if a == b || a.ParseOptions == b.ParseOptions || a.RenderOptions == b.RenderOptions {
		t.Fatal("shared mutable parser")
	}
	if !a.ParseOptions.InlineMath || !a.ParseOptions.InlineAsterisk || b.ParseOptions.InlineMath || b.ParseOptions.InlineAsterisk {
		t.Fatal("cross-owner syntax")
	}
	if !a.ParseOptions.ProtyleWYSIWYG || a.ParseOptions.Footnotes || a.ParseOptions.YamlFrontMatter || a.RenderOptions.CodeSyntaxHighlight {
		t.Fatal("original defaults changed")
	}
	if !a.RenderOptions.ChineseParagraphBeginningSpace || b.RenderOptions.ChineseParagraphBeginningSpace {
		t.Fatal("cross-owner editor option")
	}
	standard := NewStdLute(settings)
	if !standard.ParseOptions.IndentCodeBlock || standard.ParseOptions.GFMAutoLink {
		t.Fatal("original standard import options changed")
	}
	settings.InlineMath = true
	if b.ParseOptions.InlineMath {
		t.Fatal("retained settings pointer")
	}
}

func TestCustomEmojisCannotMutateOtherParsersOrDefaults(t *testing.T) {
	settings := Defaults()
	input := map[string]string{"r02-only-a": "emoji-A"}
	a := NewEditor(EditorOptions{Markdown: settings, Emojis: input})
	input["r02-only-a"] = "outside"
	b := NewEditor(EditorOptions{Markdown: settings})
	standard := NewStdLute(settings)
	if a.GetEmojis()["r02-only-a"] != "emoji-A" {
		t.Fatal("retained caller map")
	}
	for _, engine := range []*lute.Lute{b, standard, lute.New()} {
		if _, ok := engine.GetEmojis()["r02-only-a"]; ok {
			t.Fatal("custom emoji escaped to another parser or global defaults")
		}
	}
	a.PutEmojis(map[string]string{"r02-local-update": "emoji-local"})
	if _, ok := b.GetEmojis()["r02-local-update"]; ok {
		t.Fatal("late update escaped")
	}
	if _, ok := b.ParseOptions.EmojiAlias["emoji-A"]; ok {
		t.Fatal("reverse emoji map shared")
	}
}
