package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/render"
	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/docast"
)

var ErrRichText = errors.New("richtext_operation_unsupported")

// Protyle's DOM operations are converted here, never to Markdown or a separate
// source model. All operations in a batch become one CAS/journal transaction.
type ProtyleOperation struct {
	Action     string          `json:"action"`
	ID         string          `json:"id"`
	Data       json.RawMessage `json:"data,omitempty"`
	ParentID   string          `json:"parentID,omitempty"`
	PreviousID string          `json:"previousID,omitempty"`
	NextID     string          `json:"nextID,omitempty"`
}

func richEngine() *lute.Lute {
	engine := lute.New()
	engine.SetProtyleWYSIWYG(true)
	engine.SetTextMark(true)
	engine.SetBlockRef(true)
	engine.SetKramdownIAL(true)
	engine.SetSuperBlock(true)
	engine.SetTag(true)
	engine.SetSanitize(true)
	return engine
}

// Unknown node fields stay in the source. Non-string Properties are projected
// out ONLY for the renderer, whose upstream map is string-valued.
func renderProjection(node *astNode) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	for key, value := range node.fields {
		fields[key] = value
	}
	if props, ok := fields["Properties"]; ok {
		var values map[string]json.RawMessage
		if json.Unmarshal(props, &values) != nil {
			return nil, ErrArgument
		}
		for key, value := range values {
			var text string
			if json.Unmarshal(value, &text) != nil {
				delete(values, key)
			}
		}
		fields["Properties"], _ = json.Marshal(values)
	}
	if node.id() != "" {
		values := map[string]json.RawMessage{}
		_ = json.Unmarshal(fields["Properties"], &values)
		var existing string
		_ = json.Unmarshal(values["id"], &existing)
		if existing != "" && existing != node.id() {
			return nil, ErrRichText
		}
		values["id"], _ = json.Marshal(node.id())
		fields["Properties"], _ = json.Marshal(values)
	}
	if _, ok := fields["Children"]; ok {
		children := make([]json.RawMessage, 0, len(node.children))
		for _, child := range node.children {
			raw, err := renderProjection(child)
			if err != nil {
				return nil, err
			}
			children = append(children, raw)
		}
		fields["Children"], _ = json.Marshal(children)
	}
	return json.Marshal(fields)
}
func RenderDocument(ctx context.Context, raw []byte, id string) (html string, err error) {
	return renderDocumentProjection(ctx, raw, id, "editor")
}
func renderDocumentProjection(ctx context.Context, raw []byte, id string, mode string) (html string, err error) {
	defer func() {
		if recover() != nil {
			html = ""
			err = ErrRichText
		}
	}()
	parser := docast.Parser{}
	if _, _, err = parser.Parse(ctx, raw, id); err != nil {
		return
	}
	var root *astNode
	root, err = parseNode(raw)
	if err != nil {
		return
	}
	var projection []byte
	projection, err = renderProjection(root)
	if err != nil {
		return
	}
	engine := richEngine()
	tree, e := dataparser.ParseJSONWithoutFix(projection, engine.ParseOptions)
	if e != nil {
		return "", ErrRichText
	}
	if mode == "docx" {
		engine.RenderOptions.ProtyleContenteditable = false
		return string(render.NewProtyleExportDocxRenderer(tree, engine.RenderOptions).Render()), nil
	}
	if mode == "html" {
		engine.RenderOptions.ProtyleContenteditable = false
		return string(render.NewProtyleExportRenderer(tree, engine.RenderOptions).Render()), nil
	}
	if mode == "htmlmd" {
		markdown, err := lute.ProtyleExportMdNodeSync(tree.Root, engine.ParseOptions, engine.RenderOptions)
		if err != nil {
			return "", err
		}
		return engine.MarkdownStr("", markdown), nil
	}
	return engine.Tree2BlockDOM(tree, engine.RenderOptions), nil
}
func domBlock(html, id string) (node *astNode, err error) {
	defer func() {
		if recover() != nil {
			node = nil
			err = ErrRichText
		}
	}()
	if len(html) > 4<<20 || strings.Contains(html, "\x00") {
		return nil, ErrArgument
	}
	engine := richEngine()
	// Protyle's <wbr> is a transient selection marker, not document text.
	// Strip only that marker; a literal user-authored caret is still content.
	html = strings.NewReplacer("<wbr>", "", "<wbr/>", "", "<wbr />", "").Replace(html)
	tree := engine.BlockDOM2Tree(html)
	raw := render.NewJSONRenderer(tree, engine.RenderOptions).Render()
	root, err := parseNode(raw)
	if err != nil || len(root.children) != 1 {
		return nil, ErrRichText
	}
	if root.children[0].id() != id {
		return nil, ErrArgument
	}
	return root.children[0], nil
}
func nodeType(n *astNode) string {
	var kind string
	_ = json.Unmarshal(n.fields["Type"], &kind)
	return kind
}
func nativeFields() map[string]bool {
	known := map[string]bool{}
	typeOf := reflect.TypeOf(ast.Node{})
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = field.Name
		}
		known[tag] = true
	}
	return known
}

