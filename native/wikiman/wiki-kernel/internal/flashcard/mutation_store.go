package flashcard

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"sort"
	"wiki-kernel/internal/flashcard/riff"
)

// PlanMutation stages card files and tree/attribute effects together. The caller
// must commit them as one source transaction before publishing the effects.
func (s *Store) PlanMutation(ctx context.Context, q MutationRequest, ports MutationServices) (out MutationPlan, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		working := clone(s.files)
		ports.Decks = func(context.Context) (map[string]*riff.Deck, error) {
			out := map[string]*riff.Deck{}
			for id := range s.decks {
				d, e := s.decode(working, id)
				if e != nil {
					return nil, e
				}
				out[id] = d
			}
			return out, nil
		}
		ports.CreateBuiltin = func(ctx context.Context) (*riff.Deck, error) {
			for _, ext := range []string{".deck", ".cards"} {
				_, e := s.source.Read(ctx, BuiltinDeckID+ext)
				if e == nil {
					return nil, ErrReviewConflict
				}
				if !errors.Is(e, fs.ErrNotExist) {
					return nil, e
				}
			}
			return NewDeck(BuiltinDeckID, "Built-in Deck", s.settings, s.clock)
		}
		p, e := PrepareMutation(ctx, q, ports)
		if e != nil {
			return e
		}
		ids := []string{}
		for id := range p.Decks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			metadata, cards, e := p.Decks[id].Snapshot()
			if e != nil {
				return e
			}
			for _, file := range []struct {
				p string
				b []byte
			}{{id + ".deck", metadata}, {id + ".cards", cards}} {
				old, exists := s.files[file.p]
				if !exists || !bytes.Equal(old, file.b) {
					p.Changes = append(p.Changes, Change{Path: file.p, Before: bytes.Clone(old), After: bytes.Clone(file.b), Existed: exists})
				}
			}
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		out = p
		return nil
	})
	if err != nil {
		return MutationPlan{}, err
	}
	return
}
