// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
)

type sourceReadFaultFile struct {
	*os.File
	read          func([]byte) (int, error)
	stat          func() (fs.FileInfo, error)
	closeErr      error
	onClose       func() error
	reads, closes int
}

func (f *sourceReadFaultFile) Read(b []byte) (int, error) {
	f.reads++
	if f.read != nil {
		return f.read(b)
	}
	return f.File.Read(b)
}
func (f *sourceReadFaultFile) Stat() (fs.FileInfo, error) {
	if f.stat != nil {
		return f.stat()
	}
	return f.File.Stat()
}
func (f *sourceReadFaultFile) Close() error {
	f.closes++
	err := f.File.Close()
	if f.onClose != nil {
		err = errors.Join(err, f.onClose())
	}
	return errors.Join(err, f.closeErr)
}
func readFixture(t *testing.T) Source {
	t.Helper()
	s := sourceFixture(t)
	s.Data.Mkdir("box", 0700)
	if err := s.Data.WriteFile("box/doc", []byte("original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSourceReadReadOnlyCapabilityEmptyAndMultiChunkBytes(t *testing.T) {
	s := readFixture(t)
	s.Authorize = func(context.Context) error { t.Fatal("read requested write authority"); return ErrSourceWrite }
	s.Lock = func(string) func() { t.Fatal("read recursively acquired writer lock"); return func() {} }
	expected := make([]byte, 140000)
	for i := range expected {
		expected[i] = byte(i*31 + i/65536)
	}
	s.Data.WriteFile("box/big", expected, 0600)
	s.Data.WriteFile("box/empty", []byte{}, 0600)
	for _, q := range []struct {
		name string
		data []byte
	}{{"box/big", expected}, {"box/empty", []byte{}}} {
		raw, err := s.Read(context.Background(), q.name)
		if err != nil || raw == nil || !bytes.Equal(raw, q.data) {
			t.Fatal(q.name, len(raw), err)
		}
	}
	if _, err := s.Data.Stat("box/doc"); err != nil {
		t.Fatal("borrowed data root was closed", err)
	}
}
func TestSourceReadInitialMissingRetainsOriginalClassificationAndNoOpen(t *testing.T) {
	s := readFixture(t)
	opens := 0
	raw, err := s.read(context.Background(), "missing/doc", sourceReadIO{open: func(*os.Root, string) (sourceReadFile, error) { opens++; return nil, errors.New("must not open") }})
	if raw != nil || !os.IsNotExist(err) || !IsSourceNotExist(err) || errors.Is(err, ErrSourceReadIncomplete) || opens != 0 {
		t.Fatal(raw, err, opens)
	}
}
func TestSourceReadInvalidPathsDirectoriesAndLinksAreRejected(t *testing.T) {
	s := readFixture(t)
	s.Data.Symlink("doc", "box/link")
	s.Data.Symlink("box", "alias")
	for _, name := range []string{"../doc", "/doc", "box\\doc", ".", "box", "box/link", "alias/doc"} {
		opens := 0
		_, err := s.read(context.Background(), name, sourceReadIO{open: func(*os.Root, string) (sourceReadFile, error) { opens++; return nil, errors.New("must not open") }})
		if !errors.Is(err, ErrSourcePath) || opens != 0 {
			t.Fatal(name, err, opens)
		}
	}
}
func TestSourceReadOpenStatReadAndCloseFailuresReturnNoBytes(t *testing.T) {
	for _, kind := range []string{"open", "stat", "read", "close", "close-missing", "parent-close"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			primary := errors.New("source " + kind)
			if kind == "close-missing" {
				primary = fs.ErrNotExist
			}
			var input *sourceReadFaultFile
			parentCloses := 0
			op := sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
				if kind == "open" {
					return nil, primary
				}
				file, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				input = &sourceReadFaultFile{File: file}
				switch kind {
				case "stat":
					input.stat = func() (fs.FileInfo, error) { return nil, primary }
				case "read":
					input.read = func(b []byte) (int, error) { copy(b, "prefix"); return 6, primary }
				case "close", "close-missing":
					input.closeErr = primary
				}
				return input, nil
			}, closeRoot: func(root *os.Root) error {
				parentCloses++
				err := root.Close()
				if kind == "parent-close" {
					return errors.Join(err, primary)
				}
				return err
			}}
			raw, err := s.read(context.Background(), "box/doc", op)
			if raw != nil || !errors.Is(err, primary) || !errors.Is(err, ErrSourceReadIncomplete) || IsSourceNotExist(err) || parentCloses != 1 {
				t.Fatal(raw, err, parentCloses)
			}
			if input != nil && input.closes != 1 {
				t.Fatal(input.closes)
			}
			rewriteExpectBytes(t, s.Data, "box/doc", "original bytes")
		})
	}
}
func TestSourceReadPrimaryFileAndParentCloseErrorsAllSurvive(t *testing.T) {
	s := readFixture(t)
	readErr, fileErr, parentErr := errors.New("read"), errors.New("file close"), errors.New("parent close")
	var input *sourceReadFaultFile
	raw, err := s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		input = &sourceReadFaultFile{File: file, read: func([]byte) (int, error) { return 0, readErr }, closeErr: fileErr}
		return input, nil
	}, closeRoot: func(root *os.Root) error { return errors.Join(root.Close(), parentErr) }})
	if raw != nil || input.closes != 1 {
		t.Fatal(raw, input.closes)
	}
	for _, cause := range []error{readErr, fileErr, parentErr, ErrSourceReadIncomplete} {
		if !errors.Is(err, cause) {
			t.Fatal(cause, err)
		}
	}
}
func TestSourceReadCancellationBeforeDuringAndClosingRejectsPayload(t *testing.T) {
	for _, kind := range []string{"before", "read", "file-close", "parent-close"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opens := 0
			if kind == "before" {
				cancel()
			}
			raw, err := s.read(ctx, "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
				opens++
				file, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				input := &sourceReadFaultFile{File: file}
				if kind == "read" {
					input.read = func(b []byte) (int, error) { n, e := file.Read(b); cancel(); return n, e }
				}
				if kind == "file-close" {
					input.onClose = func() error { cancel(); return nil }
				}
				return input, nil
			}, closeRoot: func(root *os.Root) error {
				err := root.Close()
				if kind == "parent-close" {
					cancel()
				}
				return err
			}})
			if raw != nil || !errors.Is(err, context.Canceled) {
				t.Fatal(raw, err)
			}
			if kind == "before" && opens != 0 {
				t.Fatal(opens)
			}
		})
	}
}
func TestSourceReadObservedSourceParentAndLinkReplacementsRejectBytes(t *testing.T) {
	for _, kind := range []string{"file", "bytes", "parent", "link"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			changed := false
			raw, err := s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
				file, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				return &sourceReadFaultFile{File: file, read: func(b []byte) (int, error) {
					n, e := file.Read(b)
					if !changed {
						changed = true
						switch kind {
						case "file":
							s.Data.Rename("box/doc", "box/old")
							s.Data.WriteFile("box/doc", []byte("replacement"), 0600)
						case "bytes":
							s.Data.WriteFile("box/doc", []byte("updated source bytes"), 0600)
						case "parent":
							s.Data.Rename("box", "moved")
							s.Data.Mkdir("box", 0700)
							s.Data.WriteFile("box/doc", []byte("foreign parent"), 0600)
						case "link":
							s.Data.Rename("box/doc", "box/old")
							s.Data.Symlink("old", "box/doc")
						}
					}
					return n, e
				}}, nil
			}})
			if raw != nil || err == nil || !errors.Is(err, ErrSourceReadIncomplete) {
				t.Fatal(raw, err)
			}
			expected := "replacement"
			if kind == "bytes" {
				expected = "updated source bytes"
			}
			if kind == "parent" {
				expected = "foreign parent"
			}
			if kind == "link" {
				expected = "original bytes"
			}
			rewriteExpectBytes(t, s.Data, "box/doc", expected)
		})
	}
}
func TestSourceReadSelectedDescriptorIdentityMustMatchPath(t *testing.T) {
	s := readFixture(t)
	s.Data.WriteFile("box/other", []byte("other descriptor"), 0600)
	var input *sourceReadFaultFile
	raw, err := s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
		file, err := root.Open("other")
		if err != nil {
			return nil, err
		}
		input = &sourceReadFaultFile{File: file}
		return input, nil
	}})
	if raw != nil || !errors.Is(err, ErrSourceChanged) || input.reads != 0 || input.closes != 1 {
		t.Fatal(raw, err, input)
	}
}
func TestSourceReadLateCloseChangesAndDisappearanceAreNotMissingDefaults(t *testing.T) {
	for _, kind := range []string{"file-close", "parent-close", "disappear"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			change := func() error {
				if kind == "disappear" {
					return s.Data.Remove("box/doc")
				}
				s.Data.Rename("box/doc", "box/old")
				return s.Data.WriteFile("box/doc", []byte("late replacement"), 0600)
			}
			raw, err := s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
				file, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				input := &sourceReadFaultFile{File: file}
				if kind != "parent-close" {
					input.onClose = change
				}
				return input, nil
			}, closeRoot: func(root *os.Root) error {
				err := root.Close()
				if kind == "parent-close" {
					err = errors.Join(err, change())
				}
				return err
			}})
			if raw != nil || err == nil || IsSourceNotExist(err) {
				t.Fatal(raw, err)
			}
			if kind == "disappear" && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("missing cause lost", err)
			}
		})
	}
}
func TestSourceReadShortReadsProgressAndBorrowedResourcesCloseOnce(t *testing.T) {
	s := readFixture(t)
	var input *sourceReadFaultFile
	var parent *os.Root
	closes := 0
	raw, err := s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		input = &sourceReadFaultFile{File: file, read: func(b []byte) (int, error) { return file.Read(b[:2]) }}
		return input, nil
	}, closeRoot: func(root *os.Root) error { closes++; parent = root; return root.Close() }})
	if err != nil || string(raw) != "original bytes" || input.closes != 1 || closes != 1 {
		t.Fatal(string(raw), err, closes)
	}
	if _, err := input.File.Stat(); err == nil {
		t.Fatal("file descriptor leaked")
	}
	if _, err := parent.Stat("."); err == nil {
		t.Fatal("parent root leaked")
	}
	_, err = s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
		file, e := root.Open(name)
		return &sourceReadFaultFile{File: file, read: func([]byte) (int, error) { return 0, nil }}, e
	}})
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatal(err)
	}
}
func TestSourceReadPureMissingClassifierRejectsCompoundFailures(t *testing.T) {
	failed := errors.New("close")
	for _, q := range []struct {
		err     error
		missing bool
	}{
		{nil, false}, {fs.ErrNotExist, true}, {fmt.Errorf("open: %w", fs.ErrNotExist), true},
		{errors.Join(fs.ErrNotExist, fs.ErrNotExist), true},
		{errors.Join(fs.ErrNotExist, failed), false},
		{errors.Join(fs.ErrNotExist, ErrSourceReadIncomplete), false},
		{fmt.Errorf("read: %w", errors.Join(fs.ErrNotExist, ErrSourceReadIncomplete, failed)), false},
	} {
		if got := IsSourceNotExist(q.err); got != q.missing {
			t.Fatal(got, q.err)
		}
	}
}

