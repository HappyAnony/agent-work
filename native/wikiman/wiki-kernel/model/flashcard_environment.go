package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"sort"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/treenode"
)

type flashcardEnvironment struct {
	gate     *attributeMutationGate
	mutation flashcard.MutationServices
	review   flashcard.ReviewServices
	read     flashcard.ReadServices
	dueView  flashcard.DueViewServices
	archive  flashcard.ArchiveServices
}

func (e *flashcardEnvironment) validateMutation(ctx context.Context) error {
	if e == nil || ctx == nil || e.gate == nil || e.gate.token == nil || e.mutation.Decks == nil || e.mutation.Load == nil || e.mutation.NewID == nil || e.mutation.Apply == nil {
		return flashcard.ErrDependency
	}
	return ctx.Err()
}
func (tx *Transaction) flashcardMutation(q flashcard.MutationRequest) (out flashcard.MutationPlan, err error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.flashcard
		if err = e.validateMutation(tx.environment.ctx); err != nil {
			return out, err
		}
		err = e.gate.with(tx.environment.ctx, func() error {
			var e2 error
			out, e2 = flashcard.ExecuteMutation(tx.environment.ctx, q, e.mutation)
			return e2
		})
		return
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return out, err
	}
	defer deckLock.Unlock()
	if err = ctx.Err(); err != nil {
		return out, err
	}
	return flashcard.ExecuteMutation(ctx, q, legacyFlashcardMutationServices(tx))
}
func legacyFlashcardMutationServices(tx *Transaction) flashcard.MutationServices {
	return flashcard.MutationServices{
		Before: func(context.Context) error {
			if isSyncingStorages() {
				return flashcard.ErrSyncing
			}
			return nil
		},
		Decks: func(context.Context) (map[string]*riff.Deck, error) {
			out := map[string]*riff.Deck{}
			for id, d := range Decks {
				cloned, e := flashcard.CloneDeck(d, *Conf.Flashcard, time.Now)
				if e != nil {
					return nil, e
				}
				out[id] = cloned
			}
			return out, nil
		},
		CreateBuiltin: func(context.Context) (*riff.Deck, error) {
			d, e := loadLegacyFlashcardDeck(getRiffDir(), builtinDeckID, Conf.Flashcard.RequestRetention, Conf.Flashcard.MaximumInterval, Conf.Flashcard.Weights)
			if e != nil {
				return nil, e
			}
			d.Name = "Built-in Deck"
			return flashcard.CloneDeck(d, *Conf.Flashcard, time.Now)
		},
		Load: func(_ context.Context, id string) (*parse.Tree, error) {
			if tx == nil {
				return nil, flashcard.ErrDependency
			}
			t, _ := tx.loadTree(id)
			return t, nil
		},
		Select: legacyFlashcardSelect, NewID: ast.NewNodeID,
		Apply: func(ctx context.Context, p flashcard.MutationPlan) error {
			for _, t := range p.Trees {
				if e := ctx.Err(); e != nil {
					return e
				}
				if e := tx.writeTree(t); e != nil {
					return e
				}
			}
			ids := []string{}
			for id := range p.Decks {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				if e := ctx.Err(); e != nil {
					return e
				}
				if e := saveLegacyFlashcardSnapshot(p.Decks[id]); e != nil {
					return e
				}
			}
			for _, id := range ids {
				loaded, e := loadLegacyFlashcardDeck(getRiffDir(), id, Conf.Flashcard.RequestRetention, Conf.Flashcard.MaximumInterval, Conf.Flashcard.Weights)
				if e != nil {
					return e
				}
				Decks[id] = loaded
				delete(legacyFlashcardRounds, id)
			}
			tx.flashcardAfterCommit = append(tx.flashcardAfterCommit, p.Attributes...)
			return ctx.Err()
		},
	}
}
func legacyFlashcardSelect(_ context.Context, scope, id string) ([]string, error) {
	switch scope {
	case "tree":
		_, ids := getTreeSubTreeChildBlocks(id)
		return ids, nil
	case "notebook":
		_, ids := getBoxBlocks(id)
		return ids, nil
	}
	return nil, flashcard.ErrInvalid
}
func saveLegacyFlashcardSnapshot(d *riff.Deck) error {
	metadata, cards, e := d.Snapshot()
	if e != nil {
		return e
	}
	storage := legacyFlashcardStorage{getRiffDir()}
	if e = storage.Write(d.ID+".cards", cards); e != nil {
		return e
	}
	return storage.Write(d.ID+".deck", metadata)
}
func legacyFlashcardRound(id string) *flashcard.Round {
	r := legacyFlashcardRounds[id]
	if r == nil {
		r = flashcard.NewRound()
		legacyFlashcardRounds[id] = r
	}
	return r
}
func legacyFlashcardReviewServices() flashcard.ReviewServices {
	return flashcard.ReviewServices{
		Load: func(_ context.Context, id string) (*riff.Deck, error) {
			d := Decks[id]
			if d == nil {
				return nil, fs.ErrNotExist
			}
			return flashcard.CloneDeck(d, *Conf.Flashcard, time.Now)
		},
		Round:    func(_ context.Context, id string) (*flashcard.Round, error) { return legacyFlashcardRound(id), nil },
		Settings: *Conf.Flashcard,
		Exists: func(_ context.Context, ids []string) (map[string]bool, error) {
			return treenode.ExistBlockTrees(ids), nil
		},
		Save: func(ctx context.Context, p flashcard.ReviewPlan) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			if e := p.Deck.Save(); e != nil {
				return e
			}
			if e := saveLegacyFlashcardSnapshot(p.Deck); e != nil {
				return e
			}
			persisted, e := loadLegacyFlashcardDeck(getRiffDir(), p.Deck.ID, Conf.Flashcard.RequestRetention, Conf.Flashcard.MaximumInterval, Conf.Flashcard.Weights)
			if e != nil {
				return e
			}
			if e = persisted.SaveLog(p.Log); e != nil {
				return e
			}
			Decks[p.Deck.ID] = persisted
			return nil
		},
		Accept: func(_ context.Context, id string, r *flashcard.Round) error {
			legacyFlashcardRounds[id] = r
			return nil
		},
	}
}
func (tx *Transaction) reviewFlashcard(q flashcard.ReviewRequest) error {
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return tx.reviewFlashcardContext(ctx, q)
}
func (tx *Transaction) reviewFlashcardContext(ctx context.Context, q flashcard.ReviewRequest) error {
	if ctx == nil {
		return errors.New("flashcard_context_unbound")
	}
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.flashcard
		if e == nil || e.gate == nil || e.gate.token == nil || tx.environment.ctx == nil {
			return flashcard.ErrDependency
		}
		return e.gate.with(tx.environment.ctx, func() error { return flashcard.ExecuteReview(tx.environment.ctx, q, e.review) })
	}
	if err := deckLock.LockContext(ctx); err != nil {
		return err
	}
	defer deckLock.Unlock()
	if err := waitForSyncingStoragesContext(ctx); err != nil {
		return err
	}
	return flashcard.ExecuteReview(ctx, q, legacyFlashcardReviewServices())
}
func (tx *Transaction) resetFlashcard(q flashcard.MutationRequest) error {
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return tx.resetFlashcardContext(ctx, q)
}
func (tx *Transaction) resetFlashcardContext(ctx context.Context, q flashcard.MutationRequest) error {
	if ctx == nil {
		return errors.New("flashcard_context_unbound")
	}
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		_, e := tx.flashcardMutation(q)
		return e
	}
	if e := deckLock.LockContext(ctx); e != nil {
		return e
	}
	if e := waitForSyncingStoragesContext(ctx); e != nil {
		deckLock.Unlock()
		return e
	}
	targets, e := flashcard.ResetTargets(ctx, q, Decks, legacyFlashcardSelect)
	deckLock.Unlock()
	if e != nil {
		return e
	}
	ops := []*Operation{}
	ids := []string{}
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		blocks := targets[id]
		ops = append(ops, &Operation{Action: "removeFlashcards", DeckID: id, BlockIDs: blocks}, &Operation{Action: "addFlashcards", DeckID: id, BlockIDs: blocks})
	}
	if len(ops) == 0 {
		return nil
	}
	txs := []*Transaction{{DoOperations: ops}}
	if e = PerformTransactionsContext(ctx, &txs); e != nil {
		return e
	}
	return FlushTxQueueContext(ctx)
}
func flashcardTxError(err error, id string) *TxErr {
	if err == nil {
		return nil
	}
	if errors.Is(err, flashcard.ErrSyncing) {
		return &TxErr{code: TxErrCodeDataIsSyncing}
	}
	return &TxErr{code: TxErrCodeWriteTree, msg: err.Error(), id: id}
}

// The original cache and updateAttrs payload are published only after the
// enclosing transaction has finished, and discarded on rollback.
func (tx *Transaction) publishFlashcardEffects() {
	effects := tx.flashcardAfterCommit
	tx.flashcardAfterCommit = nil
	if tx.environment == nil || !tx.environment.legacy {
		return
	}
	for _, effect := range effects {
		cache.PutBlockIAL(effect.ID, effect.New)
		pushBroadcastAttrTransactions(effect.Old, &ast.Node{ID: effect.ID, KramdownIAL: parse.Map2IAL(effect.New), AttributeViewType: effect.AVType})
	}
}
