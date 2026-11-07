// SPDX-License-Identifier: AGPL-3.0-or-later
package docast

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/88250/lute/ast"
	av "wiki-kernel/internal/avmodel"
)

var ErrOpaqueEdit = errors.New("opaque_inline_extension_cannot_be_reassociated")

type rawObject map[string]json.RawMessage

// OriginalTreeInput hides forward-compatible non-string IAL values from the
// typed original parser. The source bytes remain the serializer preimage.
func OriginalTreeInput(raw []byte) ([]byte, error) {
	var visit func(json.RawMessage) (json.RawMessage, bool, error)
	visit = func(raw json.RawMessage) (json.RawMessage, bool, error) {
		var node rawObject
		if e := json.Unmarshal(raw, &node); e != nil {
			return nil, false, e
		}
		changed := false
		var props rawObject
		if node["Properties"] != nil {
			if e := json.Unmarshal(node["Properties"], &props); e != nil {
				return nil, false, e
			}
		}
		for name, value := range props {
			if !bytes.HasPrefix(bytes.TrimSpace(value), []byte(`"`)) {
				delete(props, name)
				changed = true
			}
		}
		if id := jsonString(node, "ID"); id != "" {
			if existing := jsonString(props, "id"); existing != "" && existing != id {
				return nil, false, ErrDocument
			}
			if props == nil {
				props = rawObject{}
			}
			if props["id"] == nil {
				props["id"], _ = json.Marshal(id)
				changed = true
			}
		}
		if changed && props != nil {
			node["Properties"], _ = json.Marshal(props)
		}
		var children []json.RawMessage
		if node["Children"] != nil {
			if e := json.Unmarshal(node["Children"], &children); e != nil {
				return nil, false, e
			}
		}
		childrenChanged := false
		for i, child := range children {
			v, change, e := visit(child)
			if e != nil {
				return nil, false, e
			}
			children[i] = v
			childrenChanged = childrenChanged || change
		}
		if childrenChanged {
			node["Children"], _ = json.Marshal(children)
			changed = true
		}
		if !changed {
			return raw, false, nil
		}
		v, e := json.Marshal(node)
		return v, true, e
	}
	v, _, e := visit(raw)
	return v, e
}

func formatLikeNext(merged, next []byte) ([]byte, error) {
	if !bytes.Contains(next, []byte("\n")) {
		return merged, nil
	}
	var b bytes.Buffer
	if e := json.Indent(&b, merged, "", "\t"); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}

func jsonString(o rawObject, key string) string {
	var s string
	_ = json.Unmarshal(o[key], &s)
	return s
}

func jsonFields(t reflect.Type) map[string]reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	ret := map[string]reflect.Type{}
	if t.Kind() != reflect.Struct {
		return ret
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" || f.PkgPath != "" {
			continue
		}
		if f.Anonymous && name == "" {
			for k, v := range jsonFields(f.Type) {
				ret[k] = v
			}
			continue
		}
		if name == "" {
			name = f.Name
		}
		ret[name] = f.Type
	}
	return ret
}

// SerializeDocument overlays only extensions on the original JSON renderer's
// result. IDs follow blocks across documents. Anonymous opaque nodes must keep
// an unambiguous position/content association, otherwise the edit fails closed.
type DocumentPreservation struct {
	// Originals are projections through the same renderer before any edit.
	Originals [][]byte
	// AnchorText contains verified dynamic changes by containing block and target ID.
	AnchorText map[[2]string]string
}

