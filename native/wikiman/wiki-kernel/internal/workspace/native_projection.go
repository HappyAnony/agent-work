package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"strings"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// These are internal derived-index operations, not document-save handlers.
// Source bytes, configured parser and dependent queries are captured under one
// identity's admission/gate; the queue owns values, never a borrowed parse.Tree.
func (l *Lease) QueueNativeReferences(ctx context.Context, box, doc string) error {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) {
		return ErrDocument
	}
	return l.enqueueNativeMain(ctx, func(r *resources) error {
		if !r.blockTreeReady {
			if err := r.rebuildBlockTree(ctx, l.workspace); err != nil {
				return err
			}
		}
		tree, err := r.originalTree(ctx, l.workspace, box, doc)
		if err != nil {
			return err
		}
		settings, err := r.configuredParserOptions(ctx, l.workspace)
		if err != nil {
			return err
		}
		refs, annotations, err := indexdb.ProjectReferences(ctx, tree, nativeparser.NewConfiguredIndex(settings), func(ctx context.Context, id string) (location indexdb.BlockLocation, found bool, err error) {
			return r.nativeDefinition(ctx, id)
		})
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		return r.contentQueue.Enqueue(indexdb.MainMutation{Action: "update_refs", ID: doc, Box: box, Path: tree.Path, Refs: refs, Annotations: annotations})
	})
}

// The document is explicit: equal block IDs in another page/document cannot
// select a global tree. Missing OCR/AV services fail only when actually used.
func (l *Lease) QueueNativeIndexNode(ctx context.Context, box, doc, id string) error {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) || !documentID.MatchString(id) {
		return ErrDocument
	}
	return l.enqueueNativeMain(ctx, func(r *resources) error { return r.queueNativeIndexNode(ctx, l.workspace, box, doc, id) })
}

func (r *resources) queueNativeIndexNode(ctx context.Context, w *WorkspaceContext, box, doc, id string) error {
	tree, err := r.originalTree(ctx, w, box, doc)
	if err != nil {
		return err
	}
	var node *ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering && n.ID == id {
			node = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if node == nil {
		return ErrDocument
	}
	content, err := indexdb.StaticContent(ctx, node, nil, true, r.configuration.Search != nil && r.configuration.Search.IndexAssetPath, true, r.nativeContent)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	content = strings.ReplaceAll(content, editor.Zwsp, "")
	return r.contentQueue.Enqueue(indexdb.MainMutation{Action: "index_node", ID: id, Content: content})
}

func (r *resources) originalTree(ctx context.Context, w *WorkspaceContext, box, doc string) (*parse.Tree, error) {
	cached, err := r.load(ctx, w, box, doc)
	if err != nil {
		return nil, err
	}
	return r.parseOriginalTree(ctx, w, cached, box, doc)
}

func (r *resources) parseOriginalTree(ctx context.Context, w *WorkspaceContext, cached cachedDocument, box, doc string) (*parse.Tree, error) {
	options, err := r.configuredParserOptions(ctx, w)
	if err != nil {
		return nil, err
	}
	parser := nativeparser.NewConfiguredIndex(options)
	// Derived reads must not synthesize empty paragraphs or migrate old Spec.
	input, err := docast.OriginalTreeInput(cached.raw)
	if err != nil {
		return nil, err
	}
	tree, err := dataparser.ParseJSONWithoutFix(input, parser.ParseOptions)
	if err != nil {
		return nil, err
	}
	if tree == nil || tree.Root == nil {
		return nil, ErrDocument
	}
	tree.ID, tree.Box, tree.Path = doc, box, cached.path
	return tree, ctx.Err()
}
