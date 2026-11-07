// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/flashcard.go add/remove/reset business rules, with explicit ports.
package flashcard

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

var ErrDependency = errors.New("flashcard_dependencies_unbound")
var ErrSyncing = errors.New("flashcard_data_syncing")

type MutationRequest struct {
	Action, DeckID, Scope, ID string
	BlockIDs                  []string
}
type AttributeEffect struct {
	ID       string
	Old, New map[string]string
	AVType   string
}
type MutationPlan struct {
	Trees        []*parse.Tree
	Decks        map[string]*riff.Deck
	Attributes   []AttributeEffect
	Changes      []Change
	SourceHashes map[string][32]byte
}

// Decks and Load transfer detached objects to this request. Apply stages the
// entire plan in one owner; it must not publish cache/notifications on failure.
type MutationServices struct {
	Decks         func(context.Context) (map[string]*riff.Deck, error)
	CreateBuiltin func(context.Context) (*riff.Deck, error)
	Load          func(context.Context, string) (*parse.Tree, error)
	Select        func(context.Context, string, string) ([]string, error)
	NewID         func() string
	Before        func(context.Context) error
	Apply         func(context.Context, MutationPlan) error
}

func NewDeck(id, name string, c conf.Flashcard, clock func() time.Time) (*riff.Deck, error) {
	if !validID(id) {
		return nil, ErrInvalid
	}
	d, e := riff.LoadDeck(memory{}, id, c.RequestRetention, c.MaximumInterval, c.Weights, clock)
	if e == nil {
		d.Name = name
	}
	return d, e
}

