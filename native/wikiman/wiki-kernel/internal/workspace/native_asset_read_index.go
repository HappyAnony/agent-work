// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"
	"wiki-kernel/internal/assetextract"
	"wiki-kernel/internal/indexdb"
)

// PrepareNativeAssetReads indexes the original supported attachment formats
// only when content search/detail requests need them. Converter children are
// canceled and joined by ExtractNativeAsset; no OCR task or source write starts.
func (l *Lease) PrepareNativeAssetReads(ctx context.Context, sourceHash string) error {
	var entries []workspaceDirectoryEntry
	cached := false
	err := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		snapshot, e := r.readSourceSnapshot(ctx, w)
		if e != nil {
			return e
		}
		if snapshot.hash != sourceHash {
			return errWorkspaceDirectorySourceChanged
		}
		cached = r.readAssetProjectionHash == sourceHash
		if cached {
			var e error
			cached, e = r.native.ReadProjectionPresent(ctx, true)
			if e != nil {
				return e
			}
		}
		entries = snapshot.entries
		return nil
	})
	if err != nil || cached {
		return err
	}
	items := []indexdb.AssetContentEntry{}
	for _, entry := range entries {
		if entry.info.IsDir() || !strings.HasPrefix(entry.name, "assets/") {
			continue
		}
		ext := strings.ToLower(path.Ext(entry.name))
		kind := assetextract.Kind(ext)
		if kind == "" {
			continue
		}
		if entry.info.Size() > assetextract.MaxInput {
			continue
		}
		var content string
		var skipped bool
		if kind == "text" {
			err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
				raw, e := readNativeFile(ctx, l.workspace, entry.name, assetextract.MaxInput)
				if e != nil {
					return e
				}
				value := assetextract.ReadText(raw)
				content, skipped = value.Content, value.Skipped
				return nil
			})
		} else {
			var value NativeAssetExtraction
			value, err = l.ExtractNativeAsset(ctx, entry.name)
			content, skipped = value.Content, value.Skipped
		}
		if err != nil {
			return err
		}
		if skipped {
			continue
		}
		// Original IDs were regenerated randomly on every reindex. An opaque stable
		// derived ID preserves detail lookup across fresh CLI/HTTP index instances.
		id := sha256.Sum256([]byte(entry.name))
		name := path.Base(entry.name)
		base := strings.TrimSuffix(name, ext)
		if len(base) > 23 && documentID.MatchString(base[len(base)-22:]) {
			name = base[:len(base)-23] + ext
		}
		items = append(items, indexdb.AssetContentEntry{ID: hex.EncodeToString(id[:]), Name: name, Ext: ext, Path: entry.name, Size: entry.info.Size(), Updated: entry.info.ModTime().Unix(), Content: content})
	}
	return l.WithNativeIndexes(ctx, func(original *indexdb.Bundle) (result error) {
		r, w := l.workspace.resources, l.workspace
		snapshot, e := r.readSourceSnapshot(ctx, w)
		if e != nil {
			return e
		}
		if snapshot.hash != sourceHash {
			return errWorkspaceDirectorySourceChanged
		}
		candidate, e := indexdb.OpenMemory(ctx)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, candidate.Close()) }()
		db, e := candidate.Database(indexdb.Assets)
		if e != nil {
			return e
		}
		queue := &indexdb.AssetQueue{}
		if e = queue.Index(items); e != nil {
			return e
		}
		if e = queue.Flush(ctx, db, nil); e != nil {
			return e
		}
		if e = original.AdoptAssetReadProjection(candidate); e != nil {
			return e
		}
		r.readAssetProjectionHash = sourceHash
		return nil
	})
}
