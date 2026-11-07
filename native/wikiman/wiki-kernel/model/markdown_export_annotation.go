// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/siyuan-note/filelock"
	"wiki-kernel/internal/indexrepair"
)

type markdownExportAnnotationIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

func annotationReference(refID string) (asset, id string, err error) {
	i := strings.LastIndexByte(refID, '/')
	if i < 0 || i == len(refID)-1 {
		return "", "", errors.New("export_annotation_id_invalid")
	}
	asset, id = strings.TrimSpace(refID[:i]), refID[i+1:]
	if j := strings.IndexByte(asset, '?'); j >= 0 {
		asset = asset[:j]
	}
	if !strings.HasPrefix(asset, "assets/") || !fs.ValidPath(asset) || strings.ContainsAny(asset, "\\\x00") {
		return "", "", indexrepair.ErrSourcePath
	}
	return asset, id, nil
}

// Original GetAssetAbsPath order: global first, then the first suffix match in
// each captured notebook's lexical traversal. No process-global path/cache use.
func (m *markdownExportContext) findAnnotationAsset(source indexrepair.Source, asset string) (string, error) {
	return m.findAnnotationAssetWithIO(source, asset, markdownExportAnnotationIO{})
}
func (m *markdownExportContext) findAnnotationAssetWithIO(source indexrepair.Source, asset string, op markdownExportAnnotationIO) (string, error) {
	if err := m.guard(); err != nil {
		return "", err
	}
	if err := source.Check(asset, false); err == nil {
		info, err := source.Data.Stat(asset)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", indexrepair.ErrSourcePath
		}
		return asset, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	readDir := op.readDir
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, folder string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, folder)
		}
	}
	var scan func(string) (string, error)
	scan = func(folder string) (string, error) {
		if err := m.guard(); err != nil {
			return "", err
		}
		if err := source.Check(folder, false); err != nil {
			return "", err
		}
		info, err := source.Data.Lstat(folder)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return "", indexrepair.ErrSourcePath
		}
		entries, err := readDir(m.read.binding.scope.ctx, source, folder)
		if err != nil {
			return "", err
		}
		if err = m.guard(); err != nil {
			return "", err
		}
		now, err := source.Data.Lstat(folder)
		if err != nil {
			return "", err
		}
		if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, info) || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) {
			return "", indexrepair.ErrSourceChanged
		}
		for _, entry := range entries {
			if isSkipFile(entry.Name()) {
				continue
			}
			name := path.Join(folder, entry.Name())
			if err = source.Check(name, false); err != nil {
				return "", err
			}
			if strings.HasSuffix(name, asset) {
				info, err := source.Data.Stat(name)
				if err != nil {
					return "", err
				}
				if !info.Mode().IsRegular() {
					return "", indexrepair.ErrSourcePath
				}
				return name, nil
			}
			if entry.IsDir() {
				found, err := scan(name)
				if err != nil || found != "" {
					return found, err
				}
			}
		}
		return "", nil
	}
	for _, box := range m.read.assetBoxes {
		found, err := scan(box)
		if err != nil || found != "" {
			return found, err
		}
	}
	return "", os.ErrNotExist
}
func (m *markdownExportContext) readAnnotationFile(asset string) (records map[string]json.RawMessage, err error) {
	if err = m.guard(); err != nil {
		return nil, err
	}
	root, err := openArchivePublicationSource(m.read.binding.scope.commit.r.data)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	source := indexrepair.Source{Data: root}
	selected, err := m.findAnnotationAsset(source, asset)
	if err != nil {
		return nil, err
	}
	sidecar := selected + ".sya"
	filename := filepath.Join(m.read.binding.scope.commit.r.data, filepath.FromSlash(sidecar))
	filelock.Lock(filename)
	defer filelock.Unlock(filename)
	raw, err := source.Read(m.read.binding.scope.ctx, sidecar)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	if records == nil {
		return nil, errors.New("export_annotation_file_invalid")
	}
	if err = m.guard(); err != nil {
		return nil, err
	}
	if m.annotationSources == nil {
		m.annotationSources = map[string]string{}
	}
	m.annotationSources[asset] = selected
	return records, nil
}
func (m *markdownExportContext) readAnnotationPage(refID string) (asset string, page int, err error) {
	if err = m.guard(); err != nil {
		return
	}
	asset, id, err := annotationReference(refID)
	if err != nil {
		return "", 0, err
	}
	records, ok := m.annotationFiles[asset]
	if !ok {
		records, err = m.readAnnotationFile(asset)
		if err != nil {
			return "", 0, err
		}
		if m.annotationFiles == nil {
			m.annotationFiles = map[string]map[string]json.RawMessage{}
		}
		m.annotationFiles[asset] = records
	}
	raw, ok := records[id]
	if !ok {
		return "", 0, errors.New("export_annotation_missing")
	}
	var value struct {
		Pages []struct {
			Index *float64 `json:"index"`
		} `json:"pages"`
	}
	if err = json.Unmarshal(raw, &value); err != nil {
		return "", 0, err
	}
	if len(value.Pages) == 0 || value.Pages[0].Index == nil {
		return "", 0, errors.New("export_annotation_pages_invalid")
	}
	index := *value.Pages[0].Index
	if math.IsNaN(index) || math.IsInf(index, 0) || index < 0 || math.Trunc(index) != index || index >= float64(int(^uint(0)>>1)) {
		return "", 0, errors.New("export_annotation_page_invalid")
	}
	return asset, int(index) + 1, m.guard()
}
func annotationDisplayName(asset string) string {
	name := strings.TrimPrefix(asset, "assets/")
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	// Valid native asset names end in '-' plus a 22-character node ID. Short or
	// nonstandard names remain readable instead of causing the legacy slice panic.
	if len(stem) > 23 && stem[len(stem)-23] == '-' && ast.IsNodeIDPattern(stem[len(stem)-22:]) {
		name = stem[:len(stem)-23] + ext
	}
	return name
}
func processOwnedFileAnnotationRef(refID string, n *ast.Node, mode int, p *exportReferenceRenderer) ast.WalkStatus {
	if p.check() != nil {
		return ast.WalkStop
	}
	asset, page, err := p.annotation(refID)
	if err != nil {
		if p.err == nil {
			p.err = err
		}
		return ast.WalkStop
	}
	if p.check() != nil {
		return ast.WalkStop
	}
	return insertFileAnnotationLink(asset, annotationDisplayName(asset), n.TextMarkTextContent, strconv.Itoa(page), n, mode)
}
