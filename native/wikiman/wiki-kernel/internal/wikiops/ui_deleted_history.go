// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"encoding/json"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

const deletedUIHistory = "native-deleted"

func uiDeletedHistoryPath(name string) (box, doc string, ok bool) {
	rel, matched := strings.CutPrefix(name, deletedUIHistory+"/")
	box, leaf, split := strings.Cut(rel, "/")
	doc, suffix := strings.CutSuffix(leaf, ".docj")
	return box, doc, matched && split && suffix && nativeID.MatchString(box) && nativeID.MatchString(doc)
}

func readUIDeletedTombstone(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, box, doc string) (sourcetx.DocumentTombstone, string, error) {
	name := (sourcetx.DocumentTombstone{PageID: r.PageID, BoxID: box, DocID: doc}).RecordPath()
	raw, err := m.SourceFile(ctx, r.StoreID, name)
	if err != nil {
		return sourcetx.DocumentTombstone{}, "", err
	}
	tomb, err := sourcetx.DecodeDocumentTombstone(raw, name)
	return tomb, sourcetx.Hash(raw), err
}

func readUIDeletedDocument(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, tomb sourcetx.DocumentTombstone) ([]byte, error) {
	for _, file := range tomb.Files {
		if file.Path != tomb.OriginalPath {
			continue
		}
		raw, err := m.SourceFile(ctx, r.StoreID, tomb.ArchivePath(file.Path))
		if err == nil && sourcetx.Hash(raw) != file.Hash {
			return nil, sourcetx.ErrConflict
		}
		return raw, err
	}
	return nil, fs.ErrNotExist
}

func indexUIDeletedHistory(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, l *workspace.Lease) (map[string]int, error) {
	files, err := m.ArchiveFiles(ctx, r.StoreID, "tombstones", r.PageID, "", "")
	if err != nil {
		return nil, err
	}
	cfg, err := l.Configuration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Editor == nil {
		return nil, ErrArgument
	}
	threshold := time.Now().Add(-time.Duration(cfg.Editor.HistoryRetentionDays) * 24 * time.Hour)
	groupSizes := map[string]int{}
	rows := make([]indexdb.HistoryEntry, 0, len(files))
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{})
	size := 0
	for _, file := range files {
		box := path.Base(path.Dir(file.Path))
		doc := strings.TrimSuffix(path.Base(file.Path), ".json")
		tomb, hash, err := readUIDeletedTombstone(ctx, m, r, box, doc)
		if err != nil {
			return nil, err
		}
		if hash != file.Hash {
			return nil, sourcetx.ErrConflict
		}
		created, _ := time.Parse(time.RFC3339, tomb.DeletedAt)
		if !created.After(threshold) {
			continue
		}
		raw, err := readUIDeletedDocument(ctx, m, r, tomb)
		if err != nil {
			return nil, err
		}
		size += len(raw)
		if len(rows) >= 2000 || size > 64<<20 {
			return nil, ErrExportLimit
		}
		input, err := docast.OriginalTreeInput(raw)
		if err != nil {
			return nil, err
		}
		tree, err := dataparser.ParseJSONWithoutFix(input, parser.ParseOptions)
		if err != nil {
			return nil, err
		}
		content, err := lute.ProtyleExportMdNodeSync(tree.Root, parser.ParseOptions, parser.RenderOptions)
		if err != nil {
			return nil, err
		}
		name := deletedUIHistory + "/" + box + "/" + doc + ".docj"
		rows = append(rows, indexdb.HistoryEntry{ID: doc, Type: 1, Op: "delete", Title: tree.Root.IALAttr("title"),
			Content: content, Created: strconv.FormatInt(created.Unix(), 10), Path: name})
		groupSizes[name] = len(tomb.Linked)
	}
	return groupSizes, l.ReplaceNativeHistoryProjection(ctx, deletedUIHistory, rows)
}

func uiRenderDeletedHistory(m *workspace.WorkspaceManager, r UIRequest) uiRoute {
	return uiRouteFor(func(q struct {
		HistoryPath, K string
		Highlight      *bool
	}) (uiCall, error) {
		box, doc, ok := uiDeletedHistoryPath(q.HistoryPath)
		if !ok || len(q.K) > 4096 {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			tomb, _, err := readUIDeletedTombstone(ctx, m, r, box, doc)
			if err != nil {
				return nil, err
			}
			raw, err := readUIDeletedDocument(ctx, m, r, tomb)
			if err != nil {
				return nil, err
			}
			raw, err = docast.OriginalTreeInput(raw)
			if err != nil {
				return nil, err
			}
			cfg, err := l.Configuration(ctx)
			if err != nil {
				return nil, err
			}
			out, err := docview.RenderHistory(ctx, raw, q.K, q.Highlight == nil || *q.Highlight, cfg.Search != nil && cfg.Search.CaseSensitive,
				nativeparser.NewEditor(nativeparser.EditorOptions{}))
			return map[string]any{"id": out.ID, "rootID": out.RootID, "content": out.Content, "isLargeDoc": out.Large}, err
		}}, nil
	})
}

func uiRecoverDeletedHistory(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView, input uiLifecycleData) (any, error) {
	box, doc, ok := uiDeletedHistoryPath(input.HistoryPath)
	if !ok || input.Notebook != box || view.Mode != "wikiDesktop" {
		return nil, ErrUnauthorized
	}
	_, hash, err := readUIDeletedTombstone(ctx, m, r, box, doc)
	if err != nil {
		return nil, err
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, BoxID: box, DocID: doc,
		Action: "document.recover", ExpectedHash: hash, OperationID: "ui-recover-" + ast.NewNodeID()}
	_, err = planContentLifecycle(ctx, m, request, func(p *lifecycleEdit) error {
		l, err := p.lease()
		if err != nil {
			return err
		}
		defer l.Release()
		cfg, err := l.Configuration(p.ctx)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		return p.restoreDeleted()
	})
	return nil, err
}

func uiHistoryPath(raw json.RawMessage) string {
	var input struct{ HistoryPath string }
	_ = json.Unmarshal(raw, &input)
	return input.HistoryPath
}
