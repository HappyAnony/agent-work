// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"io/fs"
	"path"
	"slices"
	"time"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var uiFlashcardWrites = map[string]string{
	"/api/riff/createRiffDeck":           "create",
	"/api/riff/renameRiffDeck":           "rename",
	"/api/riff/removeRiffDeck":           "remove-deck",
	"/api/riff/addRiffCards":             "add",
	"/api/riff/removeRiffCards":          "remove",
	"/api/riff/batchSetRiffCardsDueTime": "due-batch",
	"/api/riff/resetRiffCards":           "reset",
}

func runUIFlashcardWrite(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest, action string) (any, error) {
	var input struct {
		Name, DeckID, Type, ID string
		BlockIDs               []string
		CardDues               []struct{ ID, Due string }
	}
	if err := Decode(bytes.NewReader(ui.Data), &input); err != nil || len(input.Name) > 4096 || len(input.BlockIDs) > 2000 ||
		slices.ContainsFunc(input.BlockIDs, func(id string) bool { return !nativeID.MatchString(id) }) {
		return nil, ErrArgument
	}
	dates := map[string]time.Time{}
	if action == "due-batch" {
		if len(input.CardDues) == 0 || len(input.CardDues) > 2000 {
			return nil, ErrArgument
		}
		for _, card := range input.CardDues {
			due, err := time.ParseInLocation("20060102150405", card.Due, time.Local)
			if err != nil || !nativeID.MatchString(card.ID) {
				return nil, ErrArgument
			}
			dates[card.ID] = due
		}
		input.DeckID = flashcard.BuiltinDeckID
	} else if len(input.CardDues) != 0 {
		return nil, ErrArgument
	}
	if action == "create" {
		input.DeckID = ast.NewNodeID()
	}
	if (input.DeckID != "" || action != "remove" && action != "reset") && !nativeID.MatchString(input.DeckID) {
		return nil, ErrArgument
	}
	if action == "reset" && (!nativeID.MatchString(input.ID) || !slices.Contains([]string{"deck", "notebook", "tree"}, input.Type)) {
		return nil, ErrArgument
	}
	if (action == "add" || action == "remove") && len(input.BlockIDs) == 0 {
		return nil, ErrArgument
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: ui.StoreID, PageID: ui.PageID, Action: "flashcard." + action,
		OperationID: "ui-riff-" + ast.NewNodeID()}
	_, err := planContentLifecycle(ctx, m, request, func(p *lifecycleEdit) error {
		l, err := p.lease()
		if err != nil {
			return err
		}
		defer l.Release()
		cfg, err := l.Configuration(p.ctx)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		switch action {
		case "create", "rename", "remove-deck", "due-batch":
			editAction := action
			if action == "remove-deck" {
				editAction = "remove"
			}
			changes, err := l.PlanNativeFlashcard(p.ctx, flashcard.Edit{Action: editAction, DeckID: input.DeckID, Name: input.Name, DueDates: dates})
			if err != nil {
				return err
			}
			return p.stageUIFlashcardChanges(changes)
		}
		plan, err := l.PlanNativeFlashcardMutation(p.ctx, flashcard.MutationRequest{Action: action, DeckID: input.DeckID,
			Scope: input.Type, ID: input.ID, BlockIDs: input.BlockIDs})
		if err != nil {
			return err
		}
		if err = p.checkPlanInputs(plan.SourceHashes); err != nil {
			return err
		}
		engine := nativeparser.NewEditor(nativeparser.EditorOptions{})
		for _, t := range plan.Trees {
			name := path.Join(t.Box, t.Path)
			before, err := p.bytes(name)
			if err != nil {
				return err
			}
			data, err := docview.EncodeWriteTree(t, engine, true)
			if err != nil {
				return err
			}
			data, err = docast.SerializeDocument([][]byte{before}, data)
			if err != nil {
				return err
			}
			p.write(name, data)
		}
		return p.stageUIFlashcardChanges(plan.Changes)
	})
	if err != nil || input.DeckID == "" || action != "create" && action != "add" && action != "remove" {
		return nil, err
	}
	l, err := m.Acquire(ctx, ui.StoreID, ui.PageID)
	if err != nil {
		return nil, err
	}
	defer l.Release()
	decks, err := l.NativeFlashcardDecks(ctx)
	if err != nil {
		return nil, err
	}
	for _, deck := range decks {
		if deck.ID == input.DeckID {
			return uiDeck(deck), nil
		}
	}
	return nil, fs.ErrNotExist
}

func (p *lifecycleEdit) stageUIFlashcardChanges(changes []flashcard.Change) error {
	for _, change := range changes {
		hash := ""
		if change.Existed {
			hash = sourcetx.Hash(change.Before)
		}
		if p.files[change.Path].Hash != hash {
			return sourcetx.ErrConflict
		}
		p.change(p.prefix+change.Path, hash, change.After, change.Remove)
	}
	return nil
}

func uiDeck(deck flashcard.Deck) map[string]any {
	const stamp = "2006-01-02 15:04:05"
	return map[string]any{"id": deck.ID, "name": deck.Name, "size": deck.Cards,
		"created": time.UnixMilli(deck.Created).Format(stamp), "updated": time.UnixMilli(deck.Updated).Format(stamp)}
}
