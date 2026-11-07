// SPDX-License-Identifier: AGPL-3.0-or-later
package notebooks

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
)

type DeletedHistory struct{ Created, Name, Path string }

// Deleted preserves the original *-delete/*/.siyuan/conf.json projection.
// Every selected read and final identity check must succeed before results escape.
func Deleted(ctx context.Context, root *os.Root) (out []DeletedHistory, err error) {
	if ctx == nil || root == nil {
		return nil, ErrPermission
	}
	source := indexrepair.Source{Data: root}
	expected := map[string]fs.FileInfo{}
	remember := func(name string) (fs.FileInfo, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if name != "." {
			if e := source.Check(name, false); e != nil {
				return nil, e
			}
		}
		info, e := root.Lstat(name)
		if e != nil {
			return nil, e
		}
		if info.Mode()&fs.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return nil, ErrPath
		}
		expected[name] = info
		return info, nil
	}
	if _, e := remember("."); e != nil {
		return nil, e
	}
	stamps, e := source.ReadDir(ctx, ".")
	if e != nil {
		return nil, e
	}
	out = []DeletedHistory{}
	for _, stamp := range stamps {
		if !strings.HasSuffix(stamp.Name(), "-delete") {
			continue
		}
		if stamp.Type()&fs.ModeSymlink != 0 {
			return nil, ErrPath
		}
		if !stamp.IsDir() {
			continue
		}
		if _, e = remember(stamp.Name()); e != nil {
			return nil, e
		}
		boxes, e := source.ReadDir(ctx, stamp.Name())
		if e != nil {
			return nil, e
		}
		for _, box := range boxes {
			if box.Type()&fs.ModeSymlink != 0 {
				return nil, ErrPath
			}
			if !box.IsDir() {
				continue
			}
			folder := path.Join(stamp.Name(), box.Name())
			if _, e = remember(folder); e != nil {
				return nil, e
			}
			metadata := path.Join(folder, ".siyuan")
			if _, e = remember(metadata); errors.Is(e, fs.ErrNotExist) {
				continue
			} else if e != nil {
				return nil, e
			}
			name := path.Join(metadata, "conf.json")
			if _, e = remember(name); errors.Is(e, fs.ErrNotExist) {
				continue
			} else if e != nil {
				return nil, e
			}
			raw, e := source.Read(ctx, name)
			if e != nil {
				return nil, e
			}
			var value conf.BoxConf
			if e = json.Unmarshal(raw, &value); e != nil {
				return nil, e
			}
			created := strings.TrimSuffix(stamp.Name(), "-delete")
			if parsed, e := time.Parse("2006-01-02-150405", created); e == nil {
				created = parsed.Format("2006-01-02 15:04:05")
			}
			out = append(out, DeletedHistory{Created: created, Name: value.Name, Path: folder})
		}
	}
	for name, saved := range expected {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if name != "." {
			if e = source.Check(name, false); e != nil {
				return nil, e
			}
		}
		now, e := root.Lstat(name)
		if e != nil {
			return nil, e
		}
		if !os.SameFile(now, saved) || now.Mode() != saved.Mode() || now.Size() != saved.Size() || !now.ModTime().Equal(saved.ModTime()) {
			return nil, ErrChanged
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out, ctx.Err()
}
