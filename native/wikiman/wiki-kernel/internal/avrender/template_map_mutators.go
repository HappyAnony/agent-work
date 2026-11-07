// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"reflect"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	av "wiki-kernel/internal/avmodel"
)

type templateMapEdit struct {
	value   any
	deleted bool
}

var cloneTemplateRaw = sprig.TxtFuncMap()["mustDeepCopy"].(func(any) (any, error))

var originalMapMutators = func() template.FuncMap {
	all, selected := sprig.TxtFuncMap(), template.FuncMap{}
	for _, name := range []string{"set", "unset", "merge", "mustMerge", "mergeOverwrite", "mustMergeOverwrite"} {
		selected[name] = all[name]
	}
	return selected
}()

func (l *lazyTemplate) edited(m map[string]any, key string) bool {
	_, ok := l.edits[reflect.ValueOf(m).Pointer()][key]
	return ok
}

func (l *lazyTemplate) recordEdit(m map[string]any, key string, deleted bool) {
	pointer := reflect.ValueOf(m).Pointer()
	if l.mapRefs[pointer] == nil {
		return
	}
	if l.edits[pointer] == nil {
		l.edits[pointer] = map[string]templateMapEdit{}
	}
	l.edits[pointer][key] = templateMapEdit{m[key], deleted}
}

func (l *lazyTemplate) refreshMap(m, next map[string]any) {
	for key := range m {
		delete(m, key)
	}
	for key, value := range next {
		m[key] = value
	}
	for key, edit := range l.edits[reflect.ValueOf(m).Pointer()] {
		if edit.deleted {
			delete(m, key)
		} else {
			m[key] = edit.value
		}
	}
}

func (l *lazyTemplate) localRawValue(k *av.Key, v *av.Value) *av.Value {
	if copy := l.rawCopies[k.ID]; copy != nil {
		return copy
	}
	// JSON cloning loses json:"-" flags and nil/empty distinctions visible to
	// original templates. The pinned original deep-copy helper preserves Go data.
	if l.rawErr != nil {
		return nil
	}
	value, err := cloneTemplateRaw(v)
	if err != nil {
		l.rawErr = err
		return nil
	}
	copy, ok := value.(*av.Value)
	if !ok {
		l.rawErr = ErrShape
		return nil
	}
	l.rawCopies[k.ID] = copy
	return copy
}

func (l *lazyTemplate) resolve(id string) error {
	if err := l.env.Resolve(id); err != nil {
		return err
	}
	if !l.resolved[id] {
		delete(l.rawCopies, id)
		l.resolved[id] = true
	}
	return nil
}