// Reset only existing memberships. In All mode each deck gets its own block
// intersection, preventing the former loop's deckID carry-over and phantom cards.
func ResetTargets(ctx context.Context, q MutationRequest, decks map[string]*riff.Deck, selectIDs func(context.Context, string, string) ([]string, error)) (map[string][]string, error) {
	if ctx == nil {
		return nil, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	ids := slices.Clone(q.BlockIDs)
	if len(ids) == 0 {
		if q.Scope == "deck" {
			d := decks[q.ID]
			if d == nil {
				return nil, fs.ErrNotExist
			}
			if q.DeckID != "" && q.DeckID != q.ID {
				return nil, ErrInvalid
			}
			q.DeckID = q.ID
			ids = d.GetBlockIDs()
		} else {
			if selectIDs == nil {
				return nil, ErrDependency
			}
			var e error
			ids, e = selectIDs(ctx, q.Scope, q.ID)
			if e != nil {
				return nil, e
			}
		}
	}
	out := map[string][]string{}
	if q.DeckID != "" && decks[q.DeckID] == nil {
		return nil, fs.ErrNotExist
	}
	for id, d := range decks {
		if q.DeckID != "" && id != q.DeckID {
			continue
		}
		seen := map[string]bool{}
		for _, block := range ids {
			if !validID(block) {
				return nil, ErrInvalid
			}
			if !seen[block] && len(d.GetCardsByBlockID(block)) > 0 {
				seen[block] = true
				out[id] = append(out[id], block)
			}
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
func CloneDeck(d *riff.Deck, c conf.Flashcard, clock func() time.Time) (*riff.Deck, error) {
	if d == nil {
		return nil, ErrInvalid
	}
	metadata, cards, e := d.Snapshot()
	if e != nil {
		return nil, e
	}
	m := memory{d.ID + ".deck": metadata, d.ID + ".cards": cards}
	return riff.LoadDeck(m, d.ID, c.RequestRetention, c.MaximumInterval, c.Weights, clock)
}
func PrepareMutation(ctx context.Context, q MutationRequest, s MutationServices) (out MutationPlan, err error) {
	if ctx == nil || s.Decks == nil || s.Load == nil || s.NewID == nil {
		return out, ErrDependency
	}
	if q.Action != "add" && q.Action != "remove" && q.Action != "reset" {
		return out, ErrInvalid
	}
	if q.DeckID != "" && !validID(q.DeckID) {
		return out, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if s.Before != nil {
		if e := s.Before(ctx); e != nil {
			return out, e
		}
	}
	decks, e := s.Decks(ctx)
	if e != nil {
		return out, e
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	for id, d := range decks {
		if d == nil || d.ID != id || !validID(id) {
			return out, ErrInvalid
		}
	}
	ids := slices.Clone(q.BlockIDs)
	if q.Action == "reset" {
		if len(ids) == 0 && q.Scope == "deck" {
			if q.DeckID != "" && q.DeckID != q.ID {
				return out, ErrInvalid
			}
			q.DeckID = q.ID
		}
		targets, e := ResetTargets(ctx, q, decks, s.Select)
		if e != nil {
			return out, e
		}
		keys := []string{}
		for id := range targets {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		ids = nil
		for _, id := range keys {
			ids = append(ids, targets[id]...)
		}
	}
	ids = slices.CompactFunc(ids, func(a, b string) bool { return a == b })
	seenIDs := map[string]bool{}
	unique := ids[:0]
	for _, id := range ids {
		if !validID(id) {
			return out, ErrInvalid
		}
		if !seenIDs[id] {
			seenIDs[id] = true
			unique = append(unique, id)
		}
	}
	ids = unique
	if q.Action == "add" {
		if q.DeckID == "" {
			q.DeckID = BuiltinDeckID
		}
		if decks[q.DeckID] == nil {
			if q.DeckID != BuiltinDeckID {
				return out, fs.ErrNotExist
			}
			if s.CreateBuiltin == nil {
				return out, ErrDependency
			}
			d, e := s.CreateBuiltin(ctx)
			if e != nil {
				return out, e
			}
			if d == nil || d.ID != BuiltinDeckID {
				return out, ErrInvalid
			}
			decks[q.DeckID] = d
		}
	}
	if q.DeckID != "" && decks[q.DeckID] == nil {
		return out, fs.ErrNotExist
	}
	out.Decks = map[string]*riff.Deck{}
	if q.Action == "add" {
		out.Decks[q.DeckID] = decks[q.DeckID]
	}
	owned := map[string]*parse.Tree{}
	order := []string{}
	effects := map[string]AttributeEffect{}
	deckIDs := []string{}
	for id := range decks {
		if q.DeckID == "" || id == q.DeckID {
			deckIDs = append(deckIDs, id)
		}
	}
	sort.Strings(deckIDs)
	for _, id := range ids {
		if e = ctx.Err(); e != nil {
			return MutationPlan{}, e
		}
		t, e := s.Load(ctx, id)
		if e != nil {
			return MutationPlan{}, e
		}
		if e = ctx.Err(); e != nil {
			return MutationPlan{}, e
		}
		if t == nil {
			continue
		}
		if t.Root == nil || t.ID == "" || t.Root.ID != t.ID || t.Box == "" || t.Path == "" {
			return MutationPlan{}, ErrInvalid
		}
		if previous := owned[t.ID]; previous != nil {
			if previous.Box != t.Box || previous.Path != t.Path {
				return MutationPlan{}, ErrInvalid
			}
			t = previous
		} else {
			owned[t.ID] = t
			order = append(order, t.ID)
		}
		var n *ast.Node
		ast.Walk(t.Root, func(node *ast.Node, enter bool) ast.WalkStatus {
			if enter && node.ID == id {
				n = node
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		if n == nil {
			continue
		}
		old := parse.IAL2Map(n.KramdownIAL)
		members := []string{}
		for _, member := range strings.Split(n.IALAttr("custom-riff-decks"), ",") {
			if member != "" && !slices.Contains(members, member) {
				members = append(members, member)
			}
		}
		changed := false
		for _, deckID := range deckIDs {
			d := decks[deckID]
			cards := d.GetCardsByBlockID(id)
			if q.Action == "reset" && len(cards) == 0 {
				continue
			}
			switch q.Action {
			case "add":
				if !slices.Contains(members, deckID) {
					members = append(members, deckID)
				}
				if len(cards) == 0 {
					cid := s.NewID()
					if !validID(cid) || d.GetCard(cid) != nil {
						return MutationPlan{}, ErrInvalid
					}
					d.AddCard(cid, id)
				}
				changed = true
			case "remove": // Orphaned cards are also removed below, independently of tree availability.
			case "reset":
				for _, c := range cards {
					d.RemoveCard(c.ID())
				}
				cid := s.NewID()
				if !validID(cid) || d.GetCard(cid) != nil {
					return MutationPlan{}, ErrInvalid
				}
				d.AddCard(cid, id)
				if !slices.Contains(members, deckID) {
					members = append(members, deckID)
				}
				changed = true
			}
			if changed {
				out.Decks[deckID] = d
			}
			changed = false
		}
		if q.Action == "remove" {
			kept := members[:0]
			for _, member := range members {
				if q.DeckID != "" && member != q.DeckID && decks[member] != nil {
					kept = append(kept, member)
				}
			}
			members = kept
		}
		if len(members) == 0 {
			n.RemoveIALAttr("custom-riff-decks")
		} else {
			n.SetIALAttr("custom-riff-decks", strings.Join(members, ","))
		}
		effects[id] = AttributeEffect{ID: id, Old: maps.Clone(old), New: parse.IAL2Map(n.KramdownIAL), AVType: n.AttributeViewType}
	}
	if q.Action == "remove" {
		for _, deckID := range deckIDs {
			d := decks[deckID]
			cards := d.GetCardsByBlockIDs(ids)
			for _, card := range cards {
				d.RemoveCard(card.ID())
			}
			if len(cards) > 0 {
				out.Decks[deckID] = d
			}
		}
	}
	for _, id := range order {
		out.Trees = append(out.Trees, owned[id])
	}
	for _, id := range ids {
		if effect, ok := effects[id]; ok {
			out.Attributes = append(out.Attributes, effect)
		}
	}
	for _, id := range deckIDs {
		if d := out.Decks[id]; d != nil {
			if e = d.Save(); e != nil {
				return MutationPlan{}, e
			}
		}
	}
	if e = ctx.Err(); e != nil {
		return MutationPlan{}, e
	}
	return out, nil
}
func ExecuteMutation(ctx context.Context, q MutationRequest, s MutationServices) (MutationPlan, error) {
	if s.Apply == nil {
		return MutationPlan{}, ErrDependency
	}
	p, e := PrepareMutation(ctx, q, s)
	if e != nil {
		return MutationPlan{}, e
	}
	if e = ctx.Err(); e != nil {
		return MutationPlan{}, e
	}
	if e = s.Apply(ctx, p); e != nil {
		return MutationPlan{}, e
	}
	if e = ctx.Err(); e != nil {
		return MutationPlan{}, e
	}
	return p, nil
}
