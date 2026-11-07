// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
)

func (s Source) archive(ctx context.Context, name, stamp string, op sourceLifecycleIO) (dest string, err error) {
	if s.ArchiveRoot == nil {
		return "", ErrSourceWrite
	}
	if !fs.ValidPath(stamp) || stamp == "." || strings.ContainsAny(stamp, "/\\") {
		return "", ErrSourcePath
	}
	if err = s.write(ctx); err != nil {
		return "", err
	}
	unlock := s.locked(name)
	defer unlock()
	if err = s.write(ctx); err != nil {
		return "", err
	}
	if err = s.Check(name, false); err != nil {
		return "", err
	}
	op = op.defaults()
	parent, err := borrowSourceDirectory(s, path.Dir(name), op)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, op.closeRoot(parent.root)) }()
	base := path.Base(name)
	identity, err := parent.root.Lstat(base)
	if err != nil {
		return "", err
	}
	if !identity.Mode().IsRegular() {
		return "", ErrSourcePath
	}
	guard := func(checkCtx context.Context) error {
		if err := s.write(checkCtx); err != nil {
			return err
		}
		if err := parent.check(); err != nil {
			return err
		}
		return sourceEntryMatches(parent.root, base, identity)
	}
	before, err := lifecycleReadFile(ctx, parent.root, base, identity, func() error { return guard(ctx) }, op)
	if err != nil {
		return "", err
	}
	id, err := nonce()
	if err != nil {
		return "", err
	}
	dest = path.Join("corrupted", stamp+"-"+id, name)
	backupSource := Source{Data: s.ArchiveRoot, Authorize: s.Authorize}
	// Once an output path is selected, return it even on a late publication error.
	// It is a candidate for inspection, not proof that a complete backup exists.
	if err = backupSource.rewrite(ctx, dest, nil, before, op.rewrite); err != nil {
		return dest, err
	}
	backup, err := borrowSourceDirectory(backupSource, path.Dir(dest), op)
	if err != nil {
		return dest, err
	}
	defer func() { err = errors.Join(err, op.closeRoot(backup.root)) }()
	backupID, err := backup.root.Lstat(base)
	if err != nil {
		return dest, err
	}
	if !backupID.Mode().IsRegular() {
		return dest, ErrSourcePath
	}
	critical := context.WithoutCancel(ctx)
	var dirs []struct {
		name string
		info fs.FileInfo
	}
	for dir := path.Dir(dest); ; dir = path.Dir(dir) {
		if err = guard(critical); err != nil {
			return dest, err
		}
		if dir != "." {
			if err = backupSource.Check(dir, false); err != nil {
				return dest, err
			}
		}
		info, statErr := s.ArchiveRoot.Lstat(dir)
		if statErr != nil {
			return dest, statErr
		}
		if !info.IsDir() {
			return dest, ErrSourcePath
		}
		input, openErr := op.open(s.ArchiveRoot, dir)
		if openErr != nil {
			return dest, openErr
		}
		opened, statErr := input.Stat()
		if statErr == nil && !os.SameFile(info, opened) {
			statErr = ErrSourceChanged
		}
		if statErr == nil {
			statErr = input.Sync()
		}
		err = errors.Join(statErr, input.Close())
		if err != nil {
			return dest, err
		}
		dirs = append(dirs, struct {
			name string
			info fs.FileInfo
		}{dir, info})
		if dir == "." {
			break
		}
	}
	checkBackup := func() error {
		if err := guard(critical); err != nil {
			return err
		}
		if err := backup.check(); err != nil {
			return err
		}
		for _, dir := range dirs {
			if dir.name != "." {
				if err := backupSource.Check(dir.name, false); err != nil {
					return err
				}
			}
			now, err := s.ArchiveRoot.Lstat(dir.name)
			if err != nil {
				return err
			}
			if !now.IsDir() || !os.SameFile(now, dir.info) {
				return ErrSourceChanged
			}
		}
		return sourceEntryMatches(backup.root, base, backupID)
	}
	copied, err := lifecycleReadFile(critical, backup.root, base, backupID, checkBackup, op)
	if err != nil {
		return dest, err
	}
	if !bytes.Equal(copied, before) {
		return dest, ErrSourceChanged
	}
	current, err := lifecycleReadFile(critical, parent.root, base, identity, func() error { return guard(critical) }, op)
	if err != nil {
		return dest, err
	}
	if !bytes.Equal(current, before) {
		return dest, ErrSourceChanged
	}
	if err = checkBackup(); err != nil {
		return dest, err
	}
	if err = guard(critical); err != nil {
		return dest, err
	}
	if err = op.remove(parent.root, base); err != nil {
		return dest, err
	}
	return dest, nil
}
