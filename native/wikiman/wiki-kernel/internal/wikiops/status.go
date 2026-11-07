package wikiops

import (
	"context"

	"wiki-kernel/internal/buildinfo"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

type StatusRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	StoreID         string `json:"storeId"`
}

func (r StatusRequest) Validate() error {
	if r.ProtocolVersion != buildinfo.ProtocolVersion || r.StoreID == "" || len(r.StoreID) > 128 {
		return ErrArgument
	}
	return nil
}

type StatusResult struct {
	StoreID string `json:"storeId"`
	Version string `json:"version"`
	sourcetx.Inspection
}

func Status(ctx context.Context, m *workspace.WorkspaceManager, r StatusRequest) (StatusResult, error) {
	return inspectStatus(ctx, r, m.Inspect)
}

func StatusRoot(ctx context.Context, root string, r StatusRequest) (StatusResult, error) {
	return inspectStatus(ctx, r, func(ctx context.Context, storeID string) (sourcetx.Inspection, error) {
		source, err := sourcetx.OpenInspection(root)
		if err != nil {
			return sourcetx.Inspection{}, err
		}
		return source.Inspect(ctx, storeID)
	})
}

func inspectStatus(ctx context.Context, r StatusRequest, inspect func(context.Context, string) (sourcetx.Inspection, error)) (StatusResult, error) {
	out := StatusResult{StoreID: r.StoreID, Version: buildinfo.Version}
	if err := r.Validate(); err != nil {
		return out, err
	}
	var err error
	out.Inspection, err = inspect(ctx, r.StoreID)
	return out, err
}
