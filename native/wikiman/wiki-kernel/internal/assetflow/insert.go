// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/textprefs"
)

func Name(name, id string) string {
	ext := filepath.Ext(name)
	if ext == "." {
		ext = ""
	}
	base := strings.TrimSuffix(name, ext)
	last := base
	if len(last) > 22 {
		last = last[len(last)-22:]
	}
	if !ast.IsNodeIDPattern(last) {
		return base + "-" + id + ext
	}
	if ast.IsNodeIDPattern(base) {
		return name
	}
	return base[:len(base)-len(last)-1] + "-" + last + ext
}

type LocalFile struct {
	Data      []byte
	Directory bool
}
type InsertPorts struct {
	Read      func(context.Context, string) (LocalFile, error)
	Inside    func(context.Context, string) (bool, error)
	Directory func(context.Context, string) (bool, error)
	ByHash    func(context.Context, string) (string, error)
	Write     func(context.Context, string, []byte) error
	Remember  func(context.Context, string, string) error
	NewID     func() string
}

func Insert(ctx context.Context, paths []string, upload bool, p InsertPorts) (out map[string]interface{}, err error) {
	if ctx == nil || p.Read == nil || p.Inside == nil || p.Directory == nil || p.ByHash == nil || p.Write == nil || p.Remember == nil || p.NewID == nil {
		return nil, errors.New("asset_insert_unbound")
	}
	out = map[string]interface{}{}
	paths = append([]string(nil), paths...)
	for _, name := range paths {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		base := filepath.Base(name)
		clean := textprefs.FilterUploadFileName(base)
		ext := filepath.Ext(clean)
		clean = strings.TrimSuffix(clean, ext) + strings.ToLower(ext)
		if !upload {
			if !strings.HasPrefix(name, `\\`) {
				name = "file://" + name
			}
			out[base] = name
			continue
		}
		directory, e := p.Directory(ctx, name)
		if e != nil {
			return nil, e
		}
		if directory {
			if !strings.HasPrefix(name, `\\`) {
				name = "file://" + name
			}
			out[base] = name
			continue
		}
		inside, e := p.Inside(ctx, name)
		if e != nil {
			return nil, e
		}
		if inside {
			out[base] = "assets/" + clean
			continue
		}
		input, e := p.Read(ctx, name)
		if e != nil {
			return nil, e
		}
		if input.Directory {
			if !strings.HasPrefix(name, `\\`) {
				name = "file://" + name
			}
			out[base] = name
			continue
		}
		hash, e := indexdb.AssetEtag(ctx, bytes.NewReader(input.Data), int64(len(input.Data)))
		if e != nil {
			return nil, e
		}
		existing, e := p.ByHash(ctx, hash)
		if e != nil {
			return nil, e
		}
		if existing != "" {
			out[base] = existing
			continue
		}
		clean = Name(clean, p.NewID())
		dest := "assets/" + clean
		if e = p.Write(ctx, clean, input.Data); e != nil {
			return nil, e
		}
		if e = p.Remember(ctx, hash, dest); e != nil {
			return nil, e
		}
		out[base] = dest
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
