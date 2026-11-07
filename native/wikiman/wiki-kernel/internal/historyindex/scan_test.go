// SPDX-License-Identifier: AGPL-3.0-or-later
package historyindex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute"
	"wiki-kernel/internal/indexrepair"
)

type scanDirectoryFault struct {
	*os.File
	read    func(int) ([]fs.DirEntry, error)
	stat    func() (fs.FileInfo, error)
	onClose func() error
	closes  int
}

type scanNamedEntry struct {
	fs.DirEntry
	name string
}

func (e scanNamedEntry) Name() string { return e.name }

func (f *scanDirectoryFault) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.read != nil {
		return f.read(n)
	}
	return f.File.ReadDir(n)
}
func (f *scanDirectoryFault) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat()
	}
	return f.File.Stat()
}
func (f *scanDirectoryFault) Close() error {
	f.closes++
	err := f.File.Close()
	if f.onClose != nil {
		err = errors.Join(err, f.onClose())
	}
	return err
}

func scanFixture(t *testing.T) (indexrepair.Source, string, []byte) {
	t.Helper()
	s := historySource(t)
	if err := s.Data.MkdirAll(historyName+"/box", 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"ID":"20261029200000-docaaaa","Type":"NodeDocument","Properties":{"id":"20261029200000-docaaaa","title":"Original"},"Children":[{"Type":"NodeParagraph","ID":"20261029200001-paraaaa","Children":[{"Type":"NodeText","Data":"synthetic history"}]}]}`)
	name := historyName + "/box/20261029200000-docaaaa.docj"
	if err := s.Data.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return s, name, raw
}

func TestHistoryScanLexicalOrderMultiBatchAndBorrowedRoot(t *testing.T) {
	s, doc, raw := scanFixture(t)
	s.Authorize = func(context.Context) error { t.Fatal("read requested write authority"); return nil }
	s.Lock = func(string) func() { t.Fatal("read requested writer lock"); return func() {} }
	if err := s.Data.Mkdir(historyName+"/assets", 0700); err != nil {
		t.Fatal(err)
	}
	for i := 129; i >= 0; i-- {
		if err := s.Data.WriteFile(fmt.Sprintf("%s/assets/%03d.bin", historyName, i), []byte{byte(i)}, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var files []*scanDirectoryFault
	op := historyScanIO{open: func(r *os.Root, n string) (historyDirectory, error) {
		f, err := r.Open(n)
		if err != nil {
			return nil, err
		}
		w := &scanDirectoryFault{File: f}
		files = append(files, w)
		return w, nil
	}}
	out, err := planWithIO(context.Background(), s, historyName, "Untitled", time.UTC, lute.New(), op)
	if err != nil || len(out) != 131 || out[0].Path != doc {
		t.Fatal(len(out), err)
	}
	for i := 0; i < 130; i++ {
		if out[i+1].Path != fmt.Sprintf("%s/assets/%03d.bin", historyName, i) {
			t.Fatal(i, out[i+1])
		}
	}
	if len(files) != 6 {
		t.Fatal("directories not enumerated and rechecked", len(files))
	}
	for _, f := range files {
		if f.closes != 1 {
			t.Fatal(f.closes)
		}
		if _, err = f.File.Stat(); err == nil {
			t.Fatal("directory descriptor leaked")
		}
	}
	if _, err = s.Data.Stat("."); err != nil {
		t.Fatal("borrowed outer root closed", err)
	}
	after, err := s.Data.ReadFile(doc)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("source changed", err)
	}
}

func TestHistoryScanDirectoryFailuresPreserveCloseAndNoCandidates(t *testing.T) {
	for _, kind := range []string{"open", "stat", "read", "read-missing", "close", "read-and-close", "mixed-eof", "no-progress", "wrong-descriptor"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := scanFixture(t)
			primary, closing := errors.New("scan io"), errors.New("directory close")
			var file *scanDirectoryFault
			op := historyScanIO{open: func(r *os.Root, n string) (historyDirectory, error) {
				if kind == "open" {
					return nil, primary
				}
				if kind == "wrong-descriptor" {
					n = historyName + "/box"
				}
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				file = &scanDirectoryFault{File: f}
				switch kind {
				case "stat":
					file.stat = func() (fs.FileInfo, error) { return nil, primary }
				case "read", "read-and-close":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, primary }
				case "read-missing":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, os.ErrNotExist }
				case "mixed-eof":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, errors.Join(io.EOF, primary) }
				case "no-progress":
					file.read = func(int) ([]fs.DirEntry, error) { return nil, nil }
				}
				if kind == "close" || kind == "read-and-close" {
					file.onClose = func() error { return closing }
				}
				return file, nil
			}}
			out, err := planWithIO(context.Background(), s, historyName, "Untitled", time.UTC, lute.New(), op)
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
			if kind == "close" || kind == "read-and-close" {
				if !errors.Is(err, closing) {
					t.Fatal(err)
				}
			}
			if kind == "open" || kind == "stat" || kind == "read" || kind == "read-and-close" || kind == "mixed-eof" {
				if !errors.Is(err, primary) {
					t.Fatal(err)
				}
			}
			if kind == "read-missing" && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if file != nil && file.closes != 1 {
				t.Fatal("close count", file.closes)
			}
		})
	}
}

func TestHistoryScanFinalEnumerationFailureDiscardsParsedEntries(t *testing.T) {
	for _, kind := range []string{"read", "close", "both"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := scanFixture(t)
			reads := 0
			primary, closing := errors.New("final read"), errors.New("final close")
			opens := 0
			op := historyScanIO{read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
				reads++
				return s.Read(ctx, n)
			}, open: func(r *os.Root, n string) (historyDirectory, error) {
				opens++
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				w := &scanDirectoryFault{File: f}
				if opens == 3 {
					if kind != "close" {
						w.read = func(int) ([]fs.DirEntry, error) { return nil, primary }
					}
					if kind != "read" {
						w.onClose = func() error { return closing }
					}
				}
				return w, nil
			}}
			out, err := planWithIO(context.Background(), s, historyName, "Untitled", time.UTC, lute.New(), op)
			if out != nil || err == nil || reads != 1 {
				t.Fatal(out, err, reads)
			}
			if kind != "close" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			if kind != "read" && !errors.Is(err, closing) {
				t.Fatal(err)
			}
		})
	}
}

func TestHistoryScanCancellationAtEveryReadBoundary(t *testing.T) {
	for _, kind := range []string{"before", "open", "read-dir", "close", "document", "final-close"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := scanFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "before" {
				cancel()
			}
			opens, closes := 0, 0
			op := historyScanIO{open: func(r *os.Root, n string) (historyDirectory, error) {
				opens++
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				w := &scanDirectoryFault{File: f}
				if kind == "open" {
					cancel()
				}
				w.read = func(k int) ([]fs.DirEntry, error) {
					v, err := f.ReadDir(k)
					if kind == "read-dir" {
						cancel()
					}
					return v, err
				}
				w.onClose = func() error {
					closes++
					if kind == "close" || kind == "final-close" && opens == 4 {
						cancel()
					}
					return nil
				}
				return w, nil
			}, read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
				v, err := s.Read(ctx, n)
				if kind == "document" {
					cancel()
				}
				return v, err
			}}
			out, err := planWithIO(ctx, s, historyName, "Untitled", time.UTC, lute.New(), op)
			if out != nil || !errors.Is(err, context.Canceled) || opens != closes {
				t.Fatal(out, err, opens, closes)
			}
		})
	}
}

func TestHistoryScanManifestRejectsChangesAfterDocumentRead(t *testing.T) {
	for _, kind := range []string{"bytes", "inode", "delete", "add-file", "add-dir", "ignored-file", "asset", "dir", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, doc, _ := scanFixture(t)
			s.Data.Mkdir(historyName+"/assets", 0700)
			asset := historyName + "/assets/a.bin"
			s.Data.WriteFile(asset, []byte("asset"), 0600)
			ignored := historyName + "/ignored.txt"
			s.Data.WriteFile(ignored, []byte("ignored"), 0600)
			op := historyScanIO{read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
				v, err := s.Read(ctx, n)
				switch kind {
				case "bytes":
					s.Data.WriteFile(doc, []byte("changed"), 0600)
				case "inode":
					s.Data.Rename(doc, doc+".old")
					s.Data.WriteFile(doc, v, 0600)
				case "delete":
					s.Data.Remove(asset)
				case "add-file":
					s.Data.WriteFile(historyName+"/new.txt", []byte("new"), 0600)
				case "add-dir":
					s.Data.Mkdir(historyName+"/new-dir", 0700)
				case "ignored-file":
					s.Data.WriteFile(ignored, []byte("changed ignored"), 0600)
				case "asset":
					s.Data.WriteFile(asset, []byte("changed asset"), 0600)
				case "dir":
					s.Data.Rename(historyName+"/assets", historyName+"/old-assets")
					s.Data.Mkdir(historyName+"/assets", 0700)
				case "symlink":
					s.Data.Remove(ignored)
					s.Data.Symlink("assets/a.bin", ignored)
				}
				return v, err
			}}
			out, err := planWithIO(context.Background(), s, historyName, "Untitled", time.UTC, lute.New(), op)
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
		})
	}
}

func TestHistoryScanFinalCloseChecksEarlierEntryAndMembership(t *testing.T) {
	for _, kind := range []string{"earlier-file", "stable-metadata-membership"} {
		t.Run(kind, func(t *testing.T) {
			s, doc, _ := scanFixture(t)
			opens := 0
			op := historyScanIO{open: func(r *os.Root, n string) (historyDirectory, error) {
				opens++
				f, err := r.Open(n)
				if err != nil {
					return nil, err
				}
				w := &scanDirectoryFault{File: f}
				if kind == "earlier-file" && opens == 4 {
					w.onClose = func() error { return s.Data.WriteFile(doc, []byte("late"), 0600) }
				}
				if kind == "stable-metadata-membership" && opens == 3 {
					// Change only the observed listing. Metadata stays identical, so
					// this exercises membership comparison independently of stat.
					w.read = func(k int) ([]fs.DirEntry, error) {
						v, err := f.ReadDir(k)
						if len(v) > 0 {
							v = append(v, scanNamedEntry{DirEntry: v[0], name: "ghost"})
						}
						return v, err
					}
				}
				return w, nil
			}}
			out, err := planWithIO(context.Background(), s, historyName, "Untitled", time.UTC, lute.New(), op)
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
		})
	}
}

func TestHistoryScanPreflightLinksEmptyAndReadErrors(t *testing.T) {
	for _, kind := range []string{"empty", "link", "read-error"} {
		t.Run(kind, func(t *testing.T) {
			s, doc, _ := scanFixture(t)
			reads := 0
			sentinel := errors.New("document read")
			if kind == "empty" {
				s.Data.RemoveAll(historyName + "/box")
			}
			if kind == "link" {
				s.Data.Symlink("box", historyName+"/alias")
			}
			op := historyScanIO{read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
				reads++
				return []byte("partial"), sentinel
			}}
			out, err := planWithIO(context.Background(), s, historyName, "Untitled", time.UTC, lute.New(), op)
			if kind == "empty" {
				if err != nil || len(out) != 0 || reads != 0 {
					t.Fatal(out, err, reads)
				}
				return
			}
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
			if kind == "link" && reads != 0 {
				t.Fatal("read before complete manifest", reads)
			}
			if kind == "read-error" && !errors.Is(err, sentinel) {
				t.Fatal(doc, err)
			}
		})
	}
}

func TestHistoryScanFieldsAndReadOnlyBytesRemainOriginal(t *testing.T) {
	s, doc, raw := scanFixture(t)
	out, err := Plan(context.Background(), s, historyName, "Untitled", time.UTC, lute.New())
	if err != nil || len(out) != 1 || out[0].Title != "Original" || out[0].Content != "synthetic history" || out[0].Path != doc || out[0].Op != "sync" || !strings.HasSuffix(out[0].ID, "docaaaa") {
		t.Fatal(out, err)
	}
	after, err := s.Data.ReadFile(doc)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("source rewrite", err)
	}
}
