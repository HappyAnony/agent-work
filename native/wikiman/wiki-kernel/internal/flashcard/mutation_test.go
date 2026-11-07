package flashcard

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

const mutationDoc = "20261028000000-doc0001"
const mutationBox = "20261028000000-box0001"
const mutationBlock2 = "20261028000000-block02"

func mutationTree() *parse.Tree {
	root := &ast.Node{ID: mutationDoc, Type: ast.NodeDocument}
	root.SetIALAttr("id", mutationDoc)
	for _, id := range []string{blockID, mutationBlock2} {
		n := &ast.Node{ID: id, Type: ast.NodeParagraph}
		n.SetIALAttr("id", id)
		n.SetIALAttr("custom-riff-decks", BuiltinDeckID+","+otherDeck+",20261028000000-missing")
		root.AppendChild(n)
	}
	return &parse.Tree{ID: mutationDoc, Box: mutationBox, Path: "/" + mutationDoc + ".docj", Root: root}
}
func mutationPorts(t *testing.T) (MutationServices, *int) {
	t.Helper()
	next := 0
	applied := 0
	return MutationServices{
		Decks: func(context.Context) (map[string]*riff.Deck, error) {
			c := *conf.NewFlashcard()
			a, e := NewDeck(BuiltinDeckID, "builtin", c, time.Now)
			if e != nil {
				return nil, e
			}
			b, e := NewDeck(otherDeck, "other", c, time.Now)
			if e != nil {
				return nil, e
			}
			a.AddCard(cardID, blockID)
			b.AddCard("20261028000000-card002", mutationBlock2)
			return map[string]*riff.Deck{BuiltinDeckID: a, otherDeck: b}, nil
		},
		Load: func(context.Context, string) (*parse.Tree, error) { return mutationTree(), nil }, NewID: func() string {
			next++
			return []string{"20261028000001-card001", "20261028000002-card002", "20261028000003-card003", "20261028000004-card004"}[next-1]
		},
		Select: func(context.Context, string, string) ([]string, error) { return []string{blockID, mutationBlock2}, nil },
		Apply:  func(context.Context, MutationPlan) error { applied++; return nil },
	}, &applied
}
func TestFlashcardMutationSharedTreeAndResetMemberships(t *testing.T) {
	ctx := context.Background()
	ports, applied := mutationPorts(t)
	p, e := ExecuteMutation(ctx, MutationRequest{Action: "add", DeckID: BuiltinDeckID, BlockIDs: []string{blockID, mutationBlock2, blockID}}, ports)
	if e != nil || len(p.Trees) != 1 || len(p.Attributes) != 2 || p.Decks[BuiltinDeckID].CountCards() != 2 || *applied != 1 {
		t.Fatal(p, e, *applied)
	}
	ports, _ = mutationPorts(t)
	p, e = PrepareMutation(ctx, MutationRequest{Action: "remove", DeckID: BuiltinDeckID, BlockIDs: []string{blockID}}, ports)
	if e != nil || p.Decks[BuiltinDeckID].CountCards() != 0 || p.Attributes[0].New["custom-riff-decks"] != otherDeck {
		t.Fatal(p, e)
	}
	ports, _ = mutationPorts(t)
	p, e = PrepareMutation(ctx, MutationRequest{Action: "remove", BlockIDs: []string{blockID, mutationBlock2}}, ports)
	if e != nil || len(p.Decks) != 2 || p.Attributes[0].New["custom-riff-decks"] != "" || p.Attributes[1].New["custom-riff-decks"] != "" {
		t.Fatal(p, e)
	}
	ports, _ = mutationPorts(t)
	p, e = PrepareMutation(ctx, MutationRequest{Action: "reset", BlockIDs: []string{blockID}}, ports)
	if e != nil || len(p.Decks) != 1 || p.Decks[otherDeck] != nil || p.Decks[BuiltinDeckID].CountCards() != 1 || p.Decks[BuiltinDeckID].GetCard(cardID) != nil {
		t.Fatal("reset created phantom membership", p, e)
	}
	ports, _ = mutationPorts(t)
	p, e = PrepareMutation(ctx, MutationRequest{Action: "reset", Scope: "deck", ID: otherDeck}, ports)
	if e != nil || len(p.Decks) != 1 || p.Decks[BuiltinDeckID] != nil || p.Decks[otherDeck].CountCards() != 1 {
		t.Fatal("scope escaped deck", p, e)
	}
	ports, _ = mutationPorts(t)
	decks, _ := ports.Decks(ctx)
	targets, e := ResetTargets(ctx, MutationRequest{BlockIDs: []string{blockID}}, decks, nil)
	if e != nil || !reflect.DeepEqual(targets, map[string][]string{BuiltinDeckID: {blockID}}) {
		t.Fatal(targets, e)
	}
}
func TestFlashcardMutationDependenciesCancellationAndFailure(t *testing.T) {
	ctx := context.Background()
	ports, applied := mutationPorts(t)
	sentinel := errors.New("source stage failed")
	ports.Apply = func(context.Context, MutationPlan) error { return sentinel }
	if _, e := ExecuteMutation(ctx, MutationRequest{Action: "add", DeckID: BuiltinDeckID, BlockIDs: []string{blockID}}, ports); !errors.Is(e, sentinel) || *applied != 0 {
		t.Fatal(e, *applied)
	}
	ports, applied = mutationPorts(t)
	cancelled, cancel := context.WithCancel(ctx)
	ports.Load = func(context.Context, string) (*parse.Tree, error) { cancel(); return mutationTree(), nil }
	if _, e := ExecuteMutation(cancelled, MutationRequest{Action: "add", DeckID: BuiltinDeckID, BlockIDs: []string{blockID}}, ports); !errors.Is(e, context.Canceled) || *applied != 0 {
		t.Fatal(e, *applied)
	}
	if _, e := PrepareMutation(ctx, MutationRequest{Action: "add"}, MutationServices{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	ports, _ = mutationPorts(t)
	ports.Load = func(context.Context, string) (*parse.Tree, error) { return nil, nil }
	p, e := PrepareMutation(ctx, MutationRequest{Action: "remove", DeckID: BuiltinDeckID, BlockIDs: []string{blockID}}, ports)
	if e != nil || len(p.Trees) != 0 || p.Decks[BuiltinDeckID].CountCards() != 0 {
		t.Fatal("orphan cleanup lost", p, e)
	}
}
