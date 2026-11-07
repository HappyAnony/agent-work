// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var uiNotebookActions = map[string]string{
	"/api/notebook/createNotebook":     "create",
	"/api/notebook/renameNotebook":     "rename",
	"/api/notebook/openNotebook":       "closed",
	"/api/notebook/closeNotebook":      "closed",
	"/api/notebook/setNotebookIcon":    "icon",
	"/api/notebook/setNotebookConf":    "configuration",
	"/api/notebook/changeSortNotebook": "sort",
}

func runUINotebook(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, action string) (out any, err error) {
	var data struct {
		Notebook, Name, Icon, App, Callback string
		Notebooks                           []string
		Conf                                json.RawMessage
	}
	if err = Decode(bytes.NewReader(r.Data), &data); err != nil || len(data.Notebooks) > 1000 || len(data.Name) > 4096 || len(data.Icon) > 4096 {
		return nil, ErrArgument
	}
	request := sourcetx.Request{OperationID: "ui-box-" + ast.NewNodeID()}
	err = m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		tree, e := m.Tree(c, r.StoreID)
		if e != nil {
			return e
		}
		page, ok := tree.Get(r.PageID)
		if !ok || page.DeletedAt != nil {
			return fs.ErrNotExist
		}
		l, e := m.Acquire(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		defer l.Release()
		cfg, e := l.Configuration(c)
		if e != nil {
			return e
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		files, guards, e := m.PageReadGuards(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		request.ExpectedTreeDigest, request.ReadGuards = tree.SourceDigest(), guards
		hashes := make(map[string]string, len(files))
		for _, file := range files {
			hashes[file.Path] = file.Hash
		}
		var plan notebooks.MetadataPlan
		switch action {
		case "create":
			plan, e = l.PlanNativeNotebookCreate(c, data.Name, "Untitled")
		default:
			closed := r.Endpoint == "/api/notebook/closeNotebook"
			if closed && page.EntryDocument != nil && page.EntryDocument.BoxID == data.Notebook {
				return ErrArgument
			}
			plan, e = l.PlanNativeNotebookMetadata(c, notebooks.MetadataRequest{
				Kind: action, ID: data.Notebook, IDs: data.Notebooks, Name: data.Name,
				Icon: data.Icon, Untitled: "Untitled", Patch: data.Conf, Closed: closed,
			})
		}
		if e != nil {
			return e
		}
		prefix := "pages/" + r.PageID + "/data/"
		for _, change := range plan.Changes {
			name := prefix + change.Path
			hash := ""
			if change.Before != nil {
				hash = sourcetx.Hash(change.Before)
			}
			if hashes[name] != hash {
				return sourcetx.ErrConflict
			}
			after := change.After
			if action == "create" && change.ID == plan.CreatedID && plan.Configuration != nil {
				// The original create endpoint mounts the notebook before reply.
				plan.Configuration.Closed = false
				after, e = json.MarshalIndent(plan.Configuration, "", "  ")
				if e != nil {
					return e
				}
			}
			if change.Before != nil {
				// Original notebook metadata operations update typed properties.
				// Carry extension fields forward across the read/encode boundary.
				var old, next map[string]json.RawMessage
				if json.Unmarshal(change.Before, &old) != nil || json.Unmarshal(after, &next) != nil {
					return ErrArgument
				}
				for key, value := range old {
					if _, exists := next[key]; !exists {
						next[key] = value
					}
				}
				after, e = json.MarshalIndent(next, "", "  ")
				if e != nil {
					return e
				}
			}
			if !bytes.Equal(change.Before, after) {
				request.Changes = append(request.Changes, sourcetx.Change{Path: name, ExpectedHash: hash, Data: after})
			}
		}
		if action == "create" && plan.Configuration != nil {
			out = map[string]any{"notebook": map[string]any{"id": plan.CreatedID, "name": plan.Configuration.Name,
				"closed": plan.Configuration.Closed, "icon": plan.Configuration.Icon, "sort": plan.Configuration.Sort}}
		} else if action == "configuration" {
			out = plan.Configuration
		}
		return nil
	})
	if err != nil || len(request.Changes) == 0 {
		return out, err
	}
	if _, err = m.Apply(ctx, r.StoreID, request); err != nil {
		return nil, err
	}
	return out, prepareLifecycleIndex(ctx, m, r.StoreID, r.PageID)
}
