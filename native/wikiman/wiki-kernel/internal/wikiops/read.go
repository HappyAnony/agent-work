// Package wikiops implements shared application services for local handlers.
package wikiops

import (
	"context"
	"encoding/json"
	"wiki-kernel/internal/workspace"
)

type ReadRequest struct {
	StoreID string `json:"storeId"`
	PageID  string `json:"pageId"`
	BoxID   string `json:"boxId"`
	DocID   string `json:"docId"`
	BlockID string `json:"blockId,omitempty"`
}

type ReadResult struct {
	StoreID    string                   `json:"storeId"`
	PageID     string                   `json:"pageId"`
	Generation uint64                   `json:"generation"`
	AST        json.RawMessage          `json:"ast"`
	Notebook   workspace.NotebookConfig `json:"notebook"`
}

func Read(ctx context.Context, manager *workspace.WorkspaceManager, request ReadRequest) (ReadResult, error) {
	var result ReadResult
	err := manager.WithReadContext(ctx, request.StoreID, func(ctx context.Context) error { var err error; result, err = read(ctx, manager, request); return err })
	return result, err
}

func read(ctx context.Context, manager *workspace.WorkspaceManager, request ReadRequest) (ReadResult, error) {
	lease, err := manager.Acquire(ctx, request.StoreID, request.PageID)
	if err != nil {
		return ReadResult{}, err
	}
	defer lease.Release()
	notebook, err := lease.NotebookConfig(ctx, request.BoxID)
	if err != nil {
		return ReadResult{}, err
	}
	var ast json.RawMessage
	if request.BlockID == "" {
		ast, err = lease.ReadDocument(ctx, request.BoxID, request.DocID)
	} else {
		ast, err = lease.ReadBlock(ctx, request.BoxID, request.DocID, request.BlockID)
	}
	if err != nil {
		return ReadResult{}, err
	}
	return ReadResult{StoreID: lease.StoreID(), PageID: lease.PageID(), Generation: lease.Generation(), AST: ast, Notebook: notebook}, nil
}
