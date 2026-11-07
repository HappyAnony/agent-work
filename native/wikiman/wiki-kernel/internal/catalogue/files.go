// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package catalogue

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type fileRoot struct {
	root     *os.Root
	name     string
	identity fs.FileInfo
}
type fileVersion struct {
	root *os.Root
	name string
	info fs.FileInfo
}

// Files holds operation-local directory capabilities. Linked package roots are
// admitted only by the explicit compatibility/device binding, never by JSON.
type Files struct {
	mu       sync.Mutex
	roots    map[string]*fileRoot
	packages map[string]*fileRoot
	linked   bool
	closed   bool
	versions map[string]fileVersion
}

func OpenFiles(roots map[string]string, allowLinkedDirectories bool) (out *Files, err error) {
	out = &Files{roots: map[string]*fileRoot{}, packages: map[string]*fileRoot{}, linked: allowLinkedDirectories, versions: map[string]fileVersion{}}
	for kind, name := range roots {
		if !validKind(kind) || !filepath.IsAbs(name) {
			out.Close()
			return nil, ErrValue
		}
		root, e := os.OpenRoot(name)
		if absent(e) {
			out.roots[kind] = &fileRoot{name: name}
			continue
		}
		if e != nil {
			out.Close()
			return nil, e
		}
		info, e := root.Stat(".")
		if e != nil {
			root.Close()
			out.Close()
			return nil, e
		}
		out.roots[kind] = &fileRoot{root: root, name: name, identity: info}
	}
	return out, nil
}
func (f *Files) base(kind string) (*fileRoot, error) {
	r, ok := f.roots[kind]
	if !ok {
		return nil, ErrDependency
	}
	if r.root == nil {
		return nil, fs.ErrNotExist
	}
	return r, nil
}
func (f *Files) packageRoot(kind, name string) (*fileRoot, error) {
	if !validName(name) {
		return nil, ErrValue
	}
	key := kind + "/" + name
	if r := f.packages[key]; r != nil {
		return r, nil
	}
	if len(f.packages) > 2048 {
		return nil, ErrValue
	}
	base, e := f.base(kind)
	if e != nil {
		return nil, e
	}
	initial, e := base.root.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !f.linked && initial.Mode()&fs.ModeSymlink != 0 {
		return nil, ErrValue
	}
	var root *os.Root
	if f.linked {
		root, e = os.OpenRoot(filepath.Join(base.name, name))
	} else {
		root, e = base.root.OpenRoot(name)
	}
	if e != nil {
		return nil, e
	}
	info, e := root.Stat(".")
	if e != nil {
		root.Close()
		return nil, e
	}
	r := &fileRoot{root: root, name: filepath.Join(base.name, name), identity: info}
	f.packages[key] = r
	return r, nil
}
func (f *Files) Read(ctx context.Context, kind, name, rel string) (raw []byte, err error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !fs.ValidPath(rel) || strings.Contains(rel, "\\") {
		return nil, ErrValue
	}
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, fs.ErrClosed
	}
	root, e := f.packageRoot(kind, name)
	f.mu.Unlock()
	if e != nil {
		return nil, e
	}
	file, e := root.root.Open(rel)
	if e != nil {
		return nil, e
	}
	before, e := file.Stat()
	if e != nil {
		return nil, errors.Join(e, file.Close())
	}
	raw, e = io.ReadAll(io.LimitReader(file, MaxBytes+1))
	after, statErr := file.Stat()
	e = errors.Join(e, statErr)
	if statErr == nil && !sameVersion(before, after) {
		e = errors.Join(e, errors.New("catalogue_file_changed"))
	}
	e = errors.Join(e, file.Close())
	if e != nil {
		return nil, e
	}
	if len(raw) > MaxBytes {
		return nil, ErrValue
	}
	f.mu.Lock()
	e = f.remember(kind+"/"+name+"/"+rel, root.root, rel, before)
	f.mu.Unlock()
	if e != nil {
		return nil, e
	}
	return raw, ctx.Err()
}
func (f *Files) Stat(ctx context.Context, kind, name, rel string) (fs.FileInfo, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !validName(name) || !fs.ValidPath(rel) || strings.Contains(rel, "\\") {
		return nil, ErrValue
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, fs.ErrClosed
	}
	base, e := f.base(kind)
	if e != nil {
		return nil, e
	}
	if rel == "." {
		info, e := base.root.Lstat(name)
		if e != nil {
			return nil, e
		}
		if info.Mode()&fs.ModeSymlink == 0 && !info.IsDir() {
			return info, nil
		}
	}
	root, e := f.packageRoot(kind, name)
	if e != nil {
		return nil, e
	}
	info, e := root.root.Stat(rel)
	if e != nil {
		return nil, e
	}
	if e = f.remember(kind+"/"+name+"/"+rel, root.root, rel, info); e != nil {
		return nil, e
	}
	return info, ctx.Err()
}
func (f *Files) List(ctx context.Context, kind, rel string) ([]fs.DirEntry, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if rel != "" && (!fs.ValidPath(rel) || strings.Contains(rel, "\\")) {
		return nil, ErrValue
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, fs.ErrClosed
	}
	var root *os.Root
	target := "."
	if rel == "" {
		base, e := f.base(kind)
		if e != nil {
			return nil, e
		}
		root = base.root
	} else {
		parts := strings.SplitN(rel, "/", 2)
		r, e := f.packageRoot(kind, parts[0])
		if e != nil {
			return nil, e
		}
		root = r.root
		if len(parts) > 1 {
			target = parts[1]
		}
	}
	file, e := root.Open(path.Clean(target))
	if e != nil {
		return nil, e
	}
	info, e := file.Stat()
	if e != nil {
		return nil, errors.Join(e, file.Close())
	}
	entries, readErr := file.ReadDir(-1)
	after, statErr := file.Stat()
	e = errors.Join(readErr, statErr, file.Close())
	if e != nil {
		return nil, e
	}
	if !sameVersion(info, after) {
		return nil, errors.New("catalogue_directory_changed")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	if e = f.remember("dir/"+kind+"/"+rel, root, path.Clean(target), info); e != nil {
		return nil, e
	}
	return entries, ctx.Err()
}
func (f *Files) Check(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return fs.ErrClosed
	}
	for _, group := range []map[string]*fileRoot{f.roots, f.packages} {
		for _, r := range group {
			info, e := os.Stat(r.name)
			if r.root == nil {
				if absent(e) {
					continue
				}
				if e != nil {
					return e
				}
				return errors.New("catalogue_root_changed")
			}
			if e != nil {
				return e
			}
			if !sameVersion(info, r.identity) {
				return errors.New("catalogue_root_changed")
			}
		}
	}
	for _, v := range f.versions {
		info, e := v.root.Stat(v.name)
		if e != nil {
			return e
		}
		if !sameVersion(info, v.info) {
			return errors.New("catalogue_file_changed")
		}
	}
	return ctx.Err()
}
func sameVersion(a, b fs.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func (f *Files) remember(key string, root *os.Root, name string, info fs.FileInfo) error {
	if old, ok := f.versions[key]; ok {
		if !sameVersion(old.info, info) {
			return errors.New("catalogue_file_changed")
		}
		return nil
	}
	if len(f.versions) > 100000 {
		return ErrValue
	}
	f.versions[key] = fileVersion{root: root, name: name, info: info}
	return nil
}
func (f *Files) Close() (err error) {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	for _, group := range []map[string]*fileRoot{f.packages, f.roots} {
		for _, r := range group {
			if r.root != nil {
				err = errors.Join(err, r.root.Close())
			}
		}
	}
	return
}
