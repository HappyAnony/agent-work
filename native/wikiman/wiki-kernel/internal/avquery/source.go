// SPDX-License-Identifier: AGPL-3.0-or-later
package avquery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/vmihailenco/msgpack/v5"
	"io/fs"
	"os"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

type SourceCatalog struct {
	Entries   []Entry
	Relations map[string][]string
	Documents map[string][]byte
	manifest  map[string]fs.FileInfo
	absent    bool
}

func ReadSourceCatalog(ctx context.Context, source indexrepair.Source) (out *SourceCatalog, err error) {
	if ctx == nil || source.Data == nil {
		return nil, errors.New("av_query_source_unbound")
	}
	out = &SourceCatalog{Relations: map[string][]string{}, Documents: map[string][]byte{}, manifest: map[string]fs.FileInfo{}}
	entries, e := source.ReadDir(ctx, "storage/av")
	if indexrepair.IsSourceNotExist(e) {
		out.absent = true
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	if len(entries) > 100000 {
		return nil, errors.New("av_query_source_budget")
	}
	directory, e := source.Data.Lstat("storage/av")
	if e != nil {
		return nil, e
	}
	out.manifest["storage/av"] = directory
	for _, entry := range entries {
		name := "storage/av/" + entry.Name()
		if e = source.Check(name, false); e != nil {
			return nil, e
		}
		info, e := source.Data.Lstat(name)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, indexrepair.ErrSourcePath
		}
		out.manifest[name] = info
	}
	if _, exists := out.manifest["storage/av/blocks.msgpack"]; !exists {
		return out, out.Confirm(ctx, source)
	}
	raw, e := source.ReadLimited(ctx, "storage/av/blocks.msgpack", 16<<20)
	if e != nil {
		return nil, e
	}
	if e = msgpack.Unmarshal(raw, &out.Relations); e != nil {
		return nil, e
	}
	if len(out.Relations) > 100000 {
		return nil, errors.New("av_query_source_budget")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !ast.IsNodeIDPattern(id) || out.Relations[id] == nil {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := "storage/av/" + entry.Name()
		raw, e = source.ReadLimited(ctx, name, 16<<20)
		if e != nil {
			return nil, e
		}
		var doc struct{ ID, Name string }
		if e = json.Unmarshal(raw, &doc); e != nil {
			return nil, e
		}
		if doc.ID != "" && doc.ID != id {
			return nil, indexrepair.ErrSourcePath
		}
		out.Documents[id] = raw
		out.Entries = append(out.Entries, Entry{ID: id, Name: doc.Name, Updated: out.manifest[name].ModTime().UnixMilli()})
	}
	if e = out.Confirm(ctx, source); e != nil {
		return nil, e
	}
	return out, nil
}

func (c *SourceCatalog) Confirm(ctx context.Context, source indexrepair.Source) error {
	if c == nil {
		return errors.New("av_query_source_unbound")
	}
	entries, e := source.ReadDir(ctx, "storage/av")
	if c.absent {
		if indexrepair.IsSourceNotExist(e) {
			return ctx.Err()
		}
		if e != nil {
			return e
		}
		return indexrepair.ErrSourceChanged
	}
	if e != nil {
		return e
	}
	if len(entries)+1 != len(c.manifest) {
		return indexrepair.ErrSourceChanged
	}
	for name, before := range c.manifest {
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = source.Check(name, false); e != nil {
			return e
		}
		after, e := source.Data.Lstat(name)
		if e != nil {
			return e
		}
		if !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
	}
	return ctx.Err()
}
