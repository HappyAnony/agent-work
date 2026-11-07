// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"bytes"
	"context"
	"io/fs"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

type CleanResult struct {
	Paths, Hashes, Files []string
	Size                 int64
	Removed              int
}

// Clean first inventories every selection and publishes every backup. A later
// mutation failure retains the original sequential success prefix, never a
// fictitious multi-file rollback. Directory removal succeeds only when empty.
func Clean(ctx context.Context, source, history indexrepair.Source, selected []string, stamp string) (out CleanResult, err error) {
	if ctx == nil || !fs.ValidPath(stamp) || stamp == "." || strings.ContainsAny(stamp, "/\\") {
		return out, indexrepair.ErrSourcePath
	}
	files, dirs := map[string]bool{}, map[string]bool{}
	count := 0
	var scan func(string, int) error
	scan = func(name string, depth int) error {
		if depth > 255 {
			return indexrepair.ErrSourcePath
		}
		if e := source.Check(name, false); e != nil {
			return e
		}
		info, e := source.Data.Lstat(name)
		if e != nil {
			return e
		}
		count++
		if count > 1000000 {
			return indexrepair.ErrSourcePath
		}
		if info.IsDir() {
			dirs[name] = true
			entries, e := source.ReadDir(ctx, name)
			if e != nil {
				return e
			}
			for _, entry := range entries {
				if e = scan(path.Join(name, entry.Name()), depth+1); e != nil {
					return e
				}
			}
		} else if info.Mode().IsRegular() {
			files[name] = true
		} else {
			return indexrepair.ErrSourcePath
		}
		return ctx.Err()
	}
	for _, name := range selected {
		name = strings.TrimSuffix(name, "/")
		if !fs.ValidPath(name) || name == "assets" || (!strings.HasPrefix(name, "assets/") && !strings.Contains(name, "/assets/")) {
			return out, indexrepair.ErrSourcePath
		}
		if e := scan(name, 0); e != nil {
			return out, e
		}
	}
	ordered := []string{}
	for name := range dirs {
		if e := history.EnsureDirectory(ctx, path.Join(stamp, name)); e != nil {
			return out, e
		}
	}
	for name := range files {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		raw, e := source.ReadLimited(ctx, name, 512<<20)
		if e != nil {
			return out, e
		}
		dest := path.Join(stamp, name)
		before, e := history.ReadLimited(ctx, dest, 512<<20)
		if e != nil && !indexrepair.IsSourceNotExist(e) {
			return out, e
		}
		if indexrepair.IsSourceNotExist(e) {
			before = nil
		}
		if e = history.Rewrite(ctx, dest, before, raw); e != nil {
			return out, e
		}
	}
	// Verify every completed backup against its still-live source before deleting.
	for _, name := range ordered {
		raw, e := source.ReadLimited(ctx, name, 512<<20)
		if e != nil {
			return out, e
		}
		backup, e := history.ReadLimited(ctx, path.Join(stamp, name), 512<<20)
		if e != nil {
			return out, e
		}
		if !bytes.Equal(raw, backup) {
			return out, indexrepair.ErrSourceChanged
		}
	}
	for _, name := range ordered {
		raw, e := history.ReadLimited(ctx, path.Join(stamp, name), 512<<20)
		if e != nil {
			return out, e
		}
		hash, e := indexdb.AssetEtag(ctx, bytes.NewReader(raw), int64(len(raw)))
		if e != nil {
			return out, e
		}
		e = source.RemoveUnchanged(ctx, name, raw)
		if e != nil && !indexrepair.IsSourceNotExist(source.Check(name, false)) {
			return out, e
		}
		out.Files = append(out.Files, name)
		out.Removed++
		out.Hashes = append(out.Hashes, hash)
		out.Size += int64(len(raw))
		if e != nil {
			return out, e
		}
	}
	folders := []string{}
	for name := range dirs {
		folders = append(folders, name)
	}
	sort.Slice(folders, func(i, j int) bool { return len(folders[i]) > len(folders[j]) })
	for _, name := range folders {
		e := source.Remove(ctx, name)
		if e != nil && !indexrepair.IsSourceNotExist(source.Check(name, false)) {
			return out, e
		}
		out.Removed++
		if e != nil {
			return out, e
		}
	}
	out.Paths = append([]string(nil), selected...)
	return out, ctx.Err()
}
