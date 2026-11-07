package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/indexdb"
)

type NativeFlashcardExportRequest struct {
	Scope, ID   string
	DocumentIDs []string
}
type NativeFlashcardImportDocument struct {
	Box, Path string
	Data      []byte
}
type NativeFlashcardImportRequest struct {
	Files     map[string][]byte
	BlockIDs  map[string]string
	Documents []NativeFlashcardImportDocument
}

func (l *Lease) PlanNativeFlashcardExport(ctx context.Context, q NativeFlashcardExportRequest) (out flashcard.ArchivePlan, err error) {
	q.DocumentIDs = slices.Clone(q.DocumentIDs)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		query := indexdb.BlockTrees{DB: db}
		ids := slices.Clone(q.DocumentIDs)
		if q.Scope == "tree" || q.Scope == "notebook" {
			scope := "root"
			if q.Scope == "notebook" {
				scope = "box"
			}
			blockIDs, e := r.flashcardBlockIDs(ctx, w, scope, q.ID)
			if e != nil {
				return e
			}
			rows, e := query.Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: blockIDs})
			if e != nil {
				return e
			}
			ids = nil
			for _, row := range rows {
				if row.Type == "d" {
					ids = append(ids, row.ID)
				}
			}
		} else if q.Scope != "documents" {
			return flashcard.ErrInvalid
		}
		if len(ids) > 1000 {
			return ErrBudget
		}
		seen := map[string]bool{}
		trees := []*parse.Tree{}
		inputs := map[string][32]byte{}
		size := 0
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			if !documentID.MatchString(id) {
				return ErrDocument
			}
			rows, e := query.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
			if e != nil {
				return e
			}
			if len(rows) != 1 || rows[0].Type != "d" {
				return ErrDocument
			}
			row := rows[0]
			rel, e := treeRelative(row.BoxID, row.Path)
			if e != nil {
				return e
			}
			raw, e := readNativeFile(ctx, w, rel, 16<<20)
			if e != nil {
				return e
			}
			size += len(raw)
			if size > 64<<20 {
				return ErrBudget
			}
			inputs["data/"+rel] = sha256.Sum256(raw)
			t, e := r.parseOriginalTree(ctx, w, cachedDocument{raw: raw, path: row.Path}, row.BoxID, id)
			if e != nil {
				return e
			}
			trees = append(trees, t)
		}
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return e
		}
		out, e = s.PlanArchiveExport(ctx, trees)
		if e == nil {
			out.SourceHashes = inputs
		}
		return e
	})
	if err != nil {
		return flashcard.ArchivePlan{}, err
	}
	return
}
func (l *Lease) PlanNativeFlashcardImport(ctx context.Context, q NativeFlashcardImportRequest) (out flashcard.ArchivePlan, err error) {
	if len(q.Documents) > 1000 {
		return out, ErrBudget
	}
	files := map[string][]byte{}
	size := 0
	for name, b := range q.Files {
		size += len(b)
		if size > 64<<20 {
			return out, ErrBudget
		}
		files[name] = bytes.Clone(b)
	}
	ids := maps.Clone(q.BlockIDs)
	docs := slices.Clone(q.Documents)
	for i := range docs {
		size += len(docs[i].Data)
		if len(docs[i].Data) > 16<<20 || size > 64<<20 {
			return out, ErrBudget
		}
		docs[i].Data = bytes.Clone(docs[i].Data)
	}
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configurationBusiness == nil {
			return flashcard.ErrDependency
		}
		boxes, e := r.configurationBusiness.Notebooks(ctx)
		if e != nil {
			return e
		}
		allowed := map[string]bool{}
		for _, box := range boxes {
			if !box.Closed {
				allowed[box.ID] = true
			}
		}
		if !r.blockTreeReady {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		query := indexdb.BlockTrees{DB: db}
		trees := []*parse.Tree{}
		for _, doc := range docs {
			if !allowed[doc.Box] || !documentID.MatchString(doc.Box) {
				return ErrDocument
			}
			rel, e := treeRelative(doc.Box, doc.Path)
			if e != nil || !fs.ValidPath(rel) {
				return ErrDocument
			}
			id := strings.TrimSuffix(path.Base(doc.Path), ".docj")
			if !strings.HasSuffix(doc.Path, ".docj") || !documentID.MatchString(id) {
				return ErrDocument
			}
			t, e := r.parseOriginalTree(ctx, w, cachedDocument{raw: doc.Data, path: doc.Path}, doc.Box, id)
			if e != nil {
				return e
			}
			nodeIDs := []string{}
			ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if enter && n.IsBlock() {
					nodeIDs = append(nodeIDs, n.ID)
				}
				return ast.WalkContinue
			})
			existing, e := query.Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: nodeIDs})
			if e != nil {
				return e
			}
			if len(existing) > 0 {
				return flashcard.ErrReviewConflict
			}
			trees = append(trees, t)
		}
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return e
		}
		out, e = s.PlanArchiveImport(ctx, files, ids, trees, ast.NewNodeID)
		return e
	})
	if err != nil {
		return flashcard.ArchivePlan{}, err
	}
	return
}