// Protyle's inverse insert is HTML, not a native snapshot. A delete of hidden
// fields would succeed, but undo would silently recreate a lossy block. Keep
// that operation unsupported unless the caller requests bounded native history.
// Explicit CLI block.delete/document.put are separate native operations.
func opaqueForHTMLUndo(node *astNode, known map[string]bool, cache map[*astNode]bool) (opaque bool) {
	if value, ok := cache[node]; ok {
		return value
	}
	defer func() { cache[node] = opaque }()
	for key := range node.fields {
		if !known[key] {
			return true
		}
	}
	if value, ok := node.fields["Properties"]; ok {
		var properties map[string]json.RawMessage
		if json.Unmarshal(value, &properties) != nil {
			return true
		}
		for key, value := range properties {
			// Only the identity/timestamp round-trip is guaranteed for removed
			// blocks. Other attributes may be filtered by the controlled DOM.
			if key != "id" && key != "updated" {
				return true
			}
			var text string
			if json.Unmarshal(value, &text) != nil {
				return true
			}
		}
	}
	for _, child := range node.children {
		if opaqueForHTMLUndo(child, known, cache) {
			return true
		}
	}
	return false
}

// Merge converted nodes by stable block ID (or same-position/type inline).
// Bounded native history permits removal of identified subtrees: their hidden
// fields are restored from exact snapshots, never from lossy inverse HTML.
// History cannot resolve anonymous-node identity during a forward merge; those
// ambiguous edits must still fail instead of shifting metadata to a neighbour.
func preserveNative(old, next *astNode, nativeHistory bool) error {
	known := nativeFields()
	// Structural updates can remove a whole subtree. Memoize inspections so
	// checking each removed stable ID does not rescan all of its descendants.
	opaqueCache := map[*astNode]bool{}
	oldByID := map[string]*astNode{}
	var collect func(*astNode)
	collect = func(n *astNode) {
		if n.id() != "" {
			oldByID[n.id()] = n
		}
		for _, c := range n.children {
			collect(c)
		}
	}
	collect(old)
	seen := map[string]bool{}
	var merge func(*astNode, *astNode) error
	merge = func(prior, current *astNode) error {
		if current.id() != "" {
			prior = oldByID[current.id()]
			seen[current.id()] = true
		}
		if prior != nil {
			// Position is not identity for anonymous rows/inline nodes. If a
			// structural edit changes their count, do not attach hidden fields
			// from a removed row to its shifted neighbour by same-position merge.
			if len(prior.children) != len(current.children) {
				for _, child := range prior.children {
					if child.id() == "" && opaqueForHTMLUndo(child, known, opaqueCache) {
						return ErrRichText
					}
				}
			}
			for key, value := range prior.fields {
				if !known[key] {
					current.fields[key] = value
				}
			}
			// Attributes not represented in HTML remain native metadata. Explicit
			// attribute removal must use setAttrs, not an incidental DOM projection.
			var props, updates map[string]json.RawMessage
			_ = json.Unmarshal(prior.fields["Properties"], &props)
			_ = json.Unmarshal(current.fields["Properties"], &updates)
			if props == nil {
				props = map[string]json.RawMessage{}
			}
			for key, value := range updates {
				props[key] = value
			}
			if len(props) > 0 {
				current.fields["Properties"], _ = json.Marshal(props)
			}
		}
		for i, child := range current.children {
			var match *astNode
			if prior != nil && i < len(prior.children) && child.id() == "" && prior.children[i].id() == "" && nodeType(child) == nodeType(prior.children[i]) {
				match = prior.children[i]
			}
			if err := merge(match, child); err != nil {
				return err
			}
		}
		if prior != nil {
			for i, child := range prior.children {
				if child.id() == "" {
					matched := i < len(current.children) && current.children[i].id() == "" && nodeType(child) == nodeType(current.children[i])
					if !matched && opaqueForHTMLUndo(child, known, opaqueCache) {
						return ErrRichText
					}
				}
			}
		}
		current.dirty = true
		return nil
	}
	if err := merge(old, next); err != nil {
		return err
	}
	for id, prior := range oldByID {
		if !nativeHistory && !seen[id] && opaqueForHTMLUndo(prior, known, opaqueCache) {
			return ErrRichText
		}
	}
	return nil
}
