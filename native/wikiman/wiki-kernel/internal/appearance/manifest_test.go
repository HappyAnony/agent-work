package appearance

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func appearanceTreeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var out strings.Builder
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && name == root {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		out.WriteString(filepath.ToSlash(rel))
		out.WriteByte('|')
		out.WriteString(info.Mode().String())
		out.WriteByte('|')
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			out.Write(raw)
		}
		out.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func replaceAppearanceFile(t *testing.T, rootPath, name string) {
	t.Helper()
	target := filepath.Join(rootPath, filepath.FromSlash(name))
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		raw[i] = ' '
	}
	tmp := target + ".replacement"
	if err := os.WriteFile(tmp, raw, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}
}

func replaceAppearanceDirectory(t *testing.T, rootPath, name string) {
	t.Helper()
	target := filepath.Join(rootPath, filepath.FromSlash(name))
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	displaced := filepath.Join(t.TempDir(), "displaced")
	if err := os.Rename(target, displaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func TestAppearanceEntriesRejectUnstableEnumerationAndCloseFailure(t *testing.T) {
	t.Run("incomplete confirmation", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		incomplete := errors.New("incomplete appearance enumeration")
		op := appearanceManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			entries, err := fs.ReadDir(root.FS(), name)
			if name == "." {
				calls++
				if calls == 2 {
					return entries, incomplete
				}
			}
			return entries, err
		}}
		entries, err := o.entriesWithIO(context.Background(), "themes", opts.Themes, op)
		if !errors.Is(err, incomplete) || entries != nil {
			t.Fatal(entries, err)
		}
	})

	t.Run("same metadata directory replacement", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		op := appearanceManifestIO{lstat: func(root *os.Root, name string) (fs.FileInfo, error) {
			if name == "custom" {
				calls++
				if calls == 3 {
					replaceAppearanceDirectory(t, opts.Themes, name)
				}
			}
			return root.Lstat(name)
		}}
		entries, err := o.entriesWithIO(context.Background(), "themes", opts.Themes, op)
		if !errors.Is(err, errAppearanceSourceChanged) || entries != nil {
			t.Fatal(entries, err)
		}
	})

	t.Run("root close failure", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		closeErr := errors.New("appearance root close failed")
		calls := 0
		op := appearanceManifestIO{closeRoot: func(root *os.Root) error {
			calls++
			err := root.Close()
			if calls == 1 {
				return errors.Join(err, closeErr)
			}
			return err
		}}
		entries, err := o.entriesWithIO(context.Background(), "themes", opts.Themes, op)
		if !errors.Is(err, closeErr) || entries != nil {
			t.Fatal(entries, err)
		}
	})
}

func TestAppearanceInstallManifestFailuresDoNotWriteDestination(t *testing.T) {
	t.Run("incomplete confirmation", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		before := appearanceTreeSnapshot(t, opts.Destination)
		calls := 0
		incomplete := errors.New("incomplete bundled enumeration")
		op := appearanceManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			entries, err := fs.ReadDir(root.FS(), name)
			if name == "themes" {
				calls++
				if calls == 2 {
					return entries, incomplete
				}
			}
			return entries, err
		}}
		err = o.installWithIO(context.Background(), op)
		if !errors.Is(err, incomplete) || appearanceTreeSnapshot(t, opts.Destination) != before {
			t.Fatal(err)
		}
	})

	t.Run("same metadata file replacement", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		before := appearanceTreeSnapshot(t, opts.Destination)
		const name = "themes/daylight/theme.json"
		calls := 0
		op := appearanceManifestIO{lstat: func(root *os.Root, candidate string) (fs.FileInfo, error) {
			if candidate == name {
				calls++
				if calls == 3 {
					replaceAppearanceFile(t, opts.Bundled, candidate)
				}
			}
			return root.Lstat(candidate)
		}}
		err = o.installWithIO(context.Background(), op)
		if !errors.Is(err, errAppearanceSourceChanged) || appearanceTreeSnapshot(t, opts.Destination) != before {
			t.Fatal(err)
		}
	})

	t.Run("source symlink", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		before := appearanceTreeSnapshot(t, opts.Destination)
		if err := os.Symlink(t.TempDir(), filepath.Join(opts.Bundled, "themes/linked")); err != nil {
			t.Fatal(err)
		}
		err = o.installWithIO(context.Background(), appearanceManifestIO{})
		if !errors.Is(err, ErrDependency) || appearanceTreeSnapshot(t, opts.Destination) != before {
			t.Fatal(err)
		}
	})
}

func TestAppearanceInstallChecksOpenedIdentityAndFileClose(t *testing.T) {
	t.Run("replacement between visible and opened identity", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		const name = "themes/daylight/theme.json"
		replaced := false
		op := appearanceManifestIO{open: func(root *os.Root, candidate string) (*os.File, error) {
			if candidate == name && !replaced {
				replaced = true
				replaceAppearanceFile(t, opts.Bundled, candidate)
			}
			return root.Open(candidate)
		}}
		err = o.installWithIO(context.Background(), op)
		if !errors.Is(err, errAppearanceSourceChanged) || !replaced {
			t.Fatal(err)
		}
	})

	t.Run("input close failure", func(t *testing.T) {
		opts, _ := fixture(t)
		o, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		closeErr := errors.New("appearance input close failed")
		calls := 0
		op := appearanceManifestIO{closeFile: func(file *os.File) error {
			calls++
			err := file.Close()
			if calls == 1 {
				return errors.Join(err, closeErr)
			}
			return err
		}}
		err = o.installWithIO(context.Background(), op)
		if !errors.Is(err, closeErr) || calls != 1 {
			t.Fatal(calls, err)
		}
	})
}

func TestAppearanceCatalogKeepsBundledOverlayAndHiddenFiltering(t *testing.T) {
	opts, a := fixture(t)
	a.ThemeLight = "daylight"
	installed := filepath.Join(opts.Themes, "daylight/theme.json")
	if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte(`{"version":"installed","modes":["light"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(opts.Themes, ".hidden/theme.json")
	if err := os.MkdirAll(filepath.Dir(hidden), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hidden, []byte(`{"version":"hidden","modes":["light"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	o, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	out, err := o.Prepare(context.Background(), a)
	if err != nil || out.ThemeVer != "1" {
		t.Fatal(out, err)
	}
	for _, theme := range append(out.LightThemes, out.DarkThemes...) {
		if theme.Name == ".hidden" {
			t.Fatal("hidden catalog entry escaped")
		}
	}
}
