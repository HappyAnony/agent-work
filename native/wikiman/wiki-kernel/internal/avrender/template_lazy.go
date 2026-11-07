// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"fmt"
	"io"
	"reflect"
	"text/template"
	"text/template/parse"

	av "wiki-kernel/internal/avmodel"
)

// The original evaluator still owns execution, scopes, methods and short circuit
// semantics. Instrument only map reads, never evaluate a branch in a dry run.
// Maps retain their identity so aliases captured before a dependency is resolved
// see the refreshed typed values. All state belongs to this single evaluation.
type lazyTemplate struct {
	model     map[string]any
	ial       map[string]string
	kvs       []*av.KeyValues
	env       TemplateEnvironment
	maps      map[uintptr]map[string][]string
	byID      map[string]*av.KeyValues
	idMap     uintptr
	rawMap    uintptr
	mapRefs   map[uintptr]map[string]any
	edits     map[uintptr]map[string]templateMapEdit
	rawCopies map[string]*av.Value
	resolved  map[string]bool
	rawErr    error
}

const lazyPrepare = "__wikiPrepareField"
const lazyWhole = "__wikiMaterializeFields"

func bindLazyTemplate(tpl *template.Template, model map[string]any, ial map[string]string, kvs []*av.KeyValues, env TemplateEnvironment) error {
	for _, name := range []string{lazyPrepare, lazyWhole} {
		if _, exists := env.Funcs[name]; exists {
			return fmt.Errorf("reserved template function %s", name)
		}
	}
	l := &lazyTemplate{model: model, ial: ial, kvs: kvs, env: env, maps: map[uintptr]map[string][]string{}}
	l.byID = map[string]*av.KeyValues{}
	names, ids, raw := map[string][]string{}, map[string][]string{}, map[string][]string{}
	for _, kv := range kvs {
		k := kv.Key
		l.byID[k.ID] = kv
		// Multiple writers are retained in source order, including duplicate names.
		for _, name := range []string{k.Name, k.Name + "_raw"} {
			names[name] = append(names[name], k.ID)
		}
		switch k.Type {
		case av.KeyTypeDate:
			names[k.Name+"_end"] = append(names[k.Name+"_end"], k.ID)
		case av.KeyTypeMSelect:
			names[k.Name+"_str"] = append(names[k.Name+"_str"], k.ID)
		case av.KeyTypeBlock:
			for _, name := range []string{k.Name + "_created", "entryCreated", "entryUpdated"} {
				names[name] = append(names[name], k.ID)
			}
		}
		ids[k.ID] = append([]string(nil), names[k.Name]...)
		raw[k.ID] = []string{k.ID}
	}
	l.maps[reflect.ValueOf(model).Pointer()] = names
	l.maps[reflect.ValueOf(model["id_mod"]).Pointer()] = ids
	l.maps[reflect.ValueOf(model["id_mod_raw"]).Pointer()] = raw
	l.idMap = reflect.ValueOf(model["id_mod"]).Pointer()
	l.rawMap = reflect.ValueOf(model["id_mod_raw"]).Pointer()
	l.mapRefs = map[uintptr]map[string]any{
		reflect.ValueOf(model).Pointer(): model,
		l.idMap:                          model["id_mod"].(map[string]any),
		l.rawMap:                         model["id_mod_raw"].(map[string]any),
	}
	l.edits = map[uintptr]map[string]templateMapEdit{}
	l.rawCopies = map[string]*av.Value{}
	l.resolved = map[string]bool{}
	l.env.RawValue = l.localRawValue
	if err := l.refresh(); err != nil {
		return err
	}
	funcs := template.FuncMap{lazyPrepare: l.prepare, lazyWhole: l.whole}
	safe := map[string]bool{"len": true, "and": true, "or": true, "not": true}
	for _, name := range []string{"len", "and", "or", "not"} {
		if _, overridden := env.Funcs[name]; overridden {
			delete(safe, name)
		}
	}
	// Honor caller overrides: native maps use the original built-in index and
	// Sprig get, while an explicitly supplied function keeps its original handler.
	if _, overridden := env.Funcs["index"]; !overridden {
		funcs["index"] = l.index
		safe["index"] = true
	}
	l.bindMapReaders(funcs, safe)
	l.bindMapMutators(funcs, safe)
	tpl.Funcs(funcs)
	for _, t := range tpl.Templates() {
		rewriteLazyList(t.Tree.Root, safe)
	}
	return nil
}

