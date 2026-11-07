// SPDX-License-Identifier: AGPL-3.0-or-later
package sourcetx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// ReadGuards bind candidate dependencies as well as written files. Directory
// membership guards reject phantoms such as a newly added mirror document.
type ReadGuards struct {
	Missing     []string            `json:"missing,omitempty"`
	Files       map[string]string   `json:"files,omitempty"`
	Directories map[string][]string `json:"directories,omitempty"`
}

func cloneReadGuards(g *ReadGuards) *ReadGuards {
	if g == nil {
		return nil
	}
	c := ReadGuards{Missing: slices.Clone(g.Missing), Files: map[string]string{}, Directories: map[string][]string{}}
	for n, h := range g.Files {
		c.Files[n] = h
	}
	for n, v := range g.Directories {
		c.Directories[n] = slices.Clone(v)
	}
	return &c
}

func guardedSourcePath(name string) bool {
	p := strings.Split(name, "/")
	if len(p) >= 3 && p[1] == "documents" && pageID.MatchString(p[2]) {
		switch p[0] {
		case "tombstones", "deleted", "versions":
			return len(p) == 3 || len(p) == 4 && docID.MatchString(p[3]) ||
				sourcePath(name) || len(p) >= 5 && docID.MatchString(p[3]) && docID.MatchString(p[4]) &&
				(p[0] == "deleted" && (len(p) == 5 || len(p) == 6 && (p[5] == p[4] || p[5] == "effects")) ||
					p[0] == "versions" && (len(p) == 5 || len(p) == 6 && operationID.MatchString(p[5]) || len(p) == 7 && operationID.MatchString(p[5]) && p[6] == "av"))
		}
	}
	if len(p) >= 3 && p[1] == "notebooks" && pageID.MatchString(p[2]) {
		return (p[0] == "tombstones" && len(p) == 3) || sourcePath(name)
	}
	return (sourcePath(name) && len(p) > 3 && p[2] == "data") ||
		(len(p) == 3 && p[0] == "pages" && pageID.MatchString(p[1]) && p[2] == "data")
}

func verifyReadGuards(ctx context.Context, root *os.Root, guards *ReadGuards) error {
	if guards == nil {
		return ctx.Err()
	}
	if len(guards.Files)+len(guards.Directories)+len(guards.Missing) > 100000 {
		return ErrInvalid
	}
	for _, name := range guards.Missing {
		if !guardedSourcePath(name) {
			return ErrInvalid
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := safePath(root, name); err != nil {
			return err
		}
		if _, err := root.Lstat(name); err == nil {
			return ErrConflict
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	for name, hash := range guards.Files {
		if !guardedSourcePath(name) || !hashID.MatchString(hash) {
			return ErrInvalid
		}
		actual, err := guardedFileHash(ctx, root, name)
		if err != nil {
			return err
		}
		if actual != hash {
			return ErrConflict
		}
	}
	for name, expected := range guards.Directories {
		if !guardedSourcePath(name) || len(expected) > 100000 || !slices.IsSorted(expected) {
			return ErrInvalid
		}
		for i, child := range expected {
			if child == "." || child == ".." || path.Base(child) != child || strings.ContainsAny(child, "\\\x00") || (i > 0 && child == expected[i-1]) {
				return ErrInvalid
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := safePath(root, name); err != nil {
			return err
		}
		f, err := root.Open(name)
		if err != nil {
			return errors.Join(ErrConflict, err)
		}
		entries, err := f.ReadDir(-1)
		err = errors.Join(err, f.Close())
		if err != nil {
			return err
		}
		actual := make([]string, 0, len(entries))
		for _, entry := range entries {
			actual = append(actual, entry.Name())
		}
		slices.Sort(actual)
		if !slices.Equal(actual, expected) {
			return ErrConflict
		}
	}
	return ctx.Err()
}

func guardedFileHash(ctx context.Context, root *os.Root, name string) (out string, err error) {
	if err = safePath(root, name); err != nil {
		return
	}
	before, err := root.Lstat(name)
	if err != nil {
		return "", errors.Join(ErrConflict, err)
	}
	if !before.Mode().IsRegular() {
		return "", ErrInvalid
	}
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			out = ""
		}
	}()
	opened, err := f.Stat()
	if err != nil || !readInfoEqual(before, opened) {
		return "", errors.Join(ErrConflict, err)
	}
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var size int64
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, e := f.Read(buf)
		size += int64(n)
		if size > before.Size() {
			return "", ErrConflict
		}
		h.Write(buf[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		if n == 0 {
			return "", io.ErrNoProgress
		}
	}
	after, err := f.Stat()
	if err != nil || size != before.Size() || !readInfoEqual(before, after) {
		return "", errors.Join(ErrConflict, err)
	}
	visible, err := root.Lstat(name)
	if err != nil || !readInfoEqual(before, visible) {
		return "", errors.Join(ErrConflict, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
