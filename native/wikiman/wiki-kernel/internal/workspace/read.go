package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

var ErrDocument = errors.New("invalid_or_missing_document")
var documentID = regexp.MustCompile(`^\d{14}-[a-z0-9]{7}$`)

// ReadDocument preserves the original AST bytes (including unknown fields).
// No parser, block cache, document ID lookup or reference state is process-global.
// Source guards cover the entire read and reject unfinished recovery journals.
func (l *Lease) ReadDocument(ctx context.Context, boxID, docID string) (json.RawMessage, error) {
	if !documentID.MatchString(boxID) || !documentID.MatchString(docID) {
		return nil, ErrDocument
	}
	pinned, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pinned.Release()
	var result json.RawMessage
	err = pinned.sourceRead(ctx, func() error {
		return pinned.workspace.resources.serial(ctx, func() error {
			cached, err := pinned.workspace.resources.load(ctx, pinned.workspace, boxID, docID)
			if err != nil {
				return err
			}
			result = append(json.RawMessage(nil), cached.raw...)
			return nil
		})
	})
	return result, err
}

func readSource(ctx context.Context, w *WorkspaceContext, boxID, docID string) (json.RawMessage, string, error) {
	return readSourceWithIO(ctx, w, boxID, docID, workspaceReadIO{})
}

// ReadBlock is deliberately document-scoped. Identical block IDs in other
// pages/documents never resolve via a global index or a current-page fallback.
func (l *Lease) ReadBlock(ctx context.Context, boxID, docID, blockID string) (json.RawMessage, error) {
	if !documentID.MatchString(boxID) || !documentID.MatchString(docID) || !documentID.MatchString(blockID) {
		return nil, ErrDocument
	}
	pinned, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pinned.Release()
	var result json.RawMessage
	err = pinned.sourceRead(ctx, func() error {
		return pinned.workspace.resources.serial(ctx, func() error {
			r := pinned.workspace.resources
			cached, err := r.load(ctx, pinned.workspace, boxID, docID)
			if err != nil {
				return err
			}
			var start, end int
			if err := r.db.QueryRowContext(ctx, "SELECT start,end FROM blocks WHERE box=? AND doc=? AND id=?", boxID, docID, blockID).Scan(&start, &end); err != nil {
				return err
			}
			if start < 0 || end > len(cached.raw) || end <= start {
				return ErrDocument
			}
			result = append(json.RawMessage(nil), cached.raw[start:end]...)
			return nil
		})
	})
	return result, err
}
