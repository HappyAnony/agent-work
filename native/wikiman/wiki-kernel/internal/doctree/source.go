// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

import (
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

// SourceServices binds the original listing algorithm to a pinned, checked
// source. It never repairs a notebook or moves corrupt authoritative files.
func SourceServices(box string, source indexrepair.Source, base Services, sortPath ...string) Services {
	relative := func(p string) (string, error) {
		p = strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
		if !ast.IsNodeIDPattern(box) || (p != "" && !fs.ValidPath(p)) {
			return "", indexrepair.ErrSourcePath
		}
		if p == "" {
			return box, nil
		}
		return box + "/" + p, nil
	}
	stat := func(ctx context.Context, p string) (fs.FileInfo, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		name, e := relative(p)
		if e != nil {
			return nil, e
		}
		if e = source.Check(name, false); e != nil {
			return nil, e
		}
		return source.Data.Lstat(name)
	}
	readDir := func(ctx context.Context, p string, filter bool) ([]Entry, error) {
		name, e := relative(p)
		if e != nil {
			return nil, e
		}
		entries, e := source.ReadDir(ctx, name)
		if e != nil {
			return nil, e
		}
		out := []Entry{}
		for _, entry := range entries {
			n := entry.Name()
			if filter && (strings.HasPrefix(n, ".") || n == "assets" || n == "templates" || n == "widgets" || n == "emojis" || strings.HasSuffix(n, ".tmp")) {
				continue
			}
			f, e := stat(ctx, path.Join(p, n))
			if e != nil {
				return nil, e
			}
			p2 := path.Join(p, n)
			if f.IsDir() {
				p2 += "/"
			}
			out = append(out, Entry{Path: p2, Name: n, Size: f.Size(), Directory: f.IsDir()})
		}
		return out, ctx.Err()
	}
	base.List = func(ctx context.Context, p string) ([]Entry, error) {
		if strings.HasSuffix(p, ".docj") {
			p = strings.TrimSuffix(p, ".docj")
			f, e := stat(ctx, p)
			if indexrepair.IsSourceNotExist(e) {
				return []Entry{}, nil
			}
			if e != nil {
				return nil, e
			}
			if !f.IsDir() {
				return []Entry{}, nil
			}
		}
		return readDir(ctx, p, true)
	}
	base.Stat = func(ctx context.Context, p string) (*Entry, error) {
		f, e := stat(ctx, p)
		if indexrepair.IsSourceNotExist(e) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		return &Entry{Path: p, Name: f.Name(), Size: f.Size(), Directory: f.IsDir()}, ctx.Err()
	}
	base.IsDir = func(ctx context.Context, p string) (bool, error) {
		f, e := stat(ctx, p)
		if indexrepair.IsSourceNotExist(e) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		return f.IsDir(), ctx.Err()
	}
	base.Children = func(ctx context.Context, p string) ([]Entry, error) { return readDir(ctx, p, false) }
	base.IAL = func(ctx context.Context, p string) (map[string]string, error) {
		if !strings.HasSuffix(strings.ToLower(path.Base(p)), ".docj") {
			return nil, nil
		}
		name, e := relative(p)
		if e != nil {
			return nil, e
		}
		raw, e := source.ReadLimited(ctx, name, 16<<20)
		if e != nil {
			return nil, e
		}
		var doc struct {
			ID, Type   string
			Properties map[string]string
		}
		if e = json.Unmarshal(raw, &doc); e != nil {
			return nil, e
		}
		if doc.Type != "NodeDocument" || doc.ID != strings.TrimSuffix(path.Base(p), ".docj") || doc.Properties["id"] != doc.ID || !ast.IsNodeIDPattern(doc.ID) {
			return nil, indexrepair.ErrSourcePath
		}
		return doc.Properties, ctx.Err()
	}
	base.Sorts = func(ctx context.Context, files []*File) error {
		name := box + "/.wiki/sort.json"
		if len(sortPath) > 0 {
			name = sortPath[0]
		}
		raw, e := source.ReadLimited(ctx, name, 1<<20)
		if indexrepair.IsSourceNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		var values map[string]int
		if e = json.Unmarshal(raw, &values); e != nil {
			return e
		}
		for _, f := range files {
			f.Sort = values[f.ID]
		}
		return ctx.Err()
	}
	return base
}
