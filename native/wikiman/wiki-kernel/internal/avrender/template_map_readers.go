// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"reflect"
	"sort"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	av "wiki-kernel/internal/avmodel"
)

// Keep only the pinned, original functions, not a second public function map.
// A caller override is opaque even if it has the same name and signature.
var originalMapReaders = func() template.FuncMap {
	all, selected := sprig.TxtFuncMap(), template.FuncMap{}
	for _, name := range []string{"get", "hasKey", "empty", "default", "dig", "pluck", "pick", "omit", "keys", "values"} {
		selected[name] = all[name]
	}
	return selected
}()

func originalMapReader(name string, fn any) bool {
	original := originalMapReaders[name]
	return original != nil && fn != nil && reflect.TypeOf(fn) == reflect.TypeOf(original) && reflect.ValueOf(fn).Pointer() == reflect.ValueOf(original).Pointer()
}

func (l *lazyTemplate) bindMapReaders(funcs template.FuncMap, safe map[string]bool) {
	for name, fn := range l.env.Funcs {
		if !originalMapReader(name, fn) {
			continue
		}
		safe[name] = true
		switch name {
		case "get":
			original := fn.(func(map[string]any, string) any)
			funcs[name] = func(m map[string]any, key string) (any, error) {
				if _, err := l.prepare(m, key); err != nil {
					return nil, err
				}
				return original(m, key), nil
			}
		case "hasKey":
			original := fn.(func(map[string]any, string) bool)
			funcs[name] = func(m map[string]any, key string) (bool, error) {
				if err := l.preparePresence(m, key); err != nil {
					return false, err
				}
				return original(m, key), nil
			}
		case "dig":
			original := fn.(func(...any) (any, error))
			funcs[name] = func(args ...any) (any, error) {
				// Invalid arguments still reach the original validation/panic,
				// recovered by Go template, before any dependency is requested.
				if len(args) < 3 {
					return original(args...)
				}
				m, valid := args[len(args)-1].(map[string]any)
				if !valid {
					return original(args...)
				}
				keys := make([]string, len(args)-2)
				for i := range keys {
					key, ok := args[i].(string)
					if !ok {
						return original(args...)
					}
					keys[i] = key
				}
				for i, key := range keys {
					if _, err := l.prepare(m, key); err != nil {
						return nil, err
					}
					value, exists := m[key]
					if !exists || i == len(keys)-1 {
						break
					}
					m, valid = value.(map[string]any)
					if !valid {
						break
					}
				}
				return original(args...)
			}
		case "pluck":
			original := fn.(func(string, ...map[string]any) []any)
			funcs[name] = func(key string, maps ...map[string]any) ([]any, error) {
				for _, m := range maps {
					if _, err := l.prepare(m, key); err != nil {
						return nil, err
					}
				}
				return original(key, maps...), nil
			}
		case "pick":
			original := fn.(func(map[string]any, ...string) map[string]any)
			funcs[name] = func(m map[string]any, keys ...string) (map[string]any, error) {
				for _, key := range keys {
					if _, err := l.prepare(m, key); err != nil {
						return nil, err
					}
				}
				return original(m, keys...), nil
			}
		case "omit":
			original := fn.(func(map[string]any, ...string) map[string]any)
			funcs[name] = func(m map[string]any, keys ...string) (map[string]any, error) {
				excluded := map[string]bool{}
				for _, key := range keys {
					excluded[key] = true
				}
				if err := l.prepareMembershipExcept(m, excluded); err != nil {
					return nil, err
				}
				for _, key := range sortedMapKeys(m) {
					if !excluded[key] {
						if _, err := l.prepare(m, key); err != nil {
							return nil, err
						}
					}
				}
				return original(m, keys...), nil
			}
		case "keys":
			original := fn.(func(...map[string]any) []string)
			funcs[name] = func(maps ...map[string]any) ([]string, error) {
				for _, m := range maps {
					if err := l.prepareMembership(m); err != nil {
						return nil, err
					}
				}
				return original(maps...), nil // Preserve original unsorted/duplicate keys.
			}
		case "values":
			original := fn.(func(map[string]any) []any)
			funcs[name] = func(m map[string]any) ([]any, error) {
				if err := l.prepareMembership(m); err != nil {
					return nil, err
				}
				for _, key := range sortedMapKeys(m) {
					if _, err := l.prepare(m, key); err != nil {
						return nil, err
					}
				}
				return original(m), nil // Preserve original result type and order policy.
			}
		}
	}
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (l *lazyTemplate) mapWork(n int) error {
	if l.env.Context != nil {
		if err := l.env.Context.Err(); err != nil {
			return err
		}
	}
	if l.env.Charge != nil {
		return l.env.Charge(n)
	}
	return nil
}

// Presence is not a value read. Template/raw fields always exist, while optional
// computed relation/rollup display names may appear only after calculation.
// Stop at a definite writer without evaluating its value, preserving shadowing.
func (l *lazyTemplate) preparePresence(m map[string]any, key string) error {
	if err := l.mapWork(1); err != nil {
		return err
	}
	pointer := reflect.ValueOf(m).Pointer()
	if l.edited(m, key) {
		return nil
	}
	if pointer != reflect.ValueOf(l.model).Pointer() {
		return nil
	}
	deps := l.maps[pointer][key]
	for i := len(deps) - 1; i >= 0; i-- {
		id := deps[i]
		kv := l.byID[id]
		if key == kv.Key.Name && (kv.Key.Type == av.KeyTypeRelation || kv.Key.Type == av.KeyTypeRollup) {
			if err := l.resolve(id); err != nil {
				return err
			}
			if err := l.refresh(); err != nil {
				return err
			}
		}
		if l.writes(id, key) {
			break
		}
	}
	return nil
}

func (l *lazyTemplate) prepareMembership(m map[string]any) error {
	return l.prepareMembershipExcept(m, nil)
}

func (l *lazyTemplate) prepareMembershipExcept(m map[string]any, excluded map[string]bool) error {
	if err := l.mapWork(len(m)); err != nil {
		return err
	}
	pointer := reflect.ValueOf(m).Pointer()
	if pointer != reflect.ValueOf(l.model).Pointer() {
		return nil
	}
	for _, key := range sortedDependencyNames(l.maps[pointer]) {
		if excluded[key] {
			continue
		}
		if err := l.preparePresence(m, key); err != nil {
			return err
		}
	}
	return nil
}

func sortedDependencyNames(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type lazyMapVisit struct {
	kind    reflect.Type
	pointer uintptr
	length  int
}

// Read functions can return ordinary maps/slices containing a registered map.
// Traverse transparent containers before output/opaque consumers, with a local
// cycle guard and work budget. Do not inspect structs or change returned types.
func (l *lazyTemplate) materialize(value any, seen map[lazyMapVisit]uint8, depth int) (any, error) {
	if depth > 256 {
		return nil, ErrLimit
	}
	if err := l.mapWork(1); err != nil {
		return nil, err
	}
	if value == nil {
		return value, nil
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
			return nil, ErrLimit
		} // Cannot safely print cyclic containers.
		if seen[visit] == 2 {
			return value, nil
		}
		seen[visit] = 1
		defer func() { seen[visit] = 2 }()
	case reflect.Array:
	default:
		return value, nil
	}
	if m, ok := value.(map[string]any); ok && l.maps[reflect.ValueOf(m).Pointer()] != nil {
		if err := l.prepareMembership(m); err != nil {
			return nil, err
		}
		for _, key := range sortedMapKeys(m) {
			if _, err := l.prepare(m, key); err != nil {
				return nil, err
			}
		}
	}
	if err := l.mapWork(v.Len()); err != nil {
		return nil, err
	}
	if v.Kind() == reflect.Map {
		// Snapshot values: resolving a nested registered map refreshes the root.
		children := make([]any, 0, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			children = append(children, iter.Value().Interface())
		}
		for _, child := range children {
			if _, err := l.materialize(child, seen, depth+1); err != nil {
				return nil, err
			}
		}
	} else {
		for i := 0; i < v.Len(); i++ {
			if _, err := l.materialize(v.Index(i).Interface(), seen, depth+1); err != nil {
				return nil, err
			}
		}
	}
	return value, nil
}
