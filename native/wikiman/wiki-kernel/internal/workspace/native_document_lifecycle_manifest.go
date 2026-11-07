package workspace

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/doclife"
)

type nativeDocumentChildrenManifest struct {
	root        string
	missing     bool
	entries     []workspaceDirectoryEntry
	directories []workspaceDirectoryManifest
}

func nativeDocumentChildrenSourceManifest(ctx context.Context, w *WorkspaceContext, root string, op workspaceDirectoryIO) (nativeDocumentChildrenManifest, error) {
	manifest := nativeDocumentChildrenManifest{root: root}
	var scan func(string) error
	scan = func(dir string) error {
		listing, err := workspaceDirectorySourceManifest(ctx, w, dir, op)
		if errors.Is(err, fs.ErrNotExist) && dir == root {
			manifest.missing = true
			return nil
		}
		if err != nil {
			return err
		}
		manifest.directories = append(manifest.directories, listing)
		for _, entry := range listing.entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			manifest.entries = append(manifest.entries, entry)
			if entry.info.Mode()&fs.ModeSymlink != 0 {
				return ErrDocument
			}
			if entry.info.IsDir() {
				if err := scan(entry.name); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := scan(root); err != nil {
		return nativeDocumentChildrenManifest{}, err
	}
	return manifest, ctx.Err()
}

func sameNativeDocumentChildrenManifest(a, b nativeDocumentChildrenManifest) bool {
	if a.root != b.root || a.missing != b.missing || len(a.entries) != len(b.entries) || len(a.directories) != len(b.directories) {
		return false
	}
	for i := range a.entries {
		if a.entries[i].name != b.entries[i].name || !sameWorkspaceDirectoryInfo(a.entries[i].info, b.entries[i].info) {
			return false
		}
	}
	for i := range a.directories {
		if !sameWorkspaceDirectoryManifest(a.directories[i], b.directories[i]) {
			return false
		}
	}
	return true
}

func confirmedNativeDocumentChildrenManifest(ctx context.Context, w *WorkspaceContext, root string, op workspaceDirectoryIO) (nativeDocumentChildrenManifest, error) {
	manifest, err := nativeDocumentChildrenSourceManifest(ctx, w, root, op)
	if err != nil {
		return nativeDocumentChildrenManifest{}, err
	}
	confirmed, err := nativeDocumentChildrenSourceManifest(ctx, w, root, op)
	if err != nil {
		return nativeDocumentChildrenManifest{}, err
	}
	if !sameNativeDocumentChildrenManifest(manifest, confirmed) {
		return nativeDocumentChildrenManifest{}, errWorkspaceDirectorySourceChanged
	}
	return manifest, nil
}

func nativeDocumentChildrenLocations(ctx context.Context, box string, manifest nativeDocumentChildrenManifest) ([]doclife.Location, error) {
	var ret []doclife.Location
	for _, entry := range manifest.entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.info.IsDir() || !strings.HasSuffix(entry.name, ".docj") {
			continue
		}
		if len(ret) >= 10000 {
			return nil, ErrBudget
		}
		ret = append(ret, doclife.Location{Box: box, Path: strings.TrimPrefix(entry.name, box), ID: strings.TrimSuffix(path.Base(entry.name), ".docj")})
	}
	return ret, nil
}

type nativeDocumentLifecycleEnumerationGuard struct {
	w        *WorkspaceContext
	op       workspaceDirectoryIO
	expected map[string]nativeDocumentChildrenManifest
}

func newNativeDocumentLifecycleEnumerationGuard(w *WorkspaceContext, op workspaceDirectoryIO) *nativeDocumentLifecycleEnumerationGuard {
	return &nativeDocumentLifecycleEnumerationGuard{w: w, op: op, expected: map[string]nativeDocumentChildrenManifest{}}
}

func (g *nativeDocumentLifecycleEnumerationGuard) children(ctx context.Context, box, p string) ([]doclife.Location, error) {
	root := box + strings.TrimSuffix(p, ".docj")
	if !fs.ValidPath(root) {
		return nil, ErrDocument
	}
	manifest, err := confirmedNativeDocumentChildrenManifest(ctx, g.w, root, g.op)
	if err != nil {
		return nil, err
	}
	if previous, ok := g.expected[root]; ok && !sameNativeDocumentChildrenManifest(previous, manifest) {
		return nil, errWorkspaceDirectorySourceChanged
	}
	g.expected[root] = manifest
	return nativeDocumentChildrenLocations(ctx, box, manifest)
}

func (g *nativeDocumentLifecycleEnumerationGuard) Confirm(ctx context.Context) error {
	roots := make([]string, 0, len(g.expected))
	for root := range g.expected {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		manifest, err := nativeDocumentChildrenSourceManifest(ctx, g.w, root, g.op)
		if err != nil {
			return err
		}
		if !sameNativeDocumentChildrenManifest(g.expected[root], manifest) {
			return errWorkspaceDirectorySourceChanged
		}
	}
	return ctx.Err()
}