func TestSourceReadIncompleteEOFCannotReturnSuccessfulPayload(t *testing.T) {
	for _, kind := range []string{"compound", "short", "empty", "excess"} {
		t.Run(kind, func(t *testing.T) {
			s := readFixture(t)
			failure := errors.New("read failure alongside EOF")
			raw, err := s.read(context.Background(), "box/doc", sourceReadIO{open: func(root *os.Root, name string) (sourceReadFile, error) {
				file, err := root.Open(name)
				if err != nil {
					return nil, err
				}
				return &sourceReadFaultFile{File: file, read: func(b []byte) (int, error) {
					if kind == "empty" {
						return 0, io.EOF
					}
					if kind == "excess" {
						copy(b, "more than original source length")
						return 31, io.EOF
					}
					copy(b, "prefix")
					if kind == "compound" {
						return 6, errors.Join(io.EOF, failure)
					}
					return 6, io.EOF
				}}, nil
			}})
			expected := error(io.ErrUnexpectedEOF)
			if kind == "compound" {
				expected = failure
			}
			if kind == "excess" {
				expected = ErrSourceChanged
			}
			if raw != nil || !errors.Is(err, expected) || !errors.Is(err, ErrSourceReadIncomplete) {
				t.Fatal(raw, err)
			}
		})
	}
}
