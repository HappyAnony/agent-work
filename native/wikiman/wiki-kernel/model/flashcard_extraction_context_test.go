//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func extractionFixture(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	deck := filepath.Join(dir, "storage/riff", builtinDeckID+".deck")
	cards := filepath.Join(dir, "storage/riff", builtinDeckID+".cards")
	if err := os.MkdirAll(filepath.Dir(deck), 0700); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{deck: {0, 255, 1, 13, 10}, cards: {0, 7, 255}} {
		if err := os.WriteFile(name, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, deck, cards
}

func TestFlashcardExtractionExactBytesAndOriginalAbsence(t *testing.T) {
	for _, kind := range []string{"both", "deck", "cards", "empty-file", "empty-riff", "no-riff", "no-storage"} {
		t.Run(kind, func(t *testing.T) {
			dir, deck, cards := extractionFixture(t)
			switch kind {
			case "deck":
				os.Remove(cards)
			case "cards":
				os.Remove(deck)
			case "empty-file":
				os.WriteFile(deck, []byte{}, 0600)
			case "empty-riff":
				os.Remove(deck)
				os.Remove(cards)
			case "no-riff":
				os.RemoveAll(filepath.Dir(deck))
			case "no-storage":
				os.RemoveAll(filepath.Join(dir, "storage"))
			}
			out, err := readLegacyFlashcardArchive(context.Background(), dir)
			if err != nil || out == nil {
				t.Fatal(out, err)
			}
			count := 0
			for _, name := range []string{deck, cards} {
				want, err := os.ReadFile(name)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				count++
				rel, _ := filepath.Rel(dir, name)
				got, ok := out[filepath.ToSlash(rel)]
				if !ok || got == nil || !bytes.Equal(want, got) {
					t.Fatal(rel, got, want)
				}
			}
			if len(out) != count {
				t.Fatal(len(out), count)
			}
		})
	}
}

func TestFlashcardExtractionLimitAndWholeManifestPreflight(t *testing.T) {
	for _, extra := range []int64{0, 1} {
		dir, _, cards := extractionFixture(t)
		f, err := os.OpenFile(cards, os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(flashcardExtractionLimit + extra); err != nil {
			t.Fatal(err)
		}
		f.Close()
		reads := 0
		op := flashcardExtractionIO{read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
			reads++
			return s.ReadLimited(ctx, n, flashcardExtractionLimit)
		}}
		out, err := readLegacyFlashcardArchiveWithIO(context.Background(), dir, op)
		if extra == 1 {
			if out != nil || !errors.Is(err, flashcard.ErrInvalid) || reads != 0 {
				t.Fatal(len(out), err, reads)
			}
		} else if err != nil || len(out["storage/riff/"+builtinDeckID+".cards"]) != int(flashcardExtractionLimit) || reads != 2 {
			t.Fatal(len(out), err, reads)
		}
	}
}

func TestFlashcardExtractionReadAndCloseFailureNoPartialMap(t *testing.T) {
	for _, failRead := range []bool{false, true} {
		dir, _, _ := extractionFixture(t)
		readErr, closeErr := errors.New("second payload"), errors.New("extraction close")
		reads, closes := 0, 0
		var closed *os.Root
		op := flashcardExtractionIO{closeRoot: func(r *os.Root) error { closes++; closed = r; return errors.Join(r.Close(), closeErr) }, read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
			reads++
			if failRead && reads == 2 {
				return []byte("partial"), readErr
			}
			return s.ReadLimited(ctx, n, flashcardExtractionLimit)
		}}
		out, err := readLegacyFlashcardArchiveWithIO(context.Background(), dir, op)
		if out != nil || !errors.Is(err, closeErr) || (failRead && !errors.Is(err, readErr)) || reads != 2 || closes != 1 {
			t.Fatal(out, err, reads, closes)
		}
		if _, err := closed.Stat("."); err == nil {
			t.Fatal("root remains open")
		}
	}
}

