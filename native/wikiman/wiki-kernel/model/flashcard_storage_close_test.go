//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func checkedFlashcardStorage(t *testing.T) (*flashcardArchiveStorage, string, string) {
	t.Helper()
	e, _, _, _ := archiveDeckFixture(t)
	name := builtinDeckID + ".cards"
	return &flashcardArchiveStorage{b: e.binding, versions: map[string][]byte{}}, name, filepath.Join(util.DataDir, "storage/riff", name)
}
func TestFlashcardStorageCloseSourceIsCapturedAndIdempotent(t *testing.T) {
	s, _, _ := checkedFlashcardStorage(t)
	failure := errors.New("borrow close")
	first, second := 0, 0
	s.closeRoot = func(root *os.Root) error { first++; return errors.Join(root.Close(), failure) }
	source, closeIO, err := s.source(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.closeRoot = func(root *os.Root) error { second++; return root.Close() }
	for i := 0; i < 2; i++ {
		if err := closeIO(); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	if first != 1 || second != 0 {
		t.Fatal(first, second)
	}
	if _, err := source.Data.Stat("."); err == nil {
		t.Fatal("borrow root still open")
	}
}
func TestFlashcardStorageCloseReadRegistersOnlyAfterCloseAndReturnsDetachedBytes(t *testing.T) {
	s, name, file := checkedFlashcardStorage(t)
	old := []byte("old admitted revision")
	s.versions[name] = bytes.Clone(old)
	calls := 0
	s.closeRoot = func(root *os.Root) error {
		calls++
		if !bytes.Equal(s.versions[name], old) {
			t.Fatal("version advanced before close")
		}
		return root.Close()
	}
	raw, err := s.Read(name)
	if err != nil || calls != 1 || !bytes.Equal(raw, s.versions[name]) {
		t.Fatal(err, calls)
	}
	expected, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(expected, raw) {
		t.Fatal(err)
	}
	raw[0] ^= 1
	if !bytes.Equal(expected, s.versions[name]) {
		t.Fatal("caller buffer changed recorded revision")
	}
}
func TestFlashcardStorageCloseReadErrorsJoinWithoutRegisteringRevision(t *testing.T) {
	for _, kind := range []string{"success-close", "missing-close", "directory-close"} {
		t.Run(kind, func(t *testing.T) {
			s, name, _ := checkedFlashcardStorage(t)
			closing := errors.New("read root close")
			if kind == "missing-close" {
				name = "missing.cards"
			}
			if kind == "directory-close" {
				name = "logs"
				os.MkdirAll(filepath.Join(util.DataDir, "storage/riff/logs"), 0700)
			}
			s.versions[name] = []byte("prior")
			calls := 0
			s.closeRoot = func(root *os.Root) error { calls++; return errors.Join(root.Close(), closing) }
			raw, err := s.Read(name)
			if raw != nil || !errors.Is(err, closing) || !errors.Is(err, riff.ErrStorageReadIncomplete) || calls != 1 || string(s.versions[name]) != "prior" {
				t.Fatal(raw, err, calls)
			}
			if kind == "missing-close" && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("missing cause lost", err)
			}
			if kind == "directory-close" && !errors.Is(err, indexrepair.ErrSourcePath) {
				t.Fatal("read cause lost", err)
			}
		})
	}
}
func TestFlashcardStorageClosePureMissingAdmitsCreationButIncompleteMissingDoesNot(t *testing.T) {
	s, _, _ := checkedFlashcardStorage(t)
	name := "logs/missing.msgpack"
	if raw, err := s.Read(name); raw != nil || !os.IsNotExist(err) {
		t.Fatal(raw, err)
	}
	if value, known := s.versions[name]; !known || value != nil {
		t.Fatal("pure missing not admitted")
	}
	if err := s.Write(name, []byte("first creation")); err != nil {
		t.Fatal(err)
	}
	s.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), fs.ErrNotExist) }
	other := "missing.cards"
	_, err := s.Read(other)
	if !errors.Is(err, riff.ErrStorageReadIncomplete) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, known := s.versions[other]; known {
		t.Fatal("failed missing read admitted a create")
	}
}
func TestFlashcardStorageCloseWritePreservesOldVersionAndRequiresReadBeforeRetry(t *testing.T) {
	s, name, file := checkedFlashcardStorage(t)
	old, err := s.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	next := []byte("changed source bytes")
	closing := errors.New("write root close")
	calls := 0
	s.closeRoot = func(root *os.Root) error {
		calls++
		if !bytes.Equal(s.versions[name], old) {
			t.Fatal("version advanced before close")
		}
		return errors.Join(root.Close(), closing)
	}
	err = s.Write(name, next)
	if !errors.Is(err, closing) || calls != 1 || !bytes.Equal(s.versions[name], old) {
		t.Fatal(err, calls)
	}
	saved, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(saved, next) {
		t.Fatal("committed prefix unexpectedly rolled back", err)
	}
	s.closeRoot = nil
	if err = s.Write(name, []byte("retry without reread")); !errors.Is(err, indexrepair.ErrSourceChanged) {
		t.Fatal(err)
	}
	if _, err = s.Read(name); err != nil {
		t.Fatal(err)
	}
	if err = s.Write(name, []byte("retry after reread")); err != nil {
		t.Fatal(err)
	}
	if string(s.versions[name]) != "retry after reread" {
		t.Fatal("successful version not recorded")
	}
}
func TestFlashcardStorageCloseWritePrimaryAndCloseFailuresPreserveVersion(t *testing.T) {
	s, name, file := checkedFlashcardStorage(t)
	old, err := s.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, []byte("external revision"), 0600)
	closing := errors.New("close with stale write")
	s.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), closing) }
	if err = s.Write(name, []byte("must not overwrite")); !errors.Is(err, closing) || !errors.Is(err, indexrepair.ErrSourceChanged) {
		t.Fatal(err)
	}
	if !bytes.Equal(s.versions[name], old) {
		t.Fatal("failed write advanced version")
	}
	raw, _ := os.ReadFile(file)
	if string(raw) != "external revision" {
		t.Fatal(string(raw))
	}
	if err = s.Write("unadmitted.cards", []byte("no admission")); err == nil || !errors.Is(err, closing) {
		t.Fatal(err)
	}
	if _, known := s.versions["unadmitted.cards"]; known {
		t.Fatal("unadmitted version registered")
	}
}
func TestFlashcardStorageCloseLateCancellationOwnerAndReadonlyRejectVersionCommit(t *testing.T) {
	for _, action := range []string{"read", "write"} {
		for _, kind := range []string{"cancel", "owner", "readonly"} {
			t.Run(action+"/"+kind, func(t *testing.T) {
				s, name, _ := checkedFlashcardStorage(t)
				old, err := s.Read(name)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s.b = s.b.withContext(ctx)
				owner := Conf
				defer func() { Conf = owner; owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
				s.closeRoot = func(root *os.Root) error {
					err := root.Close()
					switch kind {
					case "cancel":
						cancel()
					case "owner":
						Conf = NewAppConf()
					case "readonly":
						owner.m.Lock()
						owner.persistenceReadOnly = true
						owner.m.Unlock()
					}
					return err
				}
				if action == "read" {
					var raw []byte
					raw, err = s.Read(name)
					if raw != nil {
						t.Fatal("revoked read returned bytes")
					}
				} else {
					err = s.Write(name, []byte("possible committed prefix"))
				}
				if err == nil || !bytes.Equal(s.versions[name], old) {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestFlashcardStorageCloseCachedLoadAndLogDoNotSwallowIncompleteMissing(t *testing.T) {
	for _, kind := range []string{"deck", "cards", "log", "root"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := checkedFlashcardStorage(t)
			settings := conf.NewFlashcard()
			closing := errors.New("incomplete read close")
			id := builtinDeckID
			if kind == "deck" {
				id = "20261101090000-newdeck"
			}
			if kind == "cards" {
				os.Remove(filepath.Join(util.DataDir, "storage/riff", id+".cards"))
			}
			if kind == "root" {
				original := util.DataDir
				s.b.scope.commit.r.data = filepath.Join(t.TempDir(), "missing-root")
				util.DataDir = s.b.scope.commit.r.data
				defer func() { util.DataDir = original; s.b.scope.commit.r.data = original }()
			}
			if kind == "deck" || kind == "cards" {
				calls := 0
				s.closeRoot = func(root *os.Root) error {
					calls++
					err := root.Close()
					if kind == "deck" || calls == 2 {
						return errors.Join(err, closing)
					}
					return err
				}
			}
			now := time.Now()
			d, err := riff.LoadDeck(s, id, settings.RequestRetention, settings.MaximumInterval, settings.Weights, func() time.Time { return now })
			if kind != "log" {
				if err == nil || !errors.Is(err, riff.ErrStorageReadIncomplete) {
					t.Fatal("incomplete load initialized a healthy deck", err)
				}
				if kind != "root" && !errors.Is(err, closing) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cards := d.GetCardsByBlockIDs(d.GetBlockIDs())
			if len(cards) == 0 {
				t.Fatal("fixture has no cards")
			}
			log := d.Review(cards[0].ID(), riff.Rating(3))
			s.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), closing) }
			if err = d.SaveLog(log); !errors.Is(err, closing) {
				t.Fatal("log read error swallowed", err)
			}
			key := "logs/" + now.Format("200601") + ".msgpack"
			if _, known := s.versions[key]; known {
				t.Fatal("incomplete log read admitted write")
			}
			s.closeRoot = nil
			if err = d.SaveLog(log); err != nil {
				t.Fatal("explicit retry with complete read failed", err)
			}
		})
	}
}
func TestFlashcardStorageCloseRejectedAdmissionDoesNotBorrowOrRegister(t *testing.T) {
	for _, kind := range []string{"cancel", "closed", "readonly", "path"} {
		t.Run(kind, func(t *testing.T) {
			s, name, _ := checkedFlashcardStorage(t)
			calls := 0
			s.closeRoot = func(root *os.Root) error { calls++; return root.Close() }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
				s.b = s.b.withContext(ctx)
			}
			owner := Conf
			defer func() { owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
			if kind == "closed" {
				if err := owner.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "readonly" {
				owner.m.Lock()
				owner.persistenceReadOnly = true
				owner.m.Unlock()
			}
			if kind == "path" {
				name = "../escape"
			}
			if _, err := s.Read(name); err == nil {
				t.Fatal("read admitted")
			}
			if err := s.Write(name, []byte("must not write")); err == nil {
				t.Fatal("write admitted")
			}
			if calls != 0 || len(s.versions) != 0 {
				t.Fatal(calls, s.versions)
			}
		})
	}
}
