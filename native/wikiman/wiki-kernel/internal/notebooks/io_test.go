// SPDX-License-Identifier: AGPL-3.0-or-later
package notebooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/internal/indexrepair"
)

type directoryFault struct {
	*os.File
	read    func(int) ([]fs.DirEntry, error)
	stat    func() (fs.FileInfo, error)
	onClose func() error
	closes  int
}

func (f *directoryFault) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.read != nil {
		return f.read(n)
	}
	return f.File.ReadDir(n)
}
func (f *directoryFault) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat()
	}
	return f.File.Stat()
}
func (f *directoryFault) Close() error {
	f.closes++
	err := f.File.Close()
	if f.onClose != nil {
		err = errors.Join(err, f.onClose())
	}
	return err
}

func TestNotebookIOBoundedMetadataReadAndMissingClassification(t *testing.T) {
	dir, r := fixture(t)
	name := good + "/.wiki/conf.json"
	for _, size := range []int{0, 1 << 20, (1 << 20) + 1} {
		raw := bytes.Repeat([]byte{' '}, size)
		put(t, dir, name, string(raw))
		got, err := read(context.Background(), r, name)
		if size > 1<<20 {
			if got != nil || !errors.Is(err, ErrPath) || !errors.Is(err, indexrepair.ErrSourceReadLimit) {
				t.Fatal(len(got), err)
			}
		} else if err != nil || got == nil || !bytes.Equal(got, raw) {
			t.Fatal(size, len(got), err)
		}
	}
	r.Remove(name)
	if _, err := read(context.Background(), r, name); !indexrepair.IsSourceNotExist(err) {
		t.Fatal(err)
	}
	r.Symlink("other.json", name)
	if _, err := read(context.Background(), r, name); !errors.Is(err, ErrPath) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := read(ctx, r, name); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestNotebookIOScanMissingVersusIncompleteRead(t *testing.T) {
	for _, kind := range []string{"pure", "mixed", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			dir, r := fixture(t)
			os.Mkdir(filepath.Join(dir, missing), 0700)
			closing := errors.New("metadata close")
			op := notebookIO{read: func(context.Context, *os.Root, string) ([]byte, error) {
				switch kind {
				case "pure":
					return nil, os.ErrNotExist
				case "mixed":
					return []byte("partial"), errors.Join(os.ErrNotExist, closing)
				default:
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist)
				}
			}}
			p, err := scanWithIO(context.Background(), r, ".wiki", 0, op)
			if kind == "pure" {
				if err != nil || len(p.Repairs) != 1 || !p.Repairs[0].Missing {
					t.Fatal(p, err)
				}
			} else if err == nil || len(p.Repairs) != 0 || len(p.Notebooks) != 0 {
				t.Fatal(p, err)
			}
			if kind == "mixed" && !errors.Is(err, closing) {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(dir, missing, ".wiki/conf.json")); !os.IsNotExist(err) {
				t.Fatal("scan wrote source")
			}
		})
	}
}

