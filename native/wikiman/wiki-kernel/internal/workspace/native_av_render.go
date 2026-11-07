package workspace

import (
	"context"
	"encoding/json"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

// NativeAttributeViewTable uses the original table assembly/filter/sort/calc
// against detached source copies. It is not a save or editor HTTP endpoint.
func (l *Lease) NativeAttributeViewTable(ctx context.Context, id, viewID, query string, page, pageSize int) (json.RawMessage, error) {
	return l.nativeAttributeViewRender(ctx, id, viewID, query, page, pageSize, nil, true)
}

func validateNativeAVRender(a *av.AttributeView) error {
	// Legacy formats need the separately audited compatibility/upgrader path.
	// Never silently render obsolete row/filter storage as a modern empty view.
	if a.Spec != 4 {
		return avrender.ErrUnsupported
	}
	if len(a.Views) > 128 || len(a.KeyValues) > 512 {
		return ErrBudget
	}
	keys := map[string]bool{}
	count := 0
	primary := 0
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil || kv.Key.ID == "" || keys[kv.Key.ID] {
			return avrender.ErrShape
		}
		keys[kv.Key.ID] = true
		for _, opt := range kv.Key.Options {
			if opt == nil {
				return avrender.ErrShape
			}
		}
		switch kv.Key.Type {
		case av.KeyTypeTemplate:
			if len(kv.Key.Template) > 65536 {
				return ErrBudget
			}
		case av.KeyTypeRelation:
			if kv.Key.Relation == nil || !documentID.MatchString(kv.Key.Relation.AvID) {
				return avrender.ErrShape
			}
		case av.KeyTypeRollup:
			if kv.Key.Rollup == nil || kv.Key.Rollup.RelationKeyID == "" || kv.Key.Rollup.KeyID == "" {
				return avrender.ErrShape
			}
		case av.KeyTypeBlock:
			primary++
		case av.KeyTypeText, av.KeyTypeNumber, av.KeyTypeDate, av.KeyTypeSelect, av.KeyTypeMSelect, av.KeyTypeURL, av.KeyTypeEmail, av.KeyTypePhone, av.KeyTypeMAsset, av.KeyTypeCreated, av.KeyTypeUpdated, av.KeyTypeCheckbox:
		default:
			return avrender.ErrShape
		}
		seen := map[string]bool{}
		for _, v := range kv.Values {
			count++
			if count > 100000 {
				return ErrBudget
			}
			if v == nil || v.Type != kv.Key.Type || !documentID.MatchString(v.BlockID) || v.KeyID != kv.Key.ID || v.ID == "" || seen[v.BlockID] {
				return avrender.ErrShape
			}
			seen[v.BlockID] = true
			avrender.FillNilValue(v, kv.Key.Type)
			if v.Relation != nil {
				if len(v.Relation.BlockIDs) > 10000 {
					return ErrBudget
				}
				for _, id := range v.Relation.BlockIDs {
					if !documentID.MatchString(id) {
						return avrender.ErrShape
					}
				}
			}
			if kv.Key.Type == av.KeyTypeBlock && v.Block == nil {
				return avrender.ErrShape
			}
			for _, asset := range v.MAsset {
				if asset == nil {
					return avrender.ErrShape
				}
			}
			for _, option := range v.MSelect {
				if option == nil {
					return avrender.ErrShape
				}
			}
		}
	}
	for _, kv := range a.KeyValues {
		if kv.Key.Type == av.KeyTypeRollup {
			k, _ := a.GetKey(kv.Key.Rollup.RelationKeyID)
			if k == nil || k.Type != av.KeyTypeRelation || k.Relation == nil {
				return avrender.ErrShape
			}
		}
	}
	if primary != 1 || len(a.Views) == 0 {
		return avrender.ErrShape
	}
	for _, v := range a.Views {
		if v == nil || v.ID == "" {
			return avrender.ErrShape
		}
		if v.GroupCalc != nil && v.GroupCalc.FieldCalc == nil {
			return avrender.ErrShape
		}
		for _, f := range v.Filters {
			if f == nil {
				return avrender.ErrShape
			}
			if f.Value != nil {
				avrender.FillNilValue(f.Value, f.Value.Type)
			}
		}
		for _, s := range v.Sorts {
			if s == nil {
				return avrender.ErrShape
			}
		}
		if v.Table != nil {
			if v.Table.BaseLayout == nil {
				return avrender.ErrShape
			}
			for _, col := range v.Table.Columns {
				if col == nil || col.BaseField == nil || !keys[col.ID] {
					return avrender.ErrShape
				}
			}
		}
		if v.Gallery != nil {
			if v.Gallery.BaseLayout == nil || v.Gallery.CoverFrom < av.CoverFromNone || v.Gallery.CoverFrom > av.CoverFromContentBlock {
				return avrender.ErrShape
			}
			for _, f := range v.Gallery.CardFields {
				if f == nil || f.BaseField == nil || !keys[f.ID] {
					return avrender.ErrShape
				}
			}
		}
		if v.Kanban != nil {
			if v.Kanban.BaseLayout == nil || v.Kanban.CoverFrom < av.CoverFromNone || v.Kanban.CoverFrom > av.CoverFromContentBlock {
				return avrender.ErrShape
			}
			for _, f := range v.Kanban.Fields {
				if f == nil || f.BaseField == nil || !keys[f.ID] {
					return avrender.ErrShape
				}
			}
		}
		if len(v.Groups) > 256 {
			return ErrBudget
		}
		for _, g := range v.Groups {
			if g == nil || g.LayoutType != v.LayoutType {
				return avrender.ErrShape
			}
			if g.GroupCalc != nil && g.GroupCalc.FieldCalc == nil {
				return avrender.ErrShape
			}
			if _, err := nativeAVFieldCount(g); err != nil {
				return err
			}
		}
	}
	return nil
}
