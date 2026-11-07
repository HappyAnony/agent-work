// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
)

type readDirFault struct {
	*os.File
	read    func(int) ([]fs.DirEntry, error)
	stat    func() (fs.FileInfo, error)
	onClose func() error
	closes  int
}

func (f *readDirFault) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.read != nil {
		return f.read(n)
	}
	return f.File.ReadDir(n)
}
func (f *readDirFault) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat()
	}
	return f.File.Stat()
}
func (f *readDirFault) Close() error {
	f.closes++
	err := f.File.Close()
	if f.onClose != nil {
		err = errors.Join(err, f.onClose())
	}
	return err
}

func TestSourceReadDirReadOnlyLexicalMultiBatchAndRoot(t *testing.T) {
	s := readFixture(t)
	s.Authorize = func(context.Context) error { t.Fatal("read used write authority"); return nil }
	s.Lock = func(string) func() { t.Fatal("read used writer lock"); return func() {} }
	s.Data.Mkdir("empty", 0700)
	for i := 299; i >= 0; i-- {
		s.Data.WriteFile(fmt.Sprintf("box/%03d", i), nil, 0600)
	}
	entries, err := s.ReadDir(context.Background(), "box")
	if err != nil || len(entries) != 301 {
		t.Fatal(len(entries), err)
	}
	for i := 0; i < 300; i++ {
		if entries[i].Name() != fmt.Sprintf("%03d", i) {
			t.Fatal(i, entries[i].Name())
		}
	}
	if entries, err = s.ReadDir(context.Background(), "empty"); err != nil || entries == nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	if entries, err = s.ReadDir(context.Background(), "."); err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	if _, err = s.Data.Stat("."); err != nil {
		t.Fatal("outer root closed", err)
	}
}

func TestSourceReadDirInitialMissingAndInvalidPaths(t *testing.T) {
	s := readFixture(t)
	s.Data.Symlink("box", "alias")
	for _, name := range []string{"../box", "/box", "box\\doc", "alias", "box/doc"} {
		if out, err := s.ReadDir(context.Background(), name); out != nil || !errors.Is(err, ErrSourcePath) {
			t.Fatal(name, out, err)
		}
	}
	opens := 0
	out, err := s.readDir(context.Background(), "missing", sourceReadDirIO{open: func(*os.Root, string) (sourceReadDirFile, error) { opens++; return nil, errors.New("unexpected open") }})
	if out != nil || !IsSourceNotExist(err) || !os.IsNotExist(err) || opens != 0 {
		t.Fatal(out, err, opens)
	}
}

func TestSourceReadDirReadStatAndTwoCloseFailures(t *testing.T) {
	for _, kind := range []string{"open", "stat", "read", "file-close", "root-close", "all", "mixed-missing", "mixed-eof", "no-progress", "wrong-descriptor"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			primary, a, b := errors.New("enumeration"), errors.New("file close"), errors.New("root close")
			var file *readDirFault
			rootCloses := 0
			op := sourceReadDirIO{open: func(r *os.Root, n string) (sourceReadDirFile, error) {
				if kind == "open" {
					return nil, primary
				}
				if kind == "wrong-descriptor" {
					n = "doc"
				}
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				file = &readDirFault{File: f}
				if kind == "stat" {
					file.stat = func() (fs.FileInfo, error) { return nil, primary }
				}
				switch kind {
				case "read", "all":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, primary }
				case "mixed-missing":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, errors.Join(os.ErrNotExist, primary) }
				case "mixed-eof":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, errors.Join(io.EOF, primary) }
				case "no-progress":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, nil }
				}
				if kind == "file-close" || kind == "all" {
					file.onClose = func() error { return a }
				}
				return file, nil
			}, closeRoot: func(r *os.Root) error {
				rootCloses++
				err := r.Close()
				if kind == "root-close" || kind == "all" {
					err = errors.Join(err, b)
				}
				return err
			}}
			out, err := s.readDir(context.Background(), "box", op)
			if out != nil || !errors.Is(err, ErrSourceReadIncomplete) || IsSourceNotExist(err) || rootCloses != 1 {
				t.Fatal(out, err, rootCloses)
			}
			if kind == "all" && (!errors.Is(err, primary) || !errors.Is(err, a) || !errors.Is(err, b)) {
				t.Fatal(err)
			}
			if file != nil {
				if file.closes != 1 {
					t.Fatal(file.closes)
				}
				if _, err = file.File.Stat(); err == nil {
					t.Fatal("file remains open")
				}
			}
		})
	}
}

func TestSourceReadDirCancellationAtResourceBoundaries(t *testing.T) {
	for _, kind := range []string{"before", "open", "read", "file-close", "root-close"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "before" {
				cancel()
			}
			opens, closes := 0, 0
			var file *readDirFault
			op := sourceReadDirIO{open: func(r *os.Root, n string) (sourceReadDirFile, error) {
				opens++
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				file = &readDirFault{File: f}
				if kind == "open" {
					cancel()
				}
				file.read = func(k int) ([]fs.DirEntry, error) {
					v, err := f.ReadDir(k)
					if kind == "read" {
						cancel()
					}
					return v, err
				}
				file.onClose = func() error {
					if kind == "file-close" {
						cancel()
					}
					return nil
				}
				return file, nil
			}, closeRoot: func(r *os.Root) error {
				closes++
				err := r.Close()
				if kind == "root-close" {
					cancel()
				}
				return err
			}}
			out, err := s.readDir(ctx, "box", op)
			if out != nil || !errors.Is(err, context.Canceled) || opens != closes {
				t.Fatal(out, err, opens, closes)
			}
			if file != nil && file.closes != 1 {
				t.Fatal(file.closes)
			}
		})
	}
}

func TestSourceReadDirChangesDuringReadAndClosing(t *testing.T) {
	for _, kind := range []string{"read-change", "file-close-change", "root-close-missing", "root-close-link"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			op := sourceReadDirIO{open: func(r *os.Root, n string) (sourceReadDirFile, error) {
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				v := &readDirFault{File: f}
				v.read = func(k int) ([]fs.DirEntry, error) {
					entries, err := f.ReadDir(k)
					if kind == "read-change" {
						s.Data.WriteFile("box/late", nil, 0600)
					}
					return entries, err
				}
				v.onClose = func() error {
					if kind == "file-close-change" {
						return s.Data.WriteFile("box/late", nil, 0600)
					}
					return nil
				}
				return v, nil
			}, closeRoot: func(r *os.Root) error {
				err := r.Close()
				if kind == "root-close-missing" || kind == "root-close-link" {
					s.Data.Rename("box", "old")
					if kind == "root-close-link" {
						s.Data.Symlink("old", "box")
					}
				}
				return err
			}}
			out, err := s.readDir(context.Background(), "box", op)
			if out != nil || err == nil || IsSourceNotExist(err) {
				t.Fatal(out, err)
			}
		})
	}
}