func TestNotebookIODirectoryFaultsCloseAndDiscardPlan(t *testing.T) {
	for _, kind := range []string{"open", "stat", "read", "close", "both", "mixed-eof", "no-progress", "wrong-descriptor", "mutate-close", "cancel-close", "cancel-read", "cancel-open"} {
		t.Run(kind, func(t *testing.T) {
			dir, r := fixture(t)
			put(t, dir, good+"/.wiki/conf.json", `{"name":"Good"}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			primary, closing := errors.New("enumeration read"), errors.New("enumeration close")
			var file *directoryFault
			op := notebookIO{openDirectory: func(root *os.Root) (notebookDirectory, error) {
				if kind == "open" {
					return nil, primary
				}
				name := "."
				if kind == "wrong-descriptor" {
					name = good
				}
				f, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				file = &directoryFault{File: f}
				if kind == "cancel-open" {
					cancel()
				}
				if kind == "cancel-read" {
					file.read = func(k int) ([]fs.DirEntry, error) { v, err := f.ReadDir(k); cancel(); return v, err }
				}
				switch kind {
				case "stat":
					file.stat = func() (fs.FileInfo, error) { return nil, primary }
				case "read", "both":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, primary }
				case "mixed-eof":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, errors.Join(io.EOF, primary) }
				case "no-progress":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, nil }
				}
				file.onClose = func() error {
					switch kind {
					case "close", "both":
						return closing
					case "mutate-close":
						return root.WriteFile("late", []byte("late"), 0600)
					case "cancel-close":
						cancel()
					}
					return nil
				}
				return file, nil
			}}
			p, err := scanWithIO(ctx, r, ".wiki", 0, op)
			if err == nil || len(p.Notebooks) != 0 || len(p.Repairs) != 0 {
				t.Fatal(p, err)
			}
			if kind == "close" || kind == "both" {
				if !errors.Is(err, closing) {
					t.Fatal(err)
				}
			}
			if kind == "open" || kind == "stat" || kind == "read" || kind == "both" || kind == "mixed-eof" {
				if !errors.Is(err, primary) {
					t.Fatal(err)
				}
			}
			if (kind == "cancel-close" || kind == "cancel-read" || kind == "cancel-open") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if file != nil {
				if file.closes != 1 {
					t.Fatal(file.closes)
				}
				if _, err = file.File.Stat(); err == nil {
					t.Fatal("descriptor leaked")
				}
			}
			if _, err = r.Stat("."); err != nil {
				t.Fatal("borrowed root closed", err)
			}
		})
	}
}

func TestNotebookIODirectoryBatchOrderingAndEntryLimit(t *testing.T) {
	dir, r := fixture(t)
	for i := 259; i >= 0; i-- {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("20261101090000-%07d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := notebookEntries(context.Background(), r, notebookIO{}.defaults())
	if err != nil || len(entries) != 260 {
		t.Fatal(len(entries), err)
	}
	for i, v := range entries {
		if v.Name() != fmt.Sprintf("20261101090000-%07d", i) {
			t.Fatal(i, v.Name())
		}
	}
	p, err := Scan(context.Background(), r, ".wiki", 0)
	if err != nil || len(p.Notebooks) != 260 || len(p.Repairs) != 260 {
		t.Fatal(len(p.Notebooks), len(p.Repairs), err)
	}
	op := notebookIO{openDirectory: func(r *os.Root) (notebookDirectory, error) {
		f, err := r.Open(".")
		if err != nil {
			return nil, err
		}
		return &directoryFault{File: f, read: func(int) ([]fs.DirEntry, error) {
			many := make([]fs.DirEntry, 100001)
			for i := range many {
				many[i] = entries[0]
			}
			return many, io.EOF
		}}, nil
	}}
	if p, err = scanWithIO(context.Background(), r, ".wiki", 0, op); !errors.Is(err, ErrPath) || len(p.Notebooks) != 0 {
		t.Fatal(p, err)
	}
}

func TestNotebookIOApplyReadFailuresNeverCreateOrDelete(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "delete-recheck"} {
		t.Run(kind, func(t *testing.T) {
			dir, r := fixture(t)
			os.Mkdir(filepath.Join(dir, missing), 0700)
			id := missing
			if kind != "missing" {
				id = broken
				put(t, dir, broken+"/.wiki/conf.json", "broken")
			}
			p, err := Scan(context.Background(), r, ".wiki", 0)
			if err != nil {
				t.Fatal(err)
			}
			var selected Repair
			for _, v := range p.Repairs {
				if v.ID == id {
					selected = v
				}
			}
			closing := errors.New("metadata close")
			calls := 0
			op := notebookIO{read: func(ctx context.Context, r *os.Root, n string) ([]byte, error) {
				calls++
				if kind == "delete-recheck" && calls == 1 {
					return read(ctx, r, n)
				}
				return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, closing)
			}}
			effects := 0
			err = applyWithIO(context.Background(), r, []Repair{selected}, func(context.Context, string) error { effects++; return nil }, op)
			if !errors.Is(err, ErrChanged) || !errors.Is(err, closing) || effects != 0 {
				t.Fatal(err, effects)
			}
			if kind == "missing" {
				if _, err = os.Stat(filepath.Join(dir, id, ".wiki/conf.json")); !os.IsNotExist(err) {
					t.Fatal("created after read failure")
				}
			} else {
				raw, err := r.ReadFile(selected.Path)
				if err != nil || string(raw) != "broken" {
					t.Fatal("deleted after read failure", err)
				}
			}
		})
	}
}

type writerFault struct {
	*os.File
	write   func([]byte) (int, error)
	onClose func() error
	closes  int
}

func (f *writerFault) Write(b []byte) (int, error) {
	if f.write != nil {
		return f.write(b)
	}
	return f.File.Write(b)
}
func (f *writerFault) Close() error {
	f.closes++
	err := f.File.Close()
	if f.onClose != nil {
		err = errors.Join(err, f.onClose())
	}
	return err
}

func TestNotebookIOApplyWriteCloseAndCancellationBoundaries(t *testing.T) {
	for _, kind := range []string{"short", "write-close", "close", "cancel-close", "cancel-close-error", "cancel-callback"} {
		t.Run(kind, func(t *testing.T) {
			dir, r := fixture(t)
			os.Mkdir(filepath.Join(dir, missing), 0700)
			p, err := Scan(context.Background(), r, ".wiki", 0)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			primary, closing := errors.New("repair write"), errors.New("repair close")
			var file *writerFault
			effects := 0
			op := notebookIO{create: func(r *os.Root, n string) (notebookWriter, error) {
				f, err := r.OpenFile(n, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					return nil, err
				}
				file = &writerFault{File: f}
				if kind == "short" {
					file.write = func(b []byte) (int, error) { return f.Write(b[:len(b)-1]) }
				}
				if kind == "write-close" {
					file.write = func([]byte) (int, error) { return 0, primary }
				}
				file.onClose = func() error {
					if kind == "cancel-close" || kind == "cancel-close-error" {
						cancel()
					}
					if kind == "close" || kind == "write-close" || kind == "cancel-close-error" {
						return closing
					}
					return nil
				}
				return file, nil
			}}
			err = applyWithIO(ctx, r, p.Repairs, func(context.Context, string) error {
				effects++
				if kind == "cancel-callback" {
					cancel()
				}
				return nil
			}, op)
			if err == nil || file == nil || file.closes != 1 {
				t.Fatal(err, file)
			}
			if kind == "short" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatal(err)
			}
			if (kind == "close" || kind == "write-close" || kind == "cancel-close-error") && !errors.Is(err, closing) {
				t.Fatal(err)
			}
			if kind == "write-close" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			if (kind == "cancel-close" || kind == "cancel-close-error" || kind == "cancel-callback") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind != "cancel-callback" && effects != 0 {
				t.Fatal("effect preceded successful write/close", effects)
			}
			if _, err = r.Stat(p.Repairs[0].Path); err != nil {
				t.Fatal("partial effect falsely rolled back", err)
			}
		})
	}
}

func TestNotebookIOFinalCancellationDoesNotReturnPlan(t *testing.T) {
	dir, r := fixture(t)
	put(t, dir, good+"/.wiki/conf.json", `{"name":"Good"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := scanWithIO(ctx, r, ".wiki", 0, notebookIO{read: func(ctx context.Context, r *os.Root, n string) ([]byte, error) {
		raw, err := read(ctx, r, n)
		cancel()
		return raw, err
	}})
	if !errors.Is(err, context.Canceled) || len(p.Notebooks) != 0 || len(p.Repairs) != 0 {
		t.Fatal(p, err)
	}
}
