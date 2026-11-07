// SPDX-License-Identifier: AGPL-3.0-or-later
package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"io/fs"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

type InvalidReferenceResult struct {
	Blocks                                 []*blockview.Block
	MatchedBlocks, MatchedRoots, PageCount int
}
type InvalidReferenceServices struct {
	Walk          func(context.Context, func(*parse.Tree) error) error
	Blocks        func(context.Context, []string) ([]*indexdb.BlockRecord, error)
	Locations     func(context.Context, []string) ([]indexdb.BlockTreeRecord, error)
	CaseSensitive bool
}

func InvalidReferences(ctx context.Context, page, size int, s InvalidReferenceServices) (out InvalidReferenceResult, err error) {
	if ctx == nil || s.Walk == nil || s.Blocks == nil || s.Locations == nil || page < 1 || size < 1 || size > 10000 {
		return out, errors.New("invalid_reference_query_unbound")
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	blocks := map[string]bool{}
	refs := map[string][]string{}
	err = s.Walk(ctx, func(tree *parse.Tree) error {
		if tree == nil || tree.Root == nil {
			return errors.New("invalid_reference_source_tree")
		}
		var walkErr error
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter {
				return ast.WalkContinue
			}
			if walkErr = ctx.Err(); walkErr != nil {
				return ast.WalkStop
			}
			if n.IsBlock() {
				blocks[n.ID] = true
				return ast.WalkContinue
			}
			if n.Type != ast.NodeTextMark {
				return ast.WalkContinue
			}
			def := ""
			if n.IsTextMarkType("a") && strings.HasPrefix(n.TextMarkAHref, "siyuan://blocks/") {
				def = strings.Split(strings.TrimPrefix(n.TextMarkAHref, "siyuan://blocks/"), "?")[0]
			} else if n.IsTextMarkType("block-ref") {
				def = n.TextMarkBlockRefID
			}
			if def != "" {
				parent := docview.ParentBlock(n)
				if parent == nil {
					walkErr = errors.New("invalid_reference_source_parent")
					return ast.WalkStop
				}
				refs[parent.ID] = append(refs[parent.ID], def)
			}
			return ast.WalkContinue
		})
		return walkErr
	})
	if err != nil {
		return InvalidReferenceResult{}, err
	}
	ids := []string{}
	for id, defs := range refs {
		for _, def := range defs {
			if !blocks[def] {
				ids = append(ids, id)
				break
			}
		}
	}
	sort.Strings(ids)
	locations, e := s.Locations(ctx, ids)
	if e != nil {
		return InvalidReferenceResult{}, e
	}
	roots := map[string]bool{}
	for _, loc := range locations {
		roots[loc.RootID] = true
	}
	out.MatchedBlocks, out.MatchedRoots = len(ids), len(roots)
	out.PageCount = (len(ids) + size - 1) / size
	out.Blocks = []*blockview.Block{}
	// Avoid overflow and slicing past the final page.
	if page <= out.PageCount {
		start := (page - 1) * size
		end := min(start+size, len(ids))
		rows, e := s.Blocks(ctx, ids[start:end])
		if e != nil {
			return InvalidReferenceResult{}, e
		}
		for _, row := range rows {
			if row != nil {
				out.Blocks = append(out.Blocks, blockview.FromRow(row, "", 36, s.CaseSensitive))
			}
		}
	}
	if e := ctx.Err(); e != nil {
		return InvalidReferenceResult{}, e
	}
	return out, nil
}

// WalkDocumentTrees reads every original document without repair or mutation.
func WalkDocumentTrees(ctx context.Context, boxes []string, readDir func(context.Context, string) ([]fs.DirEntry, error), read func(context.Context, string) ([]byte, error), parser *lute.Lute, visit func(*parse.Tree) error) error {
	if parser == nil || visit == nil {
		return errors.New("document_scan_unbound")
	}
	return WalkDocumentFiles(ctx, boxes, readDir, read, func(name string, raw []byte) error {
		input, e := docast.OriginalTreeInput(raw)
		if e != nil {
			return e
		}
		tree, e := dataparser.ParseJSONWithoutFix(input, parser.ParseOptions)
		if e != nil {
			return e
		}
		return visit(tree)
	})
}

func WalkDocumentFiles(ctx context.Context, boxes []string, readDir func(context.Context, string) ([]fs.DirEntry, error), read func(context.Context, string) ([]byte, error), visit func(string, []byte) error) error {
	if ctx == nil || readDir == nil || read == nil || visit == nil {
		return errors.New("document_scan_unbound")
	}

	var walk func(string, int) error
	walk = func(name string, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if depth > 255 {
			return errors.New("document_scan_depth")
		}
		entries, e := readDir(ctx, name)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			selected := entry.IsDir() || strings.HasSuffix(entry.Name(), ".docj")
			if !selected {
				continue
			}
			if strings.HasPrefix(entry.Name(), ".") && entry.IsDir() {
				continue
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return errors.New("document_scan_symlink")
			}
			child := path.Join(name, entry.Name())
			if entry.IsDir() {
				if e = walk(child, depth+1); e != nil {
					return e
				}
				continue
			}
			raw, e := read(ctx, child)
			if e != nil {
				return e
			}
			if e = visit(child, raw); e != nil {
				return e
			}
		}
		return ctx.Err()
	}
	for _, box := range boxes {
		if !fs.ValidPath(box) || strings.Contains(box, "/") || box == "." {
			return errors.New("document_scan_box")
		}
		if e := walk(box, 0); e != nil {
			return e
		}
	}
	return ctx.Err()
}
