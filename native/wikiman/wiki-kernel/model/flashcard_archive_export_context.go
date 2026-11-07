// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"

	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/indexrepair"
)

// The trusted open/close backend is captured by the compatibility service.
// Success includes export-root close; previously written pairs are not rolled back.
func writeFlashcardArchiveExport(ctx context.Context, p flashcard.ArchivePlan, exportDir string, guard func() error, open func() (*os.Root, error), closeRoot func(*os.Root) error) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	if exportDir == "" {
		return flashcard.ErrDependency
	}
	root, err := open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeRoot(root)) }()
	source := indexrepair.Source{Data: root, Authorize: func(c context.Context) error {
		if err := c.Err(); err != nil {
			return err
		}
		return guard()
	}}
	for _, ext := range []string{".cards", ".deck"} {
		name := "storage/riff/" + builtinDeckID + ext
		if err = source.Check(name, true); err != nil {
			return err
		}
		if err = source.Remove(ctx, name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, ext := range []string{".cards", ".deck"} {
		name := "storage/riff/" + builtinDeckID + ext
		if raw, ok := p.Files[name]; ok {
			if err = source.Rewrite(ctx, name, nil, raw); err != nil {
				return err
			}
		}
	}
	return guard()
}
