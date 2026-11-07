// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/flashcard.go round selection; caches now belong to the owner.
package flashcard

import (
	"slices"
	"wiki-kernel/internal/flashcard/riff"
)

type Round struct {
	Reviewed, Skipped map[string]riff.Card
	ExpectedCards     [32]byte // source afterimage owned by an external round
}

func NewRound() *Round {
	return &Round{Reviewed: map[string]riff.Card{}, Skipped: map[string]riff.Card{}}
}
func contains(s string, list []string) bool { return slices.Contains(list, s) }
func (round *Round) Due(deck *riff.Deck, reviewedCardIDs, blockIDs []string, newCardLimit, reviewCardLimit, reviewMode int, exists func([]string) map[string]bool) (ret []riff.Card, unreviewedCount, unreviewedNewCardCountInRound, unreviewedOldCardCountInRound int) {
	ret = []riff.Card{}
	var retNew, retOld []riff.Card

	dues := deck.Dues()
	toChecks := map[string]riff.Card{}
	for _, c := range dues {
		if 0 < len(blockIDs) && !contains(c.BlockID(), blockIDs) {
			continue
		}

		toChecks[c.BlockID()] = c
	}
	var toCheckBlockIDs []string
	var tmp []riff.Card
	for bID, _ := range toChecks {
		toCheckBlockIDs = append(toCheckBlockIDs, bID)
	}
	checkResult := exists(toCheckBlockIDs)
	for bID, exists := range checkResult {
		if exists {
			tmp = append(tmp, toChecks[bID])
		}
	}
	dues = tmp

	reviewedCardCount := len(reviewedCardIDs)
	if 1 > reviewedCardCount {
		// 未传入已复习的卡片 ID，说明是开始新的复习，需要清空缓存
		round.Reviewed = map[string]riff.Card{}
		round.Skipped = map[string]riff.Card{}
		round.ExpectedCards = [32]byte{}
	}

	newCount := 0
	reviewCount := 0
	for _, reviewedCard := range round.Reviewed {
		if riff.New == reviewedCard.GetState() {
			newCount++
		} else {
			reviewCount++
		}
	}

	for _, c := range dues {
		if nil != round.Skipped[c.ID()] {
			continue
		}

		if 0 < len(reviewedCardIDs) {
			if !contains(c.ID(), reviewedCardIDs) {
				unreviewedCount++
				if riff.New == c.GetState() {
					if newCount < newCardLimit {
						unreviewedNewCardCountInRound++
					}
				} else {
					if reviewCount < reviewCardLimit {
						unreviewedOldCardCountInRound++
					}
				}
			}
		} else {
			unreviewedCount++
			if riff.New == c.GetState() {
				if newCount < newCardLimit {
					unreviewedNewCardCountInRound++
				}
			} else {
				if reviewCount < reviewCardLimit {
					unreviewedOldCardCountInRound++
				}
			}
		}

		if riff.New == c.GetState() {
			if newCount >= newCardLimit {
				continue
			}

			newCount++
			retNew = append(retNew, c)
		} else {
			if reviewCount >= reviewCardLimit {
				continue
			}

			reviewCount++
			retOld = append(retOld, c)
		}

		ret = append(ret, c)
	}

	switch reviewMode {
	case 1: // 优先复习新卡
		ret = nil
		ret = append(ret, retNew...)
		ret = append(ret, retOld...)
	case 2: // 优先复习旧卡
		ret = nil
		ret = append(ret, retOld...)
		ret = append(ret, retNew...)
	}
	return
}
