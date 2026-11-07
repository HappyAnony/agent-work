package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func invokeOwnedEmojisWithIO(t *testing.T, lease *Lease, op emojiManifestIO) (out map[string]string, err error) {
	t.Helper()
	pin, err := lease.pin(true)
	if err != nil {
		return nil, err
	}
	defer pin.Release()
	err = pin.sourceRead(context.Background(), func() error {
		return pin.workspace.resources.serial(context.Background(), func() error {
			out, err = pin.workspace.resources.ownedEmojisWithIO(context.Background(), pin.workspace, op)
			return err
		})
	})
	return out, err
}

func replaceEmojiFile(t *testing.T, root, relative string) {
	t.Helper()
	target := filepath.Join(root, "pages", pageA, "data", filepath.FromSlash(relative))
	fileInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		raw[i] = 'x'
	}
	replacement := target + ".replacement"
	if err := os.WriteFile(replacement, raw, fileInfo.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(parent, parentInfo.ModTime(), parentInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func replaceEmojiDirectory(t *testing.T, root, relative string) {
	t.Helper()
	target := filepath.Join(root, "pages", pageA, "data", filepath.FromSlash(relative))
	dirInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, filepath.Join(t.TempDir(), "displaced")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, dirInfo.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, dirInfo.ModTime(), dirInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(parent, parentInfo.ModTime(), parentInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func TestOwnedEmojisRejectIncompleteReplacementAndCloseFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		op   func(*testing.T, string) (emojiManifestIO, error)
	}{
		{name: "incomplete confirmation", op: func(t *testing.T, _ string) (emojiManifestIO, error) {
			incomplete := errors.New("incomplete emoji enumeration")
			calls := 0
			return emojiManifestIO{readDir: func(name string, dir *os.File, count int) ([]fs.DirEntry, error) {
				entries, err := dir.ReadDir(count)
				if name == "emojis" {
					calls++
					if calls == 3 {
						return entries, incomplete
					}
				}
				return entries, err
			}}, incomplete
		}},
		{name: "same metadata file replacement", op: func(t *testing.T, root string) (emojiManifestIO, error) {
			calls := 0
			return emojiManifestIO{lstat: func(ctx context.Context, w *WorkspaceContext, name string) (fs.FileInfo, error) {
				if name == "emojis/stable.png" {
					calls++
					if calls == 3 {
						replaceEmojiFile(t, root, name)
					}
				}
				return treeFileStat(ctx, w, name)
			}}, errEmojiSourceChanged
		}},
		{name: "opened directory identity replacement", op: func(t *testing.T, root string) (emojiManifestIO, error) {
			faces := filepath.Join(root, "pages", pageA, "data", "emojis", "faces")
			if err := os.MkdirAll(faces, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(faces, "face.png"), []byte("face"), 0600); err != nil {
				t.Fatal(err)
			}
			replaced := false
			return emojiManifestIO{openDir: func(data *os.Root, name string) (*os.File, error) {
				if name == "emojis/faces" && !replaced {
					replaced = true
					replaceEmojiDirectory(t, root, name)
				}
				return data.Open(name)
			}}, errEmojiSourceChanged
		}},
		{name: "directory close failure", op: func(_ *testing.T, _ string) (emojiManifestIO, error) {
			closeErr := errors.New("emoji directory close failed")
			calls := 0
			return emojiManifestIO{closeDir: func(_ string, dir *os.File) error {
				calls++
				err := dir.Close()
				if calls == 1 {
					return errors.Join(err, closeErr)
				}
				return err
			}}, closeErr
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			dir := filepath.Join(root, "pages", pageA, "data", "emojis")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "stable.png"), []byte("stable"), 0600); err != nil {
				t.Fatal(err)
			}
			manager := open(t, root, Options{})
			lease := getDocLease(t, manager, pageA)
			defer lease.Release()
			if got, err := lease.CustomEmojis(context.Background()); err != nil || got["stable"] == "" {
				t.Fatal(got, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "new.png"), []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			op, expected := test.op(t, root)
			got, err := invokeOwnedEmojisWithIO(t, lease, op)
			if !errors.Is(err, expected) || got != nil {
				t.Fatal(got, err)
			}
			snapshot := lease.workspace.resources.emojis.Snapshot()
			if snapshot["stable"] != "/emojis/stable.png" || snapshot["new"] != "" || len(snapshot) != 1 {
				t.Fatal(snapshot)
			}
		})
	}
}

func TestOwnedEmojisPreserveOneLevelHiddenAndLexicalRules(t *testing.T) {
	root := fixture(t)
	dir := filepath.Join(root, "pages", pageA, "data", "emojis")
	for name, raw := range map[string]string{
		"root.png":         "root",
		"faces/a.png":      "png",
		"faces/a.svg":      "svg",
		"faces/deep/x.png": "deep",
		".bad[.png":        "hidden invalid",
	} {
		filename := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, ".hidden-link")); err != nil {
		t.Fatal(err)
	}
	manager := open(t, root, Options{})
	lease := getDocLease(t, manager, pageA)
	defer lease.Release()
	got, err := lease.CustomEmojis(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"root": "/emojis/root.png", "faces/a": "/emojis/faces/a.svg"}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for name, value := range want {
		if got[name] != value {
			t.Fatal(got)
		}
	}
	if got["faces/deep/x"] != "" {
		t.Fatal("emoji traversal exceeded one subdirectory", got)
	}
}
