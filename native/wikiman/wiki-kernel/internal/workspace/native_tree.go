package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// QueueNativeTree reads the actual pinned source. No caller-supplied row/string
// can stand in for the original AST/asset/OCR projection. It does not save source.
func (l *Lease) QueueNativeTree(ctx context.Context, box, doc, action string) error {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) || (action != "index" && action != "upsert") {
		return ErrDocument
	}
	return l.enqueueNativeMain(ctx, func(r *resources) error {
		if !r.blockTreeReady {
			if err := r.rebuildBlockTree(ctx, l.workspace); err != nil {
				return err
			}
		}
		tree, err := r.locatedTree(ctx, l.workspace, box, doc)
		if err != nil {
			return err
		}
		treeDB, err := r.native.Database(indexdb.BlockTree)
		if err != nil {
			return err
		}
		if err = (indexdb.BlockTrees{DB: treeDB}).Replace(ctx, doc, indexdb.ProjectBlockTree(tree)); err != nil {
			return err
		}

		settings, err := r.configuredParserOptions(ctx, l.workspace)
		if err != nil {
			return err
		}
		records, err := indexdb.ProjectTree(ctx, tree, nativeparser.NewConfiguredIndex(settings), indexdb.TreeServices{
			Content: r.nativeContent, PrepareOCR: r.nativeOCR, LocalDefinitions: true,
			IndexAssetPath: r.configuration.Search != nil && r.configuration.Search.IndexAssetPath,
			AssetHash: func(ctx context.Context, dest, box, doc string) (string, error) {
				return originalAssetHash(ctx, l.workspace, dest, box, doc)
			},
		}, func(ctx context.Context, id string) (indexdb.BlockLocation, bool, error) {
			return r.nativeDefinition(ctx, id)
		})
		if err != nil {
			return err
		}
		data, err := readNativeFile(ctx, l.workspace, ".wiki/indexignore", 1<<20)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		records.IgnoreLines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		return r.contentQueue.Enqueue(indexdb.MainMutation{Action: action, ID: doc, Tree: &records})
	})
}

// Reject symlink components, special files and oversized reads without creating
// config/default files. os.Root additionally confines all resolution to this page.
func openNativeFile(w *WorkspaceContext, name string) (*os.File, error) {
	if w.data == nil || !fs.ValidPath(name) {
		return nil, ErrDocument
	}
	components := strings.Split(name, "/")
	for i := range components {
		info, err := w.data.Lstat(strings.Join(components[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrDocument
		}
		if i < len(components)-1 && !info.IsDir() {
			return nil, fs.ErrNotExist
		}
		if i == len(components)-1 && !info.Mode().IsRegular() {
			return nil, ErrDocument
		}
	}
	return w.data.Open(name)
}
func readNativeFile(ctx context.Context, w *WorkspaceContext, name string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := openNativeFile(w, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrDocument
	}
	return data, ctx.Err()
}
func originalAssetHash(ctx context.Context, w *WorkspaceContext, dest, box, doc string) (string, error) {
	if !strings.HasPrefix(dest, "assets/") || !fs.ValidPath(dest) || strings.Contains(dest, "\\") {
		return "", ErrDocument
	}
	// Original doc-directory rule intentionally appends to the complete doc path.
	for _, candidate := range []string{path.Join(box, strings.TrimPrefix(doc, "/"), dest), path.Join(box, dest), dest} {
		f, err := openNativeFile(w, candidate)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return "", err
		}
		hash, err := indexdb.AssetEtag(ctx, f, info.Size())
		f.Close()
		if err == nil && hash != "" {
			w.resources.nativeAssets.SetHash(hash, candidate)
		}
		return hash, err
	}
	return "", nil // Original missing asset is recorded with an empty hash.
}
func originalHPath(ctx context.Context, w *WorkspaceContext, box, doc, title string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(path.Dir(doc), "/"), "/")
	titles := []string{}
	for i, id := range parts {
		if !documentID.MatchString(id) {
			continue
		}
		data, err := readNativeFile(ctx, w, path.Join(box, strings.Join(parts[:i+1], "/")+".docj"), 16<<20)
		if err != nil {
			return "", err
		}
		var node struct {
			ID         string
			Properties map[string]json.RawMessage
		}
		if err = json.Unmarshal(data, &node); err != nil || node.ID != id {
			return "", ErrDocument
		}
		var parentTitle string
		if title := node.Properties["title"]; title != nil && json.Unmarshal(title, &parentTitle) != nil {
			return "", ErrDocument
		}
		if parentTitle == "" {
			parentTitle = "Untitled"
		}
		titles = append(titles, html.UnescapeString(parentTitle))
	}
	return "/" + strings.Join(append(titles, title), "/"), nil
}

// NativeBlock is a bounded, owner-local copied SQL cache. All committed main
// operations invalidate it under the DB gate before external progress callbacks.
func (l *Lease) NativeBlock(ctx context.Context, id string) (row indexdb.BlockRecord, found bool, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		row, found, e = l.workspace.resources.nativeBlock(ctx, id)
		return e
	})
	return
}
func (r *resources) nativeBlock(ctx context.Context, id string) (indexdb.BlockRecord, bool, error) {
	db, e := r.native.Database(indexdb.Main)
	if e != nil {
		return indexdb.BlockRecord{}, false, e
	}
	return (indexdb.CachedBlocks{DB: db, Cache: &r.nativeBlocks}).Get(ctx, id)
}
