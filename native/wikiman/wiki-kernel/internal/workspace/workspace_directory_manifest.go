package workspace

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

var errWorkspaceDirectorySourceChanged = errors.New("workspace_directory_source_changed")

type workspaceDirectoryIO struct {
	openDir  func(*os.Root, string) (*os.File, error)
	readDir  func(string, *os.File, int) ([]fs.DirEntry, error)
	closeDir func(string, *os.File) error
	lstat    func(context.Context, *WorkspaceContext, string) (fs.FileInfo, error)
}

type workspaceDirectoryEntry struct {
	name string
	info fs.FileInfo
}

type workspaceDirectoryManifest struct {
	name    string
	info    fs.FileInfo
	entries []workspaceDirectoryEntry
}

func (op workspaceDirectoryIO) openDirectory(root *os.Root, name string) (*os.File, error) {
	if op.openDir != nil {
		return op.openDir(root, name)
	}
	return root.Open(name)
}

func (op workspaceDirectoryIO) readDirectory(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
	if op.readDir != nil {
		return op.readDir(name, dir, count)
	}
	return dir.ReadDir(count)
}

func (op workspaceDirectoryIO) closeDirectory(name string, dir *os.File) error {
	if op.closeDir != nil {
		return op.closeDir(name, dir)
	}
	return dir.Close()
}

func (op workspaceDirectoryIO) fileInfo(ctx context.Context, w *WorkspaceContext, name string) (fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if op.lstat != nil {
		return op.lstat(ctx, w, name)
	}
	if w.data == nil || !fs.ValidPath(name) {
		return nil, ErrDocument
	}
	return w.data.Lstat(name)
}

func sameWorkspaceDirectoryInfo(a, b fs.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func workspaceDirectoryPathInfo(ctx context.Context, w *WorkspaceContext, name string, op workspaceDirectoryIO) (fs.FileInfo, error) {
	if w.data == nil || !fs.ValidPath(name) {
		return nil, ErrDocument
	}
	if name == "." {
		info, err := op.fileInfo(ctx, w, name)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrDocument
		}
		return info, nil
	}
	parts := strings.Split(name, "/")
	var info fs.FileInfo
	for i := range parts {
		var err error
		info, err = op.fileInfo(ctx, w, strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() {
			return nil, ErrDocument
		}
	}
	return info, ctx.Err()
}

func workspaceDirectorySourceManifest(ctx context.Context, w *WorkspaceContext, name string, op workspaceDirectoryIO) (manifest workspaceDirectoryManifest, result error) {
	expected, err := workspaceDirectoryPathInfo(ctx, w, name, op)
	if err != nil {
		return workspaceDirectoryManifest{}, err
	}
	if !expected.IsDir() {
		return workspaceDirectoryManifest{}, ErrDocument
	}
	dir, err := op.openDirectory(w.data, name)
	if err != nil {
		return workspaceDirectoryManifest{}, err
	}
	defer func() {
		result = errors.Join(result, op.closeDirectory(name, dir))
		current, statErr := workspaceDirectoryPathInfo(ctx, w, name, op)
		if statErr != nil || !sameWorkspaceDirectoryInfo(current, expected) {
			result = errors.Join(result, errWorkspaceDirectorySourceChanged, statErr)
		}
		if result != nil {
			manifest = workspaceDirectoryManifest{}
		}
	}()
	opened, err := dir.Stat()
	if err != nil || !sameWorkspaceDirectoryInfo(opened, expected) {
		return workspaceDirectoryManifest{}, errors.Join(errWorkspaceDirectorySourceChanged, err)
	}
	var entries []fs.DirEntry
	for {
		if err = ctx.Err(); err != nil {
			return workspaceDirectoryManifest{}, err
		}
		part, readErr := op.readDirectory(name, dir, 128)
		entries = append(entries, part...)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return workspaceDirectoryManifest{}, readErr
		}
		if len(part) == 0 {
			return workspaceDirectoryManifest{}, io.ErrNoProgress
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	manifest = workspaceDirectoryManifest{name: name, info: expected, entries: make([]workspaceDirectoryEntry, 0, len(entries))}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return workspaceDirectoryManifest{}, err
		}
		if entry.Name() == "" || entry.Name() == "." || !fs.ValidPath(entry.Name()) || strings.Contains(entry.Name(), "/") {
			return workspaceDirectoryManifest{}, ErrDocument
		}
		child := path.Join(name, entry.Name())
		info, statErr := op.fileInfo(ctx, w, child)
		if statErr != nil {
			return workspaceDirectoryManifest{}, statErr
		}
		manifest.entries = append(manifest.entries, workspaceDirectoryEntry{name: child, info: info})
	}
	all := make([]workspaceDirectoryEntry, 0, len(manifest.entries)+1)
	all = append(all, workspaceDirectoryEntry{name: name, info: expected})
	all = append(all, manifest.entries...)
	for _, entry := range all {
		info, statErr := op.fileInfo(ctx, w, entry.name)
		if statErr != nil || !sameWorkspaceDirectoryInfo(info, entry.info) {
			return workspaceDirectoryManifest{}, errors.Join(errWorkspaceDirectorySourceChanged, statErr)
		}
	}
	return manifest, ctx.Err()
}

func sameWorkspaceDirectoryManifest(a, b workspaceDirectoryManifest) bool {
	if a.name != b.name || !sameWorkspaceDirectoryInfo(a.info, b.info) || len(a.entries) != len(b.entries) {
		return false
	}
	for i := range a.entries {
		if a.entries[i].name != b.entries[i].name || !sameWorkspaceDirectoryInfo(a.entries[i].info, b.entries[i].info) {
			return false
		}
	}
	return true
}

func confirmedWorkspaceDirectoryManifest(ctx context.Context, w *WorkspaceContext, name string, op workspaceDirectoryIO) (workspaceDirectoryManifest, error) {
	manifest, err := workspaceDirectorySourceManifest(ctx, w, name, op)
	if err != nil {
		return workspaceDirectoryManifest{}, err
	}
	confirmed, err := workspaceDirectorySourceManifest(ctx, w, name, op)
	if err != nil {
		return workspaceDirectoryManifest{}, err
	}
	if !sameWorkspaceDirectoryManifest(manifest, confirmed) {
		return workspaceDirectoryManifest{}, errWorkspaceDirectorySourceChanged
	}
	return manifest, nil
}
