package workspace

import (
	"context"
	"encoding/json"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeAttributeViewItemIDs(ctx context.Context, id string, boundIDs []string) (map[string]string, error) {
	return l.nativeAttributeViewIdentities(ctx, id, boundIDs, true)
}
func (l *Lease) NativeAttributeViewBoundBlockIDs(ctx context.Context, id string, itemIDs []string) (map[string]string, error) {
	return l.nativeAttributeViewIdentities(ctx, id, itemIDs, false)
}

func (l *Lease) nativeAttributeViewIdentities(ctx context.Context, id string, ids []string, toItems bool) (ret map[string]string, err error) {
	ids = append([]string(nil), ids...)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		raw, _, err := readNativeAVSource(ctx, l.workspace, id)
		if err != nil {
			return err
		}
		a := &avmodel.AttributeView{}
		if err = json.Unmarshal(raw, a); err != nil {
			return err
		}
		if a.ID != id {
			return ErrDocument
		}
		ret, err = a.IdentityMap(ctx, ids, toItems)
		return err
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
