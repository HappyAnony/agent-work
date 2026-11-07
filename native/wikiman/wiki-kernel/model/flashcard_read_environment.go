package model

import (
	"context"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func legacyFlashcardNotebookIDs(box string) ([]string, error) {
	return legacyFlashcardNotebookIDsContext(context.Background(), util.DataDir, box)
}

func legacyFlashcardNotebookIDsContext(ctx context.Context, dataRoot, box string) ([]string, error) {
	if ctx == nil {
		return nil, flashcard.ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(filepath.Join(dataRoot, box))
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if entry.IsDir() || !util.HasDocExt(entry.Name()) {
			continue
		}
		_, blocks := getTreeSubTreeChildBlocks(util.StripDocExt(entry.Name()))
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		for _, id := range blocks {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	return ids, nil
}

func (tx *Transaction) flashcardDueView(q flashcard.DueRequest) (out flashcard.DueView, err error) {
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return tx.flashcardDueViewContext(ctx, q)
}

func (tx *Transaction) flashcardDueViewContext(ctx context.Context, q flashcard.DueRequest) (out flashcard.DueView, err error) {
	if ctx == nil {
		return out, flashcard.ErrDependency
	}
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.flashcard
		if e == nil || e.gate == nil || e.gate.token == nil || tx.environment.ctx == nil {
			return out, flashcard.ErrDependency
		}
		err = e.gate.with(tx.environment.ctx, func() error {
			var e2 error
			out, e2 = flashcard.ReadDueView(tx.environment.ctx, q, e.dueView)
			return e2
		})
		return
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return out, err
	}
	defer deckLock.Unlock()
	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return out, err
	}
	settings := *Conf.Flashcard
	lang := Conf.Lang
	dataRoot := util.DataDir
	return flashcard.ReadDueView(ctx, q, flashcard.DueViewServices{
		Due: func(ctx context.Context, q flashcard.DueRequest) (flashcard.DueResult, error) {
			if e := ctx.Err(); e != nil {
				return flashcard.DueResult{}, e
			}
			d := Decks[q.DeckID]
			if d == nil {
				return flashcard.DueResult{Cards: []riff.Card{}}, nil
			}
			var ids []string
			currentSettings := settings
			switch q.Scope {
			case "deck":
			case "tree":
				_, ids = getTreeSubTreeChildBlocks(q.ID)
				if e := ctx.Err(); e != nil {
					return flashcard.DueResult{}, e
				}
				attrs := sql.GetBlockAttrs(q.ID)
				if e := ctx.Err(); e != nil {
					return flashcard.DueResult{}, e
				}
				currentSettings = flashcard.DocumentLimits(currentSettings, attrs)
			case "notebook":
				var e error
				ids, e = legacyFlashcardNotebookIDsContext(ctx, dataRoot, q.ID)
				if e != nil {
					return flashcard.DueResult{}, e
				}
			default:
				return flashcard.DueResult{}, flashcard.ErrInvalid
			}
			cards, n, newCount, oldCount := getDeckDueCards(d, q.Reviewed, ids, currentSettings.NewCardLimit, currentSettings.ReviewCardLimit, currentSettings.ReviewMode)
			return flashcard.DueResult{Cards: cards, Unreviewed: n, New: newCount, Old: oldCount}, ctx.Err()
		},
		Format: func(a, b time.Time) string { return util.HumanizeDiffTime(a, b, lang) }, Now: time.Now,
	})
}
func legacyFlashcardReadServices() flashcard.ReadServices {
	services, _ := legacyFlashcardReadServicesContext(context.Background(), util.DataDir)
	return services
}

func legacyFlashcardReadServicesContext(ctx context.Context, dataRoot string) (flashcard.ReadServices, error) {
	if ctx == nil {
		return flashcard.ReadServices{}, flashcard.ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return flashcard.ReadServices{}, e
	}
	missing := Conf.Language(180)
	caseSensitive := Conf.Search.CaseSensitive
	return flashcard.ReadServices{
		Cards: func(ctx context.Context, scope, id string, ids []string) ([]riff.Card, bool, error) {
			if e := ctx.Err(); e != nil {
				return nil, false, e
			}
			if scope == "deck" {
				cards := []riff.Card{}
				for key, d := range Decks {
					if e := ctx.Err(); e != nil {
						return nil, false, e
					}
					if id != "" && id != key {
						continue
					}
					cards = append(cards, d.GetCardsByBlockIDs(d.GetBlockIDs())...)
				}
				return cards, true, nil
			}
			d := Decks[builtinDeckID]
			if d == nil {
				return nil, false, nil
			}
			switch scope {
			case "blocks":
			case "tree":
				_, ids = getTreeSubTreeChildBlocks(id)
			case "notebook":
				var e error
				ids, e = legacyFlashcardNotebookIDsContext(ctx, dataRoot, id)
				if e != nil {
					return nil, false, e
				}
			default:
				return nil, false, flashcard.ErrInvalid
			}
			if e := ctx.Err(); e != nil {
				return nil, false, e
			}
			return d.GetCardsByBlockIDs(ids), true, ctx.Err()
		},
		Blocks: func(ctx context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			blocks := sql.GetBlocks(ids)
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			out := []*indexdb.BlockRecord{}
			for _, b := range blocks {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
				if b == nil {
					out = append(out, nil)
				} else {
					row := indexdb.BlockRecord(*b)
					out = append(out, &row)
				}
			}
			return out, nil
		},
		Missing: func() string { return missing }, Now: time.Now, CaseSensitive: caseSensitive,
	}, nil
}
func (tx *Transaction) flashcardPage(q flashcard.PageRequest) (out flashcard.PageResult, err error) {
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return tx.flashcardPageContext(ctx, q)
}

func (tx *Transaction) flashcardPageContext(ctx context.Context, q flashcard.PageRequest) (out flashcard.PageResult, err error) {
	if ctx == nil {
		return out, flashcard.ErrDependency
	}
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.flashcard
		if e == nil || e.gate == nil || e.gate.token == nil || tx.environment.ctx == nil {
			return out, flashcard.ErrDependency
		}
		err = e.gate.with(tx.environment.ctx, func() error { var e2 error; out, e2 = flashcard.ReadPage(tx.environment.ctx, q, e.read); return e2 })
		return
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return out, err
	}
	defer deckLock.Unlock()
	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return out, err
	}
	services, err := legacyFlashcardReadServicesContext(ctx, util.DataDir)
	if err != nil {
		return out, err
	}
	return flashcard.ReadPage(ctx, q, services)
}
