// SPDX-License-Identifier: AGPL-3.0-or-later
package assetflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/textprefs"
)

var ErrFilename = errors.New("asset_filename_invalid")

type RenameChange struct {
	Name          string
	Before, After []byte
	Document      bool
}
type RenamePlan struct {
	NewName, NewPath     string
	Changes              []RenameChange
	CopyFrom, CopyTo     string
	CopyData, Annotation []byte
	AnnotationPresent    bool
}

func PrepareRename(ctx context.Context, source indexrepair.Source, boxes []string, old, newName string, newID func() string) (out RenamePlan, err error) {
	defer func() {
		if err != nil {
			out = RenamePlan{}
		}
	}()
	if ctx == nil || newID == nil || !fs.ValidPath(old) || !strings.HasPrefix(old, "assets/") {
		return out, indexrepair.ErrSourcePath
	}
	newName = textprefs.FilterUploadFileName(strings.TrimSpace(newName))
	if path.Base(old) == newName || newName == "" {
		return out, ctx.Err()
	}
	if !gulu.File.IsValidFilename(newName) {
		return out, ErrFilename
	}
	out.NewName = Name(newName+filepath.Ext(old), newID())
	out.NewPath = path.Join(path.Dir(old), out.NewName)
	selected, e := Resolve(ctx, source, boxes, old)
	if e != nil {
		return out, e
	}
	out.CopyFrom = selected
	out.CopyTo = path.Join(path.Dir(selected), out.NewName)
	out.CopyData, e = source.ReadLimited(ctx, selected, 512<<20)
	if e != nil {
		return out, e
	}
	out.Annotation, e = source.ReadLimited(ctx, old+".sya", 512<<20)
	if e != nil && !indexrepair.IsSourceNotExist(e) {
		return out, e
	}
	out.AnnotationPresent = e == nil
	oldName := path.Base(old)
	budget := 0
	add := func(name string, raw []byte, document bool) error {
		needle, replacement := []byte(oldName), []byte(out.NewName)
		if !document {
			needle, replacement = []byte(old), []byte(out.NewPath)
		}
		if !bytes.Contains(raw, needle) {
			return nil
		}
		after := bytes.ReplaceAll(raw, needle, replacement)
		if !json.Valid(raw) || !json.Valid(after) {
			return indexrepair.ErrSourcePath
		}
		budget += len(raw) + len(after)
		if budget > 512<<20 {
			return indexrepair.ErrSourceReadLimit
		}
		out.Changes = append(out.Changes, RenameChange{Name: name, Before: bytes.Clone(raw), After: after, Document: document})
		return ctx.Err()
	}
	if e := docquery.WalkDocumentFiles(ctx, boxes, source.ReadDir, source.Read, func(name string, raw []byte) error { return add(name, raw, true) }); e != nil {
		return out, e
	}
	entries, e := source.ReadDir(ctx, "storage/av")
	if indexrepair.IsSourceNotExist(e) {
		return out, ctx.Err()
	}
	if e != nil {
		return out, e
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || !ast.IsNodeIDPattern(strings.TrimSuffix(entry.Name(), ".json")) {
			continue
		}
		name := "storage/av/" + entry.Name()
		raw, e := source.Read(ctx, name)
		if e != nil {
			return out, e
		}
		if e = add(name, raw, false); e != nil {
			return out, e
		}
	}
	return out, ctx.Err()
}
