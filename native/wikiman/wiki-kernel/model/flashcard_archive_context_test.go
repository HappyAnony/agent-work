//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func archiveDeckFixture(t *testing.T) (*ArchiveImportContext, *riff.Deck, string, string) {
	t.Helper()
	e, _, _ := archiveEntryFixture(t)
	c := e.binding.scope.commit.r.owner
	c.m.Lock()
	c.Flashcard = conf.NewFlashcard()
	c.m.Unlock()
	oldDecks, oldRounds := Decks, legacyFlashcardRounds
	t.Cleanup(func() { Decks = oldDecks; legacyFlashcardRounds = oldRounds })
	d, err := flashcard.NewDeck(builtinDeckID, "Built-in Deck", *conf.NewFlashcard(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	block, card := ast.NewNodeID(), ast.NewNodeID()
	d.AddCard(card, block)
	d.AddCard(ast.NewNodeID(), ast.NewNodeID())
	d.Review(card, riff.Rating(3))
	if err = d.Save(); err != nil {
		t.Fatal(err)
	}
	Decks = map[string]*riff.Deck{builtinDeckID: d}
	legacyFlashcardRounds = map[string]*flashcard.Round{}
	meta, cards, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	archiveWrite(t, util.DataDir, "storage/riff/"+builtinDeckID+".deck", meta)
	archiveWrite(t, util.DataDir, "storage/riff/"+builtinDeckID+".cards", cards)
	return e, d, block, card
}
func exportedDeckFixture(t *testing.T, e *ArchiveImportContext, block string) (flashcard.ArchivePlan, string) {
	t.Helper()
	folder := t.TempDir()
	p, err := runLegacyFlashcardArchive(e.binding, nil, nil, []*parse.Tree{archiveModelTree(block)}, folder, true)
	if err != nil {
		t.Fatal(err)
	}
	return p, folder
}
func TestFlashcardArchiveContextOriginalExportAndImportRules(t *testing.T) {
	e, d, block, card := archiveDeckFixture(t)
	base := filepath.Join(util.DataDir, "storage/riff")
	beforeMeta, _ := os.ReadFile(filepath.Join(base, builtinDeckID+".deck"))
	beforeCards, _ := os.ReadFile(filepath.Join(base, builtinDeckID+".cards"))
	p, folder := exportedDeckFixture(t, e, block)
	if p.Count != 1 || len(p.Files) != 2 {
		t.Fatal("selected builtin projection", p.Count, len(p.Files))
	}
	exportDeck, err := loadLegacyFlashcardDeck(filepath.Join(folder, "storage/riff"), builtinDeckID, .9, 36500, conf.DefaultFSRSWeights())
	if err != nil {
		t.Fatal(err)
	}
	if exportDeck.GetCard(card) == nil || exportDeck.GetCard(card).(*riff.FSRSCard).C.Reps != 0 || d.GetCard(card).(*riff.FSRSCard).C.Reps != 1 {
		t.Fatal("export learning reset/source mutated")
	}
	afterMeta, _ := os.ReadFile(filepath.Join(base, builtinDeckID+".deck"))
	afterCards, _ := os.ReadFile(filepath.Join(base, builtinDeckID+".cards"))
	if !bytes.Equal(beforeMeta, afterMeta) || !bytes.Equal(beforeCards, afterCards) {
		t.Fatal("export wrote source")
	}
	next := ast.NewNodeID()
	tree := archiveModelTree(next)
	result, err := runLegacyFlashcardArchive(e.binding, p.Files, map[string]string{block: next}, []*parse.Tree{tree}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 || tree.Root.IALAttr("custom-riff-decks") != builtinDeckID || Decks[builtinDeckID] == d || Decks[builtinDeckID].CountCards() != 3 {
		t.Fatal("import destination/binding/cache", result.Count)
	}
	fresh := Decks[builtinDeckID].GetCardsByBlockID(next)
	if len(fresh) != 1 || fresh[0].ID() == card || fresh[0].(*riff.FSRSCard).C.Reps != 0 {
		t.Fatal("fresh ID/state rule")
	}
}
func TestFlashcardArchiveContextCachedDeckOutlivesRequestAndLogsStillSave(t *testing.T) {
	e, _, block, _ := archiveDeckFixture(t)
	p, _ := exportedDeckFixture(t, e, block)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := e.binding.withContext(ctx)
	next := ast.NewNodeID()
	if _, err := runLegacyFlashcardArchive(b, p.Files, map[string]string{block: next}, []*parse.Tree{archiveModelTree(next)}, "", false); err != nil {
		t.Fatal(err)
	}
	cancel()
	d := Decks[builtinDeckID]
	card := d.GetCardsByBlockID(next)[0]
	log := d.Review(card.ID(), riff.Rating(3))
	if err := d.SaveLog(log); err != nil {
		t.Fatal("cached log tied to finished request", err)
	}
	if err := d.Save(); err != nil {
		t.Fatal("cached save tied to finished request", err)
	}
	if _, err := os.Stat(filepath.Join(util.DataDir, "storage/riff/logs", time.Now().Format("200601")+".msgpack")); err != nil {
		t.Fatal(err)
	}
	if err := Conf.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(); err == nil {
		t.Fatal("closed owner cache wrote source")
	}
}
func TestFlashcardArchiveContextDestinationVersionsAndBadSources(t *testing.T) {
	for _, kind := range []string{"corrupt", "link", "external-version", "late-deck-failure"} {
		t.Run(kind, func(t *testing.T) {
			e, d, block, _ := archiveDeckFixture(t)
			p, _ := exportedDeckFixture(t, e, block)
			target := filepath.Join(util.DataDir, "storage/riff", builtinDeckID+".deck")
			next := ast.NewNodeID()
			tree := archiveModelTree(next)
			switch kind {
			case "corrupt":
				os.WriteFile(target, []byte("bad metadata"), 0600)
			case "link":
				os.Remove(target)
				outside := archiveWrite(t, t.TempDir(), "external", []byte("external"))
				os.Symlink(outside, target)
			}
			services, err := capturedFlashcardArchiveServices(e.binding, "")
			if err != nil {
				t.Fatal(err)
			}
			plan, err := flashcard.PrepareImport(e.ctx, p.Files, map[string]string{block: next}, []*parse.Tree{tree}, services)
			if kind == "corrupt" || kind == "link" {
				if err == nil {
					t.Fatal("bad destination ignored")
				}
				if Decks[builtinDeckID] != d {
					t.Fatal("failure replaced cache")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			external := []byte("external revision")
			os.WriteFile(target, external, 0600)
			if kind == "late-deck-failure" {
				os.Remove(target)
				os.Mkdir(target, 0755)
			}
			if err = services.ApplyImport(e.ctx, plan); err == nil {
				t.Fatal("late destination error swallowed")
			}
			if Decks[builtinDeckID] != d {
				t.Fatal("partial publication replaced cache")
			}
			if kind == "external-version" {
				raw, _ := os.ReadFile(target)
				if !bytes.Equal(raw, external) {
					t.Fatal("external revision overwritten")
				}
			}
		})
	}
}
func TestFlashcardArchiveContextExportClearsOnlyPairAndRejectsLinks(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty-selection", true: "link"}[linked], func(t *testing.T) {
			e, _, _, _ := archiveDeckFixture(t)
			folder := t.TempDir()
			target := archiveWrite(t, folder, "storage/riff/"+builtinDeckID+".cards", []byte("old"))
			archiveWrite(t, folder, "storage/riff/"+builtinDeckID+".deck", []byte("old"))
			other := archiveWrite(t, folder, "storage/riff/unrelated", []byte("keep"))
			if linked {
				os.Remove(target)
				outside := archiveWrite(t, t.TempDir(), "external", []byte("keep external"))
				os.Symlink(outside, target)
			}
			_, err := runLegacyFlashcardArchive(e.binding, nil, nil, []*parse.Tree{archiveModelTree(ast.NewNodeID())}, folder, true)
			if linked {
				if err == nil {
					t.Fatal("export link accepted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("empty export kept old cards", err)
				}
			}
			raw, _ := os.ReadFile(other)
			if string(raw) != "keep" {
				t.Fatal("export cleared unrelated file")
			}
		})
	}
}
func TestFlashcardArchiveContextOwnerAndCacheChangesReject(t *testing.T) {
	for _, kind := range []string{"cancel", "old-root", "closed", "readonly", "cache-replaced"} {
		t.Run(kind, func(t *testing.T) {
			e, _, block, _ := archiveDeckFixture(t)
			services, err := capturedFlashcardArchiveServices(e.binding, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.binding.scope.ctx = ctx
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "closed":
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			case "cache-replaced":
				Decks[builtinDeckID] = nil
			}
			if _, err = services.Cards(context.Background(), []string{block}); err == nil {
				t.Fatal("source read admitted")
			}
			if err = services.ApplyImport(context.Background(), flashcard.ArchivePlan{}); err == nil {
				t.Fatal("import admitted")
			}
			if err = services.WriteExport(context.Background(), flashcard.ArchivePlan{}); err == nil {
				t.Fatal("export admitted")
			}
		})
	}
}
func TestFlashcardArchiveContextExportWaitIsCancelableAndOwnerBound(t *testing.T) {
	e, _, block, _ := archiveDeckFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	b := e.binding.withContext(ctx)
	runtime := &Conf.syncRuntime
	runtime.storages.Store(true)
	defer runtime.storages.Store(false)
	_, err := runLegacyFlashcardArchive(b, nil, nil, []*parse.Tree{archiveModelTree(block)}, t.TempDir(), true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if !deckLock.TryLock() {
		t.Fatal("wait left deck lock held")
	}
	deckLock.Unlock()
}
func TestFlashcardArchiveContextRootedStorageRejectsStaleAndUnadmittedWrites(t *testing.T) {
	e, _, _, _ := archiveDeckFixture(t)
	s := &flashcardArchiveStorage{b: e.binding, versions: map[string][]byte{}}
	name := builtinDeckID + ".cards"
	if err := s.Write(name, []byte("unadmitted")); err == nil {
		t.Fatal("unadmitted revision wrote")
	}
	before, err := s.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(util.DataDir, "storage/riff", name)
	os.WriteFile(file, []byte("external"), 0600)
	if err = s.Write(name, before); !errors.Is(err, indexrepair.ErrSourceChanged) {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	if string(raw) != "external" {
		t.Fatal("stale source overwritten")
	}
	if _, err = s.Read("../external"); err == nil {
		t.Fatal("path escape")
	}
}

func TestFlashcardArchiveContextCachedStorageIndependentOfSearchIndexHandle(t *testing.T) {
	e, _, block, _ := archiveDeckFixture(t)
	p, _ := exportedDeckFixture(t, e, block)
	next := ast.NewNodeID()
	if _, err := runLegacyFlashcardArchive(e.binding, p.Files, map[string]string{block: next}, []*parse.Tree{archiveModelTree(next)}, "", false); err != nil {
		t.Fatal(err)
	}
	r := e.binding.scope.commit.r
	old := r.main.DB
	r.main.DB = nil
	defer func() { r.main.DB = old }()
	if err := Decks[builtinDeckID].Save(); err != nil {
		t.Fatal("workspace deck writer depends on replaced search handle", err)
	}
	requestStore := &flashcardArchiveStorage{b: e.binding, versions: map[string][]byte{}}
	if _, err := requestStore.Read(builtinDeckID + ".deck"); err == nil {
		t.Fatal("request admission lost its captured handle guard")
	}
}
