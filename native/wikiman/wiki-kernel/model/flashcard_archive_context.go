// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// Borrowed while the original deck lock is held; all persistence is rooted and
// versioned. Captured legacy map publication remains a compatibility effect.
type flashcardArchiveStorage struct {
	b             *legacyAVBinding
	versions      map[string][]byte
	ownerLifetime bool
	closeRoot     func(*os.Root) error // Optional trusted backend, captured when borrowing a root.
}

// Cached deck storage is a workspace resource, independent of a request or
// replaceable search database. Request-scoped admission still checks its DBs.
func (s *flashcardArchiveStorage) guard(ctx context.Context) error {
	if s.b.err != nil {
		return s.b.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r := s.b.scope.commit.r
	if !s.ownerLifetime {
		if err := r.guard(ctx); err != nil {
			return err
		}
	} else {
		if r.owner != Conf || r.data != util.DataDir || r.workspace != util.WorkspaceDir {
			return errors.New("flashcard_storage_owner_changed")
		}
		r.owner.m.Lock()
		readonly := r.owner.persistenceReadOnly
		r.owner.m.Unlock()
		if readonly {
			return ErrSettingReadOnly
		}
	}
	r.owner.repoMu.Lock()
	closed := r.owner.repoClosed
	r.owner.repoMu.Unlock()
	if closed {
		return errors.New("flashcard_storage_owner_closed")
	}
	return nil
}
func (s *flashcardArchiveStorage) source(ctx context.Context) (indexrepair.Source, func() error, error) {
	if err := s.guard(ctx); err != nil {
		return indexrepair.Source{}, nil, err
	}
	closeRoot := s.closeRoot
	if closeRoot == nil {
		closeRoot = func(root *os.Root) error { return root.Close() }
	}
	root, err := os.OpenRoot(s.b.scope.commit.r.data)
	if err != nil {
		return indexrepair.Source{}, nil, err
	}
	closeIO := newRepairIOCloser([]*os.Root{root}, closeRoot)
	if err = s.guard(ctx); err != nil {
		return indexrepair.Source{}, nil, joinRepairCloseError(err, closeIO())
	}
	return indexrepair.Source{Data: root, Authorize: s.guard, Lock: func(name string) func() {
		full := filepath.Join(s.b.scope.commit.r.data, filepath.FromSlash(name))
		filelock.Lock(full)
		return func() { filelock.Unlock(full) }
	}}, closeIO, nil
}
func (s *flashcardArchiveStorage) Read(name string) (raw []byte, err error) {
	if !fs.ValidPath(name) {
		return nil, flashcard.ErrInvalid
	}
	ctx := s.b.scope.ctx
	source, closeIO, err := s.source(ctx)
	if err != nil {
		return nil, errors.Join(riff.ErrStorageReadIncomplete, err)
	}
	locked := false
	full := filepath.Join(s.b.scope.commit.r.data, "storage/riff", filepath.FromSlash(name))
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		if locked {
			filelock.Unlock(full)
		}
		if err != nil {
			raw = nil
		}
	}()
	filelock.Lock(full)
	locked = true
	// A root borrowed before waiting must be readmitted after its lock is acquired.
	if err = s.guard(ctx); err != nil {
		return nil, errors.Join(riff.ErrStorageReadIncomplete, err)
	}
	raw, err = source.Read(ctx, "storage/riff/"+name)
	readErr := err
	closeErr := closeIO()
	guardErr := s.guard(ctx)
	err = joinRepairCloseError(joinRepairCloseError(readErr, closeErr), guardErr)
	if closeErr != nil || guardErr != nil {
		err = errors.Join(riff.ErrStorageReadIncomplete, err)
	}
	if closeErr == nil && guardErr == nil {
		if readErr == nil {
			s.versions[name] = bytes.Clone(raw)
		} else if indexrepair.IsSourceNotExist(readErr) {
			s.versions[name] = nil
		}
	}
	return raw, err
}
func (s *flashcardArchiveStorage) Write(name string, raw []byte) (err error) {
	if !fs.ValidPath(name) {
		return flashcard.ErrInvalid
	}
	ctx := s.b.scope.ctx
	source, closeIO, err := s.source(ctx)
	if err != nil {
		return err
	}
	locked := false
	full := filepath.Join(s.b.scope.commit.r.data, "storage/riff", filepath.FromSlash(name))
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		if locked {
			filelock.Unlock(full)
		}
	}()
	filelock.Lock(full)
	locked = true
	// Keep the original per-file lock through close and version registration.
	// Rewrite must not acquire this exact non-reentrant lock a second time.
	source.Lock = nil
	if err = s.guard(ctx); err != nil {
		return err
	}
	before, known := s.versions[name]
	if !known {
		err = errors.New("flashcard_archive_revision_unbound")
	} else {
		err = source.Rewrite(ctx, "storage/riff/"+name, before, raw)
	}
	err = joinRepairCloseError(err, closeIO())
	err = joinRepairCloseError(err, s.guard(ctx))
	if err != nil {
		return err
	}
	s.versions[name] = bytes.Clone(raw)
	return nil
}
func capturedFlashcardArchiveServices(b *legacyAVBinding, exportDir string, openers ...func() (*os.Root, error)) (flashcard.ArchiveServices, error) {
	if err := b.liveSourceGuard(); err != nil {
		return flashcard.ArchiveServices{}, err
	}
	var settings conf.Flashcard
	err := b.scope.commit.r.owner.WithSettingSnapshot(b.scope.ctx, func(v conf.AppData) error {
		if v.Flashcard == nil {
			return flashcard.ErrDependency
		}
		settings = *v.Flashcard
		return nil
	})
	if err != nil {
		return flashcard.ArchiveServices{}, err
	}
	decks, rounds := Decks, legacyFlashcardRounds
	expected := decks[builtinDeckID]
	guard := func() error {
		if err := b.liveSourceGuard(); err != nil {
			return err
		}
		if Decks[builtinDeckID] != expected {
			return errors.New("flashcard_archive_deck_changed")
		}
		return nil
	}
	store := &flashcardArchiveStorage{b: b, versions: map[string][]byte{}}
	load := func() (*riff.Deck, error) {
		return riff.LoadDeck(store, builtinDeckID, settings.RequestRetention, settings.MaximumInterval, settings.Weights, time.Now)
	}
	return flashcard.ArchiveServices{Settings: settings, Now: time.Now, NewID: ast.NewNodeID,
		Cards: func(ctx context.Context, ids []string) ([]riff.Card, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := guard(); err != nil {
				return nil, err
			}
			if expected == nil {
				return nil, nil
			}
			snapshot, err := flashcard.CloneDeck(expected, settings, time.Now)
			if err != nil {
				return nil, err
			}
			return snapshot.GetCardsByBlockIDs(ids), guard()
		},
		Destination: func(ctx context.Context) (*riff.Deck, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := guard(); err != nil {
				return nil, err
			}
			disk, err := load()
			if err != nil {
				return nil, err
			}
			if expected != nil {
				return expected, guard()
			}
			if store.versions[builtinDeckID+".deck"] == nil {
				disk.Name = "Built-in Deck"
			}
			return disk, guard()
		},
		ApplyImport: func(ctx context.Context, p flashcard.ArchivePlan) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := guard(); err != nil {
				return err
			}
			if len(p.Files) == 0 {
				return nil
			}
			if decks == nil || rounds == nil {
				return flashcard.ErrDependency
			}
			for _, ext := range []string{".cards", ".deck"} {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := guard(); err != nil {
					return err
				}
				name := builtinDeckID + ext
				raw, ok := p.Files["storage/riff/"+name]
				if !ok {
					return flashcard.ErrInvalid
				}
				if err := store.Write(name, raw); err != nil {
					return err
				}
			}
			// Cached decks outlive the request; keep the captured owner/versions,
			// but use its guarded lifetime rather than the completed HTTP context.
			cachedStore := &flashcardArchiveStorage{b: b.withContext(context.WithoutCancel(b.scope.ctx)), versions: map[string][]byte{}, ownerLifetime: true}
			loaded, err := riff.LoadDeck(cachedStore, builtinDeckID, settings.RequestRetention, settings.MaximumInterval, settings.Weights, time.Now)
			if err != nil {
				return err
			}
			if err = guard(); err != nil {
				return err
			}
			decks[builtinDeckID] = loaded
			expected = loaded
			delete(rounds, builtinDeckID)
			return nil
		},
		WriteExport: func(ctx context.Context, p flashcard.ArchivePlan) error {
			open := func() (*os.Root, error) { return openArchivePublicationSource(exportDir) }
			if len(openers) > 0 {
				open = openers[0]
			}
			return writeFlashcardArchiveExport(ctx, p, exportDir, guard, open, func(root *os.Root) error { return root.Close() })
		},
	}, nil
}
func runLegacyFlashcardArchive(b *legacyAVBinding, files map[string][]byte, ids map[string]string, trees []*parse.Tree, exportDir string, export bool, openers ...func() (*os.Root, error)) (out flashcard.ArchivePlan, err error) {
	if err = b.liveSourceGuard(); err != nil {
		return
	}
	deckLock.Lock()
	defer deckLock.Unlock()
	if err = b.liveSourceGuard(); err != nil {
		return
	}
	if export {
		runtime := &b.scope.commit.r.owner.syncRuntime
		for runtime.storages.Load() || runtime.booting.Load() {
			timer := time.NewTimer(25 * time.Millisecond)
			select {
			case <-b.scope.ctx.Done():
				timer.Stop()
				return out, b.scope.ctx.Err()
			case <-timer.C:
			}
			if err = b.liveSourceGuard(); err != nil {
				return
			}
		}
	}
	services, err := capturedFlashcardArchiveServices(b, exportDir, openers...)
	if err != nil {
		return out, err
	}
	if export {
		return flashcard.ExecuteExport(b.scope.ctx, trees, services)
	}
	return flashcard.ExecuteImport(b.scope.ctx, files, ids, trees, services)
}
