package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/pageprefs"
)

type NativeStoragePlan struct {
	Value        json.RawMessage
	SourceWrites map[string]json.RawMessage
	LocalWrites  map[string]json.RawMessage
}

// WriteNativeUIState holds the same owner gate across the original read,
// merge and atomic local-state write. Its actions never write source files.
func (l *Lease) WriteNativeUIState(ctx context.Context, q pageprefs.Request) error {
	switch q.Action {
	case "local.setValue", "local.setEntry", "outline.set", "outline.remove", "recent.open", "recent.view", "recent.close":
	default:
		return ErrIdentity
	}
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		_, err := r.pagePreferences.Do(ctx, q, r.preferencesBackend(w))
		return err
	})
}

func nativePreferencePath(w *WorkspaceContext, name string) string {
	if name == "recent-doc.json" {
		return recentPath(w)
	}
	return ".local/workspaces/" + w.paths.PageID + "/temp/preferences/" + name
}
func encodeNativePreference(name string, raw []byte) ([]byte, error) {
	if name != "recent-doc.json" {
		return raw, nil
	}
	var docs []NativeRecentDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, err
	}
	return json.Marshal(nativeRecentDocument{Version: 1, Docs: docs})
}

func (r *resources) preferencesBackend(w *WorkspaceContext) pageprefs.Backend {
	localPath := func(name string) string {
		if name == "recent-doc.json" {
			return recentPath(w)
		}
		return ".local/workspaces/" + w.paths.PageID + "/temp/preferences/" + name
	}
	return pageprefs.Backend{Now: time.Now,
		Read: func(ctx context.Context, name string) ([]byte, error) {
			if name == "recent-doc.json" {
				docs, err := readNativeRecent(ctx, w)
				if err != nil {
					return nil, err
				}
				return json.Marshal(docs)
			}
			root, p := w.store.root, localPath(name)
			if name == "criteria.json" {
				if w.data == nil {
					return nil, fs.ErrNotExist
				}
				root, p = w.data, path.Join("storage", name)
			}
			f, err := openRootFile(root, p)
			if err != nil {
				return nil, err
			}
			raw, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
			err = errors.Join(err, f.Close())
			if err != nil {
				return nil, err
			}
			if len(raw) > 16<<20 {
				return nil, ErrBudget
			}
			return raw, ctx.Err()
		},
		Write: func(ctx context.Context, name string, raw []byte) error {
			if name == "criteria.json" {
				return ErrIdentity
			} // Source changes require an explicit plan and R04 commit.
			if name == "recent-doc.json" {
				var docs []NativeRecentDoc
				if err := json.Unmarshal(raw, &docs); err != nil {
					return err
				}
				var err error
				raw, err = json.Marshal(nativeRecentDocument{Version: 1, Docs: docs})
				if err != nil {
					return err
				}
			}
			return writeLocalState(ctx, w.store.root, localPath(name), raw)
		},
	}
}

// Preview source and private-state edits without writing either. The private
// runtime binding below is used only by already admitted browsing operations.
func (l *Lease) PlanNativeStorage(ctx context.Context, q pageprefs.Request) (out NativeStoragePlan, err error) {
	q.Value = append(json.RawMessage(nil), q.Value...)
	q.Keys = append([]string(nil), q.Keys...)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		b := r.preferencesBackend(w)
		out.SourceWrites = map[string]json.RawMessage{}
		out.LocalWrites = map[string]json.RawMessage{}
		b.Write = func(ctx context.Context, name string, raw []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if name == "criteria.json" {
				out.SourceWrites[path.Join("storage", name)] = append(json.RawMessage(nil), raw...)
			} else {
				encoded, e := encodeNativePreference(name, raw)
				if e != nil {
					return e
				}
				out.LocalWrites[nativePreferencePath(w, name)] = append(json.RawMessage(nil), encoded...)
			}
			return nil
		}
		v, e := r.pagePreferences.Do(ctx, q, b)
		if e != nil {
			return e
		}
		out.Value, e = json.Marshal(v)
		return e
	})
	if err != nil {
		return NativeStoragePlan{}, err
	}
	return out, nil
}
