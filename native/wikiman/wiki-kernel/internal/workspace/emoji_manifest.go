package workspace

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
)

var errEmojiSourceChanged = errors.New("emoji_source_changed")

type emojiManifestIO struct {
	openDir  func(*os.Root, string) (*os.File, error)
	readDir  func(string, *os.File, int) ([]fs.DirEntry, error)
	closeDir func(string, *os.File) error
	lstat    func(context.Context, *WorkspaceContext, string) (fs.FileInfo, error)
}

type emojiManifestEntry struct {
	name string
	info fs.FileInfo
}

func (op emojiManifestIO) openDirectory(root *os.Root, name string) (*os.File, error) {
	if op.openDir != nil {
		return op.openDir(root, name)
	}
	return root.Open(name)
}

func (op emojiManifestIO) readDirectory(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
	if op.readDir != nil {
		return op.readDir(name, dir, count)
	}
	return dir.ReadDir(count)
}

func (op emojiManifestIO) closeDirectory(name string, dir *os.File) error {
	if op.closeDir != nil {
		return op.closeDir(name, dir)
	}
	return dir.Close()
}

func (op emojiManifestIO) fileInfo(ctx context.Context, w *WorkspaceContext, name string) (fs.FileInfo, error) {
	if op.lstat != nil {
		return op.lstat(ctx, w, name)
	}
	return treeFileStat(ctx, w, name)
}

func sameEmojiInfo(a, b fs.FileInfo) bool {
	return a != nil && b != nil && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func readEmojiDirectory(ctx context.Context, w *WorkspaceContext, name string, expected fs.FileInfo, op emojiManifestIO) (entries []fs.DirEntry, result error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := op.openDirectory(w.data, name)
	if err != nil {
		return nil, err
	}
	defer func() {
		result = errors.Join(result, op.closeDirectory(name, dir))
		if current, err := op.fileInfo(ctx, w, name); err != nil || !sameEmojiInfo(current, expected) {
			result = errors.Join(result, errEmojiSourceChanged, err)
		}
		if result != nil {
			entries = nil
		}
	}()
	opened, err := dir.Stat()
	if err != nil || !sameEmojiInfo(opened, expected) {
		return nil, errors.Join(errEmojiSourceChanged, err)
	}
	for {
		if err = ctx.Err(); err != nil {
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

func emojiSourceManifest(ctx context.Context, w *WorkspaceContext, op emojiManifestIO) ([]emojiManifestEntry, error) {
	root, err := op.fileInfo(ctx, w, "emojis")
	if err != nil {
		return nil, err
	}
	if !root.IsDir() {
		return nil, ErrDocument
	}
	manifest := []emojiManifestEntry{{name: "emojis", info: root}}
	var scan func(string, int, fs.FileInfo) error
	scan = func(dir string, depth int, expected fs.FileInfo) error {
		entries, err := readEmojiDirectory(ctx, w, dir, expected, op)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Name() == "" || entry.Name()[0] == '.' {
				continue
			}
			if !validEmojiName(entry.Name()) {
				return ErrDocument
			}
			name := dir + "/" + entry.Name()
			info, err := op.fileInfo(ctx, w, name)
			if err != nil {
				return err
			}
			manifest = append(manifest, emojiManifestEntry{name: name, info: info})
			if info.IsDir() {
				if depth == 0 {
					if err := scan(name, 1, info); err != nil {
						return err
					}
				}
				continue
			}
			if !info.Mode().IsRegular() {
				return ErrDocument
			}
		}
		return nil
	}
	if err := scan("emojis", 0, root); err != nil {
		return nil, err
	}
	for _, entry := range manifest {
		info, err := op.fileInfo(ctx, w, entry.name)
		if err != nil || !sameEmojiInfo(info, entry.info) {
			return nil, errors.Join(errEmojiSourceChanged, err)
		}
	}
	return manifest, ctx.Err()
}

func sameEmojiManifest(a, b []emojiManifestEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || !sameEmojiInfo(a[i].info, b[i].info) {
			return false
		}
	}
	return true
}

func confirmedEmojiManifest(ctx context.Context, w *WorkspaceContext, op emojiManifestIO) ([]emojiManifestEntry, error) {
	manifest, err := emojiSourceManifest(ctx, w, op)
	if err != nil {
		return nil, err
	}
	confirmed, err := emojiSourceManifest(ctx, w, op)
	if err != nil {
		return nil, err
	}
	if !sameEmojiManifest(manifest, confirmed) {
		return nil, errEmojiSourceChanged
	}
	return manifest, nil
}
