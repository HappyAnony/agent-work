// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/indexrepair"
)

const flashcardExtractionLimit int64 = 32 << 20

type flashcardExtractionIO struct {
	openRoot  func(string) (*os.Root, error)
	closeRoot func(*os.Root) error
	read      func(context.Context, indexrepair.Source, string) ([]byte, error)
}

func (op flashcardExtractionIO) defaults() flashcardExtractionIO {
	if op.openRoot == nil {
		op.openRoot = os.OpenRoot
	}
	if op.closeRoot == nil {
		op.closeRoot = func(root *os.Root) error { return root.Close() }
	}
	if op.read == nil {
		op.read = func(ctx context.Context, source indexrepair.Source, name string) ([]byte, error) {
			return source.ReadLimited(ctx, name, flashcardExtractionLimit)
		}
	}
	return op
}

// The ImportSY path borrows only its already-selected extraction root and uses
// the same owner guard before reading, throughout, and after checked close.
func (e *ArchiveImportContext) readLegacyFlashcardArchive(folder string) (map[string][]byte, error) {
	if err := e.guard(); err != nil {
		return nil, err
	}
	source, closeIO, err := e.openScratch(folder)
	if err != nil {
		return nil, err
	}
	return readFlashcardExtraction(e.ctx, folder, source, closeIO, e.guard, flashcardExtractionIO{}.defaults().read)
}
func readLegacyFlashcardArchive(ctx context.Context, folder string) (map[string][]byte, error) {
	return readLegacyFlashcardArchiveWithIO(ctx, folder, flashcardExtractionIO{})
}
func readLegacyFlashcardArchiveWithIO(ctx context.Context, folder string, op flashcardExtractionIO) (out map[string][]byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	op = op.defaults()
	info, err := os.Lstat(folder)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, flashcard.ErrInvalid
	}
	root, err := op.openRoot(folder)
	if err != nil {
		return nil, err
	}
	closeIO := newRepairIOCloser([]*os.Root{root}, op.closeRoot)
	opened, err := root.Stat(".")
	if err == nil && !os.SameFile(info, opened) {
		err = indexrepair.ErrSourceChanged
	}
	if err != nil {
		return nil, joinRepairCloseError(err, closeIO())
	}
	return readFlashcardExtraction(ctx, folder, indexrepair.Source{Data: root}, closeIO, func() error { return ctx.Err() }, op.read)
}

type flashcardExtractionEntry struct {
	name string
	info fs.FileInfo
}

func readFlashcardExtraction(ctx context.Context, folder string, source indexrepair.Source, closeIO func() error, ownerGuard func() error, read func(context.Context, indexrepair.Source, string) ([]byte, error)) (out map[string][]byte, err error) {
	var base string
	var rootInfo fs.FileInfo
	var entries []flashcardExtractionEntry
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ownerGuard(); err != nil {
			return err
		}
		if rootInfo == nil {
			return nil
		}
		now, err := os.Lstat(base)
		if err != nil {
			return err
		}
		if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, rootInfo) {
			return indexrepair.ErrSourceChanged
		}
		// Fixed prefix order checks the root/directories before any leaf metadata.
		// Post-close checks use the explicit selected path, without reopening a root.
		for _, saved := range entries {
			now, err := os.Lstat(filepath.Join(base, filepath.FromSlash(saved.name)))
			if saved.info == nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				return indexrepair.ErrSourceChanged
			}
			if err != nil {
				return err
			}
			if now.Mode()&fs.ModeSymlink != 0 || now.Mode().Type() != saved.info.Mode().Type() || !os.SameFile(now, saved.info) {
				return indexrepair.ErrSourceChanged
			}
			if now.Mode().IsRegular() && (now.Size() != saved.info.Size() || !now.ModTime().Equal(saved.info.ModTime())) {
				return indexrepair.ErrSourceChanged
			}
		}
		return nil
	}
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		err = joinRepairCloseError(err, check())
		if err != nil {
			out = nil
		}
	}()
	if err = check(); err != nil {
		return nil, err
	}
	base, err = filepath.Abs(folder)
	if err != nil {
		return nil, err
	}
	rootInfo, err = source.Data.Stat(".")
	if err != nil {
		return nil, err
	}
	if err = check(); err != nil {
		return nil, err
	}
	out = map[string][]byte{}
	for _, name := range []string{"storage", "storage/riff"} {
		if err = check(); err != nil {
			return nil, err
		}
		if err = source.Check(name, false); indexrepair.IsSourceNotExist(err) {
			entries = append(entries, flashcardExtractionEntry{name: name})
			return out, nil
		} else if err != nil {
			return nil, err
		}
		info, statErr := source.Data.Lstat(name)
		if statErr != nil {
			return nil, statErr
		}
		if !info.IsDir() {
			return nil, flashcard.ErrInvalid
		}
		entries = append(entries, flashcardExtractionEntry{name: name, info: info})
	}
	for _, ext := range []string{".deck", ".cards"} {
		name := "storage/riff/" + builtinDeckID + ext
		if err = check(); err != nil {
			return nil, err
		}
		if err = source.Check(name, false); indexrepair.IsSourceNotExist(err) {
			entries = append(entries, flashcardExtractionEntry{name: name})
			continue
		} else if err != nil {
			return nil, err
		}
		info, statErr := source.Data.Lstat(name)
		if statErr != nil {
			return nil, statErr
		}
		if !info.Mode().IsRegular() || info.Size() > flashcardExtractionLimit {
			return nil, flashcard.ErrInvalid
		}
		entries = append(entries, flashcardExtractionEntry{name: name, info: info})
	}
	for _, saved := range entries {
		if saved.info == nil || !saved.info.Mode().IsRegular() {
			continue
		}
		if err = check(); err != nil {
			return nil, err
		}
		raw, readErr := read(ctx, source, saved.name)
		if readErr != nil {
			if errors.Is(readErr, indexrepair.ErrSourceReadLimit) {
				readErr = errors.Join(flashcard.ErrInvalid, readErr)
			}
			return nil, readErr
		}
		if len(raw) > int(flashcardExtractionLimit) {
			return nil, flashcard.ErrInvalid
		}
		if int64(len(raw)) != saved.info.Size() {
			return nil, indexrepair.ErrSourceChanged
		}
		if err = check(); err != nil {
			return nil, err
		}
		out[saved.name] = raw
	}
	return out, check()
}
