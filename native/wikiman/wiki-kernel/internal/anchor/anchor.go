// SPDX-License-Identifier: AGPL-3.0-or-later
// Original treenode/node.go and model transaction/blockinfo/push_reload rules.
package anchor

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"strings"
	"sync"
	"unicode/utf8"
	"wiki-kernel/internal/indexdb"
)

var ErrDependency = errors.New("dynamic_anchor_dependency_unbound")

type Cache struct {
	mu     sync.Mutex
	values map[string]string
	Limit  int
	closed bool
}

func (c *Cache) Load(key interface{}) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[key.(string)]
	return v, ok
}
func (c *Cache) Store(key, value interface{}) { _ = c.Put(key.(string), value.(string)) }
func (c *Cache) Put(key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrDependency
	}
	if c.values == nil {
		c.values = map[string]string{}
	}
	if c.Limit > 0 && len(c.values) >= c.Limit {
		if _, ok := c.values[key]; !ok {
			return ErrDependency
		}
	}
	c.values[key] = value
	return nil
}
func (c *Cache) Delete(key interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, key.(string))
}
func (c *Cache) Range(fn func(interface{}, interface{}) bool) {
	c.mu.Lock()
	copy := map[string]string{}
	for k, v := range c.values {
		copy[k] = v
	}
	c.mu.Unlock()
	for k, v := range copy {
		if !fn(k, v) {
			break
		}
	}
}
func (c *Cache) Clear() { c.mu.Lock(); defer c.mu.Unlock(); clear(c.values) }
func (c *Cache) Close() { c.mu.Lock(); defer c.mu.Unlock(); c.closed = true; clear(c.values) }
func IsRef(n *ast.Node) bool {
	return n != nil && ((n.Type == ast.NodeTextMark && n.IsTextMarkType("block-ref")) || n.Type == ast.NodeBlockRef)
}
func SetText(n *ast.Node, text string, store func(string, string) error) error {
	if !IsRef(n) {
		return nil
	}
	if n.Type == ast.NodeBlockRef {
		if id := n.ChildByType(ast.NodeBlockRefID); id != nil {
			id.InsertAfter(&ast.Node{Type: ast.NodeBlockRefDynamicText, Tokens: []byte(text)})
			id.InsertAfter(&ast.Node{Type: ast.NodeBlockRefSpace})
		}
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = n.TextMarkBlockRefID
	}
	if store == nil {
		return ErrDependency
	}
	if err := store(n.TextMarkBlockRefID, text); err != nil {
		return err
	}
	n.TextMarkBlockRefSubtype = "d"
	n.TextMarkTextContent = text
	return nil
}
func FirstLeaf(n *ast.Node) (ret *ast.Node) {
	ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
		if !enter || n.IsMarker() {
			return ast.WalkContinue
		}
		if !n.IsContainerBlock() {
			ret = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return
}

// Text uses the original static-content flags and image fallback, not Markdown
// flattening. AV titles must be supplied explicitly, never loaded globally.
func Text(ctx context.Context, n *ast.Node, max int, services indexdb.ContentServices) (string, error) {
	if n == nil {
		return "", nil
	}
	if name := n.IALAttr("name"); name != "" {
		return html.EscapeString(strings.TrimSpace(name)), nil
	}
	switch n.Type {
	case ast.NodeBlockQueryEmbed:
		return "Query Embed Block...", nil
	case ast.NodeIFrame:
		return "IFrame...", nil
	case ast.NodeThematicBreak:
		return "Thematic Break...", nil
	case ast.NodeVideo:
		return "Video...", nil
	case ast.NodeAudio:
		return "Audio...", nil
	case ast.NodeAttributeView:
		if services.AttributeView == nil {
			return "", ErrDependency
		}
		s, e := services.AttributeView(ctx, n.AttributeViewID, false)
		if e != nil {
			return "", e
		}
		if s == "" {
			s = "Database..."
		}
		return s, nil
	}
	if n.Type != ast.NodeDocument && n.IsContainerBlock() {
		n = FirstLeaf(n)
	}
	s, err := indexdb.StaticContent(ctx, n, nil, false, false, false, services)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = html.EscapeString(html.UnescapeString(s))
	s = strings.TrimSpace(s)
	if s == "" && n != nil {
		var b strings.Builder
		ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.Type == ast.NodeImage {
				if title := n.ChildByType(ast.NodeLinkTitle); title != nil {
					b.Write(title.Tokens)
				} else if alt := n.ChildByType(ast.NodeLinkText); alt != nil && len(alt.Tokens) > 0 {
					b.Write(alt.Tokens)
				} else {
					b.WriteString("image")
				}
			}
			return ast.WalkContinue
		})
		s = b.String()
	}
	if max < 0 {
		return "", ErrDependency
	}
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max]) + "..."
	}
	return s, ctx.Err()
}

