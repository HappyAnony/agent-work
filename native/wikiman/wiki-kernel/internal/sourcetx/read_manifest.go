package sourcetx

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

var errReadSourceChanged = errors.New("read_source_changed")

type readManifestIO struct {
	readDir func(*os.Root, string) ([]fs.DirEntry, error)
	lstat   func(*os.Root, string) (fs.FileInfo, error)
	open    func(*os.Root, string) (*os.File, error)
}

type readManifestOptions struct {
	skipHiddenDirectories bool
	singleLevel           bool
	maxEntries            int
}

type readManifestEntry struct {
	name string
	info fs.FileInfo
}

func readInfoEqual(a, b fs.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func sourceReadManifest(ctx context.Context, root *os.Root, base string, options readManifestOptions, op readManifestIO) ([]readManifestEntry, bool, error) {
	if op.readDir == nil {
		op.readDir = func(root *os.Root, name string) ([]fs.DirEntry, error) { return fs.ReadDir(root.FS(), name) }
	}
	if op.lstat == nil {
		op.lstat = func(root *os.Root, name string) (fs.FileInfo, error) { return root.Lstat(name) }
	}
	if err := safePath(root, base); err != nil {
		return nil, false, err
	}
	baseInfo, err := op.lstat(root, base)
	if errors.Is(err, os.ErrNotExist) {
		return []readManifestEntry{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	manifest := []readManifestEntry{}
	var walk func(string, fs.FileInfo) error
	walk = func(name string, info fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		manifest = append(manifest, readManifestEntry{name: name, info: info})
		if (options.maxEntries > 0 && len(manifest) > options.maxEntries) || info.Mode()&fs.ModeSymlink != 0 {
			return ErrInvalid
		}
		if !info.IsDir() || (options.singleLevel && name != base) || (options.skipHiddenDirectories && name != base && strings.HasPrefix(path.Base(name), ".")) {
			return nil
		}
		entries, err := op.readDir(root, name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := path.Join(name, entry.Name())
			childInfo, err := op.lstat(root, child)
			if err != nil {
				return err
			}
			if err := walk(child, childInfo); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(base, baseInfo); err != nil {
		return nil, false, err
	}
	for _, entry := range manifest {
		info, err := op.lstat(root, entry.name)
		if err != nil || !readInfoEqual(info, entry.info) {
			return nil, false, errors.Join(errReadSourceChanged, err)
		}
	}
	return manifest, true, ctx.Err()
}

func sameSourceReadManifest(a, b []readManifestEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || !readInfoEqual(a[i].info, b[i].info) {
			return false
		}
	}
	return true
}

func confirmedSourceReadManifest(ctx context.Context, root *os.Root, base string, options readManifestOptions, op readManifestIO) ([]readManifestEntry, bool, error) {
	manifest, exists, err := sourceReadManifest(ctx, root, base, options, op)
	if err != nil {
		return nil, false, err
	}
	confirmed, confirmedExists, err := sourceReadManifest(ctx, root, base, options, op)
	if err != nil {
		return nil, false, err
	}
	if exists != confirmedExists || !sameSourceReadManifest(manifest, confirmed) {
		return nil, false, errReadSourceChanged
	}
	return manifest, exists, nil
}

func readManifestData(root *os.Root, entry readManifestEntry, limit int64, op readManifestIO) ([]byte, error) {
	if op.lstat == nil {
		op.lstat = func(root *os.Root, name string) (fs.FileInfo, error) { return root.Lstat(name) }
	}
	if op.open == nil {
		op.open = func(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
	}
	visible, err := op.lstat(root, entry.name)
	if err != nil || !readInfoEqual(visible, entry.info) {
		return nil, errors.Join(errReadSourceChanged, err)
	}
	if !visible.Mode().IsRegular() || visible.Size() > limit {
		return nil, ErrInvalid
	}
	f, err := op.open(root, entry.name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !readInfoEqual(opened, entry.info) {
		return nil, errors.Join(errReadSourceChanged, err)
	}
	if !opened.Mode().IsRegular() || opened.Size() > limit {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	after, err := f.Stat()
	if err != nil || !readInfoEqual(after, entry.info) {
		return nil, errors.Join(errReadSourceChanged, err)
	}
	return data, nil
}

func confirmFinalSourceReadManifest(ctx context.Context, root *os.Root, base string, options readManifestOptions, expected []readManifestEntry, op readManifestIO) error {
	final, exists, err := sourceReadManifest(ctx, root, base, options, op)
	if err != nil {
		return err
	}
	if !exists || !sameSourceReadManifest(expected, final) {
		return errReadSourceChanged
	}
	return nil
}
