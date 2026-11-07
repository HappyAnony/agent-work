// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import "context"

// AdoptReadProjection atomically publishes both derived read databases while
// preserving this owner's asset-content/history databases and queued work.
// The caller owns its workspace serial gate and the fresh candidate exclusively.
func (b *Bundle) AdoptReadProjection(candidate *Bundle) error {
	if b == nil || candidate == nil || b == candidate {
		return ErrClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if b.closed || candidate.closed {
		return ErrClosed
	}
	for _, kind := range []Kind{Main, BlockTree} {
		b.dbs[kind], candidate.dbs[kind] = candidate.dbs[kind], b.dbs[kind]
		b.keepers[kind], candidate.keepers[kind] = candidate.keepers[kind], b.keepers[kind]
	}
	return nil
}

// AdoptAssetReadProjection replaces only this owner's disposable attachment
// content index, after all original converters have completed successfully.
func (b *Bundle) AdoptAssetReadProjection(candidate *Bundle) error {
	if b == nil || candidate == nil || b == candidate {
		return ErrClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if b.closed || candidate.closed {
		return ErrClosed
	}
	b.dbs[Assets], candidate.dbs[Assets] = candidate.dbs[Assets], b.dbs[Assets]
	b.keepers[Assets], candidate.keepers[Assets] = candidate.keepers[Assets], b.keepers[Assets]
	return nil
}

// ReadProjectionPresent checks disposable schema presence, so a matching source
// digest cannot mistake dropped databases/tables for a ready projection.
func (b *Bundle) ReadProjectionPresent(ctx context.Context, assetsOnly bool) (bool, error) {
	checks := map[Kind][]string{Main: {"blocks", "blocks_fts", "blocks_fts_case_insensitive", "spans", "assets", "refs", "file_annotation_refs", "attributes"}, BlockTree: {"blocktrees"}}
	if assetsOnly {
		checks = map[Kind][]string{Assets: {"asset_contents_fts_case_insensitive"}}
	}
	for kind, names := range checks {
		db, e := b.Database(kind)
		if e != nil {
			return false, e
		}
		for _, name := range names {
			var count int
			if e = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count); e != nil {
				return false, e
			}
			if count != 1 {
				return false, nil
			}
		}
	}
	return true, ctx.Err()
}
