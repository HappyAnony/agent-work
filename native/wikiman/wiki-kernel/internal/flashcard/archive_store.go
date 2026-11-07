package flashcard

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"io/fs"
	"sort"
	"wiki-kernel/internal/flashcard/riff"
)

func (s *Store) PlanArchiveExport(ctx context.Context, trees []*parse.Tree) (out ArchivePlan, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		var e error
		out, e = PrepareExport(ctx, trees, ArchiveServices{Settings: s.settings, Now: s.clock, Cards: func(_ context.Context, ids []string) ([]riff.Card, error) {
			d := s.decks[BuiltinDeckID]
			if d == nil {
				return nil, nil
			}
			return d.GetCardsByBlockIDs(ids), nil
		}})
		return e
	})
	if err != nil {
		return ArchivePlan{}, err
	}
	return
}
func (s *Store) PlanArchiveImport(ctx context.Context, files map[string][]byte, idMap map[string]string, trees []*parse.Tree, newID func() string) (out ArchivePlan, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		var e error
		out, e = PrepareImport(ctx, files, idMap, trees, ArchiveServices{Settings: s.settings, Now: s.clock, NewID: newID, Destination: func(ctx context.Context) (*riff.Deck, error) {
			if d := s.decks[BuiltinDeckID]; d != nil {
				return d, nil
			}
			for _, ext := range []string{".deck", ".cards"} {
				_, e := s.source.Read(ctx, BuiltinDeckID+ext)
				if e == nil {
					return nil, ErrReviewConflict
				}
				if !errors.Is(e, fs.ErrNotExist) {
					return nil, e
				}
			}
			return nil, nil
		}})
		if e != nil {
			return e
		}
		paths := []string{}
		for name := range out.Files {
			paths = append(paths, name)
		}
		sort.Strings(paths)
		for _, name := range paths {
			p := name[len(archiveBase):]
			old, exists := s.files[p]
			after := out.Files[name]
			if !exists || !bytes.Equal(old, after) {
				out.Changes = append(out.Changes, Change{Path: name, Before: bytes.Clone(old), After: bytes.Clone(after), Existed: exists})
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return ArchivePlan{}, err
	}
	return
}
