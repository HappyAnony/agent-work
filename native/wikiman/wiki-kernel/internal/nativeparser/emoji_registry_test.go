package nativeparser

import (
	"sync"
	"testing"
)

func TestEmojiRegistryOwnershipAndIndexParser(t *testing.T) {
	var a, b EmojiRegistry
	source := map[string]string{"r02-registry": "/emojis/a.png"}
	a.Replace(source)
	source["r02-registry"] = "outside"
	b.Put("r02-registry", "/emojis/b.png")
	options := EditorOptions{Markdown: Defaults(), Emojis: a.Snapshot()}
	index := NewConfiguredIndex(options)
	options.Emojis["r02-registry"] = "mutated"
	if index.GetEmojis()["r02-registry"] != "/emojis/a.png" || index.RenderOptions.KramdownBlockIAL {
		t.Fatal("index contract or emoji ownership changed")
	}
	if NewConfiguredIndex(EditorOptions{Markdown: Defaults()}).GetEmojis()["r02-registry"] != "" {
		t.Fatal("global defaults changed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); a.Put("local", "value"); a.Snapshot(); a.Clear() }()
	}
	wg.Wait()
	a.Close()
	a.Replace(source)
	a.Put("late", "value")
	if len(a.Snapshot()) != 0 || b.Snapshot()["r02-registry"] != "/emojis/b.png" {
		t.Fatal("closed owner leaked")
	}
}
