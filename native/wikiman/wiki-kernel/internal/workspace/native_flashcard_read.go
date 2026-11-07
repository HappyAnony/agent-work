package workspace

import (
	"context"
	"io/fs"
	"path"
	"strings"
	"time"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
)

func (r *resources) flashcardNotebookIDs(ctx context.Context, w *WorkspaceContext, box string) ([]string, error) {
	return r.flashcardNotebookIDsWithIO(ctx, w, box, workspaceDirectoryIO{})
}

func (r *resources) flashcardNotebookIDsWithIO(ctx context.Context, w *WorkspaceContext, box string, op workspaceDirectoryIO) ([]string, error) {
	if !documentID.MatchString(box) {
		return nil, ErrDocument
	}
	manifest, e := confirmedWorkspaceDirectoryManifest(ctx, w, box, op)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, entry := range manifest.entries {
		name := path.Base(entry.name)
		if entry.info.IsDir() || !strings.HasSuffix(name, ".docj") {
			continue
		}
		doc := strings.TrimSuffix(name, ".docj")
		if !documentID.MatchString(doc) {
			continue
		}
		if entry.info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrDocument
		}
		blocks, e := r.flashcardBlockIDs(ctx, w, "root", doc)
		if e != nil {
			return nil, e
		}
		for _, id := range blocks {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	return ids, ctx.Err()
}
func (l *Lease) NativeFlashcardPage(ctx context.Context, q flashcard.PageRequest) (out flashcard.PageResult, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil || r.configurationBusiness == nil {
			return indexdb.ErrProjection
		}
		s, e := r.flashcardStore(ctx, w)
		if e != nil {
			return e
		}
		out, e = flashcard.ReadPage(ctx, q, flashcard.ReadServices{
			Cards: func(ctx context.Context, scope, id string, ids []string) ([]riff.Card, bool, error) {
				if scope == "deck" {
					cards, e := s.Cards(ctx, id, nil)
					return cards, true, e
				}
				has, e := s.HasBuiltin(ctx)
				if e != nil || !has {
					return nil, false, e
				}
				switch scope {
				case "blocks":
					ids = append([]string{}, ids...)
				case "tree":
					ids, e = r.flashcardBlockIDs(ctx, w, "root", id)
				case "notebook":
					ids, e = r.flashcardNotebookIDs(ctx, w, id)
				default:
					return nil, false, flashcard.ErrInvalid
				}
				if e != nil {
					return nil, false, e
				}
				if ids == nil {
					ids = []string{}
				}
				cards, e := s.Cards(ctx, flashcard.BuiltinDeckID, ids)
				return cards, true, e
			},
			Blocks: func(ctx context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
				out := []*indexdb.BlockRecord{}
				for _, id := range ids {
					b, ok, e := r.nativeBlock(ctx, id)
					if e != nil {
						return nil, e
					}
					if !ok {
						out = append(out, nil)
					} else {
						out = append(out, &b)
					}
				}
				return out, ctx.Err()
			},
			Missing: func() string {
				return r.configurationBusiness.Language(r.configuration.Lang, 180, false, r.configuration.CloudRegion)
			}, Now: time.Now, CaseSensitive: r.configuration.Search.CaseSensitive,
		})
		return e
	})
	if err != nil {
		return flashcard.PageResult{}, err
	}
	return
}
func (r *resources) flashcardDocumentLimits(ctx context.Context, w *WorkspaceContext, id string) (map[string]string, error) {
	if !documentID.MatchString(id) {
		return nil, ErrDocument
	}
	if !r.blockTreeReady {
		if e := r.rebuildBlockTree(ctx, w); e != nil {
			return nil, e
		}
	}
	t, e := r.nativeTreeByBlock(ctx, w, id)
	if e != nil {
		return nil, e
	}
	if t == nil || t.Root == nil || t.Root.ID != id {
		return nil, ErrDocument
	}
	return map[string]string{"custom-riff-new-card-limit": t.Root.IALAttr("custom-riff-new-card-limit"), "custom-riff-review-card-limit": t.Root.IALAttr("custom-riff-review-card-limit")}, nil
}

type NativeFlashcardDueView = flashcard.DueView

func (r *resources) flashcardDue(ctx context.Context, w *WorkspaceContext, deck, scope, id string, reviewed []string, round *flashcard.Round) (flashcard.DueResult, error) {
	s, e := r.flashcardStore(ctx, w)
	if e != nil {
		return flashcard.DueResult{}, e
	}
	if deck == "" {
		deck = flashcard.BuiltinDeckID
	}
	var ids []string
	if scope == "notebook" {
		ids, e = r.flashcardNotebookIDs(ctx, w, id)
	} else if scope == "deck" {
		if !r.blockTreeReady {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return flashcard.DueResult{}, e
			}
		}
		cards, err := s.Cards(ctx, deck, nil)
		if err != nil {
			return flashcard.DueResult{}, err
		}
		all := []string{}
		for _, c := range cards {
			all = append(all, c.BlockID())
		}
		present, err := r.flashcardExistence(ctx, all)
		if err != nil {
			return flashcard.DueResult{}, err
		}
		for _, id := range all {
			if present[id] {
				ids = append(ids, id)
			}
		}
	} else {
		ids, e = r.flashcardBlockIDs(ctx, w, scope, id)
	}
	if e != nil {
		return flashcard.DueResult{}, e
	}
	settings := *r.configuration.Flashcard
	if scope == "root" || scope == "tree" {
		attrs, e := r.flashcardDocumentLimits(ctx, w, id)
		if e != nil {
			return flashcard.DueResult{}, e
		}
		settings = flashcard.DocumentLimits(settings, attrs)
	}
	return s.DueWithRound(ctx, deck, ids, reviewed, &settings, round)
}
func (l *Lease) NativeFlashcardDueView(ctx context.Context, deck, scope, id string, reviewed []string) (NativeFlashcardDueView, error) {
	return l.NativeFlashcardDueRound(ctx, deck, scope, id, reviewed, nil)
}

func (l *Lease) NativeFlashcardDueRound(ctx context.Context, deck, scope, id string, reviewed []string, round *flashcard.Round) (out NativeFlashcardDueView, err error) {
	if deck == "" {
		deck = flashcard.BuiltinDeckID
	}
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configurationBusiness == nil {
			return indexdb.ErrProjection
		}
		format, e := r.configurationBusiness.DiffTime(r.configuration.Lang)
		if e != nil {
			return e
		}
		out, e = flashcard.ReadDueView(ctx, flashcard.DueRequest{DeckID: deck, Scope: scope, ID: id, Reviewed: reviewed}, flashcard.DueViewServices{Due: func(ctx context.Context, q flashcard.DueRequest) (flashcard.DueResult, error) {
			return r.flashcardDue(ctx, w, q.DeckID, q.Scope, q.ID, q.Reviewed, round)
		}, Format: format, Now: time.Now})
		return e
	})
	if err != nil {
		return NativeFlashcardDueView{}, err
	}
	return
}
