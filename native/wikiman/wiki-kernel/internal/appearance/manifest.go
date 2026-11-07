package appearance

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
)

var errAppearanceSourceChanged = errors.New("appearance_source_changed")

type appearanceManifestIO struct {
	readDir   func(*os.Root, string) ([]fs.DirEntry, error)
	lstat     func(*os.Root, string) (fs.FileInfo, error)
	open      func(*os.Root, string) (*os.File, error)
	closeRoot func(*os.Root) error
	closeFile func(*os.File) error
}

type appearanceManifestOptions struct {
	singleLevel      bool
	rejectSymlinks   bool
	regularFilesOnly bool
	maxEntries       int
	maxBytes         int64
}

type appearanceManifestEntry struct {
	name string
	info fs.FileInfo
}

func (op appearanceManifestIO) readDirectory(root *os.Root, name string) ([]fs.DirEntry, error) {
	if op.readDir != nil {
		return op.readDir(root, name)
	}
	return fs.ReadDir(root.FS(), name)
}

func (op appearanceManifestIO) fileInfo(root *os.Root, name string) (fs.FileInfo, error) {
	if op.lstat != nil {
		return op.lstat(root, name)
	}
	return root.Lstat(name)
}

func (op appearanceManifestIO) openFile(root *os.Root, name string) (*os.File, error) {
	if op.open != nil {
		return op.open(root, name)
	}
	return root.Open(name)
}

func (op appearanceManifestIO) closeDirectory(root *os.Root) error {
	if op.closeRoot != nil {
		return op.closeRoot(root)
	}
	return root.Close()
}

func (op appearanceManifestIO) closeOpenedFile(file *os.File) error {
	if op.closeFile != nil {
		return op.closeFile(file)
	}
	return file.Close()
}

func appearanceInfoEqual(a, b fs.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func appearanceSourceManifest(ctx context.Context, root *os.Root, options appearanceManifestOptions, op appearanceManifestIO) ([]appearanceManifestEntry, error) {
	baseInfo, err := op.fileInfo(root, ".")
	if err != nil {
		return nil, err
	}
	manifest := []appearanceManifestEntry{}
	var total int64
	var walk func(string, fs.FileInfo) error
	walk = func(name string, info fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		manifest = append(manifest, appearanceManifestEntry{name: name, info: info})
		if options.maxEntries > 0 && len(manifest) > options.maxEntries {
			return ErrDependency
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			if options.rejectSymlinks {
				return ErrDependency
			}
			return nil
		}
		if !info.IsDir() {
			if options.regularFilesOnly && !info.Mode().IsRegular() {
				return ErrDependency
			}
			if info.Mode().IsRegular() {
				total += info.Size()
				if options.maxBytes > 0 && total > options.maxBytes {
					return ErrDependency
				}
			}
			return nil
		}
		if options.singleLevel && name != "." {
			return nil
		}
		entries, err := op.readDirectory(root, name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := path.Join(name, entry.Name())
			info, err := op.fileInfo(root, child)
			if err != nil {
				return err
			}
			if err := walk(child, info); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(".", baseInfo); err != nil {
		return nil, err
	}
	for _, entry := range manifest {
		info, err := op.fileInfo(root, entry.name)
		if err != nil || !appearanceInfoEqual(info, entry.info) {
			return nil, errors.Join(errAppearanceSourceChanged, err)
		}
	}
	return manifest, ctx.Err()
}

func sameAppearanceManifest(a, b []appearanceManifestEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || !appearanceInfoEqual(a[i].info, b[i].info) {
			return false
		}
	}
	return true
}

func confirmedAppearanceManifest(ctx context.Context, root *os.Root, options appearanceManifestOptions, op appearanceManifestIO) ([]appearanceManifestEntry, error) {
	manifest, err := appearanceSourceManifest(ctx, root, options, op)
	if err != nil {
		return nil, err
	}
	confirmed, err := appearanceSourceManifest(ctx, root, options, op)
	if err != nil {
		return nil, err
	}
	if !sameAppearanceManifest(manifest, confirmed) {
		return nil, errAppearanceSourceChanged
	}
	return manifest, nil
}

func confirmFinalAppearanceManifest(ctx context.Context, root *os.Root, options appearanceManifestOptions, expected []appearanceManifestEntry, op appearanceManifestIO) error {
	final, err := appearanceSourceManifest(ctx, root, options, op)
	if err != nil {
		return err
	}
	if !sameAppearanceManifest(expected, final) {
		return errAppearanceSourceChanged
	}
	return nil
}

func openAppearanceManifestFile(root *os.Root, entry appearanceManifestEntry, op appearanceManifestIO) (*os.File, error) {
	visible, err := op.fileInfo(root, entry.name)
	if err != nil || !appearanceInfoEqual(visible, entry.info) {
		return nil, errors.Join(errAppearanceSourceChanged, err)
	}
	file, err := op.openFile(root, entry.name)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !appearanceInfoEqual(opened, entry.info) {
		return nil, errors.Join(errAppearanceSourceChanged, err, op.closeOpenedFile(file))
	}
	return file, nil
}
