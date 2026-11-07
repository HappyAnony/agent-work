// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestNotebookSourceScanSkipsAssetsHiddenAndInvalidNames(t *testing.T) {
	s := sourceFixture(t)
	box := incrementalBox
	valid := box + "/" + incrementalID + ".docj"
	parent := "20261029200000-parenta"
	for _, name := range []string{valid, box + "/" + parent + "/" + incrementalID + ".docj", box + "/assets/attachment.docj", box + "/.siyuan/history/old.docj", box + "/bad.docj"} {
		s.Data.MkdirAll(filepath.Dir(name), 0700)
		s.Data.WriteFile(name, []byte("fixture"), 0600)
	}
	docs, e := s.Documents(context.Background(), box)
	if e != nil || len(docs) != 2 {
		t.Fatal(docs, e)
	}
	s.Data.Symlink(t.TempDir(), box+"/link")
	if _, e = s.Documents(context.Background(), box); !errors.Is(e, ErrSourcePath) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = s.Documents(ctx, box); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = s.Documents(context.Background(), "../"); !errors.Is(e, ErrSourcePath) {
		t.Fatal(e)
	}
}

func TestNotebookSourceCheckedEnumerationBeforeProjection(t *testing.T) {
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			s := sourceFixture(t)
			box := incrementalBox
			valid := box + "/" + incrementalID + ".docj"
			late := "20261102190000-latefld"
			if err := s.Data.MkdirAll(box+"/"+late, 0700); err != nil {
				t.Fatal(err)
			}
			original := []byte("original notebook document")
			if err := s.Data.WriteFile(valid, original, 0600); err != nil {
				t.Fatal(err)
			}
			primary := errors.New("notebook enumeration failed")
			op := notebookWalkIO{readDir: func(ctx context.Context, source Source, name string) ([]fs.DirEntry, error) {
				if name != box+"/"+late {
					return source.ReadDir(ctx, name)
				}
				if kind == "incomplete" {
					return nil, errors.Join(ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				old := valid + ".enumerated"
				if err := source.Data.Rename(valid, old); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = source.Data.Remove(old) })
				if err := source.Data.WriteFile(valid, original, 0600); err != nil {
					t.Fatal(err)
				}
				return source.ReadDir(ctx, name)
			}}
			docs, err := s.documentsWithIO(context.Background(), box, op)
			if docs != nil {
				t.Fatal("partial notebook documents returned", docs)
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, ErrSourceReadIncomplete) || IsSourceNotExist(err) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrSourceChanged) {
				t.Fatal(err)
			}
			after, readErr := s.Data.ReadFile(valid)
			if readErr != nil || string(after) != string(original) {
				t.Fatal("checked notebook enumeration changed source bytes", readErr)
			}
		})
	}
}