func (l *lazyTemplate) bindMapMutators(funcs template.FuncMap, safe map[string]bool) {
	for name, original := range originalMapMutators {
		fn := l.env.Funcs[name]
		if fn == nil || reflect.TypeOf(fn) != reflect.TypeOf(original) || reflect.ValueOf(fn).Pointer() != reflect.ValueOf(original).Pointer() {
			continue
		}
		safe[name] = true
		switch name {
		case "set":
			set := fn.(func(map[string]any, string, any) map[string]any)
			funcs[name] = func(m map[string]any, key string, value any) (map[string]any, error) {
				if err := l.mapWork(1); err != nil {
					return nil, err
				}
				result := set(m, key, value)
				l.recordEdit(m, key, false)
				return result, nil
			}
		case "unset":
			unset := fn.(func(map[string]any, string) map[string]any)
			funcs[name] = func(m map[string]any, key string) (map[string]any, error) {
				if err := l.mapWork(1); err != nil {
					return nil, err
				}
				result := unset(m, key)
				l.recordEdit(m, key, true)
				return result, nil
			}
		default:
			overwrite := strings.HasSuffix(name, "Overwrite")
			funcs[name] = func(dst map[string]any, srcs ...map[string]any) (any, error) {
				if err := l.mapWork(1); err != nil {
					return nil, err
				}
				// Apply one source at a time, as the original Sprig implementation
				// does, so later sources see preceding writes and original errors.
				for _, src := range srcs {
					if err := l.mergeGraph(dst, map[lazyMapVisit]uint8{}, 0); err != nil {
						return nil, err
					}
					if err := l.mergeGraph(src, map[lazyMapVisit]uint8{}, 0); err != nil {
						return nil, err
					}
					touched := map[uintptr]map[string]bool{}
					if err := l.prepareMerge(dst, src, overwrite, 0, touched); err != nil {
						return nil, err
					}
					before, err := l.snapshotMaps()
					if err != nil {
						return nil, err
					}
					var result any
					switch merge := fn.(type) {
					case func(map[string]any, ...map[string]any) any:
						result = merge(dst, src)
					case func(map[string]any, ...map[string]any) (any, error):
						result, err = merge(dst, src)
					}
					if captureErr := l.captureMapEdits(before); captureErr != nil {
						return nil, captureErr
					}
					if err != nil {
						return nil, err
					}
					if _, failed := result.(string); failed {
						return result, nil
					} // Original non-must failure is "".
					// Equal-value writes are still writes: freezing only diffs can
					// resurrect a formula overwritten with its stored old string.
					for pointer, keys := range touched {
						if err := l.mapWork(len(keys)); err != nil {
							return nil, err
						}
						for key := range keys {
							_, exists := l.mapRefs[pointer][key]
							l.recordEdit(l.mapRefs[pointer], key, !exists)
						}
					}
					if merged, ok := result.(map[string]any); ok {
						dst = merged
					}
				}
				return dst, nil
			}
		}
	}
}

// The known original template display type is string even before resolution.
// This permits a default merge to ignore an unused source template, and an
// overwrite to replace a destination template without reading its old formula.
func (l *lazyTemplate) templateString(m map[string]any, key string) bool {
	if l.edited(m, key) {
		return false
	}
	pointer := reflect.ValueOf(m).Pointer()
	deps := l.maps[pointer][key]
	if len(deps) == 0 || pointer == l.rawMap {
		return false
	}
	property := key
	if pointer == l.idMap {
		property = l.byID[key].Key.Name
	}
	for i := len(deps) - 1; i >= 0; i-- {
		id := deps[i]
		k := l.byID[id].Key
		if k.Type == av.KeyTypeTemplate && property == k.Name {
			return true
		}
		if !l.resolved[id] && property == k.Name && (k.Type == av.KeyTypeRelation || k.Type == av.KeyTypeRollup) {
			return false
		}
		if l.writes(id, property) {
			return false
		}
	}
	return false
}

func mergeScalar(value any) bool {
	if value == nil {
		return false
	}
	switch reflect.TypeOf(value).Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	}
	return false
}

// Mirror only Mergo's emptiness predicate for dependency selection; all writes,
// deep merging, nil/slice behavior and error policy remain with the original.
func mergeValueNonempty(value any) bool {
	v := reflect.ValueOf(value)
	for depth := 0; depth < 256; depth++ {
		if !v.IsValid() {
			return false
		}
		switch v.Kind() {
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return false
			}
			v = v.Elem()
			continue
		case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
			return v.Len() != 0
		case reflect.Bool:
			return v.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return v.Int() != 0
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return v.Uint() != 0
		case reflect.Float32, reflect.Float64:
			return v.Float() != 0
		case reflect.Func:
			return !v.IsNil()
		default:
			return true
		}
	}
	return false // Unknown recursive pointers are never optimized as a skipped read.
}

