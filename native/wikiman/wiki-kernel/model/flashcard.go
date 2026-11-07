// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/88250/lute/ast"
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func GetFlashcardsByBlockIDs(ids []string) []*Block {
	blocks, e := GetFlashcardsByBlockIDsContext(context.Background(), ids)
	if e != nil {
		logging.LogErrorf("flashcard blocks: %s", e)
		return []*Block{}
	}
	return blocks
}

func GetFlashcardsByBlockIDsContext(ctx context.Context, ids []string) ([]*Block, error) {
	p, e := (*Transaction)(nil).flashcardPageContext(ctx, flashcard.PageRequest{Scope: "blocks", IDs: ids})
	return p.Blocks, e
}

type SetFlashcardDueTime struct {
	ID  string `json:"id"`  // 卡片 ID
	Due string `json:"due"` // 下次复习时间，格式为 YYYYMMDDHHmmss
}

func SetFlashcardsDueTime(cardDues []*SetFlashcardDueTime) (err error) {
	return SetFlashcardsDueTimeContext(context.Background(), cardDues)
}

func SetFlashcardsDueTimeContext(ctx context.Context, cardDues []*SetFlashcardDueTime) (err error) {
	// Add internal kernel API `/api/riff/batchSetRiffCardsDueTime` https://github.com/siyuan-note/siyuan/issues/10423

	if ctx == nil {
		return errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return err
	}
	defer deckLock.Unlock()

	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return err
	}

	deck := Decks[builtinDeckID]
	if nil == deck {
		return
	}

	for _, cardDue := range cardDues {
		card := deck.GetCard(cardDue.ID)
		if nil == card {
			continue
		}

		due, parseErr := time.ParseInLocation("20060102150405", cardDue.Due, time.Local)
		if nil != parseErr {
			logging.LogErrorf("parse due time [%s] failed: %s", cardDue.Due, err)
			err = parseErr
			return
		}

		card.SetDue(due)
	}

	if err = deck.Save(); err != nil {
		logging.LogErrorf("save deck [%s] failed: %s", builtinDeckID, err)
	}
	return
}

func ResetFlashcards(typ, id, deckID string, blockIDs []string) error {
	return ResetFlashcardsContext(context.Background(), typ, id, deckID, blockIDs)
}

func ResetFlashcardsContext(ctx context.Context, typ, id, deckID string, blockIDs []string) error {
	return (*Transaction)(nil).resetFlashcardContext(ctx, flashcard.MutationRequest{Action: "reset", Scope: typ, ID: id, DeckID: deckID, BlockIDs: blockIDs})
}

func GetFlashcardNotebooks() (ret []*Box) {
	ret, err := GetFlashcardNotebooksContext(context.Background())
	if err != nil {
		logging.LogErrorf("flashcard notebooks: %s", err)
		return []*Box{}
	}
	return ret
}

func GetFlashcardNotebooksContext(ctx context.Context) (ret []*Box, err error) {
	if ctx == nil {
		return nil, errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return nil, err
	}
	defer deckLock.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	deck := Decks[builtinDeckID]
	if nil == deck {
		return []*Box{}, nil
	}

	deckBlockIDs := deck.GetBlockIDs()
	boxes := Conf.GetOpenedBoxes()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	for _, box := range boxes {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		newFlashcardCount, dueFlashcardCount, flashcardCount := countBoxFlashcard(box.ID, deck, deckBlockIDs)
		if 0 < flashcardCount {
			box.NewFlashcardCount = newFlashcardCount
			box.DueFlashcardCount = dueFlashcardCount
			box.FlashcardCount = flashcardCount
			ret = append(ret, box)
		}
	}
	return ret, ctx.Err()
}

func countTreeFlashcard(rootID string, deck *riff.Deck, deckBlockIDs []string) (newFlashcardCount, dueFlashcardCount, flashcardCount int) {
	_, blockIDs := getTreeSubTreeChildBlocks(rootID)
	c := flashcard.Count(deck, blockIDs)
	return c.New, c.Due, c.Total
}

