// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package conf

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

// PreferenceDocument persists only typed leaf fields belonging to one owner.
// Runtime authority is never serializable through this format; the original flat
// AppData remains the UI contract. nil parent objects contribute no leaf values.
type PreferenceDocument struct {
	Version int                        `json:"version"`
	Scope   Scope                      `json:"scope"`
	Values  map[string]json.RawMessage `json:"values"`
}

var ErrPreferences = errors.New("invalid_classified_preferences")

func preferenceFields() map[string]reflect.Type {
	ret := map[string]reflect.Type{}
	var walk func(reflect.Type, string)
	walk = func(t reflect.Type, prefix string) {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "" || key == "-" {
				continue
			}
			if prefix != "" {
				key = prefix + "." + key
			}
			child := f.Type
			for child.Kind() == reflect.Pointer {
				child = child.Elem()
			}
			if child.Kind() == reflect.Struct {
				walk(child, key)
			} else {
				ret[key] = f.Type
			}
		}
	}
	walk(reflect.TypeOf(AppData{}), "")
	return ret
}

func (d PreferenceDocument) Validate(scope Scope) error {
	if d.Version != 1 || d.Scope != scope || scope == RuntimeValue || (scope != DevicePreference && scope != StorePreference && scope != WorkspacePreference) || d.Values == nil {
		return ErrPreferences
	}
	fields := preferenceFields()
	for key, raw := range d.Values {
		t, ok := fields[key]
		if !ok {
			return ErrPreferences
		}
		owner, err := ScopeFor(key)
		if err != nil || owner != scope {
			return ErrPreferences
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && t.Kind() != reflect.Pointer && t.Kind() != reflect.Map && t.Kind() != reflect.Slice && t.Kind() != reflect.Interface {
			return ErrPreferences
		}
		if err = uniqueJSON(raw); err != nil {
			return ErrPreferences
		}
		if err = json.Unmarshal(raw, reflect.New(t).Interface()); err != nil {
			return ErrPreferences
		}
	}
	return nil
}

func DecodePreferences(raw []byte, scope Scope) (PreferenceDocument, error) {
	var d PreferenceDocument
	if len(raw) > 1<<20 || uniqueJSON(raw) != nil {
		return d, ErrPreferences
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return d, ErrPreferences
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return d, ErrPreferences
	}
	return d, d.Validate(scope)
}

func (d PreferenceDocument) Clone() (PreferenceDocument, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return PreferenceDocument{}, err
	}
	return DecodePreferences(raw, d.Scope)
}

func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 64 {
			return ErrPreferences
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return ErrPreferences
				}
				seen[name] = true
				if err = visit(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return ErrPreferences
			}
		case json.Delim('['):
			for d.More() {
				if err = visit(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return ErrPreferences
			}
		}
		return nil
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrPreferences
	}
	return nil
}

func (a AppData) Preferences(scope Scope) (PreferenceDocument, error) {
	d := PreferenceDocument{Version: 1, Scope: scope, Values: map[string]json.RawMessage{}}
	raw, err := json.Marshal(a)
	if err != nil {
		return d, err
	}
	var data map[string]json.RawMessage
	if err = json.Unmarshal(raw, &data); err != nil {
		return d, err
	}
	for key := range preferenceFields() {
		owner, err := ScopeFor(key)
		if err != nil {
			return d, err
		}
		if owner != scope {
			continue
		}
		parts := strings.Split(key, ".")
		current := data
		var value json.RawMessage
		ok := true
		for i, part := range parts {
			value, ok = current[part]
			if !ok {
				break
			}
			if i < len(parts)-1 {
				if bytes.Equal(value, []byte("null")) {
					ok = false
					break
				}
				current = nil
				if json.Unmarshal(value, &current) != nil {
					ok = false
					break
				}
			}
		}
		if ok {
			d.Values[key] = append(json.RawMessage(nil), value...)
		}
	}
	return d, d.Validate(scope)
}

// ApplyPreferences returns a detached complete original UI projection. Missing
// leaves inherit the explicit base; wrong-owner/unknown/runtime values fail.
func (a AppData) ApplyPreferences(d PreferenceDocument) (AppData, error) {
	if err := d.Validate(d.Scope); err != nil {
		return AppData{}, err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return AppData{}, err
	}
	var values map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&values); err != nil {
		return AppData{}, err
	}
	for key, value := range d.Values {
		parts := strings.Split(key, ".")
		target := values
		for _, part := range parts[:len(parts)-1] {
			child, ok := target[part].(map[string]interface{})
			if !ok {
				child = map[string]interface{}{}
				target[part] = child
			}
			target = child
		}
		// Keep original numeric bytes, including int64 values beyond 2^53.
		target[parts[len(parts)-1]] = json.RawMessage(value)
	}
	raw, err = json.Marshal(values)
	if err != nil {
		return AppData{}, err
	}
	var result AppData
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	// The runtime User view is intentionally not a preference field.
	clone, err := a.Clone()
	result.User = clone.User
	return result, err
}