func SerializeDocument(before [][]byte, next []byte, preservation ...DocumentPreservation) ([]byte, error) {
	known := jsonFields(reflect.TypeOf(ast.Node{}))
	for _, n := range []string{"Properties", "Children", "Type", "ID"} {
		known[n] = reflect.TypeOf("")
	}
	type previousNode struct {
		object rawObject
		raw    json.RawMessage
	}
	byID := map[string]previousNode{}
	normalized := map[string]json.RawMessage{}
	var options DocumentPreservation
	if len(preservation) > 0 {
		options = preservation[0]
	}
	var collect func(json.RawMessage) error
	collect = func(raw json.RawMessage) error {
		var o rawObject
		if err := json.Unmarshal(raw, &o); err != nil {
			return err
		}
		if id := jsonString(o, "ID"); id != "" {
			if _, exists := byID[id]; exists {
				return errors.New("duplicate_original_node_id")
			}
			byID[id] = previousNode{object: o, raw: raw}
		}
		var children []json.RawMessage
		if raw := o["Children"]; raw != nil {
			if err := json.Unmarshal(raw, &children); err != nil {
				return err
			}
		}
		for _, child := range children {
			if err := collect(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, raw := range before {
		if err := collect(raw); err != nil {
			return nil, err
		}
	}
	var collectNormalized func(json.RawMessage) error
	collectNormalized = func(raw json.RawMessage) error {
		var node rawObject
		if err := json.Unmarshal(raw, &node); err != nil {
			return err
		}
		if id := jsonString(node, "ID"); id != "" && jsonString(node, "Type") != "NodeDocument" {
			normalized[id] = raw
		}
		var children []json.RawMessage
		_ = json.Unmarshal(node["Children"], &children)
		for _, child := range children {
			if err := collectNormalized(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, raw := range options.Originals {
		if err := collectNormalized(raw); err != nil {
			return nil, err
		}
	}
	hasOpaque := func(o rawObject) bool {
		for k := range o {
			if _, ok := known[k]; !ok {
				return true
			}
		}
		var props rawObject
		_ = json.Unmarshal(o["Properties"], &props)
		for _, v := range props {
			if !bytes.HasPrefix(bytes.TrimSpace(v), []byte(`"`)) {
				return true
			}
		}
		return false
	}
	var opaqueAnonymous func(rawObject) bool
	opaqueAnonymous = func(o rawObject) bool {
		if jsonString(o, "ID") != "" {
			return false
		}
		if hasOpaque(o) {
			return true
		}
		var children []rawObject
		_ = json.Unmarshal(o["Children"], &children)
		for _, child := range children {
			if opaqueAnonymous(child) {
				return true
			}
		}
		return false
	}
	var merge func(json.RawMessage, rawObject, string) (json.RawMessage, error)
	merge = func(raw json.RawMessage, previous rawObject, owner string) (json.RawMessage, error) {
		var o rawObject
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, err
		}
		if id := jsonString(o, "ID"); id != "" {
			owner = id
			prior := byID[id]
			previous = prior.object
			// An untouched named subtree keeps its original wire representation.
			// The original renderer normalizes projection-only differences such
			// as absent id IAL, so compare two outputs of the same encoder.
			if original := normalized[id]; original != nil && prior.raw != nil && bytes.Equal(raw, original) {
				return prior.raw, nil
			}
		}
		if jsonString(o, "ID") == "" && previous != nil && opaqueAnonymous(previous) {
			for k := range known {
				if k == "Children" {
					continue
				}
				if k == "TextMarkTextContent" && jsonString(previous, "TextMarkBlockRefSubtype") == "d" {
					target := jsonString(previous, "TextMarkBlockRefID")
					if value, ok := options.AnchorText[[2]string{owner, target}]; ok && target != "" && value == jsonString(o, k) {
						continue
					}
				}
				var old, new any
				_ = json.Unmarshal(previous[k], &old)
				_ = json.Unmarshal(o[k], &new)
				if !reflect.DeepEqual(old, new) {
					return nil, ErrOpaqueEdit
				}
			}
		}
		for k, v := range previous {
			if _, exists := known[k]; !exists {
				o[k] = v
			}
		}
		var previousProps, currentProps rawObject
		_ = json.Unmarshal(previous["Properties"], &previousProps)
		_ = json.Unmarshal(o["Properties"], &currentProps)
		for k, v := range previousProps {
			if !bytes.HasPrefix(bytes.TrimSpace(v), []byte(`"`)) && currentProps[k] == nil {
				if currentProps == nil {
					currentProps = rawObject{}
				}
				currentProps[k] = v
			}
		}
		if currentProps != nil {
			o["Properties"], _ = json.Marshal(currentProps)
		}
		var children, oldChildren []json.RawMessage
		_ = json.Unmarshal(o["Children"], &children)
		_ = json.Unmarshal(previous["Children"], &oldChildren)
		for i, raw := range oldChildren {
			var old rawObject
			_ = json.Unmarshal(raw, &old)
			if !opaqueAnonymous(old) {
				continue
			}
			if len(children) != len(oldChildren) {
				return nil, ErrOpaqueEdit
			}
			var new rawObject
			_ = json.Unmarshal(children[i], &new)
			if jsonString(new, "ID") != "" {
				return nil, ErrOpaqueEdit
			}
		}
		for i, child := range children {
			var old rawObject
			if i < len(oldChildren) {
				_ = json.Unmarshal(oldChildren[i], &old)
				if jsonString(old, "ID") != "" {
					old = nil
				}
			}
			var err error
			children[i], err = merge(child, old, owner)
			if err != nil {
				return nil, err
			}
		}
		if _, exists := o["Children"]; exists {
			o["Children"], _ = json.Marshal(children)
		}
		return json.Marshal(o)
	}
	out, e := merge(next, nil, "")
	if e != nil {
		return nil, e
	}
	return formatLikeNext(out, next)
}

// SerializeAttributeView preserves unknown nested fields while respecting
// deletion/omitempty of all known original fields. Known fields use the exact
// original AV model schema, including embedded layout fields.
func SerializeAttributeView(before, next []byte) ([]byte, error) {
	out, e := mergeSchemaJSON(before, next, reflect.TypeOf(av.AttributeView{}))
	if e != nil {
		return nil, e
	}
	return formatLikeNext(out, next)
}

func mergeSchemaJSON(before, next []byte, t reflect.Type) ([]byte, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(next), []byte("null")) || len(next) == 0 {
		return next, nil
	}
	if t.Kind() == reflect.Struct {
		var old, current rawObject
		if json.Unmarshal(next, &current) != nil || json.Unmarshal(before, &old) != nil {
			return next, nil
		}
		fields := jsonFields(t)
		for k, v := range old {
			if _, known := fields[k]; !known {
				current[k] = v
			}
		}
		for k, v := range current {
			if typ, known := fields[k]; known && old[k] != nil {
				merged, err := mergeSchemaJSON(old[k], v, typ)
				if err != nil {
					return nil, err
				}
				current[k] = merged
			}
		}
		return json.Marshal(current)
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		var old, current []json.RawMessage
		if json.Unmarshal(next, &current) != nil || json.Unmarshal(before, &old) != nil {
			return next, nil
		}
		elem := t.Elem()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		identity := func(raw json.RawMessage) (id string, stable bool) {
			var o rawObject
			_ = json.Unmarshal(raw, &o)
			for _, k := range []string{"id", "ID"} {
				if id := jsonString(o, k); id != "" {
					return k + ":" + id, true
				}
			}
			var key rawObject
			_ = json.Unmarshal(o["key"], &key)
			if id := jsonString(key, "id"); id != "" {
				return "key:" + id, true
			}
			switch elem {
			case reflect.TypeOf(av.SelectOption{}):
				id = jsonString(o, "name")
			case reflect.TypeOf(av.ValueSelect{}):
				id = jsonString(o, "content")
			case reflect.TypeOf(av.ViewSort{}):
				id = jsonString(o, "column")
			}
			return id, false
		}
		byID := map[string]int{}
		duplicate := map[string]bool{}
		used := map[int]bool{}
		for i, raw := range old {
			if id, _ := identity(raw); id != "" {
				if _, exists := byID[id]; exists {
					duplicate[id] = true
				}
				byID[id] = i
			}
		}
		for i, raw := range current {
			var previous json.RawMessage
			if id, _ := identity(raw); id != "" && !duplicate[id] {
				if index, ok := byID[id]; ok && !used[index] {
					previous = old[index]
					used[index] = true
				}
			} else if len(current) == len(old) {
				oldID, _ := identity(old[i])
				if oldID == id && !used[i] && (!schemaHasExtensions(old[i], elem) || sameKnownSchema(old[i], raw, elem)) {
					previous = old[i]
					used[i] = true
				}
			}
			if previous != nil {
				merged, err := mergeSchemaJSON(previous, raw, elem)
				if err != nil {
					return nil, err
				}
				current[i] = merged
			}
		}
		for i, raw := range old {
			if _, stable := identity(raw); !used[i] && !stable && schemaHasExtensions(raw, elem) {
				return nil, ErrOpaqueEdit
			}
		}
		return json.Marshal(current)
	}
	return next, nil
}

// An anonymous element has no durable identity. Require its typed projection
// to agree before transferring extensions by position.
func sameKnownSchema(a, b []byte, t reflect.Type) bool {
	lhs, rhs := reflect.New(t).Interface(), reflect.New(t).Interface()
	return json.Unmarshal(a, lhs) == nil && json.Unmarshal(b, rhs) == nil && reflect.DeepEqual(lhs, rhs)
}

func schemaHasExtensions(raw []byte, t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		var obj rawObject
		if json.Unmarshal(raw, &obj) != nil {
			return false
		}
		fields := jsonFields(t)
		for k, v := range obj {
			typ, ok := fields[k]
			if !ok || schemaHasExtensions(v, typ) {
				return true
			}
		}
	case reflect.Array, reflect.Slice:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return false
		}
		for _, value := range values {
			if schemaHasExtensions(value, t.Elem()) {
				return true
			}
		}
	}
	return false
}