func countBoxFlashcard(boxID string, deck *riff.Deck, deckBlockIDs []string) (newFlashcardCount, dueFlashcardCount, flashcardCount int) {
	_, blockIDs := getBoxBlocks(boxID)
	c := flashcard.Count(deck, blockIDs)
	return c.New, c.Due, c.Total
}

var (
	Decks    = map[string]*riff.Deck{}
	deckLock = make(contextMutex, 1)
)

func GetNotebookFlashcards(id string, page, size int) ([]*Block, int, int) {
	blocks, total, pageCount, e := GetNotebookFlashcardsContext(context.Background(), id, page, size)
	if e != nil {
		logging.LogErrorf("flashcard page: %s", e)
		return []*Block{}, 0, 0
	}
	return blocks, total, pageCount
}

func GetNotebookFlashcardsContext(ctx context.Context, id string, page, size int) ([]*Block, int, int, error) {
	p, e := (*Transaction)(nil).flashcardPageContext(ctx, flashcard.PageRequest{Scope: "notebook", ID: id, Page: page, PageSize: size})
	return p.Blocks, p.Total, p.PageCount, e
}

func GetTreeFlashcards(id string, page, size int) ([]*Block, int, int) {
	blocks, total, pageCount, e := GetTreeFlashcardsContext(context.Background(), id, page, size)
	if e != nil {
		logging.LogErrorf("flashcard page: %s", e)
		return []*Block{}, 0, 0
	}
	return blocks, total, pageCount
}

func GetTreeFlashcardsContext(ctx context.Context, id string, page, size int) ([]*Block, int, int, error) {
	p, e := (*Transaction)(nil).flashcardPageContext(ctx, flashcard.PageRequest{Scope: "tree", ID: id, Page: page, PageSize: size})
	return p.Blocks, p.Total, p.PageCount, e
}

func getTreeSubTreeFlashcards(rootID string) (ret []riff.Card) {
	deck := Decks[builtinDeckID]
	if nil == deck {
		return
	}

	var allBlockIDs []string
	deckBlockIDs := deck.GetBlockIDs()
	treeBlockIDsMap, _ := getTreeSubTreeChildBlocks(rootID)
	for _, blockID := range deckBlockIDs {
		if treeBlockIDsMap[blockID] {
			allBlockIDs = append(allBlockIDs, blockID)
		}
	}
	allBlockIDs = gulu.Str.RemoveDuplicatedElem(allBlockIDs)
	ret = deck.GetCardsByBlockIDs(allBlockIDs)
	return
}

func getTreeFlashcards(rootID string) (ret []riff.Card) {
	deck := Decks[builtinDeckID]
	if nil == deck {
		return
	}

	var allBlockIDs []string
	deckBlockIDs := deck.GetBlockIDs()
	treeBlockIDsMap, _ := getTreeBlocks(rootID)
	for _, blockID := range deckBlockIDs {
		if treeBlockIDsMap[blockID] {
			allBlockIDs = append(allBlockIDs, blockID)
		}
	}
	allBlockIDs = gulu.Str.RemoveDuplicatedElem(allBlockIDs)
	ret = deck.GetCardsByBlockIDs(allBlockIDs)
	return
}

func GetDeckFlashcards(id string, page, size int) ([]*Block, int, int) {
	blocks, total, pageCount, e := GetDeckFlashcardsContext(context.Background(), id, page, size)
	if e != nil {
		logging.LogErrorf("flashcard page: %s", e)
		return []*Block{}, 0, 0
	}
	return blocks, total, pageCount
}

func GetDeckFlashcardsContext(ctx context.Context, id string, page, size int) ([]*Block, int, int, error) {
	p, e := (*Transaction)(nil).flashcardPageContext(ctx, flashcard.PageRequest{Scope: "deck", ID: id, Page: page, PageSize: size})
	return p.Blocks, p.Total, p.PageCount, e
}

