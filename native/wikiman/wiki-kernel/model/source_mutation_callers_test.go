//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/internal/flashcard"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestSourceMutationCallerRecreateRetainsHeldLockAndMovesChildren(t *testing.T) {
	_, r, _, file, _ := referenceModelFixture(t)
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	oldPath := "/" + filepath.Base(file)
	tree, err := r.loadRepairTree(ctx, box, oldPath, util.NewLute())
	if err != nil {
		t.Fatal(err)
	}
	oldID := tree.ID
	oldChildren := filepath.Join(filepath.Dir(file), oldID)
	if err = os.Mkdir(oldChildren, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(oldChildren, "marker"), []byte("child folder bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.recreateTree(ctx, tree, box+oldPath); err != nil {
		t.Fatal(err)
	}
	if err = (sql.LegacyMainQueue{DB: r.main.DB}).Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if tree.ID == oldID {
		t.Fatal("original reset did not change ID")
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("old source remains", err)
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(file), tree.ID+util.DocExt)); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), tree.ID, "marker"))
	if err != nil || string(raw) != "child folder bytes" {
		t.Fatal(string(raw), err)
	}
}
func TestSourceMutationCallerFlashcardExportJoinsPrimaryAndRootCloseErrors(t *testing.T) {
	for _, kind := range []string{"published", "primary"} {
		t.Run(kind, func(t *testing.T) {
			folder := t.TempDir()
			base := "storage/riff/" + builtinDeckID
			archiveWrite(t, folder, base+".cards", []byte("old cards"))
			archiveWrite(t, folder, base+".deck", []byte("old deck"))
			keep := archiveWrite(t, folder, "storage/riff/keep", []byte("keep"))
			root, err := os.OpenRoot(folder)
			if err != nil {
				t.Fatal(err)
			}
			primary, closing := errors.New("source permission revoked"), errors.New("export root close")
			calls, closes := 0, 0
			guard := func() error {
				calls++
				if kind == "primary" && calls > 1 {
					return primary
				}
				return nil
			}
			plan := flashcard.ArchivePlan{Files: map[string][]byte{base + ".cards": []byte("new cards"), base + ".deck": []byte("new deck")}}
			err = writeFlashcardArchiveExport(context.Background(), plan, folder, guard, func() (*os.Root, error) { return root, nil }, func(root *os.Root) error { closes++; return errors.Join(root.Close(), closing) })
			if !errors.Is(err, closing) || closes != 1 {
				t.Fatal(err, closes)
			}
			if kind == "primary" && !errors.Is(err, primary) {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(keep)
			if err != nil || string(raw) != "keep" {
				t.Fatal(err)
			}
			want := "new cards"
			if kind == "primary" {
				want = "old cards"
			}
			raw, err = os.ReadFile(filepath.Join(folder, filepath.FromSlash(base+".cards")))
			if err != nil || string(raw) != want {
				t.Fatal(string(raw), err)
			}
		})
	}
}