func TestFlashcardExtractionCancellationAndMutations(t *testing.T) {
	for _, kind := range []string{"cancel-before", "cancel-read", "cancel-close", "replace-deck", "replace-riff", "replace-root", "late-cards", "late-storage", "symlink-second", "symlink-parent"} {
		t.Run(kind, func(t *testing.T) {
			dir, deck, cards := extractionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads, opens := 0, 0
			switch kind {
			case "cancel-before":
				cancel()
			case "late-cards":
				os.Remove(cards)
			case "late-storage":
				os.RemoveAll(filepath.Join(dir, "storage"))
			case "symlink-second":
				os.Remove(cards)
				os.Symlink(deck, cards)
			case "symlink-parent":
				os.Rename(filepath.Join(dir, "storage"), filepath.Join(dir, "hidden"))
				os.Symlink("hidden", filepath.Join(dir, "storage"))
			}
			op := flashcardExtractionIO{openRoot: func(n string) (*os.Root, error) { opens++; return os.OpenRoot(n) }, read: func(ctx context.Context, s indexrepair.Source, n string) ([]byte, error) {
				reads++
				raw, err := s.ReadLimited(ctx, n, flashcardExtractionLimit)
				if reads == 1 {
					switch kind {
					case "cancel-read":
						cancel()
					case "replace-deck":
						os.Rename(deck, deck+".old")
						os.WriteFile(deck, raw, 0600)
					case "replace-riff":
						os.Rename(filepath.Dir(deck), filepath.Dir(deck)+".old")
						os.Mkdir(filepath.Dir(deck), 0700)
					case "late-cards":
						os.WriteFile(cards, []byte("new"), 0600)
					}
				}
				return raw, err
			}, closeRoot: func(r *os.Root) error {
				err := r.Close()
				switch kind {
				case "cancel-close":
					cancel()
				case "replace-root":
					os.Rename(dir, dir+".old")
					t.Cleanup(func() { os.RemoveAll(dir + ".old") })
					os.Mkdir(dir, 0700)
				case "late-storage":
					os.MkdirAll(filepath.Join(dir, "storage/riff"), 0700)
				}
				return err
			}}
			out, err := readLegacyFlashcardArchiveWithIO(ctx, dir, op)
			if out != nil || err == nil {
				t.Fatal(out, err)
			}
			if kind == "cancel-before" && opens != 0 {
				t.Fatal(opens)
			}
			if (kind == "symlink-second" || kind == "symlink-parent") && reads != 0 {
				t.Fatal("read before manifest validation", reads)
			}
			if len(kind) >= 6 && kind[:6] == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestFlashcardExtractionCapturedImportOwnerAndCheckedClose(t *testing.T) {
	e, _ := ownedImportFixture(t)
	owned, err := e.newImportScratch("wiki-import-")
	if err != nil {
		t.Fatal(err)
	}
	defer owned.close()
	if err = owned.root.MkdirAll("storage/riff", 0700); err != nil {
		t.Fatal(err)
	}
	name := "storage/riff/" + builtinDeckID + ".deck"
	if err = owned.root.WriteFile(name, []byte{3, 255, 0}, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := e.readLegacyFlashcardArchive(owned.folder)
	if err != nil || !bytes.Equal(out[name], []byte{3, 255, 0}) {
		t.Fatal(out, err)
	}
	closeErr := errors.New("borrow close")
	e.closeRoot = func(r *os.Root) error { return errors.Join(r.Close(), closeErr) }
	out, err = e.readLegacyFlashcardArchive(owned.folder)
	if out != nil || !errors.Is(err, closeErr) {
		t.Fatal(out, err)
	}
	if _, err = owned.root.Stat("."); err != nil {
		t.Fatal("owner closed", err)
	}
	e.closeRoot = nil
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	e.ctx = ctx
	if out, err = e.readLegacyFlashcardArchive(owned.folder); out != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(out, err)
	}
}

func TestFlashcardExtractionOwnerInvalidatedDuringClose(t *testing.T) {
	for _, kind := range []string{"root", "readonly", "closed"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := ownedImportFixture(t)
			owned, err := e.newImportScratch("wiki-import-")
			if err != nil {
				t.Fatal(err)
			}
			defer owned.close()
			oldData := util.DataDir
			defer func() { util.DataDir = oldData; Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			e.closeRoot = func(r *os.Root) error {
				err := r.Close()
				switch kind {
				case "root":
					util.DataDir = t.TempDir()
				case "readonly":
					Conf.m.Lock()
					Conf.persistenceReadOnly = true
					Conf.m.Unlock()
				case "closed":
					err = errors.Join(err, Conf.CloseCloudRepo(context.Background()))
				}
				return err
			}
			out, err := e.readLegacyFlashcardArchive(owned.folder)
			if out != nil || err == nil {
				t.Fatal("owner change returned candidates", out, err)
			}
		})
	}
}
