// SPDX-License-Identifier: AGPL-3.0-or-later
package workspacefile

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"
	"time"
	"wiki-kernel/internal/indexrepair"
)

var ErrDirectory = errors.New("file_is_directory")
var ErrNotDirectory = errors.New("file_is_not_directory")

type Request struct {
	Action, Path, NewPath string
	Data                  []byte
	Directory             bool
	ModTime               time.Time
	ReadLimit             int64
}
type Entry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"isDir"`
	IsSymlink bool   `json:"isSymlink"`
	Updated   int64  `json:"updated"`
}
type Result struct {
	Data    []byte
	Entries []Entry
}

// Name retains the compatibility API's workspace-relative leading slash.
func Name(raw string, allowRoot bool) (string, error) {
	if strings.Contains(raw, "\\") {
		return "", indexrepair.ErrSourcePath
	}
	name := path.Clean(strings.TrimLeft(raw, "/"))
	if name == "." && allowRoot {
		return name, nil
	}
	if !fs.ValidPath(name) || name == "." {
		return "", indexrepair.ErrSourcePath
	}
	return name, nil
}

// Execute uses a borrowed selected source. The caller owns admission, closes
// roots before publishing success and grants write authority only explicitly.
func Execute(ctx context.Context, s indexrepair.Source, q Request) (out Result, err error) {
	name, err := Name(q.Path, q.Action == "list")
	if err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	switch q.Action {
	case "read":
		info, e := s.Data.Lstat(name)
		if e != nil {
			return out, e
		}
		if info.IsDir() {
			return out, ErrDirectory
		}
		limit := int64(512 << 20)
		if q.ReadLimit > 0 && q.ReadLimit < limit {
			limit = q.ReadLimit
		}
		out.Data, err = s.ReadLimited(ctx, name, limit)
	case "list":
		entries, e := s.ReadDir(ctx, name)
		if e != nil {
			return out, e
		}
		out.Entries = []Entry{}
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				return Result{}, e
			}
			child := path.Join(name, entry.Name())
			if e = s.Check(child, false); e != nil {
				return Result{}, e
			}
			info, e := entry.Info()
			if e != nil {
				return Result{}, e
			}
			out.Entries = append(out.Entries, Entry{Name: entry.Name(), IsDir: info.IsDir(), IsSymlink: info.Mode()&fs.ModeSymlink != 0, Updated: info.ModTime().Unix()})
		}
	case "remove":
		err = s.Remove(ctx, name)
	case "rename":
		to, e := Name(q.NewPath, false)
		if e != nil {
			return out, e
		}
		if _, e = s.Data.Lstat(to); e == nil {
			return out, fs.ErrExist
		} else if !errors.Is(e, fs.ErrNotExist) {
			return out, e
		}
		err = s.Rename(ctx, name, to)
	case "write":
		if q.Directory {
			err = s.EnsureDirectory(ctx, name)
		} else {
			if parent := path.Dir(name); parent != "." {
				if err = s.EnsureDirectory(ctx, parent); err != nil {
					return out, err
				}
			}
			before, e := s.ReadLimited(ctx, name, 512<<20)
			if indexrepair.IsSourceNotExist(e) {
				before = nil
			} else if e != nil {
				return out, e
			}
			err = s.Rewrite(ctx, name, before, q.Data)
		}
		if err == nil && !q.ModTime.IsZero() {
			if s.Authorize == nil {
				return out, indexrepair.ErrSourceWrite
			}
			if err = s.Authorize(ctx); err != nil {
				return out, err
			}
			if err = s.Check(name, false); err != nil {
				return out, err
			}
			err = s.Data.Chtimes(name, q.ModTime, q.ModTime)
		}
	default:
		err = errors.New("invalid_workspace_file_action")
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return Result{}, err
	}
	return out, nil
}
