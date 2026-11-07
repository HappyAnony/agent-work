package workspace

import (
	"context"
	"errors"
	"html"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/nativeparser"
)

// Original emoji listing accepts one directory level. Native reads refuse bad
// names/symlinks instead of executing the old read-time rename repair.
func validEmojiName(name string) bool {
	decoded := html.UnescapeString(name)
	return strings.TrimSpace(decoded) == decoded && !strings.ContainsAny(decoded, "\\/:*?\"'<>|~[]()!{}=#%$;") && !strings.HasSuffix(decoded, ".")
}

func (r *resources) ownedEmojis(ctx context.Context, w *WorkspaceContext) (map[string]string, error) {
	return r.ownedEmojisWithIO(ctx, w, emojiManifestIO{})
}

func (r *resources) ownedEmojisWithIO(ctx context.Context, w *WorkspaceContext, op emojiManifestIO) (map[string]string, error) {
	values := map[string]string{}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if w.data == nil {
		r.emojis.Replace(values)
		return values, nil
	}
	manifest, e := confirmedEmojiManifest(ctx, w, op)
	if errors.Is(e, fs.ErrNotExist) {
		r.emojis.Replace(values)
		return values, ctx.Err()
	}
	if e != nil {
		return nil, e
	}
	for _, entry := range manifest[1:] {
		if entry.info.IsDir() {
			continue
		}
		if len(values) >= 10000 {
			return nil, ErrBudget
		}
		short := strings.TrimPrefix(entry.name, "emojis/")
		values[strings.TrimSuffix(short, path.Ext(short))] = "/emojis/" + short
	}
	r.emojis.Replace(values)
	return values, ctx.Err()
}
func (r *resources) configuredParserOptions(ctx context.Context, w *WorkspaceContext) (nativeparser.EditorOptions, error) {
	emojis, e := r.ownedEmojis(ctx, w)
	if e != nil {
		return nativeparser.EditorOptions{}, e
	}
	return r.configuration.ParserOptions(emojis)
}
func (l *Lease) CustomEmojis(ctx context.Context) (out map[string]string, err error) {
	pin, e := l.pin(true)
	if e != nil {
		return nil, e
	}
	defer pin.Release()
	err = pin.sourceRead(ctx, func() error {
		return pin.workspace.resources.serial(ctx, func() error { var e error; out, e = pin.workspace.resources.ownedEmojis(ctx, pin.workspace); return e })
	})
	if err != nil {
		return nil, err
	}
	return
}
