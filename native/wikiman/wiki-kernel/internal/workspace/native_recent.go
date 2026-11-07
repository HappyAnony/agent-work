package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/pageprefs"
)

type NativeRecentDoc = pageprefs.RecentDoc
type nativeRecentDocument struct {
	Version int               `json:"version"`
	Docs    []NativeRecentDoc `json:"docs"`
}

func recentPath(w *WorkspaceContext) string {
	return ".local/workspaces/" + w.paths.PageID + "/temp/recent-docs.json"
}

func readNativeRecent(ctx context.Context, w *WorkspaceContext) ([]NativeRecentDoc, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	f, e := openRootFile(w.store.root, recentPath(w))
	if errors.Is(e, fs.ErrNotExist) {
		return []NativeRecentDoc{}, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if e != nil {
		return nil, e
	}
	if len(raw) > 1<<20 {
		return nil, ErrBudget
	}
	var d nativeRecentDocument
	if e = json.Unmarshal(raw, &d); e != nil {
		return nil, e
	}
	if d.Version != 1 || len(d.Docs) > 32 {
		return nil, ErrDocument
	}
	seen := map[string]bool{}
	for _, v := range d.Docs {
		if !documentID.MatchString(v.RootID) || seen[v.RootID] {
			return nil, ErrDocument
		}
		seen[v.RootID] = true
	}
	return d.Docs, ctx.Err()
}
func saveNativeRecent(ctx context.Context, w *WorkspaceContext, s docview.Recent) error {
	raw, err := json.Marshal(NativeRecentDoc{RootID: s.ID, Icon: s.Icon, Title: s.Title})
	if err != nil {
		return err
	}
	_, err = w.resources.pagePreferences.Do(ctx, pageprefs.Request{Action: "recent.record", Value: raw}, w.resources.preferencesBackend(w))
	return err
}

// Local browsing state only: no source journal/receipt, goroutine or close-time
// write. GetDoc persists synchronously after successful rendering. Cancellation
// before rename rejects the write; a committed local rename is not rolled back.
func (l *Lease) NativeRecentDocs(ctx context.Context) (ret []NativeRecentDoc, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { var e error; ret, e = readNativeRecent(ctx, l.workspace); return e })
	if err != nil {
		return nil, err
	}
	return
}
