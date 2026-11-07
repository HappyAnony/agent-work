package workspace

import (
	"context"
	"crypto/sha256"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/indexdb"
)

func (r *resources) flashcardMutationServices(ctx context.Context, w *WorkspaceContext, b *indexdb.Bundle) (flashcard.MutationServices, map[string][32]byte, error) {
	if !r.blockTreeReady {
		if e := r.rebuildBlockTree(ctx, w); e != nil {
			return flashcard.MutationServices{}, nil, e
		}
	}
	db, e := b.Database(indexdb.BlockTree)
	if e != nil {
		return flashcard.MutationServices{}, nil, e
	}
	query := indexdb.BlockTrees{DB: db}
	inputs := map[string][32]byte{}
	return flashcard.MutationServices{
		Load: func(ctx context.Context, id string) (*parse.Tree, error) {
			rows, e := query.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
			if e != nil {
				return nil, e
			}
			if len(rows) == 0 {
				return nil, nil
			}
			row := rows[0]
			rel := path.Join(row.BoxID, strings.TrimPrefix(row.Path, "/"))
			if !fs.ValidPath(rel) {
				return nil, ErrDocument
			}
			raw, e := readNativeFile(ctx, w, rel, 16<<20)
			if e != nil {
				return nil, e
			}
			inputs["data/"+rel] = sha256.Sum256(raw)
			t, e := r.parseOriginalTree(ctx, w, cachedDocument{raw: raw, path: row.Path}, row.BoxID, row.RootID)
			if e != nil {
				return nil, e
			}
			return locateOriginalTree(ctx, w, t, row.BoxID)
		},
		Select: func(ctx context.Context, scope, id string) ([]string, error) {
			switch scope {
			case "tree":
				scope = "root"
			case "notebook":
				scope = "box"
			default:
				return nil, flashcard.ErrInvalid
			}
			return r.flashcardBlockIDs(ctx, w, scope, id)
		}, NewID: ast.NewNodeID,
	}, inputs, nil
}
func (l *Lease) PlanNativeFlashcardMutation(ctx context.Context, q flashcard.MutationRequest) (out flashcard.MutationPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return e
		}
		ports, inputs, e := r.flashcardMutationServices(ctx, w, b)
		if e != nil {
			return e
		}
		out, e = s.PlanMutation(ctx, q, ports)
		if e != nil {
			return e
		}
		out.SourceHashes = inputs
		for i := range out.Changes {
			out.Changes[i].Path = "storage/riff/" + out.Changes[i].Path
		}
		return nil
	})
	if err != nil {
		return flashcard.MutationPlan{}, err
	}
	return
}
func (r *resources) flashcardExistence(ctx context.Context, ids []string) (map[string]bool, error) {
	db, e := r.native.Database(indexdb.BlockTree)
	if e != nil {
		return nil, e
	}
	rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
	if e != nil {
		return nil, e
	}
	out := map[string]bool{}
	for _, row := range rows {
		out[row.ID] = true
	}
	return out, ctx.Err()
}
func (l *Lease) PrepareNativeFlashcardReview(ctx context.Context, q flashcard.ReviewRequest) (out flashcard.PreparedReview, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return e
		}
		out, e = s.PrepareReview(ctx, q, r.flashcardExistence)
		if e == nil {
			for i := range out.Changes {
				out.Changes[i].Path = "storage/riff/" + out.Changes[i].Path
			}
		}
		return e
	})
	if err != nil {
		return flashcard.PreparedReview{}, err
	}
	return
}
func (l *Lease) ConfirmNativeFlashcardReview(ctx context.Context, token string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		return s.ConfirmReview(ctx, token)
	})
}
func (l *Lease) AbortNativeFlashcardReview(ctx context.Context, token string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		return s.AbortReview(ctx, token)
	})
}

func (l *Lease) PlanNativeFlashcardRoundReview(ctx context.Context, q flashcard.ReviewRequest, round *flashcard.Round) (out flashcard.PreparedReview, next *flashcard.Round, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return e
		}
		out, next, e = s.PlanReview(ctx, q, round, r.flashcardExistence)
		for i := range out.Changes {
			out.Changes[i].Path = "storage/riff/" + out.Changes[i].Path
		}
		return e
	})
	return
}

func (l *Lease) SkipNativeFlashcardInRound(ctx context.Context, deck, card string, round *flashcard.Round) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		return s.SkipWithRound(ctx, deck, card, round)
	})
}
