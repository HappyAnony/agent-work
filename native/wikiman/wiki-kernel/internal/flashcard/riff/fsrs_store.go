// Riff - Spaced repetition.
// Copyright (c) 2022-present, b3log.org
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

package riff

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/88250/gulu"
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"github.com/vmihailenco/msgpack/v5"
)

type FSRSStore struct {
	*BaseStore

	cards     map[string]*FSRSCard
	scheduler fsrs.FSRS
}

func NewFSRSStore(id string, storage Storage, requestRetention float64, maximumInterval int, weights string, clock func() time.Time) *FSRSStore {
	params := fsrs.DefaultParam()
	params.RequestRetention = requestRetention
	params.MaximumInterval = float64(maximumInterval)
	params.W = [19]float64{0}
	for i, w := range strings.Split(weights, ",") {
		w = strings.TrimSpace(w)
		if i >= len(params.W) {
			break
		}
		params.W[i], _ = strconv.ParseFloat(w, 64)
	}
	scheduler := *fsrs.NewFSRS(params)

	return &FSRSStore{
		BaseStore: NewBaseStore(id, "fsrs", storage, clock),
		cards:     map[string]*FSRSCard{},
		scheduler: scheduler,
	}
}

func (store *FSRSStore) AddCard(id, blockID string) Card {
	store.lock.Lock()
	defer store.lock.Unlock()

	c := fsrs.NewCard()
	c.Due = store.clock()
	card := &FSRSCard{BaseCard: &BaseCard{id, blockID, nil}, C: &c}
	store.cards[id] = card
	return card
}

func (store *FSRSStore) GetCard(id string) Card {
	store.lock.Lock()
	defer store.lock.Unlock()

	ret := store.cards[id]
	if nil == ret {
		return nil
	}
	return ret
}

func (store *FSRSStore) SetCard(card Card) {
	store.lock.Lock()
	defer store.lock.Unlock()

	store.cards[card.ID()] = card.(*FSRSCard)
}

func (store *FSRSStore) RemoveCard(id string) Card {
	store.lock.Lock()
	defer store.lock.Unlock()

	card := store.cards[id]
	if nil == card {
		return nil
	}
	delete(store.cards, id)
	return card
}

func (store *FSRSStore) GetCardsByBlockID(blockID string) (ret []Card) {
	store.lock.Lock()
	defer store.lock.Unlock()

	for _, card := range store.cards {
		if card.BlockID() == blockID {
			ret = append(ret, card)
		}
	}
	return
}

func (store *FSRSStore) GetCardsByBlockIDs(blockIDs []string) (ret []Card) {
	store.lock.Lock()
	defer store.lock.Unlock()

	blockIDs = gulu.Str.RemoveDuplicatedElem(blockIDs)
	for _, card := range store.cards {
		if gulu.Str.Contains(card.BlockID(), blockIDs) {
			ret = append(ret, card)
		}
	}
	return
}

func (store *FSRSStore) GetNewCardsByBlockIDs(blockIDs []string) (ret []Card) {
	store.lock.Lock()
	defer store.lock.Unlock()

	blockIDs = gulu.Str.RemoveDuplicatedElem(blockIDs)
	for _, card := range store.cards {
		c := card.Impl().(*fsrs.Card)
		if !c.LastReview.IsZero() {
			continue
		}

		if gulu.Str.Contains(card.BlockID(), blockIDs) {
			ret = append(ret, card)
		}
	}
	return
}

func (store *FSRSStore) GetDueCardsByBlockIDs(blockIDs []string) (ret []Card) {
	store.lock.Lock()
	defer store.lock.Unlock()

	blockIDs = gulu.Str.RemoveDuplicatedElem(blockIDs)
	now := store.clock()
	for _, card := range store.cards {
		c := card.Impl().(*fsrs.Card)
		if now.Before(c.Due) {
			continue
		}

		if gulu.Str.Contains(card.BlockID(), blockIDs) {
			ret = append(ret, card)
		}
	}
	return
}

func (store *FSRSStore) GetBlockIDs() (ret []string) {
	store.lock.Lock()
	defer store.lock.Unlock()

	ret = []string{}
	for _, card := range store.cards {
		ret = append(ret, card.BlockID())
	}
	ret = gulu.Str.RemoveDuplicatedElem(ret)
	sort.Strings(ret)
	return
}