func getCardsBlocks(cards []riff.Card, page, size int) ([]*Block, int, int) {
	p, e := flashcard.ProjectPage(context.Background(), cards, page, size, legacyFlashcardReadServices())
	if e != nil {
		return []*Block{}, 0, 0
	}
	return p.Blocks, p.Total, p.PageCount
}

func getRiffCard(card *fsrs.Card) *RiffCard { return flashcard.CardView(card, time.Now()) }

var legacyFlashcardRounds = map[string]*flashcard.Round{}

func ReviewFlashcard(deckID, cardID string, rating riff.Rating, reviewedCardIDs []string) error {
	return ReviewFlashcardContext(context.Background(), deckID, cardID, rating, reviewedCardIDs)
}

func ReviewFlashcardContext(ctx context.Context, deckID, cardID string, rating riff.Rating, reviewedCardIDs []string) error {
	return (*Transaction)(nil).reviewFlashcardContext(ctx, flashcard.ReviewRequest{DeckID: deckID, CardID: cardID, Rating: rating, Reviewed: reviewedCardIDs})
}

func SkipReviewFlashcard(deckID, cardID string) (err error) {
	return SkipReviewFlashcardContext(context.Background(), deckID, cardID)
}

func SkipReviewFlashcardContext(ctx context.Context, deckID, cardID string) (err error) {
	if ctx == nil {
		return errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return err
	}
	defer deckLock.Unlock()

	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return err
	}

	deck := Decks[deckID]
	if deck == nil {
		return os.ErrNotExist
	}
	card := deck.GetCard(cardID)
	if nil == card {
		return
	}

	legacyFlashcardRound(deckID).Skipped[cardID] = card.Clone()
	return
}

type Flashcard = flashcard.DueCard

func newFlashcard(card riff.Card, deckID string, now time.Time) *Flashcard {
	return flashcard.DueCardView(card, deckID, now, func(a, b time.Time) string { return strings.TrimSpace(util.HumanizeDiffTime(a, b, Conf.Lang)) })
}

func GetNotebookDueFlashcards(id string, reviewed []string) ([]*Flashcard, int, int, int, error) {
	return GetNotebookDueFlashcardsContext(context.Background(), id, reviewed)
}

func GetNotebookDueFlashcardsContext(ctx context.Context, id string, reviewed []string) ([]*Flashcard, int, int, int, error) {
	q := flashcard.DueRequest{Scope: "notebook", ID: id, DeckID: builtinDeckID, Reviewed: reviewed}
	if q.Scope == "deck" {
		q.DeckID = id
	}
	p, e := (*Transaction)(nil).flashcardDueViewContext(ctx, q)
	return p.Cards, p.Unreviewed, p.New, p.Old, e
}

func GetTreeDueFlashcards(id string, reviewed []string) ([]*Flashcard, int, int, int, error) {
	return GetTreeDueFlashcardsContext(context.Background(), id, reviewed)
}

func GetTreeDueFlashcardsContext(ctx context.Context, id string, reviewed []string) ([]*Flashcard, int, int, int, error) {
	q := flashcard.DueRequest{Scope: "tree", ID: id, DeckID: builtinDeckID, Reviewed: reviewed}
	if q.Scope == "deck" {
		q.DeckID = id
	}
	p, e := (*Transaction)(nil).flashcardDueViewContext(ctx, q)
	return p.Cards, p.Unreviewed, p.New, p.Old, e
}

func getTreeSubTreeChildBlocks(rootID string) (treeBlockIDsMap map[string]bool, treeBlockIDs []string) {
	treeBlockIDsMap = map[string]bool{}
	root := treenode.GetBlockTree(rootID)
	if nil == root {
		return
	}

	bts := treenode.GetBlockTreesByPathPrefix(util.StripDocExt(root.Path))
	for _, bt := range bts {
		treeBlockIDsMap[bt.ID] = true
		treeBlockIDs = append(treeBlockIDs, bt.ID)
	}
	return
}

