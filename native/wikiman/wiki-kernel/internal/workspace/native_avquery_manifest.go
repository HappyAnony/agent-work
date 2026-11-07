package workspace

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
)

var errNativeAVQuerySourceChanged = errors.New("native_av_query_source_changed")

type nativeAVQueryIO struct {
	openDir   func(*os.Root, string) (*os.File, error)
	readDir   func(string, *os.File, int) ([]fs.DirEntry, error)
	closeDir  func(string, *os.File) error
	lstat     func(context.Context, *WorkspaceContext, string) (fs.FileInfo, error)
	openFile  func(*WorkspaceContext, string) (*os.File, error)
	closeFile func(string, *os.File) error
}

type nativeAVQueryManifestEntry struct {
	name string
	info fs.FileInfo
}

func (op nativeAVQueryIO) openDirectory(root *os.Root, name string) (*os.File, error) {
	if op.openDir != nil {
		return op.openDir(root, name)
	}
	return root.Open(name)
}

func (op nativeAVQueryIO) readDirectory(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
	if op.readDir != nil {
		return op.readDir(name, dir, count)
	}
	return dir.ReadDir(count)
}

func (op nativeAVQueryIO) closeDirectory(name string, dir *os.File) error {
	if op.closeDir != nil {
		return op.closeDir(name, dir)
	}
	return dir.Close()
}

func (op nativeAVQueryIO) fileInfo(ctx context.Context, w *WorkspaceContext, name string) (fs.FileInfo, error) {
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

func (op nativeAVQueryIO) openCheckedFile(w *WorkspaceContext, name string) (*os.File, error) {
	if op.openFile != nil {
		return op.openFile(w, name)
	}
	return openNativeFile(w, name)
}

func (op nativeAVQueryIO) closeCheckedFile(name string, file *os.File) error {
	if op.closeFile != nil {
		return op.closeFile(name, file)
	}
	return file.Close()
}

func sameNativeAVQueryInfo(a, b fs.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func readNativeAVQueryDirectory(ctx context.Context, w *WorkspaceContext, expected fs.FileInfo, op nativeAVQueryIO) (entries []fs.DirEntry, result error) {
	const name = "storage/av"
	dir, err := op.openDirectory(w.data, name)
	if err != nil {
		return nil, err
	}
	defer func() {
		result = errors.Join(result, op.closeDirectory(name, dir))
		current, err := op.fileInfo(ctx, w, name)
		if err != nil || !sameNativeAVQueryInfo(current, expected) {
			result = errors.Join(result, errNativeAVQuerySourceChanged, err)
		}
		if result != nil {
			entries = nil
		}
	}()
	opened, err := dir.Stat()
	if err != nil || !sameNativeAVQueryInfo(opened, expected) {
		return nil, errors.Join(errNativeAVQuerySourceChanged, err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part, readErr := op.readDirectory(name, dir, 128)
		entries = append(entries, part...)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if len(part) == 0 {
			return nil, io.ErrNoProgress
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func nativeAVQuerySourceManifest(ctx context.Context, w *WorkspaceContext, op nativeAVQueryIO) ([]nativeAVQueryManifestEntry, error) {
	var root fs.FileInfo
	for _, name := range []string{"storage", "storage/av"} {
		info, err := op.fileInfo(ctx, w, name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrDocument
		}
		root = info
	}
	entries, err := readNativeAVQueryDirectory(ctx, w, root, op)
	if err != nil {
		return nil, err
	}
	if len(entries) > 100000 {
		return nil, ErrBudget
	}
	manifest := []nativeAVQueryManifestEntry{{name: "storage/av", info: root}}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := "storage/av/" + entry.Name()
		info, err := op.fileInfo(ctx, w, name)
		if err != nil {
			return nil, err
		}
		manifest = append(manifest, nativeAVQueryManifestEntry{name: name, info: info})
	}
	for _, entry := range manifest {
		info, err := op.fileInfo(ctx, w, entry.name)
		if err != nil || !sameNativeAVQueryInfo(info, entry.info) {
			return nil, errors.Join(errNativeAVQuerySourceChanged, err)
		}
	}
	return manifest, ctx.Err()
}

func sameNativeAVQueryManifest(a, b []nativeAVQueryManifestEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || !sameNativeAVQueryInfo(a[i].info, b[i].info) {
			return false
		}
	}
	return true
}

func confirmedNativeAVQueryManifest(ctx context.Context, w *WorkspaceContext, op nativeAVQueryIO) ([]nativeAVQueryManifestEntry, error) {
	manifest, err := nativeAVQuerySourceManifest(ctx, w, op)
	if err != nil {
		return nil, err
	}
	confirmed, err := nativeAVQuerySourceManifest(ctx, w, op)
	if err != nil {
		return nil, err
	}
	if !sameNativeAVQueryManifest(manifest, confirmed) {
		return nil, errNativeAVQuerySourceChanged
	}
	return manifest, nil
}

func confirmFinalNativeAVQueryManifest(ctx context.Context, w *WorkspaceContext, expected []nativeAVQueryManifestEntry, op nativeAVQueryIO) error {
	manifest, err := nativeAVQuerySourceManifest(ctx, w, op)
	if err != nil {
		return err
	}
	if !sameNativeAVQueryManifest(expected, manifest) {
		return errNativeAVQuerySourceChanged
	}
	return nil
}

func nativeAVQueryEntries(manifest []nativeAVQueryManifestEntry) map[string]nativeAVQueryManifestEntry {
	entries := make(map[string]nativeAVQueryManifestEntry, len(manifest))
	for _, entry := range manifest {
		entries[entry.name] = entry
	}
	return entries
}

func readNativeAVQueryFile(ctx context.Context, w *WorkspaceContext, entry nativeAVQueryManifestEntry, limit int64, op nativeAVQueryIO) (raw []byte, result error) {
	if !entry.info.Mode().IsRegular() {
		return nil, ErrDocument
	}
	visible, err := op.fileInfo(ctx, w, entry.name)
	if err != nil || !sameNativeAVQueryInfo(visible, entry.info) {
		return nil, errors.Join(errNativeAVQuerySourceChanged, err)
	}
	file, err := op.openCheckedFile(w, entry.name)
	if err != nil {
		return nil, err
	}
	defer func() {
		after, statErr := file.Stat()
		if statErr == nil && !sameNativeAVQueryInfo(after, entry.info) {
			statErr = errNativeAVQuerySourceChanged
		}
		result = errors.Join(result, statErr, op.closeCheckedFile(entry.name, file))
		current, pathErr := op.fileInfo(ctx, w, entry.name)
		if pathErr != nil || !sameNativeAVQueryInfo(current, entry.info) {
			result = errors.Join(result, errNativeAVQuerySourceChanged, pathErr)
		}
		if result != nil {
			raw = nil
		}
	}()
	opened, err := file.Stat()
	if err != nil || !sameNativeAVQueryInfo(opened, entry.info) {
		return nil, errors.Join(errNativeAVQuerySourceChanged, err)
	}
	raw, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, ErrDocument
	}
	return raw, ctx.Err()
}
