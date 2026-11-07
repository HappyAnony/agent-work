// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package notebooks separates original list projection from explicit repair.
package notebooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/textprefs"
	"wiki-kernel/internal/textsort"
)

var ErrPath = errors.New("invalid_notebook_metadata_path")
var ErrChanged = errors.New("notebook_repair_source_changed")
var ErrPermission = errors.New("notebook_repair_unbound")
var idPattern = regexp.MustCompile(`^[0-9]{14}-[a-z0-9]{7}$`)

type Repair struct {
	ID, Path      string
	Missing       bool
	Before, After []byte
	Hash          [32]byte
	Unindex       bool
}
type Plan struct {
	Notebooks []conf.Notebook
	Repairs   []Repair
}

func Guide(id string) bool {
	return id == "20210808180117-czj9bvb" || id == "20210808180117-6v0mkxr" || id == "20211226090932-5lcq56f" || id == "20240530133126-axarxgx"
}
func metadata(root *os.Root, id, mode string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", ErrPath
	}
	if mode != "auto" && mode != ".wiki" && mode != ".siyuan" {
		return "", ErrPath
	}
	chosen := mode
	if mode == "auto" {
		chosen = ".wiki"
		found := 0
		for _, name := range []string{".wiki", ".siyuan"} {
			v, e := root.Lstat(path.Join(id, name))
			if errors.Is(e, fs.ErrNotExist) {
				continue
			}
			if e != nil {
				return "", e
			}
			if !v.IsDir() || v.Mode()&fs.ModeSymlink != 0 {
				return "", ErrPath
			}
			found++
			chosen = name
		}
		if found > 1 {
			return "", ErrPath
		}
	}
	return path.Join(id, chosen, "conf.json"), nil
}
func read(ctx context.Context, root *os.Root, name string) ([]byte, error) {
	raw, err := (indexrepair.Source{Data: root}).ReadLimited(ctx, name, 1<<20)
	if errors.Is(err, indexrepair.ErrSourcePath) || errors.Is(err, indexrepair.ErrSourceReadLimit) {
		err = errors.Join(ErrPath, err)
	}
	return raw, err
}
func valid(raw []byte, v *conf.BoxConf) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil && json.Unmarshal(raw, v) == nil
}

// Scan never repairs, removes, creates directories, unindexes or starts work.
// auto reads existing canonical or legacy metadata, rejecting competing roots.
func Scan(ctx context.Context, root *os.Root, mode string, order int) (Plan, error) {
	return scanWithIO(ctx, root, mode, order, notebookIO{})
}
func scanWithIO(ctx context.Context, root *os.Root, mode string, order int, op notebookIO) (Plan, error) {
	op = op.defaults()
	if root == nil {
		return Plan{}, ErrPermission
	}
	if e := ctx.Err(); e != nil {
		return Plan{}, e
	}
	entries, e := notebookEntries(ctx, root, op)
	if e != nil {
		return Plan{}, e
	}
	if len(entries) > 100000 {
		return Plan{}, ErrPath
	}
	out := Plan{Notebooks: []conf.Notebook{}}
	for _, entry := range entries {
		if !entry.IsDir() || !idPattern.MatchString(entry.Name()) {
			continue
		}
		if e := ctx.Err(); e != nil {
			return Plan{}, e
		}
		id := entry.Name()
		name, e := metadata(root, id, mode)
		if e != nil {
			return Plan{}, e
		}
		raw, e := op.read(ctx, root, name)
		missing := indexrepair.IsSourceNotExist(e)
		if e != nil && !missing {
			return Plan{}, e
		}
		v := conf.NewBoxConf()
		if missing {
			if Guide(id) {
				continue
			}
			after, _ := json.MarshalIndent(v, "", "  ")
			out.Repairs = append(out.Repairs, Repair{ID: id, Path: name, Missing: true, After: after, Unindex: true})
		} else if !valid(raw, v) {
			out.Repairs = append(out.Repairs, Repair{ID: id, Path: name, Before: bytes.Clone(raw), Hash: sha256.Sum256(raw)})
			continue
		}
		icon := v.Icon
		if strings.Contains(icon, ".") {
			icon = textprefs.FilterUploadEmojiFileName(icon)
		}
		out.Notebooks = append(out.Notebooks, conf.Notebook{ID: id, Name: v.Name, Icon: icon, Sort: v.Sort, SortMode: v.SortMode, Closed: v.Closed})
	}
	sort.Slice(out.Notebooks, func(i, j int) bool {
		a, b := out.Notebooks[i], out.Notebooks[j]
		switch order {
		case configtypes.SortModeNameASC:
			return textsort.FileTreeCompare(a.Name, b.Name)
		case configtypes.SortModeNameDESC:
			return textsort.FileTreeCompare(b.Name, a.Name)
		case configtypes.SortModeAlphanumASC:
			return textsort.NaturalCompare(a.Name, b.Name)
		case configtypes.SortModeAlphanumDESC:
			return textsort.NaturalCompare(b.Name, a.Name)
		case configtypes.SortModeCustom:
			return a.Sort < b.Sort
		case configtypes.SortModeCreatedASC:
			return a.ID < b.ID
		case configtypes.SortModeCreatedDESC:
			return a.ID > b.ID
		}
		return false
	})
	if e = ctx.Err(); e != nil {
		return Plan{}, e
	}
	return out, nil
}