func getTreeBlocks(rootID string) (treeBlockIDsMap map[string]bool, treeBlockIDs []string) {
	treeBlockIDsMap = map[string]bool{}
	bts := treenode.GetBlockTreesByRootID(rootID)
	for _, bt := range bts {
		treeBlockIDsMap[bt.ID] = true
		treeBlockIDs = append(treeBlockIDs, bt.ID)
	}
	return
}

func getBoxBlocks(boxID string) (blockIDsMap map[string]bool, blockIDs []string) {
	blockIDsMap = map[string]bool{}
	bts := treenode.GetBlockTreesByBoxID(boxID)
	for _, bt := range bts {
		blockIDsMap[bt.ID] = true
		blockIDs = append(blockIDs, bt.ID)
	}
	return
}

func GetDueFlashcards(id string, reviewed []string) ([]*Flashcard, int, int, int, error) {
	return GetDueFlashcardsContext(context.Background(), id, reviewed)
}

func GetDueFlashcardsContext(ctx context.Context, id string, reviewed []string) ([]*Flashcard, int, int, int, error) {
	q := flashcard.DueRequest{Scope: "deck", ID: id, DeckID: builtinDeckID, Reviewed: reviewed}
	if q.Scope == "deck" {
		q.DeckID = id
	}
	p, e := (*Transaction)(nil).flashcardDueViewContext(ctx, q)
	return p.Cards, p.Unreviewed, p.New, p.Old, e
}

func (tx *Transaction) doRemoveFlashcards(op *Operation) *TxErr {
	if op == nil {
		return flashcardTxError(flashcard.ErrInvalid, "")
	}
	_, e := tx.flashcardMutation(flashcard.MutationRequest{Action: "remove", DeckID: op.DeckID, BlockIDs: op.BlockIDs})
	return flashcardTxError(e, op.DeckID)
}

func (tx *Transaction) doAddFlashcards(op *Operation) *TxErr {
	if op == nil {
		return flashcardTxError(flashcard.ErrInvalid, "")
	}
	_, e := tx.flashcardMutation(flashcard.MutationRequest{Action: "add", DeckID: op.DeckID, BlockIDs: op.BlockIDs})
	return flashcardTxError(e, op.DeckID)
}

func LoadFlashcards() {
	deckLock.Lock()
	defer deckLock.Unlock()
	loadFlashcardsUnlocked()
}
func loadFlashcardsUnlocked() {
	legacyFlashcardRounds = map[string]*flashcard.Round{}
	riffSavePath := getRiffDir()
	if err := os.MkdirAll(riffSavePath, 0755); err != nil {
		logging.LogErrorf("create riff dir [%s] failed: %s", riffSavePath, err)
		return
	}

	Decks = map[string]*riff.Deck{}

	entries, err := os.ReadDir(riffSavePath)
	if err != nil {
		logging.LogErrorf("read riff dir failed: %s", err)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".deck") {
			deckID := strings.TrimSuffix(name, ".deck")
			deck, loadErr := loadLegacyFlashcardDeck(riffSavePath, deckID, Conf.Flashcard.RequestRetention, Conf.Flashcard.MaximumInterval, Conf.Flashcard.Weights)
			if nil != loadErr {
				logging.LogErrorf("load deck [%s] failed: %s", name, loadErr)
				continue
			}

			if 0 == deck.Created {
				deck.Created = time.Now().Unix()
			}
			if 0 == deck.Updated {
				deck.Updated = deck.Created
			}

			Decks[deckID] = deck
		}
	}
}

const builtinDeckID = "20230218211946-2kw8jgx"

func RenameDeck(deckID, name string) (err error) {
	return RenameDeckContext(context.Background(), deckID, name)
}

func RenameDeckContext(ctx context.Context, deckID, name string) (err error) {
	if ctx == nil {
		return errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return err
	}
	defer deckLock.Unlock()

	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return err
	}

	deck := Decks[deckID]
	if deck == nil {
		return os.ErrNotExist
	}
	deck.Name = name
	err = deck.Save()
	if err != nil {
		logging.LogErrorf("save deck [%s] failed: %s", deckID, err)
		return
	}
	return
}

