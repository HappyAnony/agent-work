package assetflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexrepair"
)

func TestAssetCleanBackupBeforeRemovalAndVersionFailure(t *testing.T) {
	for _, mode := range []string{"success", "backup-failure", "source-changed", "late-remove-failure"} {
		t.Run(mode, func(t *testing.T) {
			data, hist := t.TempDir(), t.TempDir()
			for _, name := range []string{"first.txt", "second.txt"} {
				if e := os.MkdirAll(filepath.Join(data, "assets", "folder"), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(filepath.Join(data, "assets", name), []byte(name), 0600); e != nil {
					t.Fatal(e)
				}
			}
			sourceRoot, e := os.OpenRoot(data)
			if e != nil {
				t.Fatal(e)
			}
			defer sourceRoot.Close()
			historyRoot, e := os.OpenRoot(hist)
			if e != nil {
				t.Fatal(e)
			}
			defer historyRoot.Close()
			injected := errors.New("injected failure")
			source := indexrepair.Source{Data: sourceRoot}
			history := indexrepair.Source{Data: historyRoot}
			changed := false
			source.Authorize = func(context.Context) error {
				if mode == "source-changed" && !changed {
					changed = true
					return os.WriteFile(filepath.Join(data, "assets/first.txt"), []byte("new revision"), 0600)
				}
				if mode == "late-remove-failure" {
					if _, e := os.Stat(filepath.Join(data, "assets/first.txt")); os.IsNotExist(e) {
						return injected
					}
				}
				return nil
			}
			history.Authorize = func(context.Context) error {
				if mode == "backup-failure" {
					return injected
				}
				return nil
			}
			out, e := Clean(context.Background(), source, history, []string{"assets/first.txt", "assets/second.txt", "assets/folder/"}, "stamp-clean")
			if mode == "success" {
				if e != nil || len(out.Files) != 2 || out.Removed != 3 {
					t.Fatal(out, e)
				}
				for _, name := range []string{"first.txt", "second.txt"} {
					raw, e := os.ReadFile(filepath.Join(hist, "stamp-clean/assets", name))
					if e != nil || string(raw) != name {
						t.Fatal("backup missing", e)
					}
				}
				if info, e := os.Stat(filepath.Join(hist, "stamp-clean/assets/folder")); e != nil || !info.IsDir() {
					t.Fatal("empty directory backup missing", e)
				}
			} else {
				if e == nil {
					t.Fatal("failure hidden")
				}
				raw, readErr := os.ReadFile(filepath.Join(data, "assets/second.txt"))
				if readErr != nil || string(raw) != "second.txt" {
					t.Fatal("unadmitted removal", readErr)
				}
				if mode == "source-changed" {
					raw, _ := os.ReadFile(filepath.Join(data, "assets/first.txt"))
					if string(raw) != "new revision" || !errors.Is(e, indexrepair.ErrSourceChanged) {
						t.Fatal(string(raw), e)
					}
				}
				if mode == "late-remove-failure" && len(out.Files) != 1 {
					t.Fatal("successful prefix lost", out, e)
				}
			}
		})
	}
}
