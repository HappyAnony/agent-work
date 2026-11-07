package workspace

import (
	"context"
	"io/fs"
	"path"
	"strings"
)

type nativeBlockTreeManifest struct {
	entries     []workspaceDirectoryEntry
	directories []workspaceDirectoryManifest
}

func nativeBlockTreeSourceManifest(ctx context.Context, w *WorkspaceContext, op workspaceDirectoryIO) (nativeBlockTreeManifest, error) {
	manifest := nativeBlockTreeManifest{}
	var scan func(string) error
	scan = func(dir string) error {
		listing, err := workspaceDirectorySourceManifest(ctx, w, dir, op)
		if err != nil {
			return err
		}
		manifest.directories = append(manifest.directories, listing)
		if len(manifest.entries) == 0 {
			manifest.entries = append(manifest.entries, workspaceDirectoryEntry{name: ".", info: listing.info})
		}
		for _, entry := range listing.entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			manifest.entries = append(manifest.entries, entry)
			if len(manifest.entries) > 100000 {
				return ErrBudget
			}
			rel := entry.name
			if dir == "." {
				rel = path.Base(entry.name)
			}
			parts := strings.Split(rel, "/")
			if !documentID.MatchString(parts[0]) {
				continue
			}
			if entry.info.Mode()&fs.ModeSymlink != 0 {
				return ErrDocument
			}
			if entry.info.IsDir() && !strings.HasPrefix(path.Base(entry.name), ".") {
				if err := scan(entry.name); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := scan("."); err != nil {
		return nativeBlockTreeManifest{}, err
	}
	return manifest, ctx.Err()
}

func sameNativeBlockTreeManifest(a, b nativeBlockTreeManifest) bool {
	if len(a.entries) != len(b.entries) || len(a.directories) != len(b.directories) {
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

func confirmedNativeBlockTreeManifest(ctx context.Context, w *WorkspaceContext, op workspaceDirectoryIO) (nativeBlockTreeManifest, error) {
	manifest, err := nativeBlockTreeSourceManifest(ctx, w, op)
	if err != nil {
		return nativeBlockTreeManifest{}, err
	}
	confirmed, err := nativeBlockTreeSourceManifest(ctx, w, op)
	if err != nil {
		return nativeBlockTreeManifest{}, err
	}
	if !sameNativeBlockTreeManifest(manifest, confirmed) {
		return nativeBlockTreeManifest{}, errWorkspaceDirectorySourceChanged
	}
	return manifest, nil
}

func confirmFinalNativeBlockTreeManifest(ctx context.Context, w *WorkspaceContext, expected nativeBlockTreeManifest, op workspaceDirectoryIO) error {
	manifest, err := nativeBlockTreeSourceManifest(ctx, w, op)
	if err != nil {
		return err
	}
	if !sameNativeBlockTreeManifest(expected, manifest) {
		return errWorkspaceDirectorySourceChanged
	}
	return nil
}
