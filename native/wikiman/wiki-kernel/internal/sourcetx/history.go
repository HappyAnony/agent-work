// SPDX-License-Identifier: AGPL-3.0-or-later
package sourcetx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// HistoryChanges resolves checked immutable journal images on the server.
// Clients retain a small operation handle rather than whole document snapshots.
// Undo and redo both return ordinary CAS changes for the same durable writer.
func (s *Store) HistoryChanges(ctx context.Context, operation, page string, redo bool, scope string) (out []Change, err error) {
	if !operationID.MatchString(operation) || !pageID.MatchString(page) {
		return nil, ErrInvalid
	}
	err = s.Read(ctx, func() (err error) {
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, root.Close()) }()
		r, found, err := receipt(root, operation)
		if err != nil || !found || !r.SourceCommitted {
			return errors.Join(ErrRecovery, err)
		}
		if scope != "" && scope != r.HistoryScope {
			return ErrIdentity
		}
		if len(r.ChangedPageIDs) == 0 {
			out = []Change{}
			return ctx.Err()
		}
		j, committed, err := loadJournal(root, operation)
		if err != nil {
			return err
		}
		if !committed {
			return ErrRecovery
		}
		if r.RequestHash != j.RequestHash || r.HistoryScope != j.HistoryScope {
			return errors.Join(ErrRecovery, err)
		}
		prefix := "pages/" + page + "/data/"
		changes := make([]Change, 0, len(j.Files))
		for i, file := range j.Files {
			if err = ctx.Err(); err != nil {
				return err
			}
			if !strings.HasPrefix(file.Path, prefix) {
				return ErrIdentity
			}
			expected, target, side := file.AfterHash, file.BeforeHash, "before"
			if redo {
				expected, target, side = file.BeforeHash, file.AfterHash, "after"
			}
			change := Change{Path: file.Path, ExpectedHash: expected, Delete: target == ""}
			if target != "" {
				data, exists, e := readFile(root, fmt.Sprintf(".local/staging/%s/%s/%d", operation, side, i), maxFile)
				if e != nil || !exists || Hash(data) != target {
					return errors.Join(ErrRecovery, e)
				}
				change.Data = data
			}
			changes = append(changes, change)
		}
		out = changes
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CommittedHashes is independent of later source edits: it identifies exactly
// the acknowledged operation, including an idempotent retry after response loss.
func (s *Store) CommittedHashes(ctx context.Context, operation, page string) (map[string]string, error) {
	changes, err := s.HistoryChanges(ctx, operation, page, false, "")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, c := range changes {
		out[c.Path] = c.ExpectedHash
	}
	return out, nil
}
