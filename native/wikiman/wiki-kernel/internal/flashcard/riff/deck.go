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
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// Deck 描述了一套闪卡包。
type Deck struct {
	ID      string // ID
	Name    string // 名称
	Algo    Algo   // 间隔重复算法
	Desc    string // 描述
	Created int64  // 创建时间
	Updated int64  // 更新时间

	store   Store // 底层存储
	lock    *sync.Mutex
	storage Storage
	clock   func() time.Time
}

// Snapshot preserves the original format without Save's timestamp/write effects.
// The owner serializes access to card values and metadata during this call.
func (deck *Deck) Snapshot() (metadata, cards []byte, err error) {
	deck.lock.Lock()
	defer deck.lock.Unlock()
	metadata, err = msgpack.Marshal(deck)
	if err != nil {
		return
	}
	values := map[string]*FSRSCard{}
	for _, c := range deck.store.GetCardsByBlockIDs(deck.store.GetBlockIDs()) {
		values[c.ID()] = c.Clone().(*FSRSCard)
	}
	cards, err = msgpack.Marshal(values)
	return
}

// LoadDeck reads the original deck/card format through explicit owner storage.
func LoadDeck(storage Storage, id string, requestRetention float64, maximumInterval int, weights string, clock func() time.Time) (deck *Deck, err error) {
	if storage == nil {
		return nil, errors.New("flashcard_storage_unbound")
	}
	if clock == nil {
		clock = time.Now
	}
	created := clock().UnixMilli()
	deck = &Deck{
		ID:      id,
		Name:    id,
		Algo:    AlgoFSRS,
		Created: created,
		Updated: created,
		lock:    &sync.Mutex{},
		storage: storage, clock: clock,
	}

	data, readErr := storage.Read(id + ".deck")
	if readErr != nil && !isStorageMissing(readErr) {
		return nil, readErr
	}
	if readErr == nil {
		if err = msgpack.Unmarshal(data, deck); err != nil {
			return nil, err
		}
		if deck.ID != id {
			return nil, errors.New("flashcard_deck_identity")
		}
	}

	var store Store
	switch deck.Algo {
	case AlgoFSRS:
		store = NewFSRSStore(deck.ID, storage, requestRetention, maximumInterval, weights, clock)
		err = store.Load()
	default:
		err = errors.New("not supported yet")
		return
	}
	if nil != err {
		return
	}
	deck.store = store
	return
}

// AddCard 新建一张闪卡。
func (deck *Deck) AddCard(cardID, blockID string) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	card := deck.store.GetCard(cardID)
	if nil != card {
		return
	}

	deck.store.AddCard(cardID, blockID)
	deck.Updated = deck.clock().UnixMilli()
}

// RemoveCard 删除一张闪卡。
func (deck *Deck) RemoveCard(cardID string) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	deck.store.RemoveCard(cardID)
	deck.Updated = deck.clock().UnixMilli()
}

// SetCard 设置一张闪卡。
func (deck *Deck) SetCard(card Card) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	deck.store.SetCard(card)
}

// GetCard 根据闪卡 ID 获取对应的闪卡。
func (deck *Deck) GetCard(cardID string) Card {
	deck.lock.Lock()
	defer deck.lock.Unlock()
	return deck.store.GetCard(cardID)
}

func (deck *Deck) GetCardsByBlockID(blockID string) (ret []Card) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.GetCardsByBlockID(blockID)
}

// GetCardsByBlockIDs 获取指定内容块的所有卡片。
func (deck *Deck) GetCardsByBlockIDs(blockIDs []string) (ret []Card) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.GetCardsByBlockIDs(blockIDs)
}

func (deck *Deck) GetNewCardsByBlockIDs(blockIDs []string) (ret []Card) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.GetNewCardsByBlockIDs(blockIDs)
}

func (deck *Deck) GetDueCardsByBlockIDs(blockIDs []string) (ret []Card) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.GetDueCardsByBlockIDs(blockIDs)
}

// GetBlockIDs 获取所有内容块 ID。
func (deck *Deck) GetBlockIDs() (ret []string) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.GetBlockIDs()
}

// CountCards 获取卡包中的闪卡数量。
func (deck *Deck) CountCards() int {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.CountCards()
}

// Save 保存闪卡包。
func (deck *Deck) Save() error {
	deck.lock.Lock()
	defer deck.lock.Unlock()
	deck.Updated = deck.clock().UnixMilli()
	if err := deck.store.Save(); err != nil {
		return err
	}
	data, err := msgpack.Marshal(deck)
	if err != nil {
		return err
	}
	return deck.storage.Write(deck.ID+".deck", data)
}

// SaveLog 保存闪卡包的复习日志。
func (deck *Deck) SaveLog(log *Log) (err error) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	return deck.store.SaveLog(log)
}

// Review 复习一张闪卡，rating 为复习评分结果。
func (deck *Deck) Review(cardID string, rating Rating) (ret *Log) {
	deck.lock.Lock()
	defer deck.lock.Unlock()

	ret = deck.store.Review(cardID, rating)
	deck.Updated = deck.clock().UnixMilli()
	return
}

// Dues 返回所有到期的闪卡。
func (deck *Deck) Dues() (ret []Card) {
	deck.lock.Lock()
	defer deck.lock.Unlock()
	return deck.store.Dues()
}
