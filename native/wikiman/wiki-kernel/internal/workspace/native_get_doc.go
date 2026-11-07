package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"maps"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// NativeGetDoc flushes only this owner's accepted index work, then reads trees,
// index/config/AV and writes local recent metadata under ONE source admission
// and serial gate. Every tree is a detached original-format parse. No nested
// Lease calls, global Conf/cache/DB, source repair or new worker are used.
func (l *Lease) NativeGetDoc(ctx context.Context, q docview.Request) (ret docview.Result, err error) {
	q.QueryTypes = maps.Clone(q.QueryTypes)
	q.OriginalRefs = maps.Clone(q.OriginalRefs)
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		return docview.Result{}, err
	}
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Editor == nil || r.configuration.Search == nil {
			return conf.ErrBusinessUnbound
		}
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		content := r.nativeContent
		// The renderer/anchor reads the real AV name in this Workspace; indexing's
		// optional full-AV provider is not a substitute for source identity checks.
		content.AttributeView = func(ctx context.Context, id string, _ bool) (string, error) {
			name, _, e := r.readNativeAV(ctx, w, id)
			return name, e
		}
		editorConf, searchConf := *r.configuration.Editor, *r.configuration.Search
		options := docview.VirtualOptions{Enabled: editorConf.VirtualBlockRef, CaseSensitive: searchConf.CaseSensitive, Include: editorConf.VirtualBlockRefInclude, Exclude: editorConf.VirtualBlockRefExclude}
		text := func(ctx context.Context, n *ast.Node) (string, error) {
			return anchor.Text(ctx, n, editorConf.BlockRefDynamicAnchorTextMaxLen, content)
		}
		s := &docview.Services{Ctx: ctx, CaseSensitive: searchConf.CaseSensitive, DynamicLoadBlocks: editorConf.DynamicLoadBlocks, BlockMissing: ErrDocument, TreeMissing: ErrDocument,
			Wait: func(ctx context.Context) error { return ctx.Err() },
			Load: func(ctx context.Context, id string) (*parse.Tree, error) { return r.nativeTreeByBlock(ctx, w, id) },
			Parser: func(ctx context.Context) (*lute.Lute, error) {
				opts, e := r.configuredParserOptions(ctx, w)
				if e != nil {
					return nil, e
				}
				return nativeparser.NewEditor(opts), ctx.Err()
			},
			Backlink: func(ctx context.Context, n *ast.Node, refs map[string]string) ([]*ast.Node, bool, error) {
				nodes, expand := docview.BacklinkNodes(n, refs)
				return nodes, expand, ctx.Err()
			},
			References: func(ctx context.Context, id string) (map[string]int, error) {
				return (indexdb.References{DB: db}).RootChildrenCounts(ctx, id)
			},
			Keywords: func(ctx context.Context, query string, types map[string]bool, method int, root string) ([]string, error) {
				return indexdb.DocumentKeywords(ctx, db, searchConf, query, types, method, root)
			},
			AvNames: func(ctx context.Context, ids string) (string, error) {
				var names []string
				for _, id := range strings.Split(ids, ",") {
					name, _, e := r.readNativeAV(ctx, w, id)
					if e != nil {
						return "", e
					}
					if name == "" {
						if r.configurationBusiness == nil {
							return "", conf.ErrBusinessUnbound
						}
						name = r.configurationBusiness.Language(r.configuration.Lang, 105, false, r.configuration.CloudRegion)
						if name == "" {
							return "", conf.ErrBusinessUnbound
						}
					}
					names = append(names, `<span data-av-id="`+id+`" data-popover-url="/api/av/getMirrorDatabaseBlocks" class="popover__block">`+name+`</span>`)
				}
				return strings.Join(names, "&nbsp;"), ctx.Err()
			},
			Virtual: func(ctx context.Context, n *ast.Node, unlinks *[]*ast.Node, keys []string, counts map[string]int, p *lute.Lute) (bool, error) {
				return docview.ProcessVirtual(ctx, n, unlinks, keys, counts, p, options, text)
			},
			Render: func(ctx context.Context, t *parse.Tree, p *lute.Lute) (string, error) {
				return p.Tree2BlockDOM(t, p.RenderOptions), ctx.Err()
			},
			Recent: func(ctx context.Context, s docview.Recent) error { return saveNativeRecent(ctx, w, s) },
		}
		dictionary := &docview.VirtualDictionary{Options: options, Content: content, Read: func(c context.Context) ([]string, error) {
			ignore, e := nativeIgnoreLines(c, w, "searchignore")
			if e != nil {
				return nil, e
			}
			refs, e := nativeIgnoreLines(c, w, "refsearchignore")
			if e != nil {
				return nil, e
			}
			return (indexdb.References{DB: db}).VirtualKeywords(c, searchConf.VirtualRefName, searchConf.VirtualRefAlias, searchConf.VirtualRefAnchor, searchConf.VirtualRefDoc, ignore, refs)
		}}
		s.VirtualKeywords = dictionary.Keys
		ret, e = docview.Read(q, s)
		return e
	})
	if err != nil {
		return docview.Result{}, err
	}
	return
}

func nativeIgnoreLines(ctx context.Context, w *WorkspaceContext, name string) ([]string, error) {
	raw, e := readNativeFile(ctx, w, ".wiki/"+name, 1<<20)
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}

	return indexdb.SearchIgnoreLines(raw), ctx.Err()
}
