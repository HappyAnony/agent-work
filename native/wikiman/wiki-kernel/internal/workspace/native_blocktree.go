package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

func (r *resources) locatedTree(ctx context.Context, w *WorkspaceContext, box, doc string) (*parse.Tree, error) {
	tree, err := r.originalTree(ctx, w, box, doc)
	if err != nil {
		return nil, err
	}
	return locateOriginalTree(ctx, w, tree, box)
}

func locateOriginalTree(ctx context.Context, w *WorkspaceContext, tree *parse.Tree, box string) (*parse.Tree, error) {
	var err error
	tree.HPath, err = originalHPath(ctx, w, box, tree.Path, tree.Root.IALAttr("title"))
	if err != nil {
		return nil, err
	}
	ast.Walk(tree.Root, func(n *ast.Node, e bool) ast.WalkStatus {
		if e {
			n.Box = box
			n.Path = tree.Path
		}
		return ast.WalkContinue
	})
	return tree, nil
}
func (l *Lease) RebuildNativeBlockTree(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return l.workspace.resources.rebuildBlockTree(ctx, l.workspace) })
}
func (r *resources) rebuildBlockTree(ctx context.Context, w *WorkspaceContext) error {
	return r.rebuildBlockTreeWithIO(ctx, w, workspaceDirectoryIO{})
}
func (r *resources) rebuildBlockTreeWithIO(ctx context.Context, w *WorkspaceContext, op workspaceDirectoryIO) error {
	if w.data == nil {
		return ErrDocument
	}
	manifest, err := confirmedNativeBlockTreeManifest(ctx, w, op)
	if err != nil {
		return err
	}
	var rows []indexdb.BlockTreeRecord
	seen := map[string]bool{}
	seenBlocks := map[string]bool{}
	for _, entry := range manifest.entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		name := entry.name
		if name == "." {
			continue
		}
		parts := strings.Split(name, "/")
		if !documentID.MatchString(parts[0]) {
			continue
		}
		if entry.info.Mode()&fs.ModeSymlink != 0 {
			return ErrDocument
		}
		if entry.info.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".docj") {
			continue
		}
		doc := strings.TrimSuffix(path.Base(name), ".docj")
		if !documentID.MatchString(doc) || seen[doc] {
			return ErrDocument
		}
		seen[doc] = true
		raw, e := readNativeFile(ctx, w, name, 16<<20)
		if e != nil {
			return e
		}
		if _, _, e = r.parser.Parse(ctx, raw, doc); e != nil {
			return e
		}
		options, e := r.configuredParserOptions(ctx, w)
		if e != nil {
			return e
		}
		input, e := docast.OriginalTreeInput(raw)
		if e != nil {
			return e
		}
		tree, e := dataparser.ParseJSONWithoutFix(input, nativeparser.NewConfiguredIndex(options).ParseOptions)
		if e != nil {
			return e
		}
		tree.ID, tree.Box, tree.Path = doc, parts[0], strings.TrimPrefix(name, parts[0])
		tree.HPath, e = originalHPath(ctx, w, parts[0], tree.Path, tree.Root.IALAttr("title"))
		if e != nil {
			return e
		}
		projected := indexdb.ProjectBlockTree(tree)
		for _, b := range projected {
			if seenBlocks[b.ID] {
				return ErrDocument
			}
			seenBlocks[b.ID] = true
		}
		rows = append(rows, projected...)
		if len(rows) > 1000000 {
			return ErrBudget
		}
	}
	if err = confirmFinalNativeBlockTreeManifest(ctx, w, manifest, op); err != nil {
		return err
	}
	db, err := r.native.Database(indexdb.BlockTree)
	if err != nil {
		return err
	}
	if err = (indexdb.BlockTrees{DB: db}).Replace(ctx, "", rows); err != nil {
		return err
	}
	r.blockTreeReady = true
	return nil
}
func (l *Lease) NativeBlockTrees(ctx context.Context, filter indexdb.BlockTreeFilter) (rows []indexdb.BlockTreeRecord, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		rows, e = (indexdb.BlockTrees{DB: db}).Query(ctx, filter)
		return e
	})
	return
}
func (l *Lease) NativeBlockTreeCount(ctx context.Context, filter indexdb.BlockTreeFilter) (count int, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		count, e = (indexdb.BlockTrees{DB: db}).Count(ctx, filter)
		return e
	})
	return
}
func (l *Lease) RemoveNativeBlockTrees(ctx context.Context, filter indexdb.BlockTreeFilter) (ids []string, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		ids, e = (indexdb.BlockTrees{DB: db}).Remove(ctx, filter)
		return e
	})
	return
}

// Snapshot refresh removes deleted rows; the original incremental Upsert remains
// available separately because its deletion/descendant semantics are different.
func (l *Lease) IndexNativeBlockTree(ctx context.Context, box, doc string, upsert bool) error {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) {
		return ErrDocument
	}
	return l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		tree, e := l.workspace.resources.locatedTree(ctx, l.workspace, box, doc)
		if e != nil {
			return e
		}
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		q := indexdb.BlockTrees{DB: db}
		if upsert {
			return q.Upsert(ctx, tree)
		}
		return q.Index(ctx, tree)
	})
}
func (r *resources) nativeDefinition(ctx context.Context, id string) (indexdb.BlockLocation, bool, error) {
	if !r.blockTreeReady {
		return indexdb.BlockLocation{}, false, ErrStale
	}
	db, err := r.native.Database(indexdb.BlockTree)
	if err != nil {
		return indexdb.BlockLocation{}, false, err
	}
	rows, err := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if err != nil || len(rows) == 0 {
		return indexdb.BlockLocation{}, false, err
	}
	b := rows[0]
	return indexdb.BlockLocation{ParentID: b.ParentID, RootID: b.RootID, Path: b.Path}, true, nil
}
func docFromNativePath(p string) string { return strings.TrimSuffix(path.Base(p), ".docj") }

func (l *Lease) NativeBlockTreePathDifference(ctx context.Context, box string, paths []string) (redundant, missing []string, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		redundant, missing, e = (indexdb.BlockTrees{DB: db}).PathDifference(ctx, box, paths)
		return e
	})
	return
}
func (l *Lease) NativeBlockTreeRootUpdated(ctx context.Context) (ret map[string]string, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		ret, e = (indexdb.BlockTrees{DB: db}).RootUpdated(ctx)
		return e
	})
	return
}