// Apply is a trusted compatibility writer. Native callers consume candidates
// through the future authoritative source commit; they never call this writer.
// Calls must be serialized by their owner. This is not a cross-process source
// transaction; successful earlier repairs are retained if a later IO fails.
func Apply(ctx context.Context, root *os.Root, repairs []Repair, afterCreate func(context.Context, string) error) error {
	return applyWithIO(ctx, root, repairs, afterCreate, notebookIO{})
}
func applyWithIO(ctx context.Context, root *os.Root, repairs []Repair, afterCreate func(context.Context, string) error, op notebookIO) error {
	op = op.defaults()
	if e := ctx.Err(); e != nil {
		return e
	}
	if root == nil {
		return ErrPermission
	}
	for _, r := range repairs {
		parts := strings.Split(r.Path, "/")
		if len(parts) != 3 || parts[0] != r.ID || !idPattern.MatchString(r.ID) || (parts[1] != ".wiki" && parts[1] != ".siyuan") || parts[2] != "conf.json" {
			return ErrPath
		}
		if r.Missing {
			expected, _ := json.MarshalIndent(conf.NewBoxConf(), "", "  ")
			if !bytes.Equal(expected, r.After) || !r.Unindex || len(r.Before) != 0 || afterCreate == nil {
				return ErrPermission
			}
		} else {
			if valid(r.Before, conf.NewBoxConf()) || len(r.After) != 0 || sha256.Sum256(r.Before) != r.Hash {
				return ErrPermission
			}
		}
		raw, e := op.read(ctx, root, r.Path)
		if r.Missing {
			if !indexrepair.IsSourceNotExist(e) {
				return errors.Join(ErrChanged, e)
			}
		} else if e != nil || sha256.Sum256(raw) != r.Hash {
			return errors.Join(ErrChanged, e)
		}
	}
	for _, r := range repairs {
		if e := ctx.Err(); e != nil {
			return e
		}
		if r.Missing {
			if e := root.MkdirAll(path.Dir(r.Path), 0755); e != nil {
				return e
			}
			f, e := op.create(root, r.Path)
			if e != nil {
				return e
			}
			n, writeErr := f.Write(r.After)
			e = writeErr
			if e == nil && n != len(r.After) {
				e = io.ErrShortWrite
			}
			e = errors.Join(e, f.Close(), ctx.Err())
			if e != nil {
				return e
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			if e = afterCreate(ctx, r.ID); e != nil {
				return e
			}
		} else {
			raw, e := op.read(ctx, root, r.Path)
			if e != nil || sha256.Sum256(raw) != r.Hash {
				return errors.Join(ErrChanged, e)
			}
			if e = root.Remove(r.Path); e != nil {
				return e
			}
		}
	}
	return ctx.Err()
}
