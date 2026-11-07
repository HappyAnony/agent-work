package docast

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

// A span refers to original source bytes, never a lossy AST reserialization.
var ErrDocument = errors.New("invalid_or_missing_document")
var documentID = regexp.MustCompile(`^\d{14}-[a-z0-9]{7}$`)

type Span struct {
	ID         string
	Start, End int
}
type Reference struct {
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
}
type parsedNode struct {
	id, target string
	kind       string
	data       json.RawMessage
	parent     *parsedNode
	start, end int
}
type Parser struct {
	nodes []*parsedNode
	ids   map[string]bool
}

func (p *Parser) Parse(ctx context.Context, data []byte, expectedID string) ([]Span, []Reference, error) {
	p.nodes = nil
	p.ids = map[string]bool{}
	defer func() { p.nodes = nil; p.ids = nil }()
	d := json.NewDecoder(bytes.NewReader(data))
	var node func(*parsedNode, int) error
	node = func(parent *parsedNode, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 512 || len(p.nodes) >= 100000 {
			return ErrDocument
		}
		start := int(d.InputOffset())
		for start < len(data) && (data[start] == ' ' || data[start] == '\n' || data[start] == '\r' || data[start] == '\t' || data[start] == ',' || data[start] == ':') {
			start++
		}
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return ErrDocument
		}
		n := &parsedNode{parent: parent, start: start}
		p.nodes = append(p.nodes, n)
		keys := map[string]bool{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return ErrDocument
			}
			key, ok := keyToken.(string)
			if !ok || keys[key] {
				return ErrDocument
			}
			keys[key] = true
			switch key {
			case "Type":
				if d.Decode(&n.kind) != nil {
					return ErrDocument
				}
			case "Data":
				if d.Decode(&n.data) != nil {
					return ErrDocument
				}
			case "ID":
				if d.Decode(&n.id) != nil || (n.id != "" && (!documentID.MatchString(n.id) || p.ids[n.id])) {
					return ErrDocument
				}
				if n.id != "" {
					p.ids[n.id] = true
				}
			case "TextMarkBlockRefID":
				if d.Decode(&n.target) != nil || (n.target != "" && !documentID.MatchString(n.target)) {
					return ErrDocument
				}
			case "Children":
				token, err := d.Token()
				if err != nil {
					return ErrDocument
				}
				if token == nil {
					continue
				}
				if token != json.Delim('[') {
					return ErrDocument
				}
				for d.More() {
					if err := node(n, depth+1); err != nil {
						return err
					}
				}
				if token, err = d.Token(); err != nil || token != json.Delim(']') {
					return ErrDocument
				}
			default:
				var raw json.RawMessage
				if d.Decode(&raw) != nil {
					return ErrDocument
				}
			}
		}
		if token, err = d.Token(); err != nil || token != json.Delim('}') {
			return ErrDocument
		}
		n.end = int(d.InputOffset())
		return nil
	}
	if err := node(nil, 0); err != nil {
		return nil, nil, err
	}
	if len(p.nodes) == 0 || p.nodes[0].id != expectedID {
		return nil, nil, ErrDocument
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, nil, ErrDocument
	}
	blocks := make([]Span, 0, len(p.ids))
	refs := []Reference{}
	for _, n := range p.nodes {
		if n.kind == "NodeBlockRefID" && n.parent != nil && n.parent.kind == "NodeBlockRef" {
			if json.Unmarshal(n.data, &n.target) != nil || !documentID.MatchString(n.target) {
				return nil, nil, ErrDocument
			}
		}
		if n.id != "" {
			blocks = append(blocks, Span{n.id, n.start, n.end})
		}
		if n.target != "" {
			for owner := n; owner != nil; owner = owner.parent {
				if owner.id != "" {
					refs = append(refs, Reference{owner.id, n.target})
					break
				}
			}
		}
	}
	return blocks, refs, nil
}
