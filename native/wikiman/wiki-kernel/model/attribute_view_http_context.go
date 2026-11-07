// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/avrender"
)

// Foreground AV controllers participate in the same actual owner drain as
// their source/SQL effects and delayed notices. Read bindings cannot acquire a
// source writer; compatibility mutations retain their original sequential core.
func runOwnedAV[T any](ctx context.Context, write bool, fn func(context.Context, *legacyAVBinding) (T, error)) (out T, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	err = base.run(ctx, func(c, life context.Context) (result error) {
		r, e := captureIndexRepair(c)
		if e != nil {
			return e
		}
		r.queryOnly = !write
		env := legacyTransactionEnvironmentContext(c)
		if e = bindLegacyCommit(c, env, r); e != nil {
			return e
		}
		b := bindAVOperation(env.commitContext, c)
		b.scope.readOnly = !write
		if e = b.guard(); e != nil {
			return e
		}
		if write {
			defer func() { result = errors.Join(result, r.queue.Flush(context.WithoutCancel(c))) }()
		}
		out, e = fn(c, b)
		return e
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return
}

func AttributeViewOperationContext(ctx context.Context, op *Operation) error {
	if op == nil {
		return errors.New("invalid_av_operation")
	}
	copy := *op
	_, e := runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (struct{}, error) {
		var e error
		switch {
		case isAttributeViewManagementAction(copy.Action):
			e = runAttributeViewManagement(&copy, managementWithBinding(nil, b, c))
		case isAttributeViewOrderAction(copy.Action):
			e = runAttributeViewOrder(c, &copy, orderWithBinding(b, c))
		case isAttributeViewRowAction(copy.Action):
			e = runAttributeViewRow(&copy, rowWithBinding(nil, b, c))
		default:
			e = runAttributeViewField(c, &copy, fieldWithBinding(b))
		}
		return struct{}{}, e
	})
	return e
}
func BatchReplaceAttributeViewBlocksContext(ctx context.Context, id string, detached bool, pairs []map[string]string) error {
	_, e := runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (struct{}, error) {
		return struct{}{}, batchReplaceAttributeViewBlocksWithEnvironment(id, detached, pairs, rowWithBinding(nil, b, c))
	})
	return e
}
func AppendAttributeViewRowsContext(ctx context.Context, id string, rows [][]*av.Value) error {
	_, e := runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (struct{}, error) {
		return struct{}{}, appendAttributeViewDetachedBlocksWithEnvironment(id, rows, appendWithBinding(b, c))
	})
	return e
}
func UpdateAttributeViewCellContext(ctx context.Context, id, key, item string, value interface{}) (*av.Value, error) {
	return runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (*av.Value, error) {
		return updateAttributeViewCellWithEnvironment(id, key, item, value, legacyAttributeViewCellWithBinding(nil, b))
	})
}
func BatchUpdateAttributeViewCellsContext(ctx context.Context, id string, values []interface{}) error {
	_, e := runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (struct{}, error) {
		return struct{}{}, batchUpdateAttributeViewCellsWithEnvironment(id, values, legacyAttributeViewCellWithBinding(nil, b))
	})
	return e
}
func DuplicateDatabaseBlockContext(ctx context.Context, id string) (string, string, error) {
	type result struct{ ID, Block string }
	out, e := runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) (result, error) {
		id, block, e := duplicateDatabaseBlockWithEnvironment(id, duplicateWithBinding(nil, b, c))
		return result{id, block}, e
	})
	return out.ID, out.Block, e
}
func AttributeViewIdentitiesContext(ctx context.Context, id string, ids []string, toItems bool) (map[string]string, error) {
	ids = append([]string(nil), ids...)
	return runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) (map[string]string, error) {
		return attributeViewIdentitiesWithEnvironment(id, ids, toItems, &attributeViewIdentityEnvironment{ctx: c, read: b.readCandidate})
	})
}
func AttributeViewMirrorBlocksContext(ctx context.Context, id string) ([]string, error) {
	return runOwnedAV(ctx, false, func(_ context.Context, b *legacyAVBinding) ([]string, error) {
		return b.mirrors(nil, id)
	})
}
func AttributeViewDefaultValuesContext(ctx context.Context, id, view, group, previous, block string) (map[string]*av.Value, error) {
	return runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) (map[string]*av.Value, error) {
		return attrViewAddingBlockDefaultValuesWithEnvironment(id, view, group, previous, block, previewWithBinding(b, c))
	})
}
func AttributeViewKeysContext(ctx context.Context, id, kind, keyword string, ids []string) ([]*av.Key, error) {
	ids = append([]string(nil), ids...)
	return runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) ([]*av.Key, error) {
		if e := b.waitForStorages(); e != nil {
			return nil, e
		}
		a, e := b.read(id)
		if e != nil {
			return nil, e
		}
		a, e = b.clone(a)
		if e != nil {
			return nil, e
		}
		keys := []*av.Key{}
		for _, kv := range a.KeyValues {
			if kv == nil || kv.Key == nil {
				return nil, avrender.ErrShape
			}
			keys = append(keys, kv.Key)
		}
		return avquery.SelectKeys(c, keys, func(k *av.Key) avquery.KeyMeta {
			return avquery.KeyMeta{ID: k.ID, Name: k.Name, Type: string(k.Type), Relation: k.Relation != nil}
		}, kind, keyword, ids)
	})
}
func GetAttributeViewContext(ctx context.Context, id string) (*av.AttributeView, error) {
	return runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) (*av.AttributeView, error) {
		e := readWithBinding(nil, b, c)
		e.strict = true
		return readAttributeViewWithEnvironment(id, "source", e)
	})
}
func GetAttributeViewPrimaryKeyValuesContext(ctx context.Context, id, keyword string, page, size int) (string, []string, *av.KeyValues, error) {
	out, e := runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) (attributeViewPrimaryResult, error) {
		e := readWithBinding(nil, b, c)
		e.strict = true
		return attributeViewPrimaryWithEnvironment(id, keyword, page, size, e)
	})
	return out.name, out.blocks, out.values, e
}
func GetAttributeViewFilterSortContext(ctx context.Context, id, block string) ([]*av.ViewFilter, []*av.ViewSort, error) {
	type result struct {
		Filters []*av.ViewFilter
		Sorts   []*av.ViewSort
	}
	out, e := runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) (result, error) {
		env := readWithBinding(nil, b, c)
		env.strict = true
		filters, sorts, e := attributeViewRulesWithEnvironment(id, block, env)
		return result{filters, sorts}, e
	})
	return out.Filters, out.Sorts, e
}
func GetCurrentAttributeViewImagesContext(ctx context.Context, id, view, query string) ([]string, error) {
	return runOwnedAV(ctx, false, func(c context.Context, b *legacyAVBinding) ([]string, error) {
		return currentAttributeViewImagesWithEnvironment(id, view, query, imagesWithBinding(b, c))
	})
}
func GetBlockAttributeViewKeysContext(ctx context.Context, id string) ([]*BlockAttributeViewKeys, error) {
	return runOwnedAV(ctx, true, func(c context.Context, b *legacyAVBinding) ([]*BlockAttributeViewKeys, error) {
		return blockAttributeViewKeysWithEnvironment(id, blockKeysWithBinding(nil, b, c))
	})
}
