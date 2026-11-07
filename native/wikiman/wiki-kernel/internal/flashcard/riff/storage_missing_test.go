// SPDX-License-Identifier: AGPL-3.0-or-later
package riff

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
)

func TestStorageMissingRequiresOnlyMissingCauses(t *testing.T) {
	closing := errors.New("close failed")
	for _, q := range []struct {
		name    string
		err     error
		missing bool
	}{
		{"nil", nil, false},
		{"bare", fs.ErrNotExist, true},
		{"path", &os.PathError{Op: "open", Path: "synthetic", Err: fs.ErrNotExist}, true},
		{"wrapped", fmt.Errorf("source: %w", fs.ErrNotExist), true},
		{"joined-missing", errors.Join(fs.ErrNotExist, fs.ErrNotExist), true},
		{"close", closing, false},
		{"mixed", errors.Join(fs.ErrNotExist, closing), false},
		{"nested-mixed", fmt.Errorf("read: %w", errors.Join(fs.ErrNotExist, closing)), false},
		{"incomplete", errors.Join(fs.ErrNotExist, ErrStorageReadIncomplete), false},
		{"incomplete-missing-close", errors.Join(fs.ErrNotExist, ErrStorageReadIncomplete, fmt.Errorf("close: %w", fs.ErrNotExist)), false},
	} {
		t.Run(q.name, func(t *testing.T) {
			if got := isStorageMissing(q.err); got != q.missing {
				t.Fatal(got, q.err)
			}
		})
	}
}