func (store *FSRSStore) CountCards() int {
	store.lock.Lock()
	defer store.lock.Unlock()

	return len(store.cards)
}

func (store *FSRSStore) Review(cardId string, rating Rating) (ret *Log) {
	store.lock.Lock()
	defer store.lock.Unlock()

	now := store.clock()
	card := store.cards[cardId]
	if nil == card {
		return
	}

	schedulingInfo := store.scheduler.Repeat(*card.C, now)
	updated := schedulingInfo[fsrs.Rating(rating)].Card
	card.SetImpl(&updated)
	store.cards[cardId] = card

	reviewLog := schedulingInfo[fsrs.Rating(rating)].ReviewLog
	ret = &Log{
		ID:            newID(now),
		CardID:        cardId,
		Rating:        rating,
		ScheduledDays: reviewLog.ScheduledDays,
		ElapsedDays:   reviewLog.ElapsedDays,
		Reviewed:      reviewLog.Review.Unix(),
		State:         State(reviewLog.State),
	}
	return
}

func (store *FSRSStore) Dues() (ret []Card) {
	store.lock.Lock()
	defer store.lock.Unlock()

	now := store.clock()
	for _, card := range store.cards {
		c := card.Impl().(*fsrs.Card)
		if now.Before(c.Due) {
			continue
		}

		schedulingInfos := store.scheduler.Repeat(*c, now)
		nextDues := map[Rating]time.Time{}
		for rating, schedulingInfo := range schedulingInfos {
			nextDues[Rating(rating)] = schedulingInfo.Card.Due
		}
		card.SetNextDues(nextDues)
		ret = append(ret, card)
	}
	return
}

func (store *FSRSStore) Load() error {
	store.lock.Lock()
	defer store.lock.Unlock()
	cards := map[string]*FSRSCard{}
	data, err := store.storage.Read(store.getMsgPackPath())
	if isStorageMissing(err) {
		store.cards = cards
		return nil
	}
	if err != nil {
		return err
	}
	if err = msgpack.Unmarshal(data, &cards); err != nil {
		return err
	}
	for id, c := range cards {
		if c == nil || c.BaseCard == nil || c.C == nil || c.CID != id || c.BID == "" {
			return errors.New("flashcard_card_identity")
		}
	}
	if cards == nil {
		cards = map[string]*FSRSCard{}
	}
	store.cards = cards
	return nil
}

func (store *FSRSStore) Save() error {
	store.lock.Lock()
	defer store.lock.Unlock()
	data, err := msgpack.Marshal(store.cards)
	if err != nil {
		return err
	}
	return store.storage.Write(store.getMsgPackPath(), data)
}

func (store *FSRSStore) SaveLog(log *Log) error {
	store.lock.Lock()
	defer store.lock.Unlock()
	p := "logs/" + store.clock().Format("200601") + ".msgpack"
	logs := []*Log{}
	data, err := store.storage.Read(p)
	if err != nil && !isStorageMissing(err) {
		return err
	}
	if err == nil {
		if err = msgpack.Unmarshal(data, &logs); err != nil {
			return err
		}
	}
	logs = append(logs, log)
	data, err = msgpack.Marshal(logs)
	if err != nil {
		return err
	}
	return store.storage.Write(p, data)
}

type FSRSCard struct {
	*BaseCard
	C *fsrs.Card
}

func (card *FSRSCard) Impl() interface{} {
	return card.C
}

func (card *FSRSCard) SetImpl(c interface{}) {
	card.C = c.(*fsrs.Card)
}

func (card *FSRSCard) GetLapses() int {
	return int(card.C.Lapses)
}

func (card *FSRSCard) GetReps() int {
	return int(card.C.Reps)
}

func (card *FSRSCard) GetState() State {
	return State(card.C.State)
}

func (card *FSRSCard) GetLastReview() time.Time {
	return card.C.LastReview
}

func (card *FSRSCard) Clone() Card {
	base := *card.BaseCard
	c := *card.C
	base.NDues = map[Rating]time.Time{}
	for r, t := range card.NDues {
		base.NDues[r] = t
	}
	return &FSRSCard{BaseCard: &base, C: &c}
}

func (card *FSRSCard) SetDue(due time.Time) {
	card.C.Due = due
}
