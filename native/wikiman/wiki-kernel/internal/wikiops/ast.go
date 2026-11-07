package wikiops

import (
	"context"
	"encoding/json"
	"strings"
	"wiki-kernel/internal/docast"
)

type astNode struct {
	raw      json.RawMessage
	fields   map[string]json.RawMessage
	children []*astNode
	dirty    bool
}

func parseNode(raw json.RawMessage) (*astNode, error) {
	n := &astNode{raw: raw}
	if json.Unmarshal(raw, &n.fields) != nil || n.fields == nil {
		return nil, ErrArgument
	}
	var children []json.RawMessage
	if value, ok := n.fields["Children"]; ok {
		if json.Unmarshal(value, &children) != nil {
			return nil, ErrArgument
		}
	}
	for _, raw := range children {
		child, err := parseNode(raw)
		if err != nil {
			return nil, err
		}
		n.children = append(n.children, child)
	}
	return n, nil
}
func (n *astNode) id() string { var id string; _ = json.Unmarshal(n.fields["ID"], &id); return id }
func (n *astNode) find(id string) (*astNode, *astNode, int) {
	if n.id() == id {
		return n, nil, -1
	}
	for i, c := range n.children {
		if c.id() == id {
			return c, n, i
		}
		if child, parent, index := c.find(id); child != nil {
			return child, parent, index
		}
	}
	return nil, nil, -1
}
func (n *astNode) encode() (json.RawMessage, bool, error) {
	changed := n.dirty
	children := make([]json.RawMessage, len(n.children))
	for i, c := range n.children {
		raw, dirty, err := c.encode()
		if err != nil {
			return nil, false, err
		}
		children[i] = raw
		changed = changed || dirty
	}
	if !changed {
		return n.raw, false, nil
	}
	if n.dirty || len(n.children) > 0 {
		raw, err := json.Marshal(children)
		if err != nil {
			return nil, false, err
		}
		n.fields["Children"] = raw
	}
	raw, err := json.Marshal(n.fields)
	return raw, true, err
}
func mutateBlock(ctx context.Context, raw []byte, r ContentRequest) ([]byte, error) {
	parser := docast.Parser{}
	if r.Action == "block.create" || r.Action == "block.replace" {
		if _, _, err := parser.Parse(ctx, r.AST, r.BlockID); err != nil {
			return nil, err
		}
	}
	if _, _, err := parser.Parse(ctx, raw, r.DocID); err != nil {
		return nil, err
	}
	root, err := parseNode(raw)
	if err != nil {
		return nil, err
	}
	current, parent, index := root.find(r.BlockID)
	if r.BlockID == r.DocID {
		return nil, ErrArgument
	}
	switch r.Action {
	case "block.replace":
		if current == nil {
			return nil, ErrArgument
		}
		next, err := parseNode(r.AST)
		if err != nil || next.id() != r.BlockID {
			return nil, ErrArgument
		}
		parent.children[index] = next
		parent.dirty = true
	case "block.delete", "block.move":
		if current == nil || parent == nil {
			return nil, ErrArgument
		}
		if r.Action == "block.move" {
			if child, _, _ := current.find(r.ParentID); child != nil {
				return nil, ErrArgument
			}
		}
		parent.children = append(parent.children[:index], parent.children[index+1:]...)
		parent.dirty = true
	case "block.create":
		if current != nil {
			return nil, ErrArgument
		}
		current, err = parseNode(r.AST)
		if err != nil || current.id() != r.BlockID {
			return nil, ErrArgument
		}
	default:
		return nil, ErrArgument
	}
	if r.Action == "block.create" || r.Action == "block.move" {
		destination, _, _ := root.find(r.ParentID)
		if destination == nil {
			return nil, ErrArgument
		}
		insert := len(destination.children)
		if r.BeforeID != "" {
			insert = -1
			for i, c := range destination.children {
				if c.id() == r.BeforeID {
					insert = i
					break
				}
			}
			if insert < 0 {
				return nil, ErrArgument
			}
		}
		destination.children = append(destination.children, nil)
		copy(destination.children[insert+1:], destination.children[insert:])
		destination.children[insert] = current
		destination.dirty = true
	}
	next, _, err := root.encode()
	if err != nil {
		return nil, err
	}
	if _, _, err := parser.Parse(ctx, next, r.DocID); err != nil {
		return nil, err
	}
	return next, nil
}

func moveBlockAcrossDocuments(ctx context.Context, source, target []byte, r ContentRequest) ([]byte, []byte, error) {
	parser := docast.Parser{}
	if _, _, err := parser.Parse(ctx, source, r.DocID); err != nil {
		return nil, nil, err
	}
	if _, _, err := parser.Parse(ctx, target, r.TargetDocID); err != nil {
		return nil, nil, err
	}
	a, err := parseNode(source)
	if err != nil {
		return nil, nil, err
	}
	b, err := parseNode(target)
	if err != nil {
		return nil, nil, err
	}
	current, parent, index := a.find(r.BlockID)
	destination, _, _ := b.find(r.ParentID)
	if current == nil || parent == nil || destination == nil {
		return nil, nil, ErrArgument
	}
	insert := len(destination.children)
	if r.BeforeID != "" {
		insert = -1
		for i, node := range destination.children {
			if node.id() == r.BeforeID {
				insert = i
				break
			}
		}
		if insert < 0 {
			return nil, nil, ErrArgument
		}
	}
	parent.children = append(parent.children[:index], parent.children[index+1:]...)
	parent.dirty = true
	destination.children = append(destination.children, nil)
	copy(destination.children[insert+1:], destination.children[insert:])
	destination.children[insert] = current
	destination.dirty = true
	x, _, err := a.encode()
	if err != nil {
		return nil, nil, err
	}
	y, _, err := b.encode()
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := parser.Parse(ctx, x, r.DocID); err != nil {
		return nil, nil, err
	}
	if _, _, err := parser.Parse(ctx, y, r.TargetDocID); err != nil {
		return nil, nil, err
	}
	return x, y, nil
}

// Plain text is a read projection only, never a source-format conversion. AST
// reads retain every opaque node. Unknown textual encodings fail explicitly.
func PlainText(ctx context.Context, raw json.RawMessage) (string, error) {
	var walk func(json.RawMessage, int) (string, error)
	walk = func(raw json.RawMessage, depth int) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if depth > 512 {
			return "", ErrArgument
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return "", ErrArgument
		}
		text := ""
		for _, key := range []string{"Data", "TextMarkTextContent"} {
			if value, ok := fields[key]; ok {
				var s string
				if json.Unmarshal(value, &s) != nil {
					return "", ErrArgument
				}
				if text == "" {
					text = s
				}
			}
		}
		var children []json.RawMessage
		if value, ok := fields["Children"]; ok && json.Unmarshal(value, &children) != nil {
			return "", ErrArgument
		}
		for _, child := range children {
			s, err := walk(child, depth+1)
			if err != nil {
				return "", err
			}
			text += s
		}
		var kind string
		_ = json.Unmarshal(fields["Type"], &kind)
		switch kind {
		case "NodeParagraph", "NodeHeading", "NodeCodeBlock", "NodeListItem", "NodeBlockquote":
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
		}
		return text, nil
	}
	return walk(raw, 0)
}
