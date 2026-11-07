package wikiops

import (
	"context"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

type RecoverRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	StoreID         string `json:"storeId"`
	DryRun          bool   `json:"dryRun,omitempty"`
}
type RecoverResult struct {
	StoreID  string                   `json:"storeId"`
	Receipts []sourcetx.Receipt       `json:"receipts"`
	Preview  *[]sourcetx.RecoveryStep `json:"preview,omitempty"`
}

func (r RecoverRequest) Validate() error {
	return (StatusRequest{ProtocolVersion: r.ProtocolVersion, StoreID: r.StoreID}).Validate()
}
func Recover(ctx context.Context, m *workspace.WorkspaceManager, r RecoverRequest) (RecoverResult, error) {
	result := RecoverResult{StoreID: r.StoreID}
	if err := r.Validate(); err != nil {
		return result, err
	}
	if r.DryRun {
		return previewRecovery(ctx, r.StoreID, m.PreviewRecovery)
	}
	var err error
	result.Receipts, err = m.Recover(ctx, r.StoreID)
	return result, err
}

// RecoverRoot is the same transaction recovery service with explicit registration
// before normal page-graph admission. It never scans a half-published page tree.
func RecoverRoot(ctx context.Context, root string, r RecoverRequest) (RecoverResult, error) {
	result := RecoverResult{StoreID: r.StoreID}
	if err := r.Validate(); err != nil {
		return result, err
	}
	if r.DryRun {
		return previewRecovery(ctx, r.StoreID, func(ctx context.Context, storeID string) ([]sourcetx.RecoveryStep, error) {
			source, err := sourcetx.OpenInspection(root)
			if err != nil {
				return nil, err
			}
			return source.PreviewRecovery(ctx, storeID)
		})
	}
	source, err := sourcetx.Open(root)
	if err != nil {
		return result, err
	}
	identity, err := source.Identity(ctx)
	if err != nil {
		return result, err
	}
	if identity.StoreID != r.StoreID {
		return result, workspace.ErrIdentity
	}
	writer, err := source.Writer(ctx, false)
	if err != nil {
		return result, err
	}
	result.Receipts, err = writer.Recover(ctx, nil)
	closeErr := writer.Close()
	if err == nil {
		err = closeErr
	}
	return result, err
}

func previewRecovery(ctx context.Context, id string, inspect func(context.Context, string) ([]sourcetx.RecoveryStep, error)) (RecoverResult, error) {
	out := RecoverResult{StoreID: id, Receipts: []sourcetx.Receipt{}}
	steps, err := inspect(ctx, id)
	if err == nil {
		out.Preview = &steps
	}
	return out, err
}
