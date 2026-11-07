package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// Cache a particular source reference, not fabricated projected strings or an
// AST lent by another owner. Ordinal distinguishes same-block reference nodes.
func (l *Lease) CacheNativeRef(ctx context.Context, box, doc, block string, ordinal int) error {
	if !documentID.MatchString(box) || !documentID.MatchString(doc) || !documentID.MatchString(block) || ordinal < 0 {
		return ErrDocument
	}
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if err := r.rebuildBlockTree(ctx, w); err != nil {
				return err
			}
		}
		tree, err := r.originalTree(ctx, w, box, doc)
		if err != nil {
			return err
		}
		var target *ast.Node
		count := 0
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter || !((n.Type == ast.NodeTextMark && n.IsTextMarkType("block-ref")) || n.Type == ast.NodeBlockRef) {
				return ast.WalkContinue
			}
			p := n.Parent
			for p != nil && !p.IsBlock() {
				p = p.Parent
			}
			if p != nil && p.ID == block {
				if count == ordinal {
					target = n
					return ast.WalkStop
				}
				count++
			}
			return ast.WalkContinue
		})
		if target == nil {
			return ErrDocument
		}
		options, err := r.configuredParserOptions(ctx, l.workspace)
		if err != nil {
			return err
		}
		ref, err := indexdb.ProjectReference(ctx, tree, target, nativeparser.NewConfiguredIndex(options), func(ctx context.Context, id string) (indexdb.BlockLocation, bool, error) {
			return r.nativeDefinition(ctx, id)
		})
		if err != nil {
			return err
		}
		return r.nativeRefs.Put(ref)
	})
}
func (l *Lease) NativeRefsCacheByDefinition(ctx context.Context, id string) (ret []indexdb.RefRecord, err error) {
	err = l.WithNativeReferences(ctx, func(q indexdb.References) error {
		r := l.workspace.resources
		ret = r.nativeRefs.Get(id)
		if len(ret) > 0 {
			return nil
		}
		var err error
		ret, err = q.ByDefinition(ctx, id, false)
		if err != nil {
			return err
		}
		for _, ref := range ret {
			if err = r.nativeRefs.Put(ref); err != nil {
				// This cache is optional. Never leave a partial successful query
				// cached (the original hit path treats that group as authoritative).
				r.nativeRefs.Delete(id)
				return nil
			}
		}
		return nil
	})
	return
}
func (l *Lease) RemoveNativeRefsCache(ctx context.Context, id string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { l.workspace.resources.nativeRefs.Delete(id); return nil })
}

func (l *Lease) NativeRefText(ctx context.Context, id string) (text string, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r := l.workspace.resources
		options, e := r.configuredParserOptions(ctx, l.workspace)
		if e != nil {
			return e
		}
		text, e = indexdb.ReferenceText(ctx, id, nativeparser.NewConfiguredIndex(options), r.nativeBlock)
		return e
	})
	return
}