func RemoveDeck(deckID string) (err error) {
	return RemoveDeckContext(context.Background(), deckID)
}

func RemoveDeckContext(ctx context.Context, deckID string) (err error) {
	if ctx == nil {
		return errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return err
	}
	defer deckLock.Unlock()

	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return err
	}

	riffSavePath := getRiffDir()
	deckPath := filepath.Join(riffSavePath, deckID+".deck")
	if filelock.IsExist(deckPath) {
		if err = filelock.Remove(deckPath); err != nil {
			return
		}
	}

	cardsPath := filepath.Join(riffSavePath, deckID+".cards")
	if filelock.IsExist(cardsPath) {
		if err = filelock.Remove(cardsPath); err != nil {
			return
		}
	}

	loadFlashcardsUnlocked()
	return
}

func CreateDeck(name string) (deck *riff.Deck, err error) {
	return CreateDeckContext(context.Background(), name)
}

func CreateDeckContext(ctx context.Context, name string) (deck *riff.Deck, err error) {
	if ctx == nil {
		return nil, errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return nil, err
	}
	defer deckLock.Unlock()
	return createDeckContext(ctx, name)
}

func createDeck(name string) (deck *riff.Deck, err error) {
	return createDeckContext(context.Background(), name)
}

func createDeckContext(ctx context.Context, name string) (deck *riff.Deck, err error) {
	if err = waitForSyncingStoragesContext(ctx); err != nil {
		return nil, err
	}

	deckID := ast.NewNodeID()
	deck, err = createDeck0(name, deckID)
	return
}

func createDeck0(name string, deckID string) (deck *riff.Deck, err error) {
	riffSavePath := getRiffDir()
	deck, err = loadLegacyFlashcardDeck(riffSavePath, deckID, Conf.Flashcard.RequestRetention, Conf.Flashcard.MaximumInterval, Conf.Flashcard.Weights)
	if err != nil {
		logging.LogErrorf("load deck [%s] failed: %s", deckID, err)
		return
	}
	deck.Name = name
	Decks[deckID] = deck
	err = deck.Save()
	if err != nil {
		logging.LogErrorf("save deck [%s] failed: %s", deckID, err)
		return
	}
	return
}

func GetDecks() (decks []*riff.Deck) {
	decks, err := GetDecksContext(context.Background())
	if err != nil {
		logging.LogErrorf("flashcard decks: %s", err)
		return []*riff.Deck{}
	}
	return decks
}

func GetDecksContext(ctx context.Context) (decks []*riff.Deck, err error) {
	if ctx == nil {
		return nil, errors.New("flashcard_context_unbound")
	}
	if err = deckLock.LockContext(ctx); err != nil {
		return nil, err
	}
	defer deckLock.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	for _, deck := range Decks {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if deck.ID == builtinDeckID {
			continue
		}

		decks = append(decks, deck)
	}
	if 1 > len(decks) {
		decks = []*riff.Deck{}
	}

	sort.Slice(decks, func(i, j int) bool {
		return decks[i].Updated > decks[j].Updated
	})
	return decks, ctx.Err()
}

func getRiffDir() string {
	return filepath.Join(util.DataDir, "storage", "riff")
}

func getDeckIDs() (deckIDs []string) {
	for deckID := range Decks {
		deckIDs = append(deckIDs, deckID)
	}
	return
}

func getDeckDueCards(deck *riff.Deck, reviewedCardIDs, blockIDs []string, newCardLimit, reviewCardLimit, reviewMode int) ([]riff.Card, int, int, int) {
	return legacyFlashcardRound(deck.ID).Due(deck, reviewedCardIDs, blockIDs, newCardLimit, reviewCardLimit, reviewMode, treenode.ExistBlockTrees)
}