func (l *lazyTemplate) prepareMerge(dst, src map[string]any, overwrite bool, depth int, touched map[uintptr]map[string]bool) error {
	if depth > 256 {
		return ErrLimit
	}
	if err := l.prepareMembership(src); err != nil {
		return err
	}
	if reflect.ValueOf(dst).Pointer() == reflect.ValueOf(src).Pointer() {
		return nil
	}
	for _, key := range sortedMapKeys(src) {
		pointer := reflect.ValueOf(dst).Pointer()
		if l.mapRefs[pointer] != nil {
			if touched[pointer] == nil {
				touched[pointer] = map[string]bool{}
			}
			touched[pointer][key] = true
		}
		if !overwrite {
			if _, err := l.prepare(dst, key); err != nil {
				return err
			}
			if mergeValueNonempty(dst[key]) && (mergeScalar(dst[key]) || l.templateString(src, key)) {
				continue
			}
		}
		if _, err := l.prepare(src, key); err != nil {
			return err
		}
		sv := src[key]
		if overwrite {
			skip := sv == nil || l.templateString(dst, key)
			if sv != nil {
				switch reflect.TypeOf(sv).Kind() {
				case reflect.Map, reflect.Struct, reflect.Ptr:
				default:
					skip = true
				}
			}
			if !skip {
				if _, err := l.prepare(dst, key); err != nil {
					return err
				}
			}
		}
		dm, dok := dst[key].(map[string]any)
		sm, sok := sv.(map[string]any)
		if dok && sok {
			if err := l.prepareMerge(dm, sm, overwrite, depth+1, touched); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *lazyTemplate) snapshotMaps() (map[uintptr]map[string]any, error) {
	before := map[uintptr]map[string]any{}
	for pointer, m := range l.mapRefs {
		if err := l.mapWork(len(m)); err != nil {
			return nil, err
		}
		copy := map[string]any{}
		for key, value := range m {
			copy[key] = value
		}
		before[pointer] = copy
	}
	return before, nil
}

func sameMapEntry(a, b any) bool {
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	if a == nil {
		return true
	}
	x, y := reflect.ValueOf(a), reflect.ValueOf(b)
	switch x.Kind() {
	case reflect.Map, reflect.Ptr, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return x.Pointer() == y.Pointer()
	case reflect.Slice:
		return x.Pointer() == y.Pointer() && x.Len() == y.Len() && x.Cap() == y.Cap()
	default:
		return reflect.DeepEqual(a, b)
	}
}

func (l *lazyTemplate) captureMapEdits(before map[uintptr]map[string]any) error {
	for pointer, m := range l.mapRefs {
		if err := l.mapWork(len(m) + len(before[pointer])); err != nil {
			return err
		}
		for key, old := range before[pointer] {
			value, exists := m[key]
			if !exists || !sameMapEntry(old, value) || l.edited(m, key) {
				l.recordEdit(m, key, !exists)
			}
		}
		for key := range m {
			if _, existed := before[pointer][key]; !existed {
				l.recordEdit(m, key, false)
			}
		}
	}
	return nil
}

// Mergo's map recursion is not a general cyclic-container sandbox. Check shape
// without resolving values before entering it; repeated aliases are permitted.
func (l *lazyTemplate) mergeGraph(value any, seen map[lazyMapVisit]uint8, depth int) error {
	if depth > 256 {
		return ErrLimit
	}
	if err := l.mapWork(1); err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Map, reflect.Slice:
		length := 0
		if v.Kind() == reflect.Slice {
			length = v.Len()
		}
		visit := lazyMapVisit{v.Type(), v.Pointer(), length}
		if seen[visit] == 1 {
			return ErrLimit
		}
		if seen[visit] == 2 {
			return nil
		}
		seen[visit] = 1
		defer func() { seen[visit] = 2 }()
	case reflect.Array:
	default:
		return nil
	}
	if err := l.mapWork(v.Len()); err != nil {
		return err
	}
	if v.Kind() == reflect.Map {
		iter := v.MapRange()
		for iter.Next() {
			if err := l.mergeGraph(iter.Value().Interface(), seen, depth+1); err != nil {
				return err
			}
		}
	} else {
		for i := 0; i < v.Len(); i++ {
			if err := l.mergeGraph(v.Index(i).Interface(), seen, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
