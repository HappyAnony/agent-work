package nativeparser

import (
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
)

// Lute.NewOptions references process-wide default emoji maps. A fresh Lute
// pointer alone is therefore NOT a workspace boundary: PutEmojis would mutate
// defaults seen by another page. Detach both directions before lending it out.
func detachEmojis(engine *lute.Lute) {
	parse.EmojiLock.Lock()
	defer parse.EmojiLock.Unlock()
	alias := make(map[string]string, len(engine.ParseOptions.AliasEmoji))
	for k, v := range engine.ParseOptions.AliasEmoji {
		alias[k] = v
	}
	unicode := make(map[string]string, len(engine.ParseOptions.EmojiAlias))
	for k, v := range engine.ParseOptions.EmojiAlias {
		unicode[k] = v
	}
	engine.ParseOptions.AliasEmoji = alias
	engine.ParseOptions.EmojiAlias = unicode
}
