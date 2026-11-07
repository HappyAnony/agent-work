package workspace

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
)

// Called only inside the owning resource gate and source-read admission. A
// missing directory is an absent deck, never an implicitly created default deck.
func (r *resources) flashcardStore(ctx context.Context, w *WorkspaceContext) (*flashcard.Store, error) {
	return r.flashcardStoreWithIO(ctx, w, workspaceDirectoryIO{})
}

func (r *resources) flashcardStoreWithIO(ctx context.Context, w *WorkspaceContext, op workspaceDirectoryIO) (*flashcard.Store, error) {
	if r.configuration.Flashcard == nil {
		return nil, indexdb.ErrProjection
	}
	if r.flashcards == nil {
		r.flashcards = flashcard.Open(flashcard.Source{
			List: func(ctx context.Context) ([]string, error) {
				manifest, e := confirmedWorkspaceDirectoryManifest(ctx, w, "storage/riff", op)
				if errors.Is(e, fs.ErrNotExist) {
					return []string{}, nil
				}
				if e != nil {
					return nil, e
				}
				out := []string{}
				for _, entry := range manifest.entries {
					name := path.Base(entry.name)
					if strings.HasSuffix(name, ".deck") {
						if entry.info.IsDir() || entry.info.Mode()&fs.ModeSymlink != 0 {
							return nil, ErrDocument
						}
						out = append(out, name)
					}
				}
				return out, ctx.Err()
			},
			Read: func(ctx context.Context, p string) ([]byte, error) {
				return readNativeFile(ctx, w, "storage/riff/"+p, 32<<20)
			},
		}, *r.configuration.Flashcard, nil)
	}
	if err := r.flashcards.Configure(ctx, *r.configuration.Flashcard); err != nil {
		return nil, err
	}
	return r.flashcards, nil
}

// NativeFlashcardBlockIDs preserves the distinction between one document,
// its descendant documents and a notebook. Roots are resolved within this owner.
func (r *resources) flashcardBlockIDs(ctx context.Context, w *WorkspaceContext, scope, id string) ([]string, error) {
	if !documentID.MatchString(id) {
		return nil, ErrDocument
	}
	if !r.blockTreeReady {
		if e := r.rebuildBlockTree(ctx, w); e != nil {
			return nil, e
		}
	}
	db, e := r.native.Database(indexdb.BlockTree)
	if e != nil {
		return nil, e
	}
	q := indexdb.BlockTrees{DB: db}
	var rows []indexdb.BlockTreeRecord
	switch scope {
	case "box":
		rows, e = q.Query(ctx, indexdb.BlockTreeFilter{Kind: "box", Value: id})
	case "tree":
		rows, e = q.Query(ctx, indexdb.BlockTreeFilter{Kind: "root", Value: id})
	case "root":
		root, err := q.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
		if err != nil {
			return nil, err
		}
		if len(root) == 0 {
			return []string{}, nil
		}
		if root[0].ID != root[0].RootID {
			return nil, ErrDocument
		}
		rows, e = q.Query(ctx, indexdb.BlockTreeFilter{Kind: "prefix", Value: strings.TrimSuffix(root[0].Path, ".docj")})
		if e != nil {
			return nil, e
		}
		filtered := rows[:0]
		for _, row := range rows {
			if row.BoxID == root[0].BoxID {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	default:
		return nil, ErrDocument
	}
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out, ctx.Err()
}
func (l *Lease) NativeFlashcardBlockIDs(ctx context.Context, scope, id string) (out []string, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		out, e = l.workspace.resources.flashcardBlockIDs(ctx, l.workspace, scope, id)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
func (r *resources) navigationFlashcards(w *WorkspaceContext) *docquery.Flashcards {
	var counter func(context.Context, []string) (docquery.Counts, error)
	return &docquery.Flashcards{Open: func(ctx context.Context) (bool, error) {
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return false, e
		}
		var available bool
		counter, available, e = s.Counter(ctx)
		return available, e
	}, Count: func(ctx context.Context, scope, id string) (docquery.Counts, error) {
		ids, e := r.flashcardBlockIDs(ctx, w, scope, id)
		if e != nil {
			return docquery.Counts{}, e
		}
		if counter == nil {
			return docquery.Counts{}, indexdb.ErrProjection
		}
		return counter(ctx, ids)
	}}
}
func (l *Lease) NativeFlashcardDecks(ctx context.Context) (out []flashcard.Deck, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		out, e = s.Decks(ctx)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
func (l *Lease) NativeFlashcardCards(ctx context.Context, deck string, ids []string) (out []riff.Card, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		out, e = s.Cards(ctx, deck, ids)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

// Plans retain the original byte format and exact preimages. They do not bypass
// R04/R05 source journaling or report an uncommitted review as completed.
func (l *Lease) PlanNativeFlashcard(ctx context.Context, edit flashcard.Edit) (out []flashcard.Change, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		if edit.Action == "add" {
			if !documentID.MatchString(edit.BlockID) {
				return ErrDocument
			}
			if !l.workspace.resources.blockTreeReady {
				if e := l.workspace.resources.rebuildBlockTree(ctx, l.workspace); e != nil {
					return e
				}
			}
			db, e := l.workspace.resources.native.Database(indexdb.BlockTree)
			if e != nil {
				return e
			}
			rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: edit.BlockID})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				return ErrDocument
			}
		}
		out, e = s.Plan(ctx, edit)
		if e == nil {
			for i := range out {
				out[i].Path = "storage/riff/" + out[i].Path
			}
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func (l *Lease) NativeDueFlashcards(ctx context.Context, deck, scope, id string, reviewed []string) (out flashcard.DueResult, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		out, e = l.workspace.resources.flashcardDue(ctx, l.workspace, deck, scope, id, reviewed, nil)
		return e
	})
	if err != nil {
		return flashcard.DueResult{}, err
	}
	return
}
func (l *Lease) SkipNativeFlashcard(ctx context.Context, deck, card string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		s, e := l.workspace.resources.flashcardStore(ctx, l.workspace)
		if e != nil {
			return e
		}
		return s.Skip(ctx, deck, card)
	})
}
