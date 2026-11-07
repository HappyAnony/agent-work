// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/textprefs"
)

const UploadLimit = 512 << 20

type uploadReader struct {
	ctx   context.Context
	input io.Reader
}

func (r uploadReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.input.Read(b)
	if next := r.ctx.Err(); next != nil {
		return n, next
	}
	return n, e
}

type UploadFile struct {
	Name string
	Size int64
	Open func() (io.ReadCloser, error)
}
type UploadEntry struct {
	Name      string
	Directory bool
	Data      []byte
}
type UploadResult struct {
	ErrFiles []string               `json:"errFiles"`
	SuccMap  map[string]interface{} `json:"succMap"`
}
type UploadPorts struct {
	ByHash    func(context.Context, string) (string, error)
	Match     func(context.Context, string) (string, error)
	Write     func(context.Context, string, []byte) error
	Directory func(context.Context, string, []UploadEntry) error
	Remember  func(context.Context, string, string) error
	NewID     func() string
}

func lastID(name string) string {
	ext := path.Ext(name)
	if ext == "." {
		ext = ""
	}
	id := strings.TrimSuffix(path.Base(name), ext)
	if len(id) > 22 {
		id = id[len(id)-22:]
	}
	return id
}

// DecodeRTFD checks the complete ZIP before returning the first lexical root's
// contents, preserving the original macOS bundle selection. No extraction path
// is supplied to a library or external process.
func DecodeRTFD(ctx context.Context, raw []byte) ([]UploadEntry, error) {
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		return nil, e
	}
	if len(z.File) > 100000 {
		return nil, errors.New("asset_upload_archive_budget")
	}
	entries := map[string]UploadEntry{}
	roots := map[string]bool{}
	total := uint64(0)
	for _, f := range z.File {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		name := strings.TrimSuffix(f.Name, "/")
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:") || strings.Count(name, "/") > 255 || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) {
			return nil, errors.New("asset_upload_archive_path")
		}
		if _, exists := entries[name]; exists {
			return nil, errors.New("asset_upload_archive_duplicate")
		}
		roots[strings.SplitN(name, "/", 2)[0]] = true
		entry := UploadEntry{Name: name, Directory: f.FileInfo().IsDir()}
		if !entry.Directory {
			if f.UncompressedSize64 > UploadLimit || total > UploadLimit-f.UncompressedSize64 {
				return nil, errors.New("asset_upload_archive_budget")
			}
			total += f.UncompressedSize64
			input, e := f.Open()
			if e != nil {
				return nil, e
			}
			entry.Data, e = io.ReadAll(io.LimitReader(uploadReader{ctx, input}, UploadLimit+1))
			e = errors.Join(e, input.Close())
			if e != nil {
				return nil, e
			}
			if len(entry.Data) > UploadLimit || uint64(len(entry.Data)) != f.UncompressedSize64 {
				return nil, errors.New("asset_upload_archive_size")
			}
		}
		entries[name] = entry
	}
	for name := range entries {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if entry, exists := entries[parent]; exists && !entry.Directory {
				return nil, errors.New("asset_upload_archive_collision")
			}
		}
	}
	names := []string{}
	for name := range roots {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("asset_upload_archive_empty")
	}
	selected := names[0]
	if entry, exists := entries[selected]; exists && !entry.Directory {
		return nil, errors.New("asset_upload_archive_root_not_directory")
	}
	out := []UploadEntry{}
	for name, entry := range entries {
		if strings.HasPrefix(name, selected+"/") {
			entry.Name = strings.TrimPrefix(name, selected+"/")
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, ctx.Err()
}

func Upload(ctx context.Context, files []UploadFile, relative string, skipDuplicate, darwin bool, p UploadPorts) (out UploadResult, err error) {
	out.SuccMap = map[string]interface{}{}
	if ctx == nil || p.ByHash == nil || p.Match == nil || p.Write == nil || p.Directory == nil || p.Remember == nil || p.NewID == nil {
		return out, errors.New("asset_upload_unbound")
	}
	if !fs.ValidPath(relative) || relative == "." || strings.Contains(relative, "\\") {
		return out, errors.New("asset_upload_path")
	}
	for _, file := range files {
		clean := textprefs.FilterUploadFileName(file.Name)
		ext := path.Ext(clean)
		clean = strings.TrimSuffix(clean, ext) + strings.ToLower(ext)
		ext = strings.ToLower(ext)
		fail := func(e error) (UploadResult, error) { out.ErrFiles = append(out.ErrFiles, clean); return out, e }
		if e := ctx.Err(); e != nil {
			return fail(e)
		}
		if file.Open == nil || file.Size < 0 || file.Size > UploadLimit {
			return fail(errors.New("asset_upload_size"))
		}
		input, e := file.Open()
		if e != nil {
			return fail(e)
		}
		raw, e := io.ReadAll(io.LimitReader(uploadReader{ctx, input}, UploadLimit+1))
		e = errors.Join(e, input.Close())
		if e != nil {
			return fail(e)
		}
		if len(raw) > UploadLimit || int64(len(raw)) != file.Size {
			return fail(errors.New("asset_upload_size"))
		}
		hash, e := indexdb.AssetEtag(ctx, bytes.NewReader(raw), int64(len(raw)))
		if e != nil {
			return fail(e)
		}
		existing, e := p.ByHash(ctx, hash)
		if e != nil {
			return fail(e)
		}
		if existing != "" {
			out.SuccMap[file.Name] = existing
			continue
		}
		id := lastID(file.Name)
		if !ast.IsNodeIDPattern(id) {
			id = ""
		}
		if skipDuplicate {
			pattern := strings.TrimSuffix(clean, ext) + "*" + ext
			if id != "" && id != lastID(clean) {
				pattern = "*" + id + ext
			}
			match, e := p.Match(ctx, pattern)
			if e != nil {
				return fail(e)
			}
			if match != "" {
				out.SuccMap[file.Name] = path.Join(relative, match)
				break
			}
		}
		if id == "" {
			id = p.NewID()
		}
		clean = Name(clean, id)
		base := file.Name
		if darwin && strings.HasSuffix(file.Name, ".rtfd.zip") {
			entries, e := DecodeRTFD(ctx, raw)
			if e != nil {
				return fail(e)
			}
			base = strings.TrimSuffix(file.Name, ".rtfd.zip") + ".rtfd"
			clean = textprefs.FilterUploadFileName(base)
			extension := path.Ext(clean)
			clean = strings.TrimSuffix(clean, extension) + strings.ToLower(extension)
			clean = Name(clean, p.NewID())
			if e = p.Directory(ctx, clean, entries); e != nil {
				return fail(e)
			}
		} else if e = p.Write(ctx, clean, raw); e != nil {
			return fail(e)
		}
		dest := path.Join(relative, clean)
		if e = p.Remember(ctx, hash, dest); e != nil {
			return fail(e)
		}
		out.SuccMap[base] = dest
	}
	return out, ctx.Err()
}