func (l *lazyTemplate) refresh() error {
	if l.env.Charge != nil {
		if err := l.env.Charge(len(l.kvs)); err != nil {
			return err
		}
	}
	next := templateDataModel(l.ial, l.kvs, l.env)
	if l.rawErr != nil {
		return l.rawErr
	}
	for _, name := range []string{"id_mod", "id_mod_raw"} {
		pointer := l.idMap
		if name == "id_mod_raw" {
			pointer = l.rawMap
		}
		old := l.mapRefs[pointer]
		l.refreshMap(old, next[name].(map[string]any))
		next[name] = old
	}
	l.refreshMap(l.model, next)
	return nil
}

// A later optional numeric/date/relation/rollup assignment may be absent. Resolve
// writers backwards until the original model would actually assign this name;
// overwritten fields are not dependencies, even when they contain a cycle.
func (l *lazyTemplate) writes(id, property string) bool {
	kv := l.byID[id]
	if kv == nil || len(kv.Values) == 0 {
		return false
	}
	k, v := kv.Key, kv.Values[0]
	if property == k.Name+"_raw" {
		return true
	}
	if property != k.Name {
		switch k.Type {
		case av.KeyTypeDate:
			return property == k.Name+"_end" && v.Date != nil && v.Date.IsNotEmpty2
		case av.KeyTypeMSelect:
			return property == k.Name+"_str"
		case av.KeyTypeBlock:
			return property == k.Name+"_created" || property == "entryUpdated" || property == "entryCreated" && v.Block != nil
		}
		return false
	}
	switch k.Type {
	case av.KeyTypeNumber:
		return v.Number != nil && v.Number.IsNotEmpty
	case av.KeyTypeDate:
		return v.Date != nil && v.Date.IsNotEmpty
	case av.KeyTypeRelation:
		return v.Relation != nil && len(v.Relation.Contents) > 0
	case av.KeyTypeRollup:
		return v.Rollup != nil && len(v.Rollup.Contents) > 0
	}
	return true
}

func (l *lazyTemplate) prepare(base any, path ...string) (any, error) {
	if l.env.Context != nil {
		if err := l.env.Context.Err(); err != nil {
			return nil, err
		}
	}
	current := base
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			break
		} // Original evaluator handles struct fields and methods.
		pointer := reflect.ValueOf(m).Pointer()
		if deps := l.maps[pointer]; deps != nil && !l.edited(m, key) {
			property := key
			if pointer == l.idMap {
				if kv := l.byID[key]; kv != nil {
					property = kv.Key.Name
				}
			}
			for i := len(deps[key]) - 1; i >= 0; i-- {
				id := deps[key][i]
				if err := l.resolve(id); err != nil {
					return nil, err
				}
				if pointer == l.rawMap || l.writes(id, property) {
					break
				}
			}
			if len(deps[key]) > 0 {
				if err := l.refresh(); err != nil {
					return nil, err
				}
			}
		}
		current = m[key]
	}
	return base, nil
}

// Whole-model consumers must never observe unresolved formula source. Such a
// read includes every member (and therefore legitimately detects self cycles).
// Binding/passing a map and checking its length/truthiness do not read members.
func (l *lazyTemplate) whole(value any) (any, error) {
	return l.materialize(value, map[lazyMapVisit]uint8{}, 0)
}

// Delegate ordinary index semantics (integer kinds, missing map keys, bounds,
// pointer/interface unwrapping) to Go's original built-in rather than clone it.
var templateIndex = template.Must(template.New("index").Funcs(template.FuncMap{
	"capture": func(any) string { return "" },
}).Parse(`{{capture (index .Base .Key)}}`))