type Change struct{ RootID, BlockID, DefinitionID, Text, Type string }
type Services struct {
	References func(context.Context, *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error)
	Load       func(context.Context, string) (*parse.Tree, error)
	Text       func(context.Context, *ast.Node) (string, error)
	Set        func(*ast.Node, string) error
	Embedded   func(*ast.Node) string
	Notify     func(context.Context, Change) error
	IndexRefs  func(context.Context, *parse.Tree) error
	UpdateAVs  func(context.Context, map[string]*ast.Node) error
	Stage      func(context.Context, *parse.Tree) error
	StrictLoad bool
}

func Update(ctx context.Context, node *ast.Node, defs map[string]*ast.Node, s Services) (changed bool, changes []Change, err error) {
	if s.Text == nil || s.Set == nil || s.Embedded == nil {
		return false, nil, ErrDependency
	}
	ast.Walk(node, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = ctx.Err(); err != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		if IsRef(n) {
			id, text, sub := n.TextMarkBlockRefID, n.TextMarkTextContent, n.TextMarkBlockRefSubtype
			if id == "" {
				return ast.WalkContinue
			}
			def := defs[id]
			if def == nil {
				return ast.WalkSkipChildren
			}
			changed = true
			if sub == "d" {
				text, err = s.Text(ctx, def)
				if err != nil {
					return ast.WalkStop
				}
				text = strings.TrimSpace(text)
				if text == "" {
					text = id
				}
				if err = s.Set(n, text); err != nil {
					return ast.WalkStop
				}
			}
			changes = append(changes, Change{DefinitionID: id, Text: text, Type: "ref-" + sub})
		} else if id := s.Embedded(n); id != "" {
			changed = true
			changes = append(changes, Change{DefinitionID: id, Type: "embed"})
		}
		return ast.WalkContinue
	})
	return
}

// Refresh retains the original seven-round propagation and first changed block
// per tree scheduling. Source staging/AV effects and notification timing belong
// to the caller; native planning never calls the original source writer.
func Refresh(ctx context.Context, defs map[string]*ast.Node, trees map[string]*parse.Tree, s Services) error {
	if s.References == nil || s.Load == nil || s.Notify == nil || s.IndexRefs == nil || s.UpdateAVs == nil || s.Stage == nil {
		return ErrDependency
	}
	for round := 0; round < 7; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		nextNodes, nextTrees := map[string]*ast.Node{}, map[string]*parse.Tree{}
		groups := map[string]map[string]bool{}
		var changedNodes []*ast.Node
		for _, n := range defs {
			refs, children, err := s.References(ctx, n)
			if err != nil {
				return err
			}
			changedNodes = children // Original loop retains the last definition's child expansion.
			for _, r := range refs {
				if groups[r.RootID] == nil {
					groups[r.RootID] = map[string]bool{}
				}
				groups[r.RootID][r.BlockID] = true
			}
		}
		for _, n := range changedNodes {
			defs[n.ID] = n
		}
		staged := map[string]*parse.Tree{}
		for root, ids := range groups {
			tree := trees[root]
			if tree == nil {
				var err error
				tree, err = s.Load(ctx, root)
				if err != nil {
					if s.StrictLoad {
						return err
					}
					continue
				}
			}
			if tree == nil || tree.Root == nil {
				return ErrDependency
			}
			changedTree := false
			var walkErr error
			ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if walkErr = ctx.Err(); walkErr != nil {
					return ast.WalkStop
				}
				if !enter || !n.IsBlock() || !ids[n.ID] {
					return ast.WalkContinue
				}
				changed, changes, err := Update(ctx, n, defs, s)
				if err != nil {
					walkErr = err
					return ast.WalkStop
				}
				if !changedTree && changed {
					changedTree = true
					nextNodes[n.ID] = n
					nextTrees[root] = tree
				}
				for _, c := range changes {
					if c.Type == "ref-d" {
						c.RootID = root
						c.BlockID = n.ID
						if walkErr = s.Notify(ctx, c); walkErr != nil {
							return ast.WalkStop
						}
					}
				}
				return ast.WalkContinue
			})
			if walkErr != nil {
				return walkErr
			}
			if changedTree {
				staged[root] = tree
				if err := s.IndexRefs(ctx, tree); err != nil {
					return err
				}
			}
		}
		if err := s.UpdateAVs(ctx, defs); err != nil {
			return err
		}
		for _, tree := range staged {
			if err := s.Stage(ctx, tree); err != nil {
				return err
			}
		}
		if len(nextNodes) == 0 {
			break
		}
		defs, trees = nextNodes, nextTrees
	}
	return nil
}
