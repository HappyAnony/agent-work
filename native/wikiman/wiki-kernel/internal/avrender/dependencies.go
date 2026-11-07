// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"errors"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
	av "wiki-kernel/internal/avmodel"
)

var ErrDynamicDependency = errors.New("AV template dependency cannot be determined statically")

// FieldDependencies is a conservative static inspection utility, not the native
// FieldResolver execution scheduler (which uses runtime template access hooks).
// It follows original field-name/ID/raw access, including control
// branches. Dynamic whole-model/map access is rejected, never evaluated using
// unresolved raw formulas or silently treated as a zero-dependency expression.
func FieldDependencies(a *av.AttributeView, s string, env TemplateEnvironment) ([]*av.Key, error) {
	if env.Funcs == nil {
		return nil, ErrEnvironmentUnbound
	}
	t, e := template.New("").Delims(".action{", "}").Funcs(env.Funcs).Parse(s)
	if e != nil {
		return nil, e
	}
	keys := []*av.Key{}
	seen := map[string]bool{}
	add := func(name string, id bool) {
		var hit *av.Key
		for _, kv := range a.KeyValues {
			if (id && kv.Key.ID == name) || (!id && (kv.Key.Name == name || kv.Key.Name+"_raw" == name || kv.Key.Name+"_str" == name || kv.Key.Name+"_end" == name || kv.Key.Name+"_created" == name)) {
				hit = kv.Key
			}
		}
		if hit != nil && !seen[hit.ID] {
			seen[hit.ID] = true
			keys = append(keys, hit)
		}
	}
	visiting := map[string]bool{}
	localDepth := 0
	var walk func(parse.Node) error
	walk = func(node parse.Node) error {
		if node == nil {
			return nil
		}
		switch n := node.(type) {
		case *parse.ListNode:
			if n == nil {
				return nil
			}
			for _, x := range n.Nodes {
				if e := walk(x); e != nil {
					return e
				}
			}
		case *parse.ActionNode:
			return walk(n.Pipe)
		case *parse.PipeNode:
			if n == nil {
				return nil
			}
			for _, x := range n.Cmds {
				if e := walk(x); e != nil {
					return e
				}
			}
		case *parse.CommandNode:
			// Literal index/get selectors cover the original name and id_mod contracts.
			if len(n.Args) >= 3 {
				if f, ok := n.Args[0].(*parse.IdentifierNode); ok && (f.Ident == "index" || f.Ident == "get") {
					_, root := n.Args[1].(*parse.DotNode)
					root = root && localDepth == 0
					m, field := n.Args[1].(*parse.FieldNode)
					id := localDepth == 0 && field && len(m.Ident) == 1 && (m.Ident[0] == "id_mod" || m.Ident[0] == "id_mod_raw")
					if v, ok := n.Args[1].(*parse.VariableNode); ok && len(v.Ident) > 0 && v.Ident[0] == "$" {
						root = len(v.Ident) == 1
						id = len(v.Ident) == 2 && (v.Ident[1] == "id_mod" || v.Ident[1] == "id_mod_raw")
					}
					if root || id {
						v, ok := n.Args[2].(*parse.StringNode)
						if !ok {
							return ErrDynamicDependency
						}
						add(v.Text, id)
						for _, x := range n.Args[3:] {
							if e := walk(x); e != nil {
								return e
							}
						}
						return nil
					}
				}
			}
			for _, x := range n.Args {
				if e := walk(x); e != nil {
					return e
				}
			}
		case *parse.FieldNode:
			if localDepth > 0 {
				return nil
			}
			if len(n.Ident) > 0 {
				if n.Ident[0] == "id_mod" || n.Ident[0] == "id_mod_raw" {
					if len(n.Ident) < 2 {
						return ErrDynamicDependency
					}
					add(n.Ident[1], true)
				} else {
					add(n.Ident[0], false)
				}
			}
		case *parse.ChainNode:
			if e := walk(n.Node); e != nil {
				return e
			}
		case *parse.DotNode:
			if localDepth > 0 {
				return nil
			}
			return ErrDynamicDependency
		case *parse.VariableNode:
			if len(n.Ident) > 1 && n.Ident[0] == "$" {
				if n.Ident[1] == "id_mod" || n.Ident[1] == "id_mod_raw" {
					if len(n.Ident) < 3 {
						return ErrDynamicDependency
					}
					add(n.Ident[2], true)
				} else {
					add(n.Ident[1], false)
				}
			}
		case *parse.IfNode:
			if e := walk(n.Pipe); e != nil {
				return e
			}
			if e := walk(n.List); e != nil {
				return e
			}
			return walk(n.ElseList)
		case *parse.RangeNode:
			if e := walk(n.Pipe); e != nil {
				return e
			}
			localDepth++
			err := walk(n.List)
			localDepth--
			if err != nil {
				return err
			}
			return walk(n.ElseList)
		case *parse.WithNode:
			if e := walk(n.Pipe); e != nil {
				return e
			}
			localDepth++
			err := walk(n.List)
			localDepth--
			if err != nil {
				return err
			}
			return walk(n.ElseList)
		case *parse.TemplateNode:
			if visiting[n.Name] {
				return fmt.Errorf("%w: template %s", ErrTemplateCycle, n.Name)
			}
			child := t.Lookup(n.Name)
			if child == nil {
				return fmt.Errorf("undefined template %s", n.Name)
			}
			visiting[n.Name] = true
			defer delete(visiting, n.Name)
			if n.Pipe != nil && n.Pipe.String() != "." {
				if e := walk(n.Pipe); e != nil {
					return e
				}
			}
			return walk(child.Tree.Root)
		}
		return nil
	}
	if e = walk(t.Tree.Root); e != nil {
		return nil, e
	}
	return keys, nil
}

type FieldRef struct{ AV, Field, Row string }

func (f FieldRef) String() string { return f.AV + "/" + f.Field + "/" + f.Row }

var ErrFieldCycle = errors.New("AV field dependency cycle")

type FieldCycleError struct{ Path []FieldRef }

func (e *FieldCycleError) Error() string {
	p := []string{}
	for _, f := range e.Path {
		p = append(p, f.String())
	}
	return ErrFieldCycle.Error() + ": " + strings.Join(p, " -> ")
}
func (e *FieldCycleError) Unwrap() error { return ErrFieldCycle }