func (l *lazyTemplate) index(base any, keys ...any) (any, error) {
	for _, key := range keys {
		if s, ok := key.(string); ok {
			if _, err := l.prepare(base, s); err != nil {
				return nil, err
			}
		}
		var result any
		t, err := templateIndex.Clone()
		if err != nil {
			return nil, err
		}
		t.Funcs(template.FuncMap{"capture": func(v any) string { result = v; return "" }})
		if err = t.Execute(io.Discard, struct{ Base, Key any }{base, key}); err != nil {
			return nil, err
		}
		base = result
	}
	return base, nil
}

func lazyCall(name string, args ...parse.Node) *parse.PipeNode {
	return &parse.PipeNode{NodeType: parse.NodePipe, Cmds: []*parse.CommandNode{{NodeType: parse.NodeCommand, Args: append([]parse.Node{parse.NewIdentifier(name)}, args...)}}}
}

func lazyAccess(base parse.Node, fields []string) parse.Node {
	args := []parse.Node{base}
	for _, field := range fields {
		args = append(args, &parse.StringNode{NodeType: parse.NodeString, Text: field, Quoted: fmt.Sprintf("%q", field)})
	}
	return &parse.ChainNode{NodeType: parse.NodeChain, Node: lazyCall(lazyPrepare, args...), Field: fields}
}

func rewriteLazyExpr(n parse.Node, safe map[string]bool) parse.Node {
	switch v := n.(type) {
	case *parse.FieldNode:
		return lazyAccess(&parse.DotNode{NodeType: parse.NodeDot}, v.Ident)
	case *parse.VariableNode:
		if len(v.Ident) > 1 {
			return lazyAccess(&parse.VariableNode{NodeType: parse.NodeVariable, Ident: v.Ident[:1]}, v.Ident[1:])
		}
	case *parse.ChainNode:
		return lazyAccess(rewriteLazyExpr(v.Node, safe), v.Field)
	case *parse.PipeNode:
		rewriteLazyPipe(v, safe)
	}
	return n
}

func rewriteLazyPipe(p *parse.PipeNode, safe map[string]bool) {
	if p == nil {
		return
	}
	var commands []*parse.CommandNode
	for i, cmd := range p.Cmds {
		name := ""
		if id, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
			name = id.Ident
		}
		// Map selectors, scope/truthiness helpers and aliases must remain lazy.
		lazy := safe[name]
		if i > 0 && !lazy {
			commands = append(commands, lazyCall(lazyWhole).Cmds[0])
		}
		for j, arg := range cmd.Args {
			cmd.Args[j] = rewriteLazyExpr(arg, safe)
			if name != "" && j > 0 && !lazy {
				cmd.Args[j] = lazyCall(lazyWhole, cmd.Args[j])
			}
		}
		commands = append(commands, cmd)
	}
	p.Cmds = commands
}

func rewriteLazyList(list *parse.ListNode, safe map[string]bool) {
	if list == nil {
		return
	}
	for _, node := range list.Nodes {
		switch n := node.(type) {
		case *parse.ActionNode:
			rewriteLazyPipe(n.Pipe, safe)
			if len(n.Pipe.Decl) == 0 {
				n.Pipe.Cmds = append(n.Pipe.Cmds, lazyCall(lazyWhole).Cmds[0])
			}
		case *parse.IfNode:
			rewriteLazyPipe(n.Pipe, safe)
			rewriteLazyList(n.List, safe)
			rewriteLazyList(n.ElseList, safe)
		case *parse.WithNode:
			rewriteLazyPipe(n.Pipe, safe)
			rewriteLazyList(n.List, safe)
			rewriteLazyList(n.ElseList, safe)
		case *parse.RangeNode:
			rewriteLazyPipe(n.Pipe, safe)
			n.Pipe.Cmds = append(n.Pipe.Cmds, lazyCall(lazyWhole).Cmds[0])
			rewriteLazyList(n.List, safe)
			rewriteLazyList(n.ElseList, safe)
		case *parse.TemplateNode:
			rewriteLazyPipe(n.Pipe, safe)
		}
	}
}
