// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package historyindex

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/dataparser"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

var ErrDirectory = errors.New("invalid_history_directory")

// Plan performs only rooted reads, original read-only parsing and value
// projection. No queue or partial batch is published after a failed scan.
func Plan(ctx context.Context, source indexrepair.Source, name, untitled string, location *time.Location, engine *lute.Lute) (out []indexdb.HistoryEntry, err error) {
	return planWithIO(ctx, source, name, untitled, location, engine, historyScanIO{})
}

func planWithIO(ctx context.Context, source indexrepair.Source, name, untitled string, location *time.Location, engine *lute.Lute, backend historyScanIO) (out []indexdb.HistoryEntry, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if engine == nil {
		return nil, ErrDirectory
	}
	op, created, e := Directory(name, location)
	if e != nil {
		return nil, e
	}
	backend = backend.defaults()
	manifest, e := captureHistoryManifest(ctx, source, name, backend)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, manifest.check(ctx))
		if err != nil {
			out = nil
		}
	}()
	var docs, assets []string
	for _, saved := range manifest.nodes {
		if saved.info.IsDir() {
			continue
		}
		if strings.HasSuffix(path.Base(saved.name), ".docj") {
			docs = append(docs, saved.name)
		} else if strings.Contains(saved.name, "assets/") {
			assets = append(assets, saved.name)
		}
	}
	for _, p := range docs {
		if e = manifest.checkEntry(ctx, manifest.byName[p]); e != nil {
			return nil, e
		}
		data, e := backend.read(ctx, source, p)
		if e != nil {
			return nil, e
		}
		if e = manifest.checkEntry(ctx, manifest.byName[p]); e != nil {
			return nil, e
		}
		input, e := docast.OriginalTreeInput(data)
		if e != nil {
			return nil, e
		}
		tree, e := dataparser.ParseJSONWithoutFix(input, engine.ParseOptions)
		if e != nil {
			return nil, e
		}
		if tree == nil || tree.Root == nil {
			return nil, ErrDirectory
		}
		title := tree.Root.IALAttr("title")
		if title == "" {
			title = untitled
		}
		out = append(out, indexdb.HistoryEntry{ID: tree.Root.ID, Type: 1, Op: op, Title: title, Content: tree.Root.Content(), Path: p, Created: created})
	}
	for _, p := range assets {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		base := path.Base(p)
		ext := path.Ext(base)
		if ext == "." {
			ext = ""
		}
		id := strings.TrimSuffix(base, ext)
		if len(id) > 22 {
			id = id[len(id)-22:]
		}
		if !ast.IsNodeIDPattern(id) {
			id = ""
		}
		out = append(out, indexdb.HistoryEntry{ID: id, Type: 2, Op: op, Title: base, Path: p, Created: created})
	}
	return out, ctx.Err()
}

func Directory(name string, location *time.Location) (string, string, error) {
	if !fs.ValidPath(name) || strings.Contains(name, "/") || location == nil {
		return "", "", ErrDirectory
	}
	split := strings.LastIndex(name, "-")
	if split < 0 {
		return "", "", ErrDirectory
	}
	op := name[split+1:]
	if !slices.Contains([]string{"clean", "update", "delete", "format", "sync", "replace", "outline"}, op) {
		return "", "", ErrDirectory
	}
	stamp, e := time.ParseInLocation("2006-01-02-150405", name[:split], location)
	if e != nil {
		return "", "", ErrDirectory
	}
	return op, fmt.Sprint(stamp.Unix()), nil
}
