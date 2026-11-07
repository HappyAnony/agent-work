// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"context"
	av "wiki-kernel/internal/avmodel"
)

// FieldResolver is operation-local. It schedules dependencies, while original
// FillBaseValue/FillBasicAutoValues/RenderTemplateField/BuildContents own results.
// No workspace, I/O, language, global cache, save, or UI dependency is implicit.
type FieldResolver struct {
	Context            context.Context
	Read               func(string) (*av.AttributeView, error)
	Prepare            func(*av.AttributeView, av.Collection) error
	Templates          TemplateEnvironment
	MaxNodes, MaxDepth int
	rows               map[string]map[string]*av.Value
	states             map[FieldRef]uint8
	values             map[FieldRef]*av.Value
	stack              []FieldRef
	work               int
}

func (r *FieldResolver) row(a *av.AttributeView, row string) (map[string]*av.Value, error) {
	if r.rows == nil {
		r.rows = map[string]map[string]*av.Value{}
	}
	id := a.ID + "/" + row
	if values := r.rows[id]; values != nil {
		return values, nil
	}
	primary := a.GetBlockKeyValues()
	if primary == nil || primary.GetValue(row) == nil {
		return nil, nil
	}
	r.work += len(a.KeyValues)
	if r.work > r.MaxNodes {
		return nil, ErrLimit
	}
	values := map[string]*av.Value{}
	item := &av.TableRow{ID: row}
	for _, kv := range a.KeyValues {
		v := kv.GetValue(row)
		if v != nil {
			v = v.Clone()
		}
		base := &av.BaseValue{ID: row + "-" + kv.Key.ID, ValueType: kv.Key.Type, Value: v}
		if v != nil {
			base.ID = v.ID
		}
		isTime := kv.Key.Date != nil && kv.Key.Date.FillSpecificTime
		FillBaseValue(base, kv.Key.ID, row, kv.Key.NumberFormat, kv.Key.Template, isTime)
		item.Cells = append(item.Cells, &av.TableCell{BaseValue: base})
		values[kv.Key.ID] = base.Value
	}
	if e := r.Prepare(a, &av.Table{Rows: []*av.TableRow{item}}); e != nil {
		return nil, e
	}
	r.rows[id] = values
	return values, nil
}

func (r *FieldResolver) Resolve(a *av.AttributeView, keyID, row string) (ret *av.Value, err error) {
	if r.Context == nil || r.Read == nil || r.Prepare == nil || r.MaxNodes <= 0 || r.MaxDepth <= 0 {
		return nil, ErrEnvironmentUnbound
	}
	if e := r.Context.Err(); e != nil {
		return nil, e
	}
	r.work++
	if r.work > r.MaxNodes {
		return nil, ErrLimit
	}
	f := FieldRef{a.ID, keyID, row}
	if r.states == nil {
		r.states = map[FieldRef]uint8{}
		r.values = map[FieldRef]*av.Value{}
	}
	if r.states[f] == 2 {
		return r.values[f], nil
	}
	if r.states[f] == 1 {
		path := append([]FieldRef(nil), r.stack...)
		path = append(path, f)
		return nil, &FieldCycleError{Path: path}
	}
	if len(r.stack) >= r.MaxDepth {
		return nil, ErrLimit
	}
	k, e := a.GetKey(keyID)
	if e != nil {
		return nil, e
	}
	values, e := r.row(a, row)
	if e != nil || values == nil {
		return nil, e
	}
	r.states[f] = 1
	r.stack = append(r.stack, f)
	defer func() {
		r.stack = r.stack[:len(r.stack)-1]
		if err != nil {
			delete(r.states, f)
		}
	}()
	v := values[keyID]
	switch k.Type {
	case av.KeyTypeTemplate:
		kvs := []*av.KeyValues{}
		for _, kv := range a.KeyValues {
			kvs = append(kvs, &av.KeyValues{Key: kv.Key, Values: []*av.Value{values[kv.Key.ID]}})
		}
		ial := map[string]string{} // Native Prepare supplies canonical IAL through the row's block value below.
		if r.Templates.IAL != nil {
			ial, e = r.Templates.IAL(a, row)
			if e != nil {
				return nil, e
			}
		}
		env := r.Templates
		env.Context = r.Context
		env.Charge = func(n int) error {
			r.work += n
			if r.work > r.MaxNodes {
				return ErrLimit
			}
			return r.Context.Err()
		}
		env.Resolve = func(id string) error {
			_, err := r.Resolve(a, id, row)
			return err
		}
		s, e := RenderTemplateField(ial, kvs, k.Template, env)
		if e != nil {
			return nil, e
		}
		v.Template.Content = s
	case av.KeyTypeRelation:
		if k.Relation == nil {
			return nil, ErrShape
		}
		dest, e := r.Read(k.Relation.AvID)
		if e != nil {
			return nil, e
		}
		if dest == nil || dest.GetBlockKey() == nil {
			return nil, ErrShape
		}
		v.Relation.Contents = nil
		for _, id := range v.Relation.BlockIDs {
			val, e := r.Resolve(dest, dest.GetBlockKey().ID, id)
			if e != nil {
				return nil, e
			}
			if val != nil {
				v.Relation.Contents = append(v.Relation.Contents, val.Clone())
			}
		}
	case av.KeyTypeRollup:
		if k.Rollup == nil {
			return nil, ErrShape
		}
		rel, e := r.Resolve(a, k.Rollup.RelationKeyID, row)
		if e != nil {
			return nil, e
		}
		if rel == nil || rel.Relation == nil {
			return nil, ErrShape
		}
		relKey, e := a.GetKey(k.Rollup.RelationKeyID)
		if e != nil || relKey.Relation == nil {
			return nil, ErrShape
		}
		dest, e := r.Read(relKey.Relation.AvID)
		if e != nil {
			return nil, e
		}
		if dest == nil {
			return nil, ErrShape
		}
		target, e := dest.GetKey(k.Rollup.KeyID)
		if e != nil {
			return nil, e
		}
		if a.DurationFormatter == nil && k.Rollup.Calc != nil && k.Rollup.Calc.Operator == av.CalcOperatorRange && (target.Type == av.KeyTypeDate || target.Type == av.KeyTypeCreated || target.Type == av.KeyTypeUpdated) {
			return nil, av.ErrDurationFormatterUnbound
		}
		kv := &av.KeyValues{Key: target}
		for _, id := range rel.Relation.BlockIDs {
			val, e := r.Resolve(dest, target.ID, id)
			if e != nil {
				return nil, e
			}
			if val != nil {
				kv.Values = append(kv.Values, val.Clone())
			}
		}
		v.Rollup.BuildContents([]*av.KeyValues{kv}, target, rel, k.Rollup.Calc, nil, a.DurationFormatter)
	}
	if e := r.Context.Err(); e != nil {
		return nil, e
	}
	r.states[f] = 2
	r.values[f] = v
	return v, nil
}

func (r *FieldResolver) Fill(a *av.AttributeView, c av.Collection) error {
	if r.Context == nil {
		return ErrEnvironmentUnbound
	}
	for _, item := range c.GetItems() {
		for _, v := range item.GetValues() {
			if v.Type != av.KeyTypeTemplate && v.Type != av.KeyTypeRelation && v.Type != av.KeyTypeRollup {
				continue
			}
			resolved, e := r.Resolve(a, v.KeyID, item.GetID())
			if e != nil {
				return e
			}
			if resolved != nil {
				*v = *resolved.Clone()
			}
		}
	}
	return r.Context.Err()
}
