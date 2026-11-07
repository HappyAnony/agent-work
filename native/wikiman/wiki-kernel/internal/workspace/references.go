package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
)

// WithNativeReferences executes the original upper-level reference statistics
// algorithms with this page's actual index, source admission and lease. Borrowed
// services must not escape the callback or reenter the lease's serial gate.
func (l *Lease) WithNativeReferences(ctx context.Context, fn func(indexdb.References) error) error {
	if fn == nil {
		return ErrIdentity
	}
	return l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, err := bundle.Database(indexdb.Main)
		if err != nil {
			return err
		}
		return fn(indexdb.References{DB: db})
	})
}
