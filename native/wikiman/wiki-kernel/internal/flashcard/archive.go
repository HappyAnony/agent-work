// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original ImportSY/exportSYZip flashcard phase with explicit owner ports.
package flashcard

import (
	"bytes"
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"maps"
	"sort"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

const archiveBase = "storage/riff/"

type ArchivePlan struct {
	Files          map[string][]byte
	Changes        []Change
	Trees          []*parse.Tree
	Attributes     []AttributeEffect
	Count, Skipped int
	SourceHashes   map[string][32]byte
}
type ArchiveServices struct {
	Settings    conf.Flashcard
	Now         func() time.Time
	NewID       func() string
	Cards       func(context.Context, []string) ([]riff.Card, error)
	Destination func(context.Context) (*riff.Deck, error)
	ApplyImport func(context.Context, ArchivePlan) error
	WriteExport func(context.Context, ArchivePlan) error
}

func archiveNodes(ctx context.Context, trees []*parse.Tree) (map[string]*ast.Node, error) {
	nodes := map[string]*ast.Node{}
	roots := map[string]bool{}
	for _, t := range trees {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if t == nil || t.Root == nil || t.ID != t.Root.ID || !validID(t.ID) || roots[t.ID] {
			return nil, ErrInvalid
		}
		roots[t.ID] = true
		var walkErr error
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter || !n.IsBlock() {
				return ast.WalkContinue
			}
			if !validID(n.ID) || nodes[n.ID] != nil || len(nodes) >= 100000 {
				walkErr = ErrInvalid
				return ast.WalkStop
			}
			nodes[n.ID] = n
			return ast.WalkContinue
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return nodes, ctx.Err()
}
func snapshotFiles(d *riff.Deck) (map[string][]byte, error) {
	metadata, cards, e := d.Snapshot()
	if e != nil {
		return nil, e
	}
	return map[string][]byte{archiveBase + d.ID + ".deck": metadata, archiveBase + d.ID + ".cards": cards}, nil
}

// Export keeps card IDs and block IDs but resets learning, as the original
// archive did by AddCard into a new builtin deck. Custom decks/logs are excluded.
func PrepareExport(ctx context.Context, trees []*parse.Tree, s ArchiveServices) (ArchivePlan, error) {
	if ctx == nil || s.Now == nil || s.Cards == nil {
		return ArchivePlan{}, ErrDependency
	}
	nodes, e := archiveNodes(ctx, trees)
	if e != nil {
		return ArchivePlan{}, e
	}
	ids := []string{}
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	cards, e := s.Cards(ctx, ids)
	if e != nil {
		return ArchivePlan{}, e
	}
	if e = ctx.Err(); e != nil {
		return ArchivePlan{}, e
	}
	d, e := NewDeck(BuiltinDeckID, BuiltinDeckID, s.Settings, s.Now)
	if e != nil {
		return ArchivePlan{}, e
	}
	out := ArchivePlan{Files: map[string][]byte{}}
	for _, c := range cards {
		if e = ctx.Err(); e != nil {
			return ArchivePlan{}, e
		}
		if c == nil || !validID(c.ID()) || nodes[c.BlockID()] == nil {
			return ArchivePlan{}, ErrInvalid
		}
		d.AddCard(c.ID(), c.BlockID())
	}
	out.Count = d.CountCards()
	if out.Count == 0 {
		return out, nil
	}
	if e = d.Save(); e != nil {
		return ArchivePlan{}, e
	}
	out.Files, e = snapshotFiles(d)
	if e != nil {
		return ArchivePlan{}, e
	}
	return out, ctx.Err()
}

// Trees are detached, already ID-remapped import candidates. Only mappings into
// these actual imported block nodes can receive a card. Unknown source cards are
// skipped rather than becoming empty-ID or external-workspace bindings.
func PrepareImport(ctx context.Context, files map[string][]byte, idMap map[string]string, trees []*parse.Tree, s ArchiveServices) (ArchivePlan, error) {
	if ctx == nil || s.Now == nil || s.NewID == nil || s.Destination == nil {
		return ArchivePlan{}, ErrDependency
	}
	nodes, e := archiveNodes(ctx, trees)
	if e != nil {
		return ArchivePlan{}, e
	}
	idMap = maps.Clone(idMap)
	used := map[string]bool{}
	for old, newID := range idMap {
		if !validID(old) || !validID(newID) || used[newID] {
			return ArchivePlan{}, ErrInvalid
		}
		used[newID] = true
	}
	data := memory{}
	size := 0
	for name, b := range files {
		if name != archiveBase+BuiltinDeckID+".deck" && name != archiveBase+BuiltinDeckID+".cards" {
			return ArchivePlan{}, ErrInvalid
		}
		size += len(b)
		if len(b) > 32<<20 || size > 64<<20 {
			return ArchivePlan{}, ErrInvalid
		}
		data[name[len(archiveBase):]] = bytes.Clone(b)
	}
	out := ArchivePlan{Files: map[string][]byte{}, Trees: trees}
	_, hasDeck := data[BuiltinDeckID+".deck"]
	_, hasCards := data[BuiltinDeckID+".cards"]
	if hasDeck != hasCards {
		return ArchivePlan{}, ErrInvalid
	}
	bound := map[string]bool{}
	var source *riff.Deck
	if hasDeck {
		source, e = riff.LoadDeck(data, BuiltinDeckID, s.Settings.RequestRetention, s.Settings.MaximumInterval, s.Settings.Weights, s.Now)
		if e != nil {
			return ArchivePlan{}, e
		}
	}
	var dest *riff.Deck
	if source != nil {
		ids := source.GetBlockIDs()
		cards := source.GetCardsByBlockIDs(ids)
		sort.Slice(cards, func(i, j int) bool { return cards[i].ID() < cards[j].ID() })
		for _, c := range cards {
			if e = ctx.Err(); e != nil {
				return ArchivePlan{}, e
			}
			target := idMap[c.BlockID()]
			if target == "" {
				out.Skipped++
				continue
			}
			if nodes[target] == nil {
				return ArchivePlan{}, ErrInvalid
			}
			if bound[target] {
				out.Skipped++
				continue
			}
			if dest == nil {
				dest, e = s.Destination(ctx)
				if e != nil {
					return ArchivePlan{}, e
				}
				if e = ctx.Err(); e != nil {
					return ArchivePlan{}, e
				}
				if dest == nil {
					dest, e = NewDeck(BuiltinDeckID, "Built-in Deck", s.Settings, s.Now)
				} else {
					if dest.ID != BuiltinDeckID {
						return ArchivePlan{}, ErrInvalid
					}
					dest, e = CloneDeck(dest, s.Settings, s.Now)
				}
				if e != nil {
					return ArchivePlan{}, e
				}
			}
			if len(dest.GetCardsByBlockID(target)) > 0 {
				return ArchivePlan{}, ErrInvalid
			}
			cid := s.NewID()
			if !validID(cid) || dest.GetCard(cid) != nil {
				return ArchivePlan{}, ErrInvalid
			}
			dest.AddCard(cid, target)
			bound[target] = true
			out.Count++
		}
	}
	if dest != nil {
		if e = dest.Save(); e != nil {
			return ArchivePlan{}, e
		}
		out.Files, e = snapshotFiles(dest)
		if e != nil {
			return ArchivePlan{}, e
		}
	}
	ids := []string{}
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if e = ctx.Err(); e != nil {
			return ArchivePlan{}, e
		}
		n := nodes[id]
		old := parse.IAL2Map(n.KramdownIAL)
		if bound[id] {
			n.SetIALAttr("custom-riff-decks", BuiltinDeckID)
		} else {
			n.RemoveIALAttr("custom-riff-decks")
		}
		after := parse.IAL2Map(n.KramdownIAL)
		if !maps.Equal(old, after) {
			out.Attributes = append(out.Attributes, AttributeEffect{ID: id, Old: old, New: after, AVType: n.AttributeViewType})
		}
	}
	return out, ctx.Err()
}
func ExecuteExport(ctx context.Context, trees []*parse.Tree, s ArchiveServices) (ArchivePlan, error) {
	if s.WriteExport == nil {
		return ArchivePlan{}, ErrDependency
	}
	p, e := PrepareExport(ctx, trees, s)
	if e != nil {
		return ArchivePlan{}, e
	}
	if e = ctx.Err(); e != nil {
		return ArchivePlan{}, e
	}
	if e = s.WriteExport(ctx, p); e != nil {
		return ArchivePlan{}, e
	}
	return p, ctx.Err()
}
func ExecuteImport(ctx context.Context, files map[string][]byte, ids map[string]string, trees []*parse.Tree, s ArchiveServices) (ArchivePlan, error) {
	if s.ApplyImport == nil {
		return ArchivePlan{}, ErrDependency
	}
	p, e := PrepareImport(ctx, files, ids, trees, s)
	if e != nil {
		return ArchivePlan{}, e
	}
	if e = ctx.Err(); e != nil {
		return ArchivePlan{}, e
	}
	if e = s.ApplyImport(ctx, p); e != nil {
		return ArchivePlan{}, e
	}
	return p, ctx.Err()
}
