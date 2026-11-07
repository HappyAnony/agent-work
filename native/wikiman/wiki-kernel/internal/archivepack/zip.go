// SPDX-License-Identifier: AGPL-3.0-or-later
// Original checked ZIP algorithm; all source/output/guard capabilities explicit.
package archivepack

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

type WalkIO struct {
	ReadDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}
type Entry struct {
	Name  string
	Info  fs.FileInfo
	Empty bool
}

func CheckManifest(source indexrepair.Source, entries []Entry, guard func() error) error {
	for _, entry := range entries {
		if err := guard(); err != nil {
			return err
		}
		if entry.Name != "." {
			if err := source.Check(entry.Name, false); err != nil {
				return err
			}
		}
		info, err := source.Data.Lstat(entry.Name)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.Info.IsDir() || info.Mode().Type() != entry.Info.Mode().Type() || !os.SameFile(info, entry.Info) || info.Size() != entry.Info.Size() || !info.ModTime().Equal(entry.Info.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
	}
	return guard()
}

func Manifest(ctx context.Context, source indexrepair.Source, guard func() error, op WalkIO) ([]Entry, error) {
	if ctx == nil || source.Data == nil || guard == nil {
		return nil, indexrepair.ErrSourcePath
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if op.ReadDir == nil {
		op.ReadDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	root, err := source.Data.Lstat(".")
	if err != nil {
		return nil, err
	}
	if !root.IsDir() || root.Mode()&fs.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	out := []Entry{{Name: ".", Info: root}}
	var scan func(string, int) error
	scan = func(folder string, index int) error {
		if len(out) > 1000000 || strings.Count(folder, "/") > 255 {
			return indexrepair.ErrSourcePath
		}
		if err := guard(); err != nil {
			return err
		}
		entries, err := op.ReadDir(ctx, source, folder)
		if err != nil {
			return err
		}
		out[index].Empty = len(entries) == 0
		for _, entry := range entries {
			if err = guard(); err != nil {
				return err
			}
			name := path.Join(folder, entry.Name())
			if err = source.Check(name, false); err != nil {
				return err
			}
			info, statErr := source.Data.Lstat(name)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
				return indexrepair.ErrSourceChanged
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return indexrepair.ErrSourcePath
			}
			next := len(out)
			out = append(out, Entry{Name: name, Info: info})
			if info.IsDir() {
				if err = scan(name, next); err != nil {
					return err
				}
			}
		}
		return guard()
	}
	if err = scan(".", 0); err != nil {
		return nil, err
	}
	if err = CheckManifest(source, out, guard); err != nil {
		return nil, err
	}
	return out, nil
}

// Keep Deflate, optional original root name, modes/mtimes and empty directories. The
// complete lexical manifest is admitted before any member; source links are never followed.
func Write(source indexrepair.Source, prefix string, dst io.Writer, guard func() error, copy func(io.Writer, io.Reader) error) error {
	return WriteWithIO(context.Background(), source, prefix, dst, guard, copy, WalkIO{})
}
func WriteWithIO(ctx context.Context, source indexrepair.Source, prefix string, dst io.Writer, guard func() error, copy func(io.Writer, io.Reader) error, op WalkIO) (err error) {
	if dst == nil || copy == nil || (prefix != "" && (!fs.ValidPath(prefix) || prefix == "." || strings.Contains(prefix, "\\"))) {
		return indexrepair.ErrSourcePath
	}
	entries, err := Manifest(ctx, source, guard, op)
	if err != nil {
		return err
	}
	z := zip.NewWriter(dst)
	defer func() { err = errors.Join(err, z.Close()) }()
	for _, entry := range entries {
		if err = CheckManifest(source, []Entry{entry}, guard); err != nil {
			return err
		}
		if entry.Info.IsDir() && !entry.Empty {
			continue
		}
		if entry.Name == "." && prefix == "" {
			continue // A flat Markdown archive has no wrapper, including when empty.
		}
		header, err := zip.FileInfoHeader(entry.Info)
		if err != nil {
			return err
		}
		header.Name = path.Join(prefix, entry.Name)
		header.Method = zip.Deflate
		if entry.Info.IsDir() {
			header.Name += "/"
		}
		out, err := z.CreateHeader(header)
		if err != nil {
			return err
		}
		if entry.Info.IsDir() {
			continue
		}
		in, err := source.Data.Open(entry.Name)
		if err != nil {
			return err
		}
		copyErr := copy(out, in)
		info, statErr := in.Stat()
		err = errors.Join(copyErr, statErr, in.Close())
		if err == nil && (info == nil || !os.SameFile(info, entry.Info) || info.Size() != entry.Info.Size() || !info.ModTime().Equal(entry.Info.ModTime())) {
			err = indexrepair.ErrSourceChanged
		}
		if err = errors.Join(err, CheckManifest(source, []Entry{entry}, guard)); err != nil {
			return err
		}
	}
	return CheckManifest(source, entries, guard)
}

func CopyContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 64<<10)
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		n, e := src.Read(buf)
		if stop := ctx.Err(); stop != nil {
			return stop
		}
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if e == io.EOF {
			return ctx.Err()
		}
		if e != nil {
			return e
		}
	}
}
